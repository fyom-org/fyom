//! Playback commands — Phase 2 native playback (libmpv) integration.
//!
//! This module provides the Tauri command surface for libmpv-backed playback. It honors
//! the Phase 9.7 guardrail contract exactly:
//!   `invoke('play_media',  { mediaUrl, posterUrl? }) -> { success: bool, error?: string }`
//!   `invoke('stop_media')                              -> { success: bool, error?: string }`
//!
//! ## Phase 2.2 scope (this file)
//! - `get_playback_backend_info` reports the **real** libmpv version + active hwdec +
//!   current volume/pause (fetched via `mpv_get_property`, ported from soia's
//!   `get_runtime_versions`).
//! - `play_media` / `stop_media` drive the `MpvInstance` (9.7 contract, unchanged).
//! - New typed commands: `seek` / `seek_relative` / `toggle_pause` / `set_pause` /
//!   `set_volume` / `set_speed` / `set_audio_track` / `set_subtitle_track` /
//!   `mpv_keypress` / `mpv_command` (the command surface ported from soia's
//!   `commands/playback.rs`, reimplemented on `libmpv2::Mpv`).
//! - `play_test_media` PoC convenience command retained.
//! - No GL rendering yet (Phase 2.3); video is a black frame, audio plays. The event
//!   pump (Phase 2.2) emits `fyom://mpv/*` events (additive — no 9.7 contract change).
//! - The browser `<video>` fallback path stays green — if the `MpvState` failed to
//!   initialize, every command returns `{success:false, error}` and the frontend falls
//!   back to `<video>` (the 9.7 guardrail).
//!
//! ## Attribution
//! The command surface (`seek` / `cycle_pause` / `set_volume` / `mpv_run_command` +
//! the eof-restart-after-eof logic) is ported from soia (`FengZeng/soia`, GPL-3.0)
//! `src-tauri/src/commands/playback.rs`, reimplemented on `libmpv2::Mpv` (soia's
//! hand-written `MpvHandle`/`with_mpv`/`mpv_command_checked` are not ported). See
//! `docs/libmpv-assessment.md` §2.1 for the reuse inventory.

use serde_json::json;
use tauri::State;

use crate::mpv::{MpvInstance, TrackSelection};
use crate::state::MpvState;

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/// Build a `{success: true}` JSON value.
fn ok() -> serde_json::Value {
    json!({ "success": true })
}

/// Build a `{success: false, error: <msg>}` JSON value.
fn err(msg: impl Into<String>) -> serde_json::Value {
    json!({ "success": false, "error": msg.into() })
}

/// The error returned when the mpv instance failed to initialize (native playback off).
fn no_instance_error(mpv_state: &MpvState) -> serde_json::Value {
    err(
        mpv_state
            .init_error
            .clone()
            .unwrap_or_else(|| "libmpv not initialized".to_string()),
    )
}

/// If mpv has reached EOF, restart from the beginning + resume (so the user can press
/// play after a file ends and get a fresh start instead of a no-op). Ported from soia's
/// `restart_from_beginning_after_eof` + `cycle_pause` eof guard.
fn restart_from_eof_if_needed(instance: &MpvInstance) -> Result<(), String> {
    let eof: bool = instance.get_property("eof-reached").unwrap_or(false);
    if eof {
        instance.command("seek", &["0", "absolute", "exact"])?;
        instance.set_property("pause", false)?;
    }
    Ok(())
}

/// Strip the `mpv ` prefix from a libmpv version string (soia's
/// `normalize_mpv_version`).
fn normalize_mpv_version(raw: Option<String>) -> Option<String> {
    raw.map(|v| v.trim().to_string())
        .filter(|v| !v.is_empty())
        .map(|v| v.strip_prefix("mpv ").unwrap_or(&v).to_string())
}

/// Trim + drop-empty for a generic version string (soia's
/// `normalize_generic_version`).
fn normalize_generic_version(raw: Option<String>) -> Option<String> {
    raw.map(|v| v.trim().to_string()).filter(|v| !v.is_empty())
}

// ---------------------------------------------------------------------------
// Backend info (upgraded in Phase 2.2 to report real libmpv version + hwdec)
// ---------------------------------------------------------------------------

/// Get playback backend version info.
///
/// Returns real libmpv backend info once Phase 2 is active (mpv-version + ffmpeg-version
/// + the active hwdec + current volume/pause, fetched via `mpv_get_property`); `none`
/// if the mpv instance failed to initialize (frontend falls back to browser `<video>`).
///
/// PORTED_FROM_SOIA `get_runtime_versions` (the version-fetch + normalize logic).
#[tauri::command]
pub async fn get_playback_backend_info(
    mpv_state: State<'_, MpvState>,
) -> Result<serde_json::Value, String> {
    let Some(instance) = mpv_state.instance.get() else {
        return Ok(json!({
            "backend": mpv_state.init_error.as_ref().map(|e| format!("libmpv (init failed: {e})")).unwrap_or_else(|| "none".to_string()),
            "version": "0.1.0",
            "capabilities": [],
            "native_playback": false,
            "ready": false,
        }));
    };
    let instance: &MpvInstance = instance;

    // Fetch live properties (each is a cheap, thread-safe mpv_get_property call).
    let mpv_version = normalize_mpv_version(instance.get_property::<String>("mpv-version").ok());
    let ffmpeg_version =
        normalize_generic_version(instance.get_property::<String>("ffmpeg-version").ok());
    let hwdec_current =
        normalize_generic_version(instance.get_property::<String>("hwdec-current").ok());
    let volume: i64 = instance.get_property("volume").unwrap_or(0);
    let paused: bool = instance.get_property("pause").unwrap_or(true);

    Ok(json!({
        "backend": "libmpv",
        "version": mpv_version.clone().unwrap_or_else(|| "libmpv-0.41".to_string()),
        "mpv_version": mpv_version,
        "ffmpeg_version": ffmpeg_version,
        "capabilities": ["audio", "video", "subtitles", "hardware-decode"],
        "native_playback": true,
        "ready": true,
        "hwdec": "auto-safe",
        "hwdec_current": hwdec_current,
        "volume": volume,
        "paused": paused,
    }))
}

// ---------------------------------------------------------------------------
// 9.7 guardrail contract (unchanged from Phase 2.0)
// ---------------------------------------------------------------------------

/// Play a media URL via libmpv. Honors the Phase 9.7 contract.
///
/// `media_url` is a presigned HTTP URL (directly loadable by libmpv over the network —
/// Q3: direct `loadfile`, no `stream_proxy` for v1). `poster_url` is accepted
/// (forward-looking) but not yet rendered (Phase 2.4 will wire it as `--cover-art-auto`
/// or a pre-load placeholder).
#[tauri::command]
pub async fn play_media(
    mpv_state: State<'_, MpvState>,
    media_url: String,
    poster_url: Option<String>,
) -> Result<serde_json::Value, String> {
    let _ = poster_url; // Phase 2.4: pass to mpv as cover-art / placeholder.
    let Some(instance) = mpv_state.instance.get() else {
        return Ok(no_instance_error(&mpv_state));
    };
    let instance: &MpvInstance = instance;
    match instance.loadfile(&media_url) {
        Ok(()) => Ok(ok()),
        Err(e) => Ok(err(e)),
    }
}

/// Stop playback via libmpv. Honors the Phase 9.7 contract.
#[tauri::command]
pub async fn stop_media(mpv_state: State<'_, MpvState>) -> Result<serde_json::Value, String> {
    let Some(instance) = mpv_state.instance.get() else {
        return Ok(no_instance_error(&mpv_state));
    };
    let instance: &MpvInstance = instance;
    match instance.stop() {
        Ok(()) => Ok(ok()),
        Err(e) => Ok(err(e)),
    }
}

// ---------------------------------------------------------------------------
// Phase 2.2 command surface (ported from soia, reimplemented on libmpv2)
// ---------------------------------------------------------------------------

/// Seek to an absolute position (seconds). If playback had reached EOF, resume from
/// the seeked position (ported from soia's `seek_video` eof guard).
#[tauri::command]
pub async fn seek(
    mpv_state: State<'_, MpvState>,
    position: f64,
) -> Result<serde_json::Value, String> {
    let Some(instance) = mpv_state.instance.get() else {
        return Ok(no_instance_error(&mpv_state));
    };
    let instance: &MpvInstance = instance;
    match instance.seek_absolute(position) {
        Ok(()) => {
            let _ = restart_from_eof_if_needed(instance);
            Ok(ok())
        }
        Err(e) => Ok(err(e)),
    }
}

/// Seek by a relative offset (seconds; negative = backward).
#[tauri::command]
pub async fn seek_relative(
    mpv_state: State<'_, MpvState>,
    seconds: f64,
) -> Result<serde_json::Value, String> {
    let Some(instance) = mpv_state.instance.get() else {
        return Ok(no_instance_error(&mpv_state));
    };
    let instance: &MpvInstance = instance;
    match instance.seek_relative(seconds) {
        Ok(()) => Ok(ok()),
        Err(e) => Ok(err(e)),
    }
}

/// Toggle play/pause. If playback had reached EOF, restart from the beginning + resume
/// (ported from soia's `cycle_pause` eof-restart logic).
#[tauri::command]
pub async fn toggle_pause(mpv_state: State<'_, MpvState>) -> Result<serde_json::Value, String> {
    let Some(instance) = mpv_state.instance.get() else {
        return Ok(no_instance_error(&mpv_state));
    };
    let instance: &MpvInstance = instance;
    if let Err(e) = restart_from_eof_if_needed(instance) {
        return Ok(err(e));
    }
    match instance.cycle_pause() {
        Ok(()) => Ok(ok()),
        Err(e) => Ok(err(e)),
    }
}

/// Explicitly set the pause state (`true` = paused, `false` = playing).
#[tauri::command]
pub async fn set_pause(
    mpv_state: State<'_, MpvState>,
    paused: bool,
) -> Result<serde_json::Value, String> {
    let Some(instance) = mpv_state.instance.get() else {
        return Ok(no_instance_error(&mpv_state));
    };
    let instance: &MpvInstance = instance;
    match instance.set_pause(paused) {
        Ok(()) => Ok(ok()),
        Err(e) => Ok(err(e)),
    }
}

/// Set the volume (0–100; mpv clamps to `volume-max`).
#[tauri::command]
pub async fn set_volume(
    mpv_state: State<'_, MpvState>,
    volume: i64,
) -> Result<serde_json::Value, String> {
    let Some(instance) = mpv_state.instance.get() else {
        return Ok(no_instance_error(&mpv_state));
    };
    let instance: &MpvInstance = instance;
    match instance.set_property("volume", volume) {
        Ok(()) => Ok(ok()),
        Err(e) => Ok(err(e)),
    }
}

/// Set the playback speed (1.0 = normal).
#[tauri::command]
pub async fn set_speed(
    mpv_state: State<'_, MpvState>,
    speed: f64,
) -> Result<serde_json::Value, String> {
    let Some(instance) = mpv_state.instance.get() else {
        return Ok(no_instance_error(&mpv_state));
    };
    let instance: &MpvInstance = instance;
    match instance.set_property("speed", speed) {
        Ok(()) => Ok(ok()),
        Err(e) => Ok(err(e)),
    }
}

/// Select the audio track (`Some(id)` to select, `None` to disable audio).
#[tauri::command]
pub async fn set_audio_track(
    mpv_state: State<'_, MpvState>,
    track_id: Option<i64>,
) -> Result<serde_json::Value, String> {
    let Some(instance) = mpv_state.instance.get() else {
        return Ok(no_instance_error(&mpv_state));
    };
    let instance: &MpvInstance = instance;
    let selection = match track_id {
        Some(id) => TrackSelection::Track(id),
        None => TrackSelection::None,
    };
    match instance.set_property("aid", selection.to_string()) {
        Ok(()) => Ok(ok()),
        Err(e) => Ok(err(e)),
    }
}

/// Select the subtitle track (`Some(id)` to select, `None` to disable subtitles).
#[tauri::command]
pub async fn set_subtitle_track(
    mpv_state: State<'_, MpvState>,
    track_id: Option<i64>,
) -> Result<serde_json::Value, String> {
    let Some(instance) = mpv_state.instance.get() else {
        return Ok(no_instance_error(&mpv_state));
    };
    let instance: &MpvInstance = instance;
    let selection = match track_id {
        Some(id) => TrackSelection::Track(id),
        None => TrackSelection::None,
    };
    match instance.set_property("sid", selection.to_string()) {
        Ok(()) => Ok(ok()),
        Err(e) => Ok(err(e)),
    }
}

/// Forward a keypress to mpv. The frontend assembles the keystr (mpv's input format,
/// e.g. "Space", "Ctrl+Right", "Volume+", "KP+0") from webview keyboard events.
///
/// Ported from tsukimi's `press_key` (the GTK keyval → keystr translation is dropped —
/// fyom does that translation in the frontend).
#[tauri::command]
pub async fn mpv_keypress(
    mpv_state: State<'_, MpvState>,
    key: String,
) -> Result<serde_json::Value, String> {
    let Some(instance) = mpv_state.instance.get() else {
        return Ok(no_instance_error(&mpv_state));
    };
    let instance: &MpvInstance = instance;
    match instance.mpv_keypress(&key) {
        Ok(()) => Ok(ok()),
        Err(e) => Ok(err(e)),
    }
}

/// Generic mpv command passthrough (for power-user commands not covered by the typed
/// facade). Ported from soia's `mpv_run_command`.
#[tauri::command]
pub async fn mpv_command(
    mpv_state: State<'_, MpvState>,
    args: Vec<String>,
) -> Result<serde_json::Value, String> {
    let Some(instance) = mpv_state.instance.get() else {
        return Ok(no_instance_error(&mpv_state));
    };
    let instance: &MpvInstance = instance;
    if args.is_empty() {
        return Ok(err("mpv_command requires at least the command name"));
    }
    let args_ref: Vec<&str> = args.iter().map(String::as_str).collect();
    let (cmd, rest) = args_ref.split_first().expect("checked non-empty");
    match instance.command(cmd, rest) {
        Ok(()) => Ok(ok()),
        Err(e) => Ok(err(e)),
    }
}

// ---------------------------------------------------------------------------
// PoC convenience (retained from Phase 2.0)
// ---------------------------------------------------------------------------

/// PoC convenience command: load a hardcoded test file via libmpv.
///
/// This exists for the Phase 2.0 spike — a dev can verify libmpv loads + plays audio in
/// a transparent Tauri window without the full frontend flow. The test file path can be
/// overridden via the `FYOM_MPV_TEST_MEDIA` env var; otherwise a synthetic tone URL is
/// attempted (libmpv supports `lavfi://` as a test source).
#[tauri::command]
pub async fn play_test_media(
    mpv_state: State<'_, MpvState>,
) -> Result<serde_json::Value, String> {
    let Some(instance) = mpv_state.instance.get() else {
        return Ok(no_instance_error(&mpv_state));
    };
    let instance: &MpvInstance = instance;
    let test_media = std::env::var("FYOM_MPV_TEST_MEDIA")
        .unwrap_or_else(|_| "lavfi://sine=frequency=440:duration=10".to_string());
    match instance.loadfile(&test_media) {
        Ok(()) => Ok(json!({ "success": true, "media": test_media })),
        Err(e) => Ok(err(e)),
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
