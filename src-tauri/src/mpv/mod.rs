//! libmpv native playback integration.
//!
//! This module is the top-level namespace for fyom's native playback backend. It wraps
//! `libmpv2` and exposes a thread-safe `MpvInstance` to the Tauri command layer.
//!
//! ## Module layout
//!
//! - [`handle`] — owns `MpvInstance`, the thread-safe facade around `libmpv2::Mpv`.
//!   It is responsible for mpv creation, initialization, command/property helpers,
//!   event-thread lifecycle, and render-thread lifecycle.
//!
//! - [`event_loop`] — the mpv event pump. It runs a dedicated thread around
//!   `wait_event`, observes mpv properties, parses mpv nodes into strongly typed Rust
//!   structs, and emits `fyom://mpv/*` events to the frontend.
//!
//! - [`render`] — the OpenGL render context and render loop. It hosts
//!   `libmpv2::render::RenderContext` on a platform-specific GL surface provided by
//!   `crate::platform`.
//!
//! - [`options_matcher`] — pure integer-to-mpv-option matchers, ported from tsukimi
//!   and wired by higher-level playback/settings code.
//!
//! ## Re-export policy
//!
//! Keep this module intentionally narrow.
//!
//! `MpvInstance` is the only public facade re-exported from `crate::mpv` because the
//! command layer and application state treat it as the native playback service.
//!
//! Event-loop and render types should be imported from their concrete modules instead:
//!
//! - `crate::mpv::event_loop::TrackSelection`
//! - `crate::mpv::event_loop::MpvTrack`
//! - `crate::mpv::render::RenderSurface`
//! - `crate::mpv::render::RenderThreadState`
//!
//! This avoids unused barrel re-exports and makes dependency ownership explicit.

pub mod event_loop;
pub mod handle;
pub mod options_matcher;
pub mod render;

pub use handle::MpvInstance;
