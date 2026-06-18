//! Per-platform GL surface for mpv rendering (Phase 2.3).
//!
//! fyom gets the `RawWindowHandle` from Tauri (Tauri windows implement
//! `HasRawWindowHandle` via the `raw-window-handle` crate). Then per platform:
//!
//! - **macOS** (`macos.rs`): create an `NSOpenGLContext` + `NSOpenGLView` as a child layer
//!   **behind** the `WKWebView` (the webview's `WKWebView` has a transparent background
//!   when `.video-mode` is active). `get_proc_address` via `dlsym` to the OpenGL framework.
//! - **Windows** (`windows.rs`): create a child `HWND` + WGL context via
//!   `wglCreateContext` + `wglMakeCurrent`. `get_proc_address` via `wglGetProcAddress`.
//! - **Linux** (`default.rs`): child X11 `Window` (XID) + GLX context via
//!   `glXCreateContext` + `glXMakeCurrent`. XWayland fallback for v1 (works under both
//!   X11 + Wayland via XWayland).
//!
//! ## Attribution
//! The window-lifecycle / transparency logic direction is ported from soia
//! (`FengZeng/soia`, GPL-3.0) `src-tauri/src/platform/{mod,macos,windows,default}.rs` —
//! but soia's `libsoia_utils` Metal-layer surface is replaced with the simpler NSOpenGL
//! path (soia's ~400 LOC Metal-layer code → fyom's ~250 LOC NSOpenGL code). See
//! `docs/libmpv-assessment.md` §3.3 for the rationale.

use crate::mpv::render::RenderSurface;

#[cfg(target_os = "macos")]
pub(crate) mod macos;

#[cfg(target_os = "windows")]
pub(crate) mod windows;

#[cfg(not(any(target_os = "macos", target_os = "windows")))]
mod default;

// ---------------------------------------------------------------------------
// Factory — dispatch to the platform impl.
// ---------------------------------------------------------------------------

/// Create the platform GL surface for the main Tauri window.
///
/// Called once from the Tauri `setup` hook (after `MpvState` is created + the event loop
/// is spawned). On failure, returns `Err` — the caller logs + continues without GL
/// rendering (the 9.7 `<video>` fallback stays green; mpv plays audio with a black frame).
///
/// The returned `Box<dyn RenderSurface>` is moved into the render thread (which is the
/// sole consumer of the GL context).
pub fn create_platform_surface(
    window: &tauri::WebviewWindow,
) -> Result<Box<dyn RenderSurface>, String> {
    #[cfg(target_os = "macos")]
    {
        return macos::create_surface(window);
    }
    #[cfg(target_os = "windows")]
    {
        return windows::create_surface(window);
    }
    #[cfg(not(any(target_os = "macos", target_os = "windows")))]
    {
        return default::create_surface(window);
    }
}
