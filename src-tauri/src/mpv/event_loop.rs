//! libmpv event pump.
//!
//! Owns the mpv event thread:
//! - registers mpv property observers
//! - converts raw mpv events into typed Rust events
//! - emits frontend events through Tauri
//! - keeps a bounded internal channel for future Rust-side consumers

use std::collections::HashMap;
use std::sync::atomic::{AtomicU32, Ordering};
use std::sync::{Arc, LazyLock};
use std::thread::JoinHandle;
use std::time::Duration;

use flume::{bounded, Receiver, Sender};
use libmpv2::events::{Event, EventContext, PropertyData};
use libmpv2::mpv_node::MpvNode;
use libmpv2::{Format, GetData, Mpv};
use serde::{Deserialize, Serialize};
use serde_json::json;
use tauri::{AppHandle, Emitter};
use tracing::{debug, info, warn};

// -----------------------------------------------------------------------------
// Event-thread state
// -----------------------------------------------------------------------------

pub const PAUSED: u32 = 0;
pub const ACTIVE: u32 = 1;
pub const SHUTDOWN: u32 = 2;

const EVENT_WAIT_SECS: f64 = 0.25;
const PAUSED_SLEEP_MS: u64 = 50;
const INTERNAL_EVENT_BUFFER: usize = 2048;

// -----------------------------------------------------------------------------
// Typed payloads
// -----------------------------------------------------------------------------

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct MpvTrack {
    pub id: i64,
    pub title: String,
    pub lang: String,

    #[serde(rename = "type")]
    pub type_: String,

    #[serde(default)]
    pub selected: bool,

    #[serde(default)]
    pub external: bool,

    #[serde(default, rename = "src_id")]
    pub src_id: i64,
}

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct MpvTracks {
    pub audio_tracks: Vec<MpvTrack>,
    pub sub_tracks: Vec<MpvTrack>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Chapter {
    pub title: String,
    pub time: f64,
}

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct ChapterList(pub Vec<Chapter>);

#[derive(Debug, Clone, Serialize)]
#[serde(tag = "type", content = "data")]
#[allow(clippy::large_enum_variant)]
pub enum MpvEvent {
    Seek,
    PlaybackRestart,
    EndFile(u32),
    FileLoaded,

    Duration(f64),
    Pause(bool),
    CacheSpeed(i64),
    Error(String),
    TrackList(MpvTracks),
    Volume(i64),
    Speed(f64),
    Shutdown,
    DemuxerCacheTime(f64),
    TimePos(f64),
    PausedForCache(bool),
    ChapterList(ChapterList),

    HwdecCurrent(String),
    Aid(i64),
    Sid(i64),
    SubDelay(f64),
    AudioDelay(f64),
    Brightness(f64),
    Contrast(f64),
    Saturation(f64),
    Gamma(f64),
    Hue(f64),
    Chapter(i64),
    EofReached(bool),
}

// -----------------------------------------------------------------------------
// Internal event channel
// -----------------------------------------------------------------------------

pub struct MpvEventChannel {
    pub tx: Sender<MpvEvent>,

    #[allow(dead_code)]
    pub rx: Receiver<MpvEvent>,
}

pub static MPV_EVENT_CHANNEL: LazyLock<MpvEventChannel> = LazyLock::new(|| {
    let (tx, rx) = bounded::<MpvEvent>(INTERNAL_EVENT_BUFFER);
    MpvEventChannel { tx, rx }
});

// -----------------------------------------------------------------------------
// Spawn
// -----------------------------------------------------------------------------

pub fn spawn_event_loop(
    mpv: Arc<Mpv>,
    app_handle: AppHandle,
    alive: Arc<AtomicU32>,
) -> JoinHandle<()> {
    std::thread::Builder::new()
        .name("fyom-mpv-event".into())
        .spawn(move || {
            let mut event_context = EventContext::new(mpv.ctx);

            configure_event_context(&mut event_context);

            run_event_loop(event_context, mpv, app_handle, alive);
        })
        .expect("failed to spawn fyom mpv event thread")
}

fn configure_event_context(event_context: &mut EventContext) {
    if let Err(error) = event_context.disable_deprecated_events() {
        warn!("[mpv/event] disable_deprecated_events failed: {error}");
    }

    const OBSERVED_PROPERTIES: &[(&str, Format, u64)] = &[
        ("duration", Format::Double, 0),
        ("pause", Format::Flag, 1),
        ("cache-speed", Format::Int64, 2),
        ("track-list", Format::Node, 3),
        ("paused-for-cache", Format::Flag, 4),
        ("demuxer-cache-time", Format::Double, 5),
        ("time-pos", Format::Double, 6),
        ("volume", Format::Int64, 7),
        ("chapter-list", Format::Node, 8),
        ("speed", Format::Double, 9),
        ("hwdec-current", Format::Node, 10),
        ("aid", Format::Int64, 11),
        ("sid", Format::Int64, 12),
        ("sub-delay", Format::Double, 13),
        ("audio-delay", Format::Double, 14),
        ("brightness", Format::Double, 15),
        ("contrast", Format::Double, 16),
        ("saturation", Format::Double, 17),
        ("gamma", Format::Double, 18),
        ("hue", Format::Double, 19),
        ("chapter", Format::Int64, 20),
        ("eof-reached", Format::Flag, 21),
    ];

    for (name, format, userdata) in OBSERVED_PROPERTIES {
        observe_property(event_context, name, *format, *userdata);
    }
}

fn run_event_loop(
    mut event_context: EventContext,
    mpv: Arc<Mpv>,
    app_handle: AppHandle,
    alive: Arc<AtomicU32>,
) {
    info!("[mpv/event] event loop started");

    loop {
        match alive.load(Ordering::SeqCst) {
            SHUTDOWN => break,
            PAUSED => {
                std::thread::sleep(Duration::from_millis(PAUSED_SLEEP_MS));
                continue;
            }
            ACTIVE => {}
            other => {
                warn!("[mpv/event] unknown alive state: {other}");
            }
        }

        match event_context.wait_event(EVENT_WAIT_SECS) {
            Some(Ok(event)) => handle_event(event, &mpv, &app_handle),
            Some(Err(error)) => emit_error(&app_handle, error.to_string()),
            None => {}
        }
    }

    info!("[mpv/event] event loop exited");
}

// -----------------------------------------------------------------------------
// Event handling
// -----------------------------------------------------------------------------

fn handle_event(event: Event, mpv: &Mpv, app_handle: &AppHandle) {
    match event {
        Event::PropertyChange { name, change, .. } => {
            handle_property_change(name, change, mpv, app_handle);
        }

        Event::Seek { .. } => {
            send_internal(MpvEvent::Seek);
            emit_void(app_handle, "fyom://mpv/seek");
        }

        Event::PlaybackRestart { .. } => {
            send_internal(MpvEvent::PlaybackRestart);
            emit_void(app_handle, "fyom://mpv/playback-restart");
        }

        Event::EndFile(reason) => {
            send_internal(MpvEvent::EndFile(reason));
            emit_payload(
                app_handle,
                "fyom://mpv/end-file",
                json!({
                    "reason": reason,
                    "reason_name": endfile_reason_name(reason),
                }),
            );
        }

        Event::FileLoaded => {
            send_internal(MpvEvent::FileLoaded);

            let path: Option<String> = mpv.get_property("path").ok();

            emit_payload(
                app_handle,
                "fyom://mpv/file-loaded",
                json!({
                    "path": path,
                }),
            );
        }

        Event::Shutdown => {
            send_internal(MpvEvent::Shutdown);
            emit_void(app_handle, "fyom://mpv/shutdown");
        }

        _ => {}
    }
}

#[allow(clippy::too_many_lines)]
fn handle_property_change(
    name: &str,
    change: PropertyData<'_>,
    mpv: &Mpv,
    app_handle: &AppHandle,
) {
    match name {
        "duration" => {
            let PropertyData::Double(duration) = change else {
                return unexpected_property(name);
            };

            send_internal(MpvEvent::Duration(duration));
            emit_payload(
                app_handle,
                "fyom://mpv/duration",
                json!({ "duration": duration }),
            );
        }

        "pause" => {
            let PropertyData::Flag(paused) = change else {
                return unexpected_property(name);
            };

            send_internal(MpvEvent::Pause(paused));
            emit_payload(app_handle, "fyom://mpv/pause", json!({ "paused": paused }));
        }

        "cache-speed" => {
            let PropertyData::Int64(speed) = change else {
                return unexpected_property(name);
            };

            send_internal(MpvEvent::CacheSpeed(speed));
            emit_payload(
                app_handle,
                "fyom://mpv/cache-speed",
                json!({ "speed": speed }),
            );
        }

        "track-list" => {
            let PropertyData::Node(node) = change else {
                return unexpected_property(name);
            };

            let tracks = node_to_tracks(node);

            send_internal(MpvEvent::TrackList(tracks.clone()));
            emit_payload(
                app_handle,
                "fyom://mpv/track-list",
                json!({
                    "audio_tracks": tracks.audio_tracks,
                    "sub_tracks": tracks.sub_tracks,
                }),
            );
        }

        "chapter-list" => {
            let PropertyData::Node(node) = change else {
                return unexpected_property(name);
            };

            let chapters = node_to_chapter_list(node);

            send_internal(MpvEvent::ChapterList(chapters.clone()));
            emit_payload(
                app_handle,
                "fyom://mpv/chapter-list",
                json!({
                    "chapters": chapters.0,
                }),
            );
        }

        "volume" => {
            let PropertyData::Int64(volume) = change else {
                return unexpected_property(name);
            };

            send_internal(MpvEvent::Volume(volume));
            emit_payload(app_handle, "fyom://mpv/volume", json!({ "volume": volume }));
        }

        "speed" => {
            let PropertyData::Double(speed) = change else {
                return unexpected_property(name);
            };

            send_internal(MpvEvent::Speed(speed));
            emit_payload(app_handle, "fyom://mpv/speed", json!({ "speed": speed }));
        }

        "demuxer-cache-time" => {
            let PropertyData::Double(time) = change else {
                return unexpected_property(name);
            };

            send_internal(MpvEvent::DemuxerCacheTime(time));
            emit_payload(
                app_handle,
                "fyom://mpv/demuxer-cache-time",
                json!({ "time": time }),
            );
        }

        "time-pos" => {
            let PropertyData::Double(position) = change else {
                return unexpected_property(name);
            };

            let duration: f64 = get_property_or_default(mpv, "duration", 0.0);

            send_internal(MpvEvent::TimePos(position));
            emit_payload(
                app_handle,
                "fyom://mpv/time-pos",
                json!({
                    "position": position,
                    "duration": duration,
                }),
            );
        }

        "paused-for-cache" => {
            let PropertyData::Flag(paused_for_cache) = change else {
                return unexpected_property(name);
            };

            let seeking: bool = get_property_or_default(mpv, "seeking", false);
            let paused = paused_for_cache || seeking;

            send_internal(MpvEvent::PausedForCache(paused));
            emit_payload(
                app_handle,
                "fyom://mpv/paused-for-cache",
                json!({ "paused": paused }),
            );
        }

        "hwdec-current" => {
            let PropertyData::Node(node) = change else {
                return unexpected_property(name);
            };

            let hwdec = node.str().unwrap_or_default().to_string();

            send_internal(MpvEvent::HwdecCurrent(hwdec.clone()));
            emit_payload(
                app_handle,
                "fyom://mpv/hwdec-current",
                json!({ "hwdec": hwdec }),
            );
        }

        "aid" => {
            let PropertyData::Int64(id) = change else {
                return unexpected_property(name);
            };

            send_internal(MpvEvent::Aid(id));
            emit_payload(app_handle, "fyom://mpv/aid", json!({ "id": id }));
        }

        "sid" => {
            let PropertyData::Int64(id) = change else {
                return unexpected_property(name);
            };

            send_internal(MpvEvent::Sid(id));
            emit_payload(app_handle, "fyom://mpv/sid", json!({ "id": id }));
        }

        "sub-delay" => {
            let PropertyData::Double(delay) = change else {
                return unexpected_property(name);
            };

            send_internal(MpvEvent::SubDelay(delay));
            emit_payload(
                app_handle,
                "fyom://mpv/sub-delay",
                json!({ "delay": delay }),
            );
        }

        "audio-delay" => {
            let PropertyData::Double(delay) = change else {
                return unexpected_property(name);
            };

            send_internal(MpvEvent::AudioDelay(delay));
            emit_payload(
                app_handle,
                "fyom://mpv/audio-delay",
                json!({ "delay": delay }),
            );
        }

        "brightness" => {
            let PropertyData::Double(value) = change else {
                return unexpected_property(name);
            };

            send_internal(MpvEvent::Brightness(value));
            emit_payload(
                app_handle,
                "fyom://mpv/brightness",
                json!({ "value": value }),
            );
        }

        "contrast" => {
            let PropertyData::Double(value) = change else {
                return unexpected_property(name);
            };

            send_internal(MpvEvent::Contrast(value));
            emit_payload(
                app_handle,
                "fyom://mpv/contrast",
                json!({ "value": value }),
            );
        }

        "saturation" => {
            let PropertyData::Double(value) = change else {
                return unexpected_property(name);
            };

            send_internal(MpvEvent::Saturation(value));
            emit_payload(
                app_handle,
                "fyom://mpv/saturation",
                json!({ "value": value }),
            );
        }

        "gamma" => {
            let PropertyData::Double(value) = change else {
                return unexpected_property(name);
            };

            send_internal(MpvEvent::Gamma(value));
            emit_payload(app_handle, "fyom://mpv/gamma", json!({ "value": value }));
        }

        "hue" => {
            let PropertyData::Double(value) = change else {
                return unexpected_property(name);
            };

            send_internal(MpvEvent::Hue(value));
            emit_payload(app_handle, "fyom://mpv/hue", json!({ "value": value }));
        }

        "chapter" => {
            let PropertyData::Int64(index) = change else {
                return unexpected_property(name);
            };

            send_internal(MpvEvent::Chapter(index));
            emit_payload(
                app_handle,
                "fyom://mpv/chapter",
                json!({ "index": index }),
            );
        }

        "eof-reached" => {
            let PropertyData::Flag(eof) = change else {
                return unexpected_property(name);
            };

            send_internal(MpvEvent::EofReached(eof));
            emit_payload(app_handle, "fyom://mpv/eof-reached", json!({ "eof": eof }));
        }

        _ => {}
    }
}

// -----------------------------------------------------------------------------
// Node parsing
// -----------------------------------------------------------------------------

fn node_to_tracks(node: MpvNode) -> MpvTracks {
    let Some(array) = node.array() else {
        return MpvTracks::default();
    };

    let mut audio_tracks = Vec::new();
    let mut sub_tracks = Vec::new();

    for entry in array {
        let Some(map) = entry.map() else {
            continue;
        };

        let fields: HashMap<String, MpvNode> = map.collect();

        let type_ = node_string(&fields, "type", "");

        if type_ != "audio" && type_ != "sub" {
            continue;
        }

        let track = MpvTrack {
            id: node_i64(&fields, "id", 0),
            title: node_string(&fields, "title", ""),
            lang: node_string(&fields, "lang", ""),
            type_,
            selected: node_bool(&fields, "selected", false),
            external: node_bool(&fields, "external", false),
            src_id: node_i64(&fields, "src-id", 0),
        };

        match track.type_.as_str() {
            "audio" => audio_tracks.push(track),
            "sub" => sub_tracks.push(track),
            _ => {}
        }
    }

    MpvTracks {
        audio_tracks,
        sub_tracks,
    }
}

fn node_to_chapter_list(node: MpvNode) -> ChapterList {
    let Some(array) = node.array() else {
        return ChapterList::default();
    };

    let mut chapters = Vec::new();

    for entry in array {
        let Some(map) = entry.map() else {
            continue;
        };

        let fields: HashMap<String, MpvNode> = map.collect();

        chapters.push(Chapter {
            title: node_string(&fields, "title", ""),
            time: node_f64(&fields, "time", 0.0),
        });
    }

    ChapterList(chapters)
}

fn node_i64(fields: &HashMap<String, MpvNode>, key: &str, default: i64) -> i64 {
    fields.get(key).and_then(MpvNode::i64).unwrap_or(default)
}

fn node_f64(fields: &HashMap<String, MpvNode>, key: &str, default: f64) -> f64 {
    fields.get(key).and_then(MpvNode::f64).unwrap_or(default)
}

fn node_string(fields: &HashMap<String, MpvNode>, key: &str, default: &str) -> String {
    fields
        .get(key)
        .and_then(MpvNode::str)
        .unwrap_or(default)
        .to_string()
}

fn node_bool(fields: &HashMap<String, MpvNode>, key: &str, default: bool) -> bool {
    fields
        .get(key)
        .and_then(MpvNode::i64)
        .map(|value| value != 0)
        .unwrap_or(default)
}

// -----------------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------------

fn observe_property(
    event_context: &mut EventContext,
    name: &'static str,
    format: Format,
    reply_userdata: u64,
) {
    if let Err(error) = event_context.observe_property(name, format, reply_userdata) {
        warn!("[mpv/event] observe_property({name}) failed: {error}");
    }
}

fn send_internal(event: MpvEvent) {
    if let Err(error) = MPV_EVENT_CHANNEL.tx.try_send(event) {
        debug!("[mpv/event] internal event dropped: {error}");
    }
}

fn emit_payload<T>(app_handle: &AppHandle, event_name: &str, payload: T)
where
    T: Serialize + Clone,
{
    if let Err(error) = app_handle.emit(event_name, payload) {
        debug!("[mpv/event] emit {event_name} failed: {error}");
    }
}

fn emit_void(app_handle: &AppHandle, event_name: &str) {
    if let Err(error) = app_handle.emit(event_name, ()) {
        debug!("[mpv/event] emit {event_name} failed: {error}");
    }
}

fn emit_error(app_handle: &AppHandle, message: String) {
    warn!("[mpv/event] {message}");

    send_internal(MpvEvent::Error(message.clone()));

    emit_payload(
        app_handle,
        "fyom://mpv/error",
        json!({
            "message": message,
        }),
    );
}

fn unexpected_property(name: &str) {
    debug!("[mpv/event] ignored unexpected property payload for {name}");
}

fn endfile_reason_name(reason: u32) -> &'static str {
    match reason {
        0 => "eof",
        1 => "stop",
        2 => "quit",
        3 => "error",
        4 => "redirect",
        _ => "unknown",
    }
}

fn get_property_or_default<T>(mpv: &Mpv, name: &str, default: T) -> T
where
    T: GetData,
{
    mpv.get_property(name).unwrap_or(default)
}
