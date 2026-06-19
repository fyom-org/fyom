//! Linux Wayland render surface scaffold for native mpv embedding.
//!
//! ## Architecture
//!
//! Wayland native embedding is fundamentally different from X11.
//!
//! X11:
//! - `--wid=<xwindow>` works because an X11 window has an integer id.
//!
//! Wayland:
//! - `wl_surface` is a pointer, not a stable integer window id.
//! - mpv `--wid` is not meaningful for native Wayland embedding.
//! - Passing a fake integer id would either fail or force XWayland fallback.
//!
//! Correct native Wayland embedding requires mpv-side support for attaching its
//! video surface as a `wl_subsurface` of the host application's `wl_surface`.
//!
//! Intended future mpv patch direction:
//!
//! ```text
//! --wl-parent-surface=<hex wl_surface*>
//! ```
//!
//! Then mpv can call:
//!
//! ```c
//! wl_subcompositor_get_subsurface(
//!     wl->subcompositor,
//!     wl->surface,
//!     parent_surface
//! );
//! wl_subsurface_set_desync(wl->sub);
//! ```
//!
//! ## Current behavior
//!
//! This module intentionally does not expose `native_window_id()`.
//! Returning a value there would make upper layers pass it as mpv `wid`, which is
//! wrong for native Wayland.
//!
//! Until the mpv `wl-parent-surface` patch exists, this backend is diagnostic-only
//! and should fail explicitly at attach time instead of silently using XWayland.

use std::ffi::c_void;
use std::ptr;

use raw_window_handle::{HasDisplayHandle, HasWindowHandle, RawDisplayHandle, RawWindowHandle};

use crate::mpv::render::RenderSurface;

// -----------------------------------------------------------------------------
// WaylandSurface
// -----------------------------------------------------------------------------

/// Native Wayland host surface metadata.
///
/// This struct intentionally does not claim to be a valid mpv `wid` target.
/// It only retains the raw Wayland pointers needed by the future mpv
/// `wl-parent-surface` integration.
pub struct WaylandSurface {
    /// Host application's Wayland display pointer.
    ///
    /// This is owned by the windowing toolkit. Do not destroy it here.
    display: *mut c_void,

    /// Host application's parent `wl_surface*`.
    ///
    /// This is owned by the windowing toolkit. Do not destroy it here.
    parent_surface: *mut c_void,
}

// SAFETY:
// This struct only stores raw toolkit-owned Wayland pointers for diagnostics and
// future option passing. It does not mutate or destroy Wayland objects.
// Actual Wayland protocol interaction must remain on the owning toolkit thread.
unsafe impl Send for WaylandSurface {}
unsafe impl Sync for WaylandSurface {}

impl WaylandSurface {
    pub fn new(display: *mut c_void, parent_surface: *mut c_void) -> Result<Self, String> {
        if display.is_null() {
            return Err("Wayland wl_display pointer is null".to_string());
        }

        if parent_surface.is_null() {
            return Err("Wayland wl_surface pointer is null".to_string());
        }

        Ok(Self {
            display,
            parent_surface,
        })
    }

    /// Returns the host `wl_display*`.
    ///
    /// Diagnostic/future integration only.
    pub fn display_ptr(&self) -> *mut c_void {
        self.display
    }

    /// Returns the host parent `wl_surface*`.
    ///
    /// Diagnostic/future integration only.
    pub fn parent_surface_ptr(&self) -> *mut c_void {
        self.parent_surface
    }

    /// Hex string form intended for a future mpv option:
    ///
    /// ```text
    /// wl-parent-surface=0x...
    /// ```
    ///
    /// Do not pass this to mpv `wid`.
    pub fn parent_surface_hex(&self) -> String {
        format!("0x{:x}", self.parent_surface as usize)
    }
}

impl RenderSurface for WaylandSurface {
    /// In native Wayland embedding, FYOM does not own mpv's GPU context.
    fn make_current(&self) -> Result<(), String> {
        Ok(())
    }

    /// In native Wayland embedding, mpv resolves GPU symbols internally.
    fn get_proc_address(&self, _name: &str) -> *mut c_void {
        ptr::null_mut()
    }

    /// Raw window handles do not expose reliable drawable dimensions here.
    ///
    /// Upper layers should drive layout through explicit resize commands once the
    /// mpv Wayland subsurface patch exists.
    fn drawable_size(&self) -> (i32, i32) {
        (0, 0)
    }

    /// In native Wayland embedding, mpv presents internally.
    fn swap_buffers(&self) {}

    /// Wayland native surfaces are not valid mpv `wid` targets.
    ///
    /// Returning `None` is intentional. It prevents the upper playback layer from
    /// passing a `wl_surface*` pointer as `--wid`, which would be incorrect.
    fn native_window_id(&self) -> Option<String> {
        None
    }

    fn backend_name(&self) -> &'static str {
        "linux-wayland-wl-surface"
    }
}

// -----------------------------------------------------------------------------
// Factory
// -----------------------------------------------------------------------------

/// Create a diagnostic Wayland surface from a Tauri WebviewWindow.
///
/// This does not make mpv native Wayland embedding work by itself. It only
/// captures the parent `wl_surface*` needed by the future mpv patch.
///
/// Current expected upper-layer result:
///
/// ```text
/// platform surface does not expose a native window id for mpv wid embedding
/// ```
///
/// That failure is correct until mpv supports `wl-parent-surface`.
pub fn create_surface(window: &tauri::WebviewWindow) -> Result<Box<dyn RenderSurface>, String> {
    let window_handle = window
        .window_handle()
        .map_err(|error| format!("failed to get raw window handle: {error}"))?;

    let display_handle = window
        .display_handle()
        .map_err(|error| format!("failed to get raw display handle: {error}"))?;

    let wayland_window = match window_handle.as_ref() {
        RawWindowHandle::Wayland(handle) => handle,
        other => {
            return Err(format!(
                "expected Wayland raw window handle, got {}",
                raw_window_handle_name(other)
            ));
        }
    };

    let wayland_display = match display_handle.as_ref() {
        RawDisplayHandle::Wayland(handle) => handle,
        other => {
            return Err(format!(
                "expected Wayland raw display handle, got {}",
                raw_display_handle_name(other)
            ));
        }
    };

    let display = wayland_display.display.as_ptr() as *mut c_void;
    let parent_surface = wayland_window.surface.as_ptr() as *mut c_void;

    let surface = WaylandSurface::new(display, parent_surface)?;

    tracing::info!(
        "[platform/wayland] captured host wl_display and wl_surface for future native embedding; display={:?}; parent_surface={}",
        surface.display_ptr(),
        surface.parent_surface_hex()
    );

    tracing::warn!(
        "[platform/wayland] native mpv embedding is not active yet; mpv --wid is invalid on Wayland; requires mpv wl-parent-surface patch"
    );

    Ok(Box::new(surface))
}

// -----------------------------------------------------------------------------
// Diagnostics
// -----------------------------------------------------------------------------

fn raw_window_handle_name(handle: &RawWindowHandle) -> &'static str {
    match handle {
        RawWindowHandle::UiKit(_) => "UIKit",
        RawWindowHandle::AppKit(_) => "AppKit",
        RawWindowHandle::Orbital(_) => "Orbital",
        RawWindowHandle::Xlib(_) => "Xlib",
        RawWindowHandle::Xcb(_) => "Xcb",
        RawWindowHandle::Wayland(_) => "Wayland",
        RawWindowHandle::Drm(_) => "Drm",
        RawWindowHandle::Gbm(_) => "Gbm",
        RawWindowHandle::Win32(_) => "Win32",
        RawWindowHandle::WinRt(_) => "WinRt",
        RawWindowHandle::Web(_) => "Web",
        RawWindowHandle::WebCanvas(_) => "WebCanvas",
        RawWindowHandle::WebOffscreenCanvas(_) => "WebOffscreenCanvas",
        RawWindowHandle::AndroidNdk(_) => "AndroidNdk",
        RawWindowHandle::Haiku(_) => "Haiku",
        _ => "Unknown",
    }
}

fn raw_display_handle_name(handle: &RawDisplayHandle) -> &'static str {
    match handle {
        RawDisplayHandle::UiKit(_) => "UIKit",
        RawDisplayHandle::AppKit(_) => "AppKit",
        RawDisplayHandle::Orbital(_) => "Orbital",
        RawDisplayHandle::Xlib(_) => "Xlib",
        RawDisplayHandle::Xcb(_) => "Xcb",
        RawDisplayHandle::Wayland(_) => "Wayland",
        RawDisplayHandle::Drm(_) => "Drm",
        RawDisplayHandle::Gbm(_) => "Gbm",
        RawDisplayHandle::Windows(_) => "Windows",
        RawDisplayHandle::Web(_) => "Web",
        RawDisplayHandle::Android(_) => "Android",
        RawDisplayHandle::Haiku(_) => "Haiku",
        _ => "Unknown",
    }
}
