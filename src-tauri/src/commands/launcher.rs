//! External player launcher commands.
//!
//! Replaces the native libmpv playback backend. FYOM Desktop now delegates
//! playback to the OS or a configured external media player.

use serde_json::{Value, json};
use tauri::State;

use crate::AppState;

// -----------------------------------------------------------------------------
// Response helpers
// -----------------------------------------------------------------------------

fn ok() -> Value {
    json!({ "success": true })
}

fn err(message: impl Into<String>) -> Value {
    json!({
        "success": false,
        "error": message.into(),
    })
}

// -----------------------------------------------------------------------------
// Resolve media URL
// -----------------------------------------------------------------------------

fn resolve_media_url(app_state: &AppState, media_url: &str) -> Result<String, String> {
    let media_url = media_url.trim();

    if media_url.is_empty() {
        return Err("media_url must not be empty".to_string());
    }

    if media_url.starts_with("http://")
        || media_url.starts_with("https://")
        || media_url.starts_with("file://")
    {
        return Ok(media_url.to_string());
    }

    if media_url.starts_with('/') {
        let api_base_url = app_state.sidecar_state.get_api_base_url().map_err(|e| {
            format!(
                "sidecar API is not ready; cannot resolve relative media URL `{media_url}`: {e}"
            )
        })?;

        let api_base_url = api_base_url.trim_end_matches('/');
        let media_url = media_url.trim_start_matches('/');

        return Ok(format!("{api_base_url}/{media_url}"));
    }

    Ok(media_url.to_string())
}

// -----------------------------------------------------------------------------
// Cross-platform external player launch
// -----------------------------------------------------------------------------

fn open_url_in_external_player(url: &str) -> Result<(), String> {
    #[cfg(target_os = "macos")]
    {
        std::process::Command::new("open")
            .arg(url)
            .spawn()
            .map_err(|e| format!("failed to open URL with `open`: {e}"))?;
    }

    #[cfg(target_os = "linux")]
    {
        std::process::Command::new("xdg-open")
            .arg(url)
            .spawn()
            .map_err(|e| format!("failed to open URL with `xdg-open`: {e}"))?;
    }

    #[cfg(target_os = "windows")]
    {
        std::process::Command::new("cmd")
            .args(["/C", "start", "", url])
            .spawn()
            .map_err(|e| format!("failed to open URL with `cmd /C start`: {e}"))?;
    }

    #[cfg(not(any(target_os = "macos", target_os = "linux", target_os = "windows")))]
    {
        return Err("unsupported target OS: no external player launcher available".to_string());
    }

    Ok(())
}

// -----------------------------------------------------------------------------
// Tauri commands
// -----------------------------------------------------------------------------

/// Open a media URL in the system's default external player.
///
/// In Tauri mode the URL is first resolved against the sidecar API base URL
/// if it is a relative `/api/v1/...` path. The resolved absolute URL is then
/// handed to the OS via `open` (macOS), `xdg-open` (Linux), or `cmd /C start`
/// (Windows).
#[tauri::command]
pub async fn open_external_player(
    app_state: State<'_, AppState>,
    media_url: String,
) -> Result<Value, String> {
    let resolved = resolve_media_url(&app_state, &media_url)?;

    tracing::info!(
        "[launcher] opening external player: input={} resolved={}",
        media_url,
        resolved
    );

    open_url_in_external_player(&resolved)?;

    Ok(ok())
}

/// Get the sidecar API base URL for the current session.
#[tauri::command]
pub async fn get_api_base_url(state: State<'_, AppState>) -> Result<String, String> {
    state
        .sidecar_state
        .get_api_base_url()
        .map_err(|e| e.to_string())
}
