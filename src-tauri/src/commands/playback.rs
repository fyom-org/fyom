//! Playback commands — placeholder for Phase 2 native playback integration.
//!
//! This module is intentionally minimal. In Phase 2, this will contain
//! Tauri commands for libmpv integration. For now, it only provides
//! a version info command so the frontend can check playback backend status.
//!
//! Architecture note: Playback commands should interact with a dedicated
//! playback backend module, not directly with the sidecar or Tauri state.

/// Get playback backend version info.
/// Returns info about the playback capabilities of the desktop shell.
#[tauri::command]
pub async fn get_playback_backend_info() -> Result<serde_json::Value, String> {
    // Phase 1: No native playback backend yet
    Ok(serde_json::json!({
        "backend": "none",
        "version": "0.1.0",
        "capabilities": [],
        "native_playback": false,
    }))
}

/// Get the sidecar API base URL for the frontend to use.
/// This allows the frontend to make direct REST calls to the Go backend.
#[tauri::command]
pub async fn get_api_base_url(
    state: tauri::State<'_, crate::AppState>,
) -> Result<String, String> {
    state
        .sidecar_state
        .get_api_base_url()
        .map_err(|e| e.to_string())
}
