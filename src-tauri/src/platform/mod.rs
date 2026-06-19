//! Platform-specific native render surface dispatch for mpv embedding.
//!
//! This module is the single entry point for selecting the platform surface
//! implementation used by FYOM's native playback backend.
//!
//! ## Current architecture
//!
//! FYOM currently uses native surface embedding instead of `mpv_render_context`.
//!
//! macOS:
//! - creates a dedicated `NSView`
//! - attaches a `CAMetalLayer`
//! - exposes the `NSView` pointer as mpv `wid`
//! - lets mpv own Vulkan/MoltenVK/Metal rendering
//!
//! Linux X11:
//! - can expose an X11 window id as mpv `wid`
//!
//! Linux Wayland:
//! - does not support mpv `wid` natively
//! - captures `wl_surface*` for diagnostics/future mpv patching
//! - intentionally returns no `native_window_id` until mpv supports a
//!   `wl-parent-surface` style option
//!
//! Windows:
//! - kept behind the platform module boundary
//! - not part of the current macOS/Wayland debugging path
//!
//! ## Design principles
//!
//! - No fake native window ids.
//! - No silent XWayland fallback for native Wayland.
//! - No `mpv_render_context` initialization in this dispatch layer.
//! - Each platform module owns its surface lifecycle and constraints.
//! - Linux backend selection is explicit and logged.
//!
//! ## Linux backend override
//!
//! For debugging, set:
//!
//! ```text
//! FYOM_LINUX_VIDEO_BACKEND=x11
//! FYOM_LINUX_VIDEO_BACKEND=wayland
//! FYOM_LINUX_VIDEO_BACKEND=auto
//! ```
//!
//! If unset or set to `auto`, the dispatcher prefers Wayland when
//! `WAYLAND_DISPLAY` is present, otherwise X11 when `DISPLAY` is present.

use crate::mpv::render::RenderSurface;

// -----------------------------------------------------------------------------
// Platform modules
// -----------------------------------------------------------------------------

#[cfg(target_os = "macos")]
pub(crate) mod macos;

#[cfg(target_os = "windows")]
pub(crate) mod windows;

#[cfg(target_os = "linux")]
pub(crate) mod linux_x11;

#[cfg(target_os = "linux")]
pub(crate) mod linux_wayland;

// -----------------------------------------------------------------------------
// Public factory
// -----------------------------------------------------------------------------

/// Create the platform-specific native render surface.
///
/// This function must:
///
/// - return a valid `RenderSurface` when the selected backend supports it
/// - return `Err` with a clear diagnostic message when unsupported
/// - avoid silent fallback between incompatible Linux backends
///
/// The caller is responsible for:
///
/// - logging the error
/// - deciding whether to fall back to web playback or standalone mpv window mode
pub fn create_platform_surface(
    window: &tauri::WebviewWindow,
) -> Result<Box<dyn RenderSurface>, String> {
    // -------------------------
    // macOS
    // -------------------------
    #[cfg(target_os = "macos")]
    {
        tracing::info!("[platform] selecting macOS NSView + CAMetalLayer surface");
        return macos::create_surface(window);
    }

    // -------------------------
    // Windows
    // -------------------------
    #[cfg(target_os = "windows")]
    {
        tracing::info!("[platform] selecting Windows native surface");
        return windows::create_surface(window);
    }

    // -------------------------
    // Linux
    // -------------------------
    #[cfg(target_os = "linux")]
    {
        match select_linux_backend() {
            LinuxVideoBackend::Wayland => {
                tracing::info!("[platform] selecting Linux Wayland wl_surface backend");
                return linux_wayland::create_surface(window);
            }

            LinuxVideoBackend::X11 => {
                tracing::info!("[platform] selecting Linux X11 backend");
                return linux_x11::create_surface(window);
            }
        }
    }

    // -------------------------
    // Unsupported platform
    // -------------------------
    #[cfg(not(any(target_os = "macos", target_os = "windows", target_os = "linux")))]
    {
        Err("unsupported target OS: no native render surface implementation available".to_string())
    }
}

// -----------------------------------------------------------------------------
// Linux backend selection
// -----------------------------------------------------------------------------

#[cfg(target_os = "linux")]
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum LinuxVideoBackend {
    X11,
    Wayland,
}

#[cfg(target_os = "linux")]
fn select_linux_backend() -> LinuxVideoBackend {
    match linux_backend_override().as_deref() {
        Some("x11") => {
            tracing::info!("[platform/linux] backend override: x11");
            return LinuxVideoBackend::X11;
        }

        Some("wayland") => {
            tracing::info!("[platform/linux] backend override: wayland");
            return LinuxVideoBackend::Wayland;
        }

        Some("auto") | None => {}

        Some(other) => {
            tracing::warn!(
                "[platform/linux] unknown FYOM_LINUX_VIDEO_BACKEND={other}; falling back to auto"
            );
        }
    }

    let has_wayland = env_is_non_empty("WAYLAND_DISPLAY");
    let has_x11 = env_is_non_empty("DISPLAY");
    let session_type = std::env::var("XDG_SESSION_TYPE")
        .ok()
        .map(|value| value.trim().to_ascii_lowercase());

    if has_wayland {
        tracing::info!(
            "[platform/linux] auto detected Wayland; WAYLAND_DISPLAY is set; XDG_SESSION_TYPE={:?}",
            session_type
        );

        return LinuxVideoBackend::Wayland;
    }

    if matches!(session_type.as_deref(), Some("wayland")) {
        tracing::info!("[platform/linux] auto detected Wayland; XDG_SESSION_TYPE=wayland");

        return LinuxVideoBackend::Wayland;
    }

    if has_x11 {
        tracing::info!("[platform/linux] auto detected X11; DISPLAY is set");
        return LinuxVideoBackend::X11;
    }

    tracing::warn!(
        "[platform/linux] neither WAYLAND_DISPLAY nor DISPLAY is set; defaulting to Wayland for explicit failure"
    );

    LinuxVideoBackend::Wayland
}

#[cfg(target_os = "linux")]
fn linux_backend_override() -> Option<String> {
    std::env::var("FYOM_LINUX_VIDEO_BACKEND")
        .ok()
        .map(|value| value.trim().to_ascii_lowercase())
        .filter(|value| !value.is_empty())
}

#[cfg(target_os = "linux")]
fn env_is_non_empty(name: &str) -> bool {
    std::env::var(name)
        .map(|value| !value.trim().is_empty())
        .unwrap_or(false)
}
