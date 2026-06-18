//! libmpv native playback integration (Phase 2).
//!
//! This module wraps the `libmpv2` crate (the same safe Rust binding tsukimi uses) and
//! exposes a thread-safe `MpvInstance` to the Tauri command layer.
//!
//! ## Module layout
//! - [`handle`] — `MpvInstance`: thread-safe wrapper around `libmpv2::Mpv` (create +
//!   initialize + command/property facade + event-thread lifecycle + render-thread
//!   lifecycle).
//! - [`event_loop`] — the event pump: a dedicated thread looping `wait_event`,
//!   `observe_property` × 10, `MpvEvent` enum (ported from tsukimi's `ListenEvent`),
//!   `node_to_tracks` / `node_to_chapter_list` parsers, and `AppHandle::emit` adapters
//!   that broadcast `fyom://mpv/*` to the frontend.
//! - [`render`] — the GL render context + render loop (Phase 2.3): a dedicated thread
//!   hosting `libmpv2::render::RenderContext` on a per-platform GL surface (see
//!   `crate::platform`). Ports tsukimi's `mpvglarea.rs` pattern (drop the GTK `GLArea`
//!   shell — fyom owns its own NSOpenGL / WGL / GLX surface).
//! - [`options_matcher`] — pure integer → mpv-option-string matchers (ported verbatim
//!   from tsukimi; Phase 2.4 wires them to fyom's settings store).
//!
//! ## Phase 2.3 scope (this module + `commands/playback.rs` + `platform/*`)
//! - Render-thread lifecycle: `MpvInstance::spawn_render_thread(surface)` spawns the
//!   thread (lazy — called from the `attach_render_surface` Tauri command after the
//!   frontend has a window handle). `shutdown_render_thread` joins it on app exit
//!   (before `shutdown_event_loop`).
//! - The render thread creates `RenderContext::new(OpenGl, OpenGLInitParams{get_proc_address})`
//!   on the platform GL surface + registers `set_update_callback` → `RENDER_UPDATE` flume
//!   channel → render-thread wake (port tsukimi's `RENDER_UPDATE` pattern).
//! - The render loop reads the current FBO via `glow::get_parameter_i32(FRAMEBUFFER_BINDING)`
//!   + calls `ctx.render::<()>(fbo, w, h, true)` (port tsukimi's `render()` body verbatim).
//! - Per-platform GL surfaces: macOS NSOpenGLContext, Windows WGL, Linux GLX (XWayland
//!   fallback for Wayland — native Wayland + EGL is Phase 2.5+).
//! - Frontend: `.video-mode` CSS class toggles the webview root background to
//!   `transparent !important` when a file loads (soia's z-order trick, render-backend-
//!   agnostic — works for OpenGL exactly as it worked for soia's Vulkan).
//! - The 9.7 guardrail is honored: if the platform surface or render-context creation
//!   fails, the `<video>` fallback stays green (mpv plays audio with a black frame).
//!
//! ## Attribution
//! The event pump (`event_loop.rs`) + `options_matcher.rs` are ported from tsukimi
//! (`tsukinaha/tsukimi`, GPL-3.0) `src/ui/mpv/{tsukimi_mpv,options_matcher}.rs`.
//! The render loop pattern (`render.rs`) is ported from tsukimi's
//! `src/ui/mpv/mpvglarea.rs` (drop the GTK `GLArea` shell). The initializer property set +
//! the `Arc<Mpv>` + `unsafe impl Send/Sync` pattern (in `handle.rs`) are ported from
//! tsukimi's `TsukimiMPV::default`. See `docs/libmpv-assessment.md` §2.2 + §3.3 + §3.4
//! for the reuse inventory.

pub mod event_loop;
pub mod handle;
pub mod options_matcher;
pub mod render;

pub use event_loop::{
    Chapter, ChapterList, MpvEvent, MpvTrack, MpvTracks, TrackSelection, ACTIVE, PAUSED, SHUTDOWN,
};
pub use handle::MpvInstance;
pub use render::{RenderSurface, RenderThreadState};
