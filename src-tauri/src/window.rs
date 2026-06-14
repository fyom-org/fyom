//! Window lifecycle management

use anyhow::Result;
use tauri::{AppHandle, Manager, Runtime};

use crate::MAIN_WINDOW_LABEL;

/// Setup the main window. Window starts invisible and is shown only
/// after the sidecar is ready.
pub fn setup_main_window<R: Runtime>(_app: &tauri::App<R>) -> Result<()> {
    Ok(())
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
