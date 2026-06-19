//! libmpv native playback integration.
//!
//! This module is the top-level namespace for fyom's native playback backend.
//! It wraps `libmpv2` and exposes a thread-safe `MpvInstance` to the Tauri
//! command layer.
//!
//! ## Current rendering architecture
//!
//! FYOM currently uses native window embedding, not mpv's render API.
//!
//! macOS production path:
//!
//! 1. Create a dedicated AppKit `NSView`.
//! 2. Attach a `CAMetalLayer` to that view.
//! 3. Pass the `NSView` pointer to mpv through the `wid` option.
//! 4. Set GPU startup options before `mpv_initialize()`.
//! 5. Let mpv own Vulkan/MoltenVK/Metal rendering internally.
//!
//! Important:
//!
//! `mpv_render_context` must not be initialized for the normal embedded playback
//! path. It is only relevant for a future texture-sharing architecture where FYOM
//! owns the GPU context and asks mpv to render into host-managed targets.
//!
//! ## Module layout
//!
//! - [`handle`]
//!   Owns `MpvInstance`, the thread-safe facade around `libmpv2::Mpv`.
//!   It is responsible for mpv creation, initialization, command/property helpers,
//!   event-thread lifecycle, and native surface lifecycle retention.
//!
//! - [`event_loop`]
//!   Owns the mpv event pump. It runs a dedicated thread around `wait_event`,
//!   observes mpv properties, parses mpv nodes into strongly typed Rust structs,
//!   and emits `fyom://mpv/*` events to the frontend.
//!
//! - [`render`]
//!   Defines the platform render-surface abstraction.
//!
//!   Despite the module name, the current production path does not create an
//!   `mpv_render_context` and does not drive an OpenGL render loop.
//!
//!   In `--wid` mode, this module only provides:
//!
//!   - `RenderSurface` trait
//!   - surface lifecycle thread compatibility
//!   - explicit documentation that mpv owns the GPU context
//!
//! - [`options_matcher`]
//!   Pure integer-to-mpv-option matchers, ported from tsukimi and wired by
//!   higher-level playback/settings code.
//!
//! ## Startup invariant
//!
//! For native video output to work, platform window embedding options must be
//! configured before `mpv_initialize()`.
//!
//! Required macOS startup options:
//!
//! ```text
//! wid=<decimal_nsview_pointer>
//! vo=gpu
//! gpu-api=vulkan
//! gpu-context=macvk
//! log-file=/tmp/mpv-fyom.log
//! msg-level=all=v
//! ```
//!
//! If `wid` is set after `mpv_initialize()`, mpv will not bind to the native
//! child view correctly.
//!
//! ## Re-export policy
//!
//! Keep this module intentionally narrow.
//!
//! `MpvInstance` is the only public facade re-exported from `crate::mpv` because
//! the command layer and application state treat it as the native playback service.
//!
//! Event-loop and render types should be imported from their concrete modules
//! instead:
//!
//! - `crate::mpv::event_loop::TrackSelection`
//! - `crate::mpv::event_loop::MpvTrack`
//! - `crate::mpv::render::RenderSurface`
//!
//! This avoids unused barrel re-exports and makes dependency ownership explicit.

pub mod event_loop;
pub mod handle;
pub mod options_matcher;
pub mod render;

pub use handle::MpvInstance;
