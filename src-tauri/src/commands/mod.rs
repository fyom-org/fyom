//! Tauri command handlers.
//!
//! This module owns application-level Tauri invoke handlers.
//!
//! Keep this module thin:
//! - window visibility commands
//! - application quit commands
//! - sidecar status read model

pub mod launcher;

use tauri::{AppHandle, Manager, State};

use crate::sidecar;
use crate::{AppState, SidecarStatus};

// -----------------------------------------------------------------------------
// Window commands
// -----------------------------------------------------------------------------

/// Show the main window.
///
/// This is intentionally a thin command wrapper around `crate::window`.
#[tauri::command]
pub async fn show_window(app: AppHandle) -> Result<(), String> {
    crate::window::show_main_window(&app).map_err(|error| error.to_string())
}

/// Hide the main window to tray.
///
/// This is intentionally a thin command wrapper around `crate::window`.
#[tauri::command]
pub async fn hide_window(app: AppHandle) -> Result<(), String> {
    crate::window::hide_to_tray(&app).map_err(|error| error.to_string())
}

// -----------------------------------------------------------------------------
// Quit flow
// -----------------------------------------------------------------------------

/// Request application quit.
///
/// This function is intentionally idempotent:
/// - it only exits when `exit_intent` is set
/// - it only starts shutdown once
/// - it shuts down the sidecar before exiting the app process
///
/// This function is not marked as a Tauri command directly because callers should
/// normally go through `quit_app`, which marks explicit exit intent first.
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

    tracing::info!("[commands] shutdown started");

    sidecar::shutdown_sidecar(&state)
        .await
        .map_err(|error| error.to_string())?;

    tracing::info!("[commands] sidecar shutdown complete; exiting app");

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

// -----------------------------------------------------------------------------
// Sidecar status
// -----------------------------------------------------------------------------

/// Get the sidecar status as a stable frontend payload.
///
/// Response shape:
///
/// ```json
/// {
///   "status": "stopped" | "starting" | "ready" | "error",
///   "ready": true | false,
///   "api_base_url": "...",
///   "message": "...",
///   "timeout": true | false
/// }
/// ```
///
/// Fields that are not meaningful for a status are omitted.
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
