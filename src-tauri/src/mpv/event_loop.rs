//! libmpv event pump — ports tsukimi's `process_events` + `ListenEvent` + node parsers.
//!
//! PORTED_FROM_TSUKIMI @ v26.6.3 (`src/ui/mpv/tsukimi_mpv.rs`)
//!
//! Adapted for fyom:
//! - tsukimi's `flume` `MPV_EVENT_CHANNEL` is **retained** (reserved for Phase 2.5
//!   Rust-side consumers — watched-status / progress logic); the primary consumer
//!   path is `AppHandle::emit("fyom://mpv/*")` to the frontend (Tauri emit is
//!   thread-safe; callable directly from the event thread).
//! - tsukimi's GTK `press_key` / `KEYSTRING_MAP` / `get_full_keystr` are **dropped**
//!   (fyom forwards keys from the webview via a Tauri `mpv_keypress` command; the
//!   keystr assembly is the frontend's job — mpv accepts keystrs like "Space",
//!   "Ctrl+Right", "Volume+").
//! - tsukimi's `RefCell<Option<RenderContext>>` is **dropped** (Phase 2.3 wires the
//!   GL render context in a separate module).
//! - The `atomic_wait` PAUSED/ACTIVE/SHUTDOWN state machine is **retained**; fyom
//!   starts the thread `ACTIVE` (runs immediately on spawn) and transitions to
//!   `SHUTDOWN` on app exit.
//! - tsukimi's `node_to_tracks` / `node_to_chapter_list` are ported **verbatim**
//!   (with a defensive `unwrap_or` instead of `unwrap` on node field access — a
//!   malformed track node should not crash the event thread).
//!
//! See `docs/libmpv-assessment.md` §3.4 for the port rationale + adaptation list.

use std::collections::HashMap;
use std::sync::atomic::{AtomicU32, Ordering};
use std::sync::{Arc, LazyLock};
use std::thread::JoinHandle;

use flume::{Receiver, Sender, unbounded};
use libmpv2::events::{Event, EventContext, PropertyData};
use libmpv2::mpv_node::MpvNode;
use libmpv2::{Format, Mpv};
use serde::{Deserialize, Serialize};
use tauri::{AppHandle, Emitter};
use tracing::{debug, warn};

// ---------------------------------------------------------------------------
// Event-thread state machine (ported verbatim from tsukimi).
// ---------------------------------------------------------------------------

/// Event-loop thread is paused (waiting on `atomic_wait`).
pub const PAUSED: u32 = 0;
/// Event-loop thread is running.
pub const ACTIVE: u32 = 1;
/// Event-loop thread should exit on the next loop iteration.
pub const SHUTDOWN: u32 = 2;

// ---------------------------------------------------------------------------
// Typed event payloads (ported from tsukimi's `ListenEvent` → `MpvEvent`).
// ---------------------------------------------------------------------------

/// A single libmpv track (audio or subtitle), parsed from the `track-list` node.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct MpvTrack {
    pub id: i64,
    pub title: String,
    pub lang: String,
    /// Track type: `"audio"` or `"sub"`. Renamed to `type` in JSON (mpv's field name).
    #[serde(rename = "type")]
    pub type_: String,
}

/// Parsed track list.
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

/// Track selection for the `aid`/`sid` properties: a track id, or `None` to disable.
#[derive(Debug, Clone)]
pub enum TrackSelection {
    Track(i64),
    None,
}

impl std::fmt::Display for TrackSelection {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            TrackSelection::Track(id) => write!(f, "{id}"),
            TrackSelection::None => write!(f, "no"),
        }
    }
}

/// Internal strongly-typed event enum (ported from tsukimi's `ListenEvent`).
///
/// Sent into the internal `MPV_EVENT_CHANNEL` flume channel for Rust-side consumers
/// (Phase 2.5 watched-status / progress logic). The frontend receives the same data
/// via `AppHandle::emit("fyom://mpv/*")` with event-specific payloads (see
/// `spawn_event_loop`).
#[derive(Debug, Clone, Serialize)]
#[serde(tag = "type", content = "data")]
#[allow(clippy::large_enum_variant)] // TrackList / ChapterList are small in practice
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
    DemuxerCacheTime(i64),
    TimePos(i64),
    PausedForCache(bool),
    ChapterList(ChapterList),
}

// ---------------------------------------------------------------------------
// Internal event channel (reserved for Phase 2.5 Rust-side consumers).
// ---------------------------------------------------------------------------

/// A flume pair carrying `MpvEvent`s to Rust-side consumers.
///
/// The sender is fed by the event-pump thread; the receiver is reserved for Phase 2.5
/// (watched-status / progress logic that should not round-trip through the frontend).
pub struct MpvEventChannel {
    pub tx: Sender<MpvEvent>,
    /// Unused until Phase 2.5 wires a Rust-side consumer.
    #[allow(dead_code)]
    pub rx: Receiver<MpvEvent>,
}

pub static MPV_EVENT_CHANNEL: LazyLock<MpvEventChannel> = LazyLock::new(|| {
    let (tx, rx) = unbounded::<MpvEvent>();
    MpvEventChannel { tx, rx }
});

/// Map an mpv `EndFileReason` code to a human-readable name (per mpv/client.h).
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

// ---------------------------------------------------------------------------
// Event-pump spawn (the core port — tsukimi's `process_events`).
// ---------------------------------------------------------------------------

/// Spawn the mpv event-pump thread.
///
/// Creates an `EventContext` on the current thread, registers the 10 `observe_property`
/// observers (ported verbatim from tsukimi), then spawns a dedicated thread that loops
/// `wait_event(1000.0)`, decodes each event, sends a typed `MpvEvent` into the internal
/// flume channel, AND emits a frontend event `fyom://mpv/<name>` with a typed payload.
///
/// The thread exits when `event_thread_alive` is set to `SHUTDOWN`. Returns the
/// `JoinHandle` so the caller can join on shutdown.
///
/// # Panics
/// Panics if `EventContext` setup (`disable_deprecated_events` / `observe_property`)
/// fails — these are non-recoverable libmpv state errors.
pub fn spawn_event_loop(
    mpv: Arc<Mpv>,
    app_handle: AppHandle,
    event_thread_alive: Arc<AtomicU32>,
) -> JoinHandle<()> {
    // Build + configure the EventContext on this thread (tsukimi's pattern), then move
    // it into the dedicated event thread. `mpv.ctx` is the raw `mpv_handle*`; the
    // `Arc<Mpv>` kept alive in the thread ensures the handle outlives the context.
    let mut event_context = EventContext::new(mpv.ctx);
    event_context
        .disable_deprecated_events()
        .expect("mpv: failed to disable deprecated events");

    // observe_property set — ported verbatim from tsukimi's `process_events`.
    // (reply_userdata ids 0–9 mirror tsukimi; we match on `name` so the ids are
    // informational only.)
    event_context
        .observe_property("duration", Format::Double, 0u64)
        .expect("mpv: observe_property(duration) failed");
    event_context
        .observe_property("pause", Format::Flag, 1u64)
        .expect("mpv: observe_property(pause) failed");
    event_context
        .observe_property("cache-speed", Format::Int64, 2u64)
        .expect("mpv: observe_property(cache-speed) failed");
    event_context
        .observe_property("track-list", Format::Node, 3u64)
        .expect("mpv: observe_property(track-list) failed");
    event_context
        .observe_property("paused-for-cache", Format::Flag, 4u64)
        .expect("mpv: observe_property(paused-for-cache) failed");
    event_context
        .observe_property("demuxer-cache-time", Format::Int64, 5u64)
        .expect("mpv: observe_property(demuxer-cache-time) failed");
    event_context
        .observe_property("time-pos", Format::Int64, 6u64)
        .expect("mpv: observe_property(time-pos) failed");
    event_context
        .observe_property("volume", Format::Int64, 7u64)
        .expect("mpv: observe_property(volume) failed");
    event_context
        .observe_property("chapter-list", Format::Node, 8u64)
        .expect("mpv: observe_property(chapter-list) failed");
    event_context
        .observe_property("speed", Format::Double, 9u64)
        .expect("mpv: observe_property(speed) failed");

    let alive = event_thread_alive.clone();
    let mpv_for_thread = Arc::clone(&mpv);

    std::thread::Builder::new()
        .name("fyom mpv event loop".into())
        .spawn(move || {
            loop {
                // State machine (ported verbatim). SHUTDOWN → exit; PAUSED → block until
                // state changes; ACTIVE → drain events.
                let state = alive.load(Ordering::SeqCst);
                match state {
                    SHUTDOWN => break,
                    PAUSED => atomic_wait::wait(&alive, PAUSED),
                    _ => (),
                }

                match event_context.wait_event(1000.0) {
                    Some(Ok(event)) => match event {
                        Event::PropertyChange { name, change, .. } => {
                            handle_property_change(name, change, &mpv_for_thread, &app_handle);
                        }
                        Event::Seek { .. } => {
                            let _ = MPV_EVENT_CHANNEL.tx.send(MpvEvent::Seek);
                            let _ = app_handle.emit("fyom://mpv/seek", ());
                        }
                        Event::PlaybackRestart { .. } => {
                            let _ = MPV_EVENT_CHANNEL.tx.send(MpvEvent::PlaybackRestart);
                            let _ = app_handle.emit("fyom://mpv/playback-restart", ());
                        }
                        Event::EndFile(reason) => {
                            let _ = MPV_EVENT_CHANNEL.tx.send(MpvEvent::EndFile(reason));
                            let payload = serde_json::json!({
                                "reason": reason,
                                "reason_name": endfile_reason_name(reason),
                            });
                            let _ = app_handle.emit("fyom://mpv/end-file", payload);
                        }
                        Event::FileLoaded => {
                            let _ = MPV_EVENT_CHANNEL.tx.send(MpvEvent::FileLoaded);
                            // mpv doesn't know fyom's media_id; emit the loaded path so
                            // the frontend can correlate with its pending play request.
                            let path: Option<String> = mpv_for_thread.get_property("path").ok();
                            let payload = serde_json::json!({ "path": path });
                            let _ = app_handle.emit("fyom://mpv/file-loaded", payload);
                        }
                        Event::Shutdown => {
                            let _ = MPV_EVENT_CHANNEL.tx.send(MpvEvent::Shutdown);
                            let _ = app_handle.emit("fyom://mpv/shutdown", ());
                        }
                        _ => {}
                    },
                    Some(Err(e)) => {
                        let msg = e.to_string();
                        warn!("[mpv] event error: {}", msg);
                        let _ = MPV_EVENT_CHANNEL
                            .tx
                            .send(MpvEvent::Error(msg.clone()));
                        let _ = app_handle.emit(
                            "fyom://mpv/error",
                            serde_json::json!({ "message": msg }),
                        );
                    }
                    None => {}
                }
            }
            debug!("[mpv] event loop thread exited");
        })
        .expect("failed to spawn fyom mpv event loop thread")
}

/// Decode a property-change event: send a typed `MpvEvent` into the internal channel +
/// emit a `fyom://mpv/<name>` frontend event with a typed payload.
///
/// Payload shapes (per `ROADMAP.md` Phase 2.2):
/// - `fyom://mpv/duration`           → `{ "duration": f64 }`
/// - `fyom://mpv/pause`              → `{ "paused": bool }`
/// - `fyom://mpv/cache-speed`        → `{ "speed": i64 }`
/// - `fyom://mpv/track-list`         → `{ "audio_tracks": [...], "sub_tracks": [...] }`
/// - `fyom://mpv/chapter-list`       → `{ "chapters": [...] }`
/// - `fyom://mpv/volume`             → `{ "volume": i64 }`
/// - `fyom://mpv/speed`              → `{ "speed": f64 }`
/// - `fyom://mpv/demuxer-cache-time` → `{ "time": i64 }`
/// - `fyom://mpv/time-pos`           → `{ "position": i64, "duration": f64 }`
/// - `fyom://mpv/paused-for-cache`   → `{ "paused": bool }`
#[allow(clippy::too_many_lines)] // faithful 1:1 port of tsukimi's match arms
fn handle_property_change(
    name: &str,
    change: PropertyData<'_>,
    mpv: &Mpv,
    app_handle: &AppHandle,
) {
    match name {
        "duration" => {
            if let PropertyData::Double(dur) = change {
                let _ = MPV_EVENT_CHANNEL.tx.send(MpvEvent::Duration(dur));
                let _ = app_handle.emit(
                    "fyom://mpv/duration",
                    serde_json::json!({ "duration": dur }),
                );
            }
        }
        "pause" => {
            if let PropertyData::Flag(pause) = change {
                let _ = MPV_EVENT_CHANNEL.tx.send(MpvEvent::Pause(pause));
                let _ = app_handle.emit(
                    "fyom://mpv/pause",
                    serde_json::json!({ "paused": pause }),
                );
            }
        }
        "cache-speed" => {
            if let PropertyData::Int64(speed) = change {
                let _ = MPV_EVENT_CHANNEL.tx.send(MpvEvent::CacheSpeed(speed));
                let _ = app_handle.emit(
                    "fyom://mpv/cache-speed",
                    serde_json::json!({ "speed": speed }),
                );
            }
        }
        "track-list" => {
            if let PropertyData::Node(node) = change {
                let tracks = node_to_tracks(node);
                let _ = MPV_EVENT_CHANNEL
                    .tx
                    .send(MpvEvent::TrackList(tracks.clone()));
                let _ = app_handle.emit(
                    "fyom://mpv/track-list",
                    serde_json::json!({
                        "audio_tracks": tracks.audio_tracks,
                        "sub_tracks": tracks.sub_tracks,
                    }),
                );
            }
        }
        "chapter-list" => {
            if let PropertyData::Node(node) = change {
                let chapters = node_to_chapter_list(node);
                let _ = MPV_EVENT_CHANNEL
                    .tx
                    .send(MpvEvent::ChapterList(chapters.clone()));
                let _ = app_handle.emit(
                    "fyom://mpv/chapter-list",
                    serde_json::json!({ "chapters": chapters.0 }),
                );
            }
        }
        "volume" => {
            if let PropertyData::Int64(volume) = change {
                let _ = MPV_EVENT_CHANNEL.tx.send(MpvEvent::Volume(volume));
                let _ = app_handle.emit(
                    "fyom://mpv/volume",
                    serde_json::json!({ "volume": volume }),
                );
            }
        }
        "speed" => {
            if let PropertyData::Double(speed) = change {
                let _ = MPV_EVENT_CHANNEL.tx.send(MpvEvent::Speed(speed));
                let _ = app_handle.emit(
                    "fyom://mpv/speed",
                    serde_json::json!({ "speed": speed }),
                );
            }
        }
        "demuxer-cache-time" => {
            if let PropertyData::Int64(time) = change {
                let _ = MPV_EVENT_CHANNEL
                    .tx
                    .send(MpvEvent::DemuxerCacheTime(time));
                let _ = app_handle.emit(
                    "fyom://mpv/demuxer-cache-time",
                    serde_json::json!({ "time": time }),
                );
            }
        }
        "time-pos" => {
            if let PropertyData::Int64(time) = change {
                // Include the current duration so the frontend can render a progress
                // bar from a single event (per the ROADMAP payload contract). `duration`
                // is observed separately + rarely changes; a cheap get_property here is
                // fine (time-pos fires ~1×/sec by default).
                let duration: f64 = mpv.get_property("duration").unwrap_or(0.0);
                let _ = MPV_EVENT_CHANNEL.tx.send(MpvEvent::TimePos(time));
                let _ = app_handle.emit(
                    "fyom://mpv/time-pos",
                    serde_json::json!({ "position": time, "duration": duration }),
                );
            }
        }
        "paused-for-cache" => {
            if let PropertyData::Flag(pause) = change {
                // tsukimi ORs in `seeking` so the UI shows a buffering indicator while
                // seeking too — ported verbatim.
                let seeking: bool = mpv.get_property("seeking").unwrap_or(false);
                let buffered_paused = pause || seeking;
                let _ = MPV_EVENT_CHANNEL
                    .tx
                    .send(MpvEvent::PausedForCache(buffered_paused));
                let _ = app_handle.emit(
                    "fyom://mpv/paused-for-cache",
                    serde_json::json!({ "paused": buffered_paused }),
                );
            }
        }
        _ => {}
    }
}

// ---------------------------------------------------------------------------
// Node parsers (ported from tsukimi, with defensive `unwrap_or`).
// ---------------------------------------------------------------------------

/// Parse the `track-list` property node into `MpvTracks`.
///
/// Ported from tsukimi's `node_to_tracks` with a defensive twist: missing `id`/`title`/
/// `lang`/`type` fields fall back to sane defaults instead of panicking (a malformed
/// track node should not crash the event thread).
fn node_to_tracks(node: MpvNode) -> MpvTracks {
    let mut audio_tracks = Vec::new();
    let mut sub_tracks = Vec::new();
    let Some(array) = node.array() else {
        return MpvTracks::default();
    };
    for entry in array {
        let Some(map) = entry.map() else {
            continue;
        };
        let range = map.collect::<HashMap<_, _>>();
        let id = range.get("id").and_then(|v| v.i64()).unwrap_or(0);
        let title = range
            .get("title")
            .and_then(|v| v.str())
            .unwrap_or("unknown")
            .to_string();
        let lang = range
            .get("lang")
            .and_then(|v| v.str())
            .unwrap_or("unknown")
            .to_string();
        let type_ = range
            .get("type")
            .and_then(|v| v.str())
            .unwrap_or("unknown")
            .to_string();
        let track = MpvTrack {
            id,
            title,
            lang,
            type_,
        };
        if track.type_ == "audio" {
            audio_tracks.push(track);
        } else if track.type_ == "sub" {
            sub_tracks.push(track);
        }
    }
    MpvTracks {
        audio_tracks,
        sub_tracks,
    }
}

/// Parse the `chapter-list` property node into `ChapterList`.
///
/// Ported from tsukimi's `node_to_chapter_list` with a defensive `unwrap_or` on field
/// access.
fn node_to_chapter_list(node: MpvNode) -> ChapterList {
    let mut chapters = Vec::new();
    let Some(array) = node.array() else {
        return ChapterList::default();
    };
    for entry in array {
        let Some(map) = entry.map() else {
            continue;
        };
        let range = map.collect::<HashMap<_, _>>();
        let title = range
            .get("title")
            .and_then(|v| v.str())
            .unwrap_or("unknown")
            .to_string();
        let time = range.get("time").and_then(|v| v.f64()).unwrap_or(0.0);
        chapters.push(Chapter { title, time });
    }
    ChapterList(chapters)
}
