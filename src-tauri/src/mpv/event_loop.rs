//! libmpv event pump.
//!
//! This module owns the mpv event thread. It observes mpv properties, converts raw mpv
//! events into strongly typed Rust events, sends them to an internal flume channel, and
//! emits `fyom://mpv/*` events to the frontend.
//!
//! PORTED_FROM_TSUKIMI @ v26.6.3 (`src/ui/mpv/tsukimi_mpv.rs`)
//!
//! Adapted for fyom:
//! - The internal flume channel is retained for future Rust-side consumers.
//! - The primary current consumer is the frontend via Tauri `AppHandle::emit`.
//! - GTK-specific key handling is intentionally not included.
//! - Rendering is handled separately in `crate::mpv::render`.
//! - Property parsing is defensive: malformed mpv node payloads must not crash the event
//!   thread.

use std::collections::HashMap;
use std::sync::atomic::{AtomicU32, Ordering};
use std::sync::{Arc, LazyLock};
use std::thread::JoinHandle;

use flume::{Receiver, Sender, unbounded};
use libmpv2::events::{Event, EventContext, PropertyData};
use libmpv2::mpv_node::MpvNode;
use libmpv2::{Format, GetData, Mpv};
use serde::{Deserialize, Serialize};
use serde_json::json;
use tauri::{AppHandle, Emitter};
use tracing::{debug, warn};

// ---------------------------------------------------------------------------
// Event-thread state machine.
// ---------------------------------------------------------------------------

/// Event-loop thread is paused.
pub const PAUSED: u32 = 0;

/// Event-loop thread is running.
pub const ACTIVE: u32 = 1;

/// Event-loop thread should exit.
pub const SHUTDOWN: u32 = 2;

// ---------------------------------------------------------------------------
// Typed payloads.
// ---------------------------------------------------------------------------

/// A single libmpv track, parsed from the `track-list` node.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct MpvTrack {
    pub id: i64,
    pub title: String,
    pub lang: String,

    /// Track type: `"audio"` or `"sub"`.
    #[serde(rename = "type")]
    pub type_: String,

    /// Whether this track is currently selected.
    #[serde(default)]
    pub selected: bool,

    /// Whether this track is external.
    #[serde(default)]
    pub external: bool,

    /// Source id. `0` usually means the main file.
    #[serde(default, rename = "src_id")]
    pub src_id: i64,
}

/// Parsed audio/subtitle track list.
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct MpvTracks {
    pub audio_tracks: Vec<MpvTrack>,
    pub sub_tracks: Vec<MpvTrack>,
}

/// A chapter marker.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Chapter {
    pub title: String,
    pub time: f64,
}

/// Parsed chapter list.
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct ChapterList(pub Vec<Chapter>);

/// Track selection for `aid` / `sid`.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum TrackSelection {
    Track(i64),
    None,
}

impl std::fmt::Display for TrackSelection {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::Track(id) => write!(f, "{id}"),
            Self::None => write!(f, "no"),
        }
    }
}

/// Internal strongly typed event enum.
///
/// The frontend receives equivalent event-specific payloads via `fyom://mpv/*`.
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

    // Phase 2.4 additions.
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

// ---------------------------------------------------------------------------
// Internal event channel.
// ---------------------------------------------------------------------------

/// Internal event channel reserved for Rust-side consumers.
pub struct MpvEventChannel {
    pub tx: Sender<MpvEvent>,

    /// Reserved for future Rust-side consumers.
    #[allow(dead_code)]
    pub rx: Receiver<MpvEvent>,
}

pub static MPV_EVENT_CHANNEL: LazyLock<MpvEventChannel> = LazyLock::new(|| {
    let (tx, rx) = unbounded::<MpvEvent>();
    MpvEventChannel { tx, rx }
});

// ---------------------------------------------------------------------------
// Small helpers.
// ---------------------------------------------------------------------------

fn send_internal(event: MpvEvent) {
    let _ = MPV_EVENT_CHANNEL.tx.send(event);
}

fn emit_payload<T>(app_handle: &AppHandle, event_name: &str, payload: T)
where
    T: Serialize + Clone,
{
    let _ = app_handle.emit(event_name, payload);
}

fn emit_void(app_handle: &AppHandle, event_name: &str) {
    let _ = app_handle.emit(event_name, ());
}

/// Observe an mpv property. Failure should not crash the whole app; a failed observer
/// only means that one frontend event stream will not be produced.
fn observe_property(
    event_context: &mut EventContext,
    name: &'static str,
    format: Format,
    reply_userdata: u64,
) {
    if let Err(e) = event_context.observe_property(name, format, reply_userdata) {
        warn!(
            "[mpv] observe_property({}) failed; this event stream will be disabled: {}",
            name, e
        );
    }
}

/// Map an mpv EndFile reason code to a stable frontend string.
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

/// Read a property from mpv and return a default on error.
fn get_property_or_default<T>(mpv: &Mpv, name: &str, default: T) -> T
where
    T: GetData,
{
    mpv.get_property(name).unwrap_or(default)
}

// ---------------------------------------------------------------------------
// Event-pump spawn.
// ---------------------------------------------------------------------------

/// Spawn the mpv event-pump thread.
///
/// The returned handle must be joined during application shutdown.
pub fn spawn_event_loop(
    mpv: Arc<Mpv>,
    app_handle: AppHandle,
    event_thread_alive: Arc<AtomicU32>,
) -> JoinHandle<()> {
    let mut event_context = EventContext::new(mpv.ctx);

    if let Err(e) = event_context.disable_deprecated_events() {
        warn!("[mpv] failed to disable deprecated events: {}", e);
    }

    // Core Phase 2.2 observers.
    observe_property(&mut event_context, "duration", Format::Double, 0);
    observe_property(&mut event_context, "pause", Format::Flag, 1);
    observe_property(&mut event_context, "cache-speed", Format::Int64, 2);
    observe_property(&mut event_context, "track-list", Format::Node, 3);
    observe_property(&mut event_context, "paused-for-cache", Format::Flag, 4);
    observe_property(&mut event_context, "demuxer-cache-time", Format::Double, 5);
    observe_property(&mut event_context, "time-pos", Format::Double, 6);
    observe_property(&mut event_context, "volume", Format::Int64, 7);
    observe_property(&mut event_context, "chapter-list", Format::Node, 8);
    observe_property(&mut event_context, "speed", Format::Double, 9);

    // Phase 2.4 observers.
    //
    // `hwdec-current` is observed as Node so we can extract it through `MpvNode::str()`
    // without depending on a specific `PropertyData::String` variant.
    observe_property(&mut event_context, "hwdec-current", Format::Node, 10);
    observe_property(&mut event_context, "aid", Format::Int64, 11);
    observe_property(&mut event_context, "sid", Format::Int64, 12);
    observe_property(&mut event_context, "sub-delay", Format::Double, 13);
    observe_property(&mut event_context, "audio-delay", Format::Double, 14);
    observe_property(&mut event_context, "brightness", Format::Double, 15);
    observe_property(&mut event_context, "contrast", Format::Double, 16);
    observe_property(&mut event_context, "saturation", Format::Double, 17);
    observe_property(&mut event_context, "gamma", Format::Double, 18);
    observe_property(&mut event_context, "hue", Format::Double, 19);
    observe_property(&mut event_context, "chapter", Format::Int64, 20);
    observe_property(&mut event_context, "eof-reached", Format::Flag, 21);

    let alive = Arc::clone(&event_thread_alive);
    let mpv_for_thread = Arc::clone(&mpv);

    std::thread::Builder::new()
        .name("fyom mpv event loop".into())
        .spawn(move || {
            run_event_loop(event_context, mpv_for_thread, app_handle, alive);
        })
        .expect("failed to spawn fyom mpv event loop thread")
}

fn run_event_loop(
    mut event_context: EventContext,
    mpv: Arc<Mpv>,
    app_handle: AppHandle,
    alive: Arc<AtomicU32>,
) {
    loop {
        match alive.load(Ordering::SeqCst) {
            SHUTDOWN => break,
            PAUSED => {
                atomic_wait::wait(&alive, PAUSED);
                continue;
            }
            _ => {}
        }

        match event_context.wait_event(1000.0) {
            Some(Ok(event)) => handle_event(event, &mpv, &app_handle),
            Some(Err(e)) => {
                let message = e.to_string();
                warn!("[mpv] event error: {}", message);

                send_internal(MpvEvent::Error(message.clone()));
                emit_payload(
                    &app_handle,
                    "fyom://mpv/error",
                    json!({ "message": message }),
                );
            }
            None => {}
        }
    }

    debug!("[mpv] event loop thread exited");
}

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
                json!({ "path": path }),
            );
        }
        Event::Shutdown => {
            send_internal(MpvEvent::Shutdown);
            emit_void(app_handle, "fyom://mpv/shutdown");
        }
        _ => {}
    }
}

// ---------------------------------------------------------------------------
// Property decoding.
// ---------------------------------------------------------------------------

#[allow(clippy::too_many_lines)]
fn handle_property_change(name: &str, change: PropertyData<'_>, mpv: &Mpv, app_handle: &AppHandle) {
    match name {
        "duration" => {
            if let PropertyData::Double(duration) = change {
                send_internal(MpvEvent::Duration(duration));
                emit_payload(
                    app_handle,
                    "fyom://mpv/duration",
                    json!({ "duration": duration }),
                );
            }
        }
        "pause" => {
            if let PropertyData::Flag(paused) = change {
                send_internal(MpvEvent::Pause(paused));
                emit_payload(app_handle, "fyom://mpv/pause", json!({ "paused": paused }));
            }
        }
        "cache-speed" => {
            if let PropertyData::Int64(speed) = change {
                send_internal(MpvEvent::CacheSpeed(speed));
                emit_payload(
                    app_handle,
                    "fyom://mpv/cache-speed",
                    json!({ "speed": speed }),
                );
            }
        }
        "track-list" => {
            if let PropertyData::Node(node) = change {
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
        }
        "chapter-list" => {
            if let PropertyData::Node(node) = change {
                let chapters = node_to_chapter_list(node);

                send_internal(MpvEvent::ChapterList(chapters.clone()));
                emit_payload(
                    app_handle,
                    "fyom://mpv/chapter-list",
                    json!({ "chapters": chapters.0 }),
                );
            }
        }
        "volume" => {
            if let PropertyData::Int64(volume) = change {
                send_internal(MpvEvent::Volume(volume));
                emit_payload(app_handle, "fyom://mpv/volume", json!({ "volume": volume }));
            }
        }
        "speed" => {
            if let PropertyData::Double(speed) = change {
                send_internal(MpvEvent::Speed(speed));
                emit_payload(app_handle, "fyom://mpv/speed", json!({ "speed": speed }));
            }
        }
        "demuxer-cache-time" => {
            if let PropertyData::Double(time) = change {
                send_internal(MpvEvent::DemuxerCacheTime(time));
                emit_payload(
                    app_handle,
                    "fyom://mpv/demuxer-cache-time",
                    json!({ "time": time }),
                );
            }
        }
        "time-pos" => {
            if let PropertyData::Double(position) = change {
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
        }
        "paused-for-cache" => {
            if let PropertyData::Flag(paused_for_cache) = change {
                let seeking: bool = get_property_or_default(mpv, "seeking", false);
                let paused = paused_for_cache || seeking;

                send_internal(MpvEvent::PausedForCache(paused));
                emit_payload(
                    app_handle,
                    "fyom://mpv/paused-for-cache",
                    json!({ "paused": paused }),
                );
            }
        }
        "hwdec-current" => {
            if let PropertyData::Node(node) = change {
                let hwdec = node.str().unwrap_or_default().to_string();

                send_internal(MpvEvent::HwdecCurrent(hwdec.clone()));
                emit_payload(
                    app_handle,
                    "fyom://mpv/hwdec-current",
                    json!({ "hwdec": hwdec }),
                );
            }
        }
        "aid" => {
            if let PropertyData::Int64(id) = change {
                send_internal(MpvEvent::Aid(id));
                emit_payload(app_handle, "fyom://mpv/aid", json!({ "id": id }));
            }
        }
        "sid" => {
            if let PropertyData::Int64(id) = change {
                send_internal(MpvEvent::Sid(id));
                emit_payload(app_handle, "fyom://mpv/sid", json!({ "id": id }));
            }
        }
        "sub-delay" => {
            if let PropertyData::Double(delay) = change {
                send_internal(MpvEvent::SubDelay(delay));
                emit_payload(
                    app_handle,
                    "fyom://mpv/sub-delay",
                    json!({ "delay": delay }),
                );
            }
        }
        "audio-delay" => {
            if let PropertyData::Double(delay) = change {
                send_internal(MpvEvent::AudioDelay(delay));
                emit_payload(
                    app_handle,
                    "fyom://mpv/audio-delay",
                    json!({ "delay": delay }),
                );
            }
        }
        "brightness" => {
            if let PropertyData::Double(value) = change {
                send_internal(MpvEvent::Brightness(value));
                emit_payload(
                    app_handle,
                    "fyom://mpv/brightness",
                    json!({ "value": value }),
                );
            }
        }
        "contrast" => {
            if let PropertyData::Double(value) = change {
                send_internal(MpvEvent::Contrast(value));
                emit_payload(app_handle, "fyom://mpv/contrast", json!({ "value": value }));
            }
        }
        "saturation" => {
            if let PropertyData::Double(value) = change {
                send_internal(MpvEvent::Saturation(value));
                emit_payload(
                    app_handle,
                    "fyom://mpv/saturation",
                    json!({ "value": value }),
                );
            }
        }
        "gamma" => {
            if let PropertyData::Double(value) = change {
                send_internal(MpvEvent::Gamma(value));
                emit_payload(app_handle, "fyom://mpv/gamma", json!({ "value": value }));
            }
        }
        "hue" => {
            if let PropertyData::Double(value) = change {
                send_internal(MpvEvent::Hue(value));
                emit_payload(app_handle, "fyom://mpv/hue", json!({ "value": value }));
            }
        }
        "chapter" => {
            if let PropertyData::Int64(index) = change {
                send_internal(MpvEvent::Chapter(index));
                emit_payload(app_handle, "fyom://mpv/chapter", json!({ "index": index }));
            }
        }
        "eof-reached" => {
            if let PropertyData::Flag(eof) = change {
                send_internal(MpvEvent::EofReached(eof));
                emit_payload(app_handle, "fyom://mpv/eof-reached", json!({ "eof": eof }));
            }
        }
        _ => {}
    }
}

// ---------------------------------------------------------------------------
// Node parsers.
// ---------------------------------------------------------------------------

/// Parse the `track-list` property node into `MpvTracks`.
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

        // Current libmpv2 exposes node maps as `(String, MpvNode)` pairs.
        // Keep this as `HashMap<String, MpvNode>` to match the crate's concrete API.
        let fields: HashMap<String, MpvNode> = map.collect();

        let id = node_i64(&fields, "id", 0);
        let title = node_string(&fields, "title", "");
        let lang = node_string(&fields, "lang", "");
        let type_ = node_string(&fields, "type", "");

        let selected = node_bool(&fields, "selected", false);
        let external = node_bool(&fields, "external", false);
        let src_id = node_i64(&fields, "src-id", 0);

        let track = MpvTrack {
            id,
            title,
            lang,
            type_,
            selected,
            external,
            src_id,
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

/// Parse the `chapter-list` property node into `ChapterList`.
fn node_to_chapter_list(node: MpvNode) -> ChapterList {
    let Some(array) = node.array() else {
        return ChapterList::default();
    };

    let mut chapters = Vec::new();

    for entry in array {
        let Some(map) = entry.map() else {
            continue;
        };

        // Current libmpv2 exposes node maps as `(String, MpvNode)` pairs.
        // Keep this as `HashMap<String, MpvNode>` to match the crate's concrete API.
        let fields: HashMap<String, MpvNode> = map.collect();

        let title = node_string(&fields, "title", "");
        let time = node_f64(&fields, "time", 0.0);

        chapters.push(Chapter { title, time });
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
