//! Platform-specific GL surface dispatch for mpv rendering.
//!
//! This module is the **single entry point** for selecting the render surface
//! implementation based on the target platform at compile time.
//
//! ## Design Principles
//!
//! - No implicit "default" platform.
//! - Linux must explicitly select a backend (currently X11).
//! - Each platform module owns its surface lifecycle & constraints.
//! - The dispatcher must remain deterministic and transparent.
//!
//! ## Current backends
//!
//! - macOS   → `macos` (NSOpenGL)
//! - Windows → `windows` (WGL)
//! - Linux   → `linux_x11` (X11 + GLX)
//!
//! ## Future
//!
//! - `linux_wayland` (EGL) will be added in Phase 2.5+
//! - runtime negotiation (X11 vs Wayland) will move out of compile-time cfg

use crate::mpv::render::RenderSurface;

// -----------------------------------------------------------------------------
// Platform modules.
// -----------------------------------------------------------------------------

#[cfg(target_os = "macos")]
pub(crate) mod macos;

#[cfg(target_os = "windows")]
pub(crate) mod windows;

#[cfg(target_os = "linux")]
#[path = "linux-x11.rs"]
pub(crate) mod linux_x11;

// -----------------------------------------------------------------------------
// Public factory.
// -----------------------------------------------------------------------------

/// Create the platform-specific GL render surface.
///
/// This function must:
///
/// - Return a valid `RenderSurface` when the platform supports it
/// - Return `Err` with a **clear diagnostic message** when unsupported
///
/// The caller (Tauri setup) is responsible for:
///
/// - logging the error
/// - falling back to `<video>`
///
/// ## Guarantees
///
/// - Deterministic: no runtime guessing
/// - No silent fallback
/// - No "default.rs"
pub fn create_platform_surface(
    window: &tauri::WebviewWindow,
) -> Result<Box<dyn RenderSurface>, String> {
    // -------------------------
    // macOS
    // -------------------------
    #[cfg(target_os = "macos")]
    {
        tracing::info!("[platform] selecting macOS (NSOpenGL) surface");
        return macos::create_surface(window);
    }

    // -------------------------
    // Windows
    // -------------------------
    #[cfg(target_os = "windows")]
    {
        tracing::info!("[platform] selecting Windows (WGL) surface");
        return windows::create_surface(window);
    }

    // -------------------------
    // Linux (X11)
    // -------------------------
    #[cfg(target_os = "linux")]
    {
        tracing::info!("[platform] selecting Linux X11 (GLX) surface");
        return linux_x11::create_surface(window);
    }

    // -------------------------
    // Unsupported platform
    // -------------------------
    #[cfg(not(any(target_os = "macos", target_os = "windows", target_os = "linux")))]
    {
        return Err(
            "unsupported target OS: no GL render surface implementation available".to_string(),
        );
    }
}
