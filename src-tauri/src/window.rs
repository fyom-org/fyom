//! Window lifecycle management

use anyhow::Result;
use tauri::{AppHandle, Manager, Runtime};

use crate::MAIN_WINDOW_LABEL;

/// Setup the main window with close-to-tray behavior.
pub fn setup_main_window<R: Runtime>(app: &tauri::App<R>) -> Result<()> {
    // Window is configured as invisible in tauri.conf.json.
    // We show it only when the sidecar is ready.
    Ok(())
}

/// Configure the window to hide to tray instead of closing.
/// This is called from the window event handler.
pub fn on_window_close_requested<R: Runtime>(
    app: &AppHandle<R>,
    // The tauri Window is obtained from the AppHandle using MAIN_WINDOW_LABEL
) -> bool {
    if let Some(window) = app.get_webview_window(MAIN_WINDOW_LABEL) {
        if let Err(e) = window.hide() {
            tracing::error!("Failed to hide window: {}", e);
            return false; // Allow close if hide fails
        }
        return true; // Prevent close
    }
    false
}

/// Show and focus the main window.
pub fn show_main_window<R: Runtime>(app: &AppHandle<R>) -> Result<()> {
    if let Some(window) = app.get_webview_window(MAIN_WINDOW_LABEL) {
        window.show()?;
        window.set_focus()?;
    }
    Ok(())
}

/// Hide the main window to tray.
pub fn hide_to_tray<R: Runtime>(app: &AppHandle<R>) -> Result<()> {
    if let Some(window) = app.get_webview_window(MAIN_WINDOW_LABEL) {
        window.hide()?;
    }
    Ok(())
}
