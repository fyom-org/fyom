//! Tauri command handlers
//!
//! This module contains all Tauri invoke handlers.
//! Playback commands will be added in Phase 2 when native playback is integrated.

pub mod playback;

use tauri::{AppHandle, Manager, State};

use crate::AppState;
use crate::sidecar;

/// Show the main window
#[tauri::command]
pub async fn show_window(app: AppHandle) -> Result<(), String> {
    crate::window::show_main_window(&app).map_err(|e| e.to_string())
}

/// Hide the main window to tray
#[tauri::command]
pub async fn hide_window(app: AppHandle) -> Result<(), String> {
    crate::window::hide_to_tray(&app).map_err(|e| e.to_string())
}

/// Request quit the application (async-safe version for spawning).
/// Idempotent: only the first call performs real shutdown.
pub async fn request_quit(app: AppHandle) -> Result<(), String> {
    let state: State<'_, AppState> = app.state();

    // Guard: only proceed if exit was intentionally requested.
    if !state.has_exit_intent() {
        tracing::debug!("request_quit called without exit_intent, ignoring");
        return Ok(());
    }

    // Idempotency guard: if shutdown is already in progress, return immediately.
    if state.is_shutting_down() {
        tracing::debug!("Shutdown already in progress, skipping duplicate request_quit");
        return Ok(());
    }
    state.mark_shutdown();

    // Shutdown sidecar (only the first intentional call reaches here).
    sidecar::shutdown_sidecar(&state)
        .await
        .map_err(|e| e.to_string())?;

    // Exit the app.
    app.exit(0);
    Ok(())
}

/// Quit the application
#[tauri::command]
pub async fn quit_app(app: AppHandle) -> Result<(), String> {
    request_quit(app).await
}

/// Get the sidecar status
#[tauri::command]
pub async fn get_sidecar_status(state: State<'_, AppState>) -> Result<serde_json::Value, String> {
    let status = state.sidecar_state.get_status();
    match status {
        crate::SidecarStatus::Starting => Ok(serde_json::json!({"status": "starting"})),
        crate::SidecarStatus::Ready { api_base_url } => {
            Ok(serde_json::json!({"status": "ready", "api_base_url": api_base_url}))
        }
        crate::SidecarStatus::Error { message } => {
            Ok(serde_json::json!({"status": "error", "message": message}))
        }
    }
}
