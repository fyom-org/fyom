//! Playback commands — Phase 2 native playback (libmpv) integration.
//!
//! This module provides the Tauri command surface for libmpv-backed playback. It honors
//! the Phase 9.7 guardrail contract exactly:
//!   `invoke('play_media',  { mediaUrl, posterUrl? }) -> { success: bool, error?: string }`
//!   `invoke('stop_media')                              -> { success: bool, error?: string }`
//!
//! ## Phase 2.0 scope (PoC spike)
//! - `get_playback_backend_info` reports a real libmpv backend (not `none`).
//! - `play_media` / `stop_media` drive the `MpvInstance` via `loadfile` / `stop`.
//! - A `play_test_media` convenience command loads a hardcoded test file for the spike
//!   (lets a dev verify libmpv loads + plays audio without the full frontend flow).
//! - No GL rendering yet (Phase 2.3); video is a black frame, audio plays. No event pump
//!   yet (Phase 2.2); `fyom://mpv/*` events are additive and come later.
//! - The browser `<video>` fallback path stays green — if the `MpvState` failed to
//!   initialize, `play_media` returns `{success:false, error}` and the frontend falls
//!   back to `<video>` (the 9.7 guardrail).

use serde_json::json;
use tauri::State;

use crate::mpv::MpvInstance;
use crate::state::MpvState;

/// Get playback backend version info.
/// Returns real libmpv backend info once Phase 2 is active; `none` if the mpv instance
/// failed to initialize (frontend falls back to browser `<video>`).
#[tauri::command]
pub async fn get_playback_backend_info(
    mpv_state: State<'_, MpvState>,
) -> Result<serde_json::Value, String> {
    if let Some(instance) = mpv_state.instance.get() {
        // The instance initialized successfully — native playback is available.
        // (Phase 2.2 will surface the real libmpv version string via `mpv_get_property`
        //  "mpv-version"; for the PoC we report a static version.)
        Ok(json!({
            "backend": "libmpv",
            "version": "libmpv-0.41",
            "capabilities": ["audio", "video", "subtitles", "hardware-decode"],
            "native_playback": true,
            "ready": true,
            "hwdec": "auto-safe",
        }))
    } else {
        // MpvState exists but the instance failed to initialize — native playback off.
        Ok(json!({
            "backend": mpv_state.init_error.as_ref().map(|e| format!("libmpv (init failed: {e})")).unwrap_or_else(|| "none".to_string()),
            "version": "0.1.0",
            "capabilities": [],
            "native_playback": false,
            "ready": false,
        }))
    }
}

/// Play a media URL via libmpv. Honors the Phase 9.7 contract.
///
/// `mediaUrl` is a presigned HTTP URL (directly loadable by libmpv over the network).
/// `posterUrl` is accepted (forward-looking) but not yet rendered (Phase 2.4 will wire
/// it as `--cover-art-auto` or a pre-load placeholder).
#[tauri::command]
pub async fn play_media(
    mpv_state: State<'_, MpvState>,
    media_url: String,
    poster_url: Option<String>,
) -> Result<serde_json::Value, String> {
    let _ = poster_url; // Phase 2.4: pass to mpv as cover-art / placeholder.
    let Some(instance) = mpv_state.instance.get() else {
        return Ok(json!({
            "success": false,
            "error": mpv_state.init_error.clone().unwrap_or_else(|| "libmpv not initialized".to_string()),
        }));
    };
    let instance: &MpvInstance = instance;
    match instance.loadfile(&media_url) {
        Ok(()) => Ok(json!({ "success": true })),
        Err(e) => Ok(json!({ "success": false, "error": e })),
    }
}

/// Stop playback via libmpv. Honors the Phase 9.7 contract.
#[tauri::command]
pub async fn stop_media(
    mpv_state: State<'_, MpvState>,
) -> Result<serde_json::Value, String> {
    let Some(instance) = mpv_state.instance.get() else {
        return Ok(json!({
            "success": false,
            "error": mpv_state.init_error.clone().unwrap_or_else(|| "libmpv not initialized".to_string()),
        }));
    };
    let instance: &MpvInstance = instance;
    match instance.stop() {
        Ok(()) => Ok(json!({ "success": true })),
        Err(e) => Ok(json!({ "success": false, "error": e })),
    }
}

/// PoC convenience command: load a hardcoded test file via libmpv.
///
/// This exists purely for the Phase 2.0 spike — a dev can verify libmpv loads + plays
/// audio in a transparent Tauri window without the full frontend flow. The test file
/// path can be overridden via the `FYOM_MPV_TEST_MEDIA` env var; otherwise a synthetic
/// tone URL is attempted (libmpv supports lavfi:// as a test source).
#[tauri::command]
pub async fn play_test_media(
    mpv_state: State<'_, MpvState>,
) -> Result<serde_json::Value, String> {
    let Some(instance) = mpv_state.instance.get() else {
        return Ok(json!({
            "success": false,
            "error": mpv_state.init_error.clone().unwrap_or_else(|| "libmpv not initialized".to_string()),
        }));
    };
    let instance: &MpvInstance = instance;
    // lavfi://sine=... is an ffmpeg lavfi source — libmpv can play it without a file.
    // For a real test, set FYOM_MPV_TEST_MEDIA=/path/to/file.mp4.
    let test_media = std::env::var("FYOM_MPV_TEST_MEDIA")
        .unwrap_or_else(|_| "lavfi://sine=frequency=440:duration=10".to_string());
    match instance.loadfile(&test_media) {
        Ok(()) => Ok(json!({ "success": true, "media": test_media })),
        Err(e) => Ok(json!({ "success": false, "error": e })),
    }
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
