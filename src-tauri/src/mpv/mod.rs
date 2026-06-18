//! libmpv native playback integration (Phase 2).
//!
//! This module wraps the `libmpv2` crate (the same safe Rust binding tsukimi uses) and
//! exposes a thread-safe `MpvInstance` to the Tauri command layer.
//!
//! ## Module layout
//! - [`handle`] — `MpvInstance`: thread-safe wrapper around `libmpv2::Mpv` (create +
//!   initialize + command/property facade + event-thread lifecycle).
//! - [`event_loop`] — the event pump: a dedicated thread looping `wait_event`,
//!   `observe_property` × 10, `MpvEvent` enum (ported from tsukimi's `ListenEvent`),
//!   `node_to_tracks` / `node_to_chapter_list` parsers, and `AppHandle::emit` adapters
//!   that broadcast `fyom://mpv/*` to the frontend.
//! - [`options_matcher`] — pure integer → mpv-option-string matchers (ported verbatim
//!   from tsukimi; Phase 2.4 wires them to fyom's settings store).
//!
//! ## Phase 2.2 scope (this module + `commands/playback.rs`)
//! - Event-pump thread spawns on app startup, observes 10 properties, and emits typed
//!   `fyom://mpv/*` events to the frontend (additive — no 9.7 contract change).
//! - Command surface expanded: seek / pause / volume / speed / audio-track /
//!   subtitle-track / `mpv_keypress` / generic `mpv_command`.
//! - `get_playback_backend_info` reports the real libmpv version + active hwdec.
//! - No GL rendering yet (Phase 2.3); video is a black frame, audio plays. The
//!   browser `<video>` fallback stays green (9.7 guardrail).
//!
//! ## Attribution
//! The event pump (`event_loop.rs`) + `options_matcher.rs` are ported from tsukimi
//! (`tsukinaha/tsukimi`, GPL-3.0) `src/ui/mpv/{tsukimi_mpv,options_matcher}.rs`.
//! The initializer property set + the `Arc<Mpv>` + `unsafe impl Send/Sync` pattern
//! (in `handle.rs`) are ported from tsukimi's `TsukimiMPV::default`. See
//! `docs/libmpv-assessment.md` §2.2 + §3.4 for the reuse inventory.

pub mod event_loop;
pub mod handle;
pub mod options_matcher;

pub use event_loop::{
    Chapter, ChapterList, MpvEvent, MpvTrack, MpvTracks, TrackSelection, ACTIVE, PAUSED, SHUTDOWN,
};
pub use handle::MpvInstance;
