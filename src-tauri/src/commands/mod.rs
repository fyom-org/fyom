//! Tauri command handlers.
//!
//! This module contains application-level Tauri invoke handlers.
//! Native playback commands live in `commands::playback`.

pub mod playback;

use tauri::{AppHandle, Manager, State};

use crate::sidecar;
use crate::{AppState, SidecarStatus};

/// Show the main window.
#[tauri::command]
pub async fn show_window(app: AppHandle) -> Result<(), String> {
    crate::window::show_main_window(&app).map_err(|error| error.to_string())
}

/// Hide the main window to tray.
#[tauri::command]
pub async fn hide_window(app: AppHandle) -> Result<(), String> {
    crate::window::hide_to_tray(&app).map_err(|error| error.to_string())
}

/// Request application quit.
///
/// This function is intentionally idempotent:
/// - only exits when `exit_intent` is set
/// - only the first shutdown request performs sidecar shutdown
pub async fn request_quit(app: AppHandle) -> Result<(), String> {
    let state: State<'_, AppState> = app.state();

    if !state.has_exit_intent() {
        tracing::debug!("[commands] request_quit ignored; exit_intent=false");
        return Ok(());
    }

    if state.is_shutting_down() {
        tracing::debug!("[commands] request_quit ignored; shutdown already started");
        return Ok(());
    }

    state.mark_shutdown();

    sidecar::shutdown_sidecar(&state)
        .await
        .map_err(|error| error.to_string())?;

    app.exit(0);

    Ok(())
}

/// Quit the application.
///
/// This command marks explicit exit intent first, then enters the shared quit path.
#[tauri::command]
pub async fn quit_app(app: AppHandle) -> Result<(), String> {
    let state: State<'_, AppState> = app.state();
    state.mark_exit_intent();

    request_quit(app).await
}

/// Get the sidecar status as a stable frontend payload.
#[tauri::command]
pub async fn get_sidecar_status(state: State<'_, AppState>) -> Result<serde_json::Value, String> {
    let status = state.sidecar_state.get_status();

    match status {
        SidecarStatus::Stopped => Ok(serde_json::json!({
            "status": "stopped",
            "ready": false,
        })),

        SidecarStatus::Starting => Ok(serde_json::json!({
            "status": "starting",
            "ready": false,
            "timeout": state.sidecar_state.is_startup_timeout(),
        })),

        SidecarStatus::Ready { api_base_url } => Ok(serde_json::json!({
            "status": "ready",
            "ready": true,
            "api_base_url": api_base_url,
        })),

        SidecarStatus::Error { message } => Ok(serde_json::json!({
            "status": "error",
            "ready": false,
            "message": message,
        })),
    }
}
