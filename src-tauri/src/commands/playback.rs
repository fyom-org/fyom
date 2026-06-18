//! Playback commands — Phase 2 native playback (libmpv) integration.
//!
//! This module provides the Tauri command surface for libmpv-backed playback. It honors
//! the Phase 9.7 guardrail contract exactly:
//!   `invoke('play_media',  { mediaUrl, posterUrl? }) -> { success: bool, error?: string }`
//!   `invoke('stop_media')                              -> { success: bool, error?: string }`
//!
//! ## Phase 2.2 scope
//! - `get_playback_backend_info` reports the real libmpv version + active hwdec +
//!   current volume/pause.
//! - `play_media` / `stop_media` drive the `MpvInstance`.
//! - Typed commands: seek / pause / volume / speed / track selection / keypress /
//!   generic mpv command.
//! - Render-surface commands for Phase 2.3.
//! - Subtitle/audio/color/chapter/property commands for Phase 2.4.
//!
//! ## Import policy
//! `crate::mpv` only re-exports the public facade `MpvInstance`. Internal event/render
//! types must be imported from their concrete modules. This keeps `mpv/mod.rs` from
//! becoming a warning-prone barrel module.

use serde_json::json;
use tauri::State;

use crate::mpv::MpvInstance;
use crate::mpv::event_loop::TrackSelection;
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

/// The error returned when the mpv instance failed to initialize.
fn no_instance_error(mpv_state: &MpvState) -> serde_json::Value {
    err(mpv_state
        .init_error
        .clone()
        .unwrap_or_else(|| "libmpv not initialized".to_string()))
}

/// Get the initialized mpv instance or return the standard guardrail error payload.
fn get_instance_or_error<'a>(
    mpv_state: &'a MpvState,
) -> Result<&'a MpvInstance, serde_json::Value> {
    mpv_state
        .instance
        .get()
        .map(|instance| instance as &MpvInstance)
        .ok_or_else(|| no_instance_error(mpv_state))
}

/// If mpv has reached EOF, restart from the beginning + resume.
///
/// This mirrors the UX behavior from soia: pressing play after EOF should restart
/// playback instead of becoming a no-op.
fn restart_from_eof_if_needed(instance: &MpvInstance) -> Result<(), String> {
    let eof: bool = instance.get_property("eof-reached").unwrap_or(false);

    if eof {
        instance.command("seek", &["0", "absolute", "exact"])?;
        instance.set_property("pause", false)?;
    }

    Ok(())
}

/// Strip the `mpv ` prefix from a libmpv version string.
fn normalize_mpv_version(raw: Option<String>) -> Option<String> {
    raw.map(|v| v.trim().to_string())
        .filter(|v| !v.is_empty())
        .map(|v| v.strip_prefix("mpv ").unwrap_or(&v).to_string())
}

/// Trim + drop-empty for a generic version string.
fn normalize_generic_version(raw: Option<String>) -> Option<String> {
    raw.map(|v| v.trim().to_string()).filter(|v| !v.is_empty())
}

// ---------------------------------------------------------------------------
// Backend info
// ---------------------------------------------------------------------------

/// Get playback backend version info.
///
/// Returns real libmpv backend info when mpv initialized successfully, otherwise reports
/// `native_playback: false` so the frontend can stay on the browser `<video>` fallback.
#[tauri::command]
pub async fn get_playback_backend_info(
    mpv_state: State<'_, MpvState>,
) -> Result<serde_json::Value, String> {
    let Ok(instance) = get_instance_or_error(&mpv_state) else {
        return Ok(json!({
            "backend": mpv_state
                .init_error
                .as_ref()
                .map(|e| format!("libmpv (init failed: {e})"))
                .unwrap_or_else(|| "none".to_string()),
            "version": "0.1.0",
            "capabilities": [],
            "native_playback": false,
            "ready": false,
        }));
    };

    let mpv_version = normalize_mpv_version(instance.get_property::<String>("mpv-version").ok());
    let ffmpeg_version =
        normalize_generic_version(instance.get_property::<String>("ffmpeg-version").ok());
    let hwdec_current =
        normalize_generic_version(instance.get_property::<String>("hwdec-current").ok());
    let volume: i64 = instance.get_property("volume").unwrap_or(0);
    let paused: bool = instance.get_property("pause").unwrap_or(true);

    Ok(json!({
        "backend": "libmpv",
        "version": mpv_version.clone().unwrap_or_else(|| "libmpv".to_string()),
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
// Phase 9.7 guardrail contract
// ---------------------------------------------------------------------------

/// Play a media URL via libmpv.
///
/// This preserves the existing frontend contract:
/// `{ success: boolean, error?: string }`.
#[tauri::command]
pub async fn play_media(
    mpv_state: State<'_, MpvState>,
    media_url: String,
    poster_url: Option<String>,
) -> Result<serde_json::Value, String> {
    let _ = poster_url;

    let instance = match get_instance_or_error(&mpv_state) {
        Ok(instance) => instance,
        Err(payload) => return Ok(payload),
    };

    match instance.loadfile(&media_url) {
        Ok(()) => Ok(ok()),
        Err(e) => Ok(err(e)),
    }
}

/// Stop playback via libmpv.
///
/// This preserves the existing frontend contract:
/// `{ success: boolean, error?: string }`.
#[tauri::command]
pub async fn stop_media(mpv_state: State<'_, MpvState>) -> Result<serde_json::Value, String> {
    let instance = match get_instance_or_error(&mpv_state) {
        Ok(instance) => instance,
        Err(payload) => return Ok(payload),
    };

    match instance.stop() {
        Ok(()) => Ok(ok()),
        Err(e) => Ok(err(e)),
    }
}

// ---------------------------------------------------------------------------
// Phase 2.2 command surface
// ---------------------------------------------------------------------------

/// Seek to an absolute position in seconds.
#[tauri::command]
pub async fn seek(
    mpv_state: State<'_, MpvState>,
    position: f64,
) -> Result<serde_json::Value, String> {
    let instance = match get_instance_or_error(&mpv_state) {
        Ok(instance) => instance,
        Err(payload) => return Ok(payload),
    };

    match instance.seek_absolute(position) {
        Ok(()) => {
            let _ = restart_from_eof_if_needed(instance);
            Ok(ok())
        }
        Err(e) => Ok(err(e)),
    }
}

/// Seek by a relative offset in seconds.
#[tauri::command]
pub async fn seek_relative(
    mpv_state: State<'_, MpvState>,
    seconds: f64,
) -> Result<serde_json::Value, String> {
    let instance = match get_instance_or_error(&mpv_state) {
        Ok(instance) => instance,
        Err(payload) => return Ok(payload),
    };

    match instance.seek_relative(seconds) {
        Ok(()) => Ok(ok()),
        Err(e) => Ok(err(e)),
    }
}

/// Toggle play/pause.
///
/// If playback is currently at EOF, this restarts from the beginning and resumes.
#[tauri::command]
pub async fn toggle_pause(mpv_state: State<'_, MpvState>) -> Result<serde_json::Value, String> {
    let instance = match get_instance_or_error(&mpv_state) {
        Ok(instance) => instance,
        Err(payload) => return Ok(payload),
    };

    if let Err(e) = restart_from_eof_if_needed(instance) {
        return Ok(err(e));
    }

    match instance.cycle_pause() {
        Ok(()) => Ok(ok()),
        Err(e) => Ok(err(e)),
    }
}

/// Explicitly set the pause state.
#[tauri::command]
pub async fn set_pause(
    mpv_state: State<'_, MpvState>,
    paused: bool,
) -> Result<serde_json::Value, String> {
    let instance = match get_instance_or_error(&mpv_state) {
        Ok(instance) => instance,
        Err(payload) => return Ok(payload),
    };

    match instance.set_pause(paused) {
        Ok(()) => Ok(ok()),
        Err(e) => Ok(err(e)),
    }
}

/// Set the volume. mpv clamps according to its own `volume-max`.
#[tauri::command]
pub async fn set_volume(
    mpv_state: State<'_, MpvState>,
    volume: i64,
) -> Result<serde_json::Value, String> {
    let instance = match get_instance_or_error(&mpv_state) {
        Ok(instance) => instance,
        Err(payload) => return Ok(payload),
    };

    match instance.set_property("volume", volume) {
        Ok(()) => Ok(ok()),
        Err(e) => Ok(err(e)),
    }
}

/// Set playback speed. `1.0` is normal speed.
#[tauri::command]
pub async fn set_speed(
    mpv_state: State<'_, MpvState>,
    speed: f64,
) -> Result<serde_json::Value, String> {
    let instance = match get_instance_or_error(&mpv_state) {
        Ok(instance) => instance,
        Err(payload) => return Ok(payload),
    };

    match instance.set_property("speed", speed) {
        Ok(()) => Ok(ok()),
        Err(e) => Ok(err(e)),
    }
}

/// Select the audio track. `None` disables audio.
#[tauri::command]
pub async fn set_audio_track(
    mpv_state: State<'_, MpvState>,
    track_id: Option<i64>,
) -> Result<serde_json::Value, String> {
    let instance = match get_instance_or_error(&mpv_state) {
        Ok(instance) => instance,
        Err(payload) => return Ok(payload),
    };

    let selection = match track_id {
        Some(id) => TrackSelection::Track(id),
        None => TrackSelection::None,
    };

    match instance.set_property("aid", selection.to_string()) {
        Ok(()) => Ok(ok()),
        Err(e) => Ok(err(e)),
    }
}

/// Select the subtitle track. `None` disables subtitles.
#[tauri::command]
pub async fn set_subtitle_track(
    mpv_state: State<'_, MpvState>,
    track_id: Option<i64>,
) -> Result<serde_json::Value, String> {
    let instance = match get_instance_or_error(&mpv_state) {
        Ok(instance) => instance,
        Err(payload) => return Ok(payload),
    };

    let selection = match track_id {
        Some(id) => TrackSelection::Track(id),
        None => TrackSelection::None,
    };

    match instance.set_property("sid", selection.to_string()) {
        Ok(()) => Ok(ok()),
        Err(e) => Ok(err(e)),
    }
}

/// Forward a keypress to mpv.
#[tauri::command]
pub async fn mpv_keypress(
    mpv_state: State<'_, MpvState>,
    key: String,
) -> Result<serde_json::Value, String> {
    let instance = match get_instance_or_error(&mpv_state) {
        Ok(instance) => instance,
        Err(payload) => return Ok(payload),
    };

    match instance.mpv_keypress(&key) {
        Ok(()) => Ok(ok()),
        Err(e) => Ok(err(e)),
    }
}

/// Generic mpv command passthrough.
#[tauri::command]
pub async fn mpv_command(
    mpv_state: State<'_, MpvState>,
    args: Vec<String>,
) -> Result<serde_json::Value, String> {
    let instance = match get_instance_or_error(&mpv_state) {
        Ok(instance) => instance,
        Err(payload) => return Ok(payload),
    };

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
// PoC convenience
// ---------------------------------------------------------------------------

/// Load a test media source through libmpv.
#[tauri::command]
pub async fn play_test_media(mpv_state: State<'_, MpvState>) -> Result<serde_json::Value, String> {
    let instance = match get_instance_or_error(&mpv_state) {
        Ok(instance) => instance,
        Err(payload) => return Ok(payload),
    };

    let test_media = std::env::var("FYOM_MPV_TEST_MEDIA")
        .unwrap_or_else(|_| "lavfi://sine=frequency=440:duration=10".to_string());

    match instance.loadfile(&test_media) {
        Ok(()) => Ok(json!({ "success": true, "media": test_media })),
        Err(e) => Ok(err(e)),
    }
}

// ---------------------------------------------------------------------------
// Phase 2.3: render surface commands
// ---------------------------------------------------------------------------

/// Attach the platform GL surface to the main Tauri window and spawn the mpv render thread.
///
/// Idempotent at the `MpvInstance` layer: if the render thread is already running,
/// `spawn_render_thread` should return success without creating a duplicate thread.
#[tauri::command]
pub async fn attach_render_surface(
    mpv_state: State<'_, MpvState>,
    app_handle: tauri::AppHandle,
) -> Result<serde_json::Value, String> {
    use tauri::Manager;

    let instance = match get_instance_or_error(&mpv_state) {
        Ok(instance) => instance,
        Err(payload) => return Ok(payload),
    };

    let Some(window) = app_handle.get_webview_window(crate::MAIN_WINDOW_LABEL) else {
        return Ok(err("main window not found"));
    };

    let surface = match crate::platform::create_platform_surface(&window) {
        Ok(surface) => surface,
        Err(e) => {
            tracing::warn!(
                "[mpv] platform GL surface creation failed; GL rendering disabled: {}",
                e
            );
            return Ok(err(format!("platform surface creation failed: {}", e)));
        }
    };

    match instance.spawn_render_thread(surface) {
        Ok(()) => Ok(ok()),
        Err(e) => {
            tracing::warn!("[mpv] spawn_render_thread failed: {}", e);
            Ok(err(format!("spawn_render_thread failed: {}", e)))
        }
    }
}

/// Notify the backend that the frontend entered or exited `.video-mode`.
///
/// This is informational. The actual visibility mechanism is the frontend CSS class.
#[tauri::command]
pub async fn set_video_mode(
    _mpv_state: State<'_, MpvState>,
    enabled: bool,
) -> Result<serde_json::Value, String> {
    tracing::info!(
        "[mpv] video-mode {} ({})",
        if enabled { "enabled" } else { "disabled" },
        if enabled {
            "transparent webview background; mpv GL layer visible"
        } else {
            "opaque webview background; mpv GL layer covered"
        }
    );

    Ok(ok())
}

/// Resize hook for platform render surfaces.
///
/// The render loop currently polls `RenderSurface::drawable_size()` every frame, so this
/// command remains a no-op until platform-specific resize hooks are added.
#[tauri::command]
pub async fn resize_render_surface(
    _mpv_state: State<'_, MpvState>,
    _width: u32,
    _height: u32,
    _scale_factor: f64,
) -> Result<serde_json::Value, String> {
    Ok(ok())
}

/// Get the sidecar API base URL for the frontend.
#[tauri::command]
pub async fn get_api_base_url(state: tauri::State<'_, crate::AppState>) -> Result<String, String> {
    state
        .sidecar_state
        .get_api_base_url()
        .map_err(|e| e.to_string())
}

// ---------------------------------------------------------------------------
// Phase 2.4: subtitle / audio / color / chapter / property commands
// ---------------------------------------------------------------------------

/// Find external subtitle files matching a local media file.
#[tauri::command]
pub async fn find_external_subtitles(
    _mpv_state: State<'_, MpvState>,
    media_path: String,
    media_title: Option<String>,
) -> Result<serde_json::Value, String> {
    let payload = crate::subtitles::ExternalSubtitleMatchesPayloadResolved {
        media_path,
        media_title,
    };

    let matches = crate::subtitles::find_external_subtitles_impl(payload).await?;

    Ok(json!({
        "success": true,
        "matches": matches,
    }))
}

/// Add an external subtitle file to current playback.
#[tauri::command]
pub async fn sub_add(
    mpv_state: State<'_, MpvState>,
    path: String,
    mode: Option<String>,
    title: Option<String>,
    lang: Option<String>,
) -> Result<serde_json::Value, String> {
    let instance = match get_instance_or_error(&mpv_state) {
        Ok(instance) => instance,
        Err(payload) => return Ok(payload),
    };

    let mode = mode.as_deref().unwrap_or("select");

    match instance.sub_add(&path, mode, title.as_deref(), lang.as_deref()) {
        Ok(()) => Ok(ok()),
        Err(e) => Ok(err(e)),
    }
}

/// Remove an external subtitle track by id.
#[tauri::command]
pub async fn sub_remove(
    mpv_state: State<'_, MpvState>,
    track_id: i64,
) -> Result<serde_json::Value, String> {
    let instance = match get_instance_or_error(&mpv_state) {
        Ok(instance) => instance,
        Err(payload) => return Ok(payload),
    };

    match instance.sub_remove(track_id) {
        Ok(()) => Ok(ok()),
        Err(e) => Ok(err(e)),
    }
}

/// Reload a subtitle track by id.
#[tauri::command]
pub async fn sub_reload(
    mpv_state: State<'_, MpvState>,
    track_id: i64,
) -> Result<serde_json::Value, String> {
    let instance = match get_instance_or_error(&mpv_state) {
        Ok(instance) => instance,
        Err(payload) => return Ok(payload),
    };

    match instance.sub_reload(track_id) {
        Ok(()) => Ok(ok()),
        Err(e) => Ok(err(e)),
    }
}

/// Add an external audio track.
#[tauri::command]
pub async fn audio_add(
    mpv_state: State<'_, MpvState>,
    path: String,
    mode: Option<String>,
) -> Result<serde_json::Value, String> {
    let instance = match get_instance_or_error(&mpv_state) {
        Ok(instance) => instance,
        Err(payload) => return Ok(payload),
    };

    let mode = mode.as_deref().unwrap_or("select");

    match instance.audio_add(&path, mode) {
        Ok(()) => Ok(ok()),
        Err(e) => Ok(err(e)),
    }
}

/// Set subtitle delay in seconds.
#[tauri::command]
pub async fn set_sub_delay(
    mpv_state: State<'_, MpvState>,
    seconds: f64,
) -> Result<serde_json::Value, String> {
    let instance = match get_instance_or_error(&mpv_state) {
        Ok(instance) => instance,
        Err(payload) => return Ok(payload),
    };

    match instance.set_sub_delay(seconds) {
        Ok(()) => Ok(ok()),
        Err(e) => Ok(err(e)),
    }
}

/// Set audio delay in seconds.
#[tauri::command]
pub async fn set_audio_delay(
    mpv_state: State<'_, MpvState>,
    seconds: f64,
) -> Result<serde_json::Value, String> {
    let instance = match get_instance_or_error(&mpv_state) {
        Ok(instance) => instance,
        Err(payload) => return Ok(payload),
    };

    match instance.set_audio_delay(seconds) {
        Ok(()) => Ok(ok()),
        Err(e) => Ok(err(e)),
    }
}

/// Set subtitle font scale.
#[tauri::command]
pub async fn set_sub_scale(
    mpv_state: State<'_, MpvState>,
    scale: f64,
) -> Result<serde_json::Value, String> {
    let instance = match get_instance_or_error(&mpv_state) {
        Ok(instance) => instance,
        Err(payload) => return Ok(payload),
    };

    match instance.set_sub_scale(scale) {
        Ok(()) => Ok(ok()),
        Err(e) => Ok(err(e)),
    }
}

/// Set a color adjustment.
#[tauri::command]
pub async fn set_color_adjustment(
    mpv_state: State<'_, MpvState>,
    name: String,
    value: f64,
) -> Result<serde_json::Value, String> {
    let instance = match get_instance_or_error(&mpv_state) {
        Ok(instance) => instance,
        Err(payload) => return Ok(payload),
    };

    let result = match name.as_str() {
        "brightness" => instance.set_brightness(value),
        "contrast" => instance.set_contrast(value),
        "saturation" => instance.set_saturation(value),
        "gamma" => instance.set_gamma(value),
        "hue" => instance.set_hue(value),
        other => Err(format!("unknown color adjustment: {other}")),
    };

    match result {
        Ok(()) => Ok(ok()),
        Err(e) => Ok(err(e)),
    }
}

/// Generic mpv option-string setter.
#[tauri::command]
pub async fn mpv_set_option_string(
    mpv_state: State<'_, MpvState>,
    name: String,
    value: String,
) -> Result<serde_json::Value, String> {
    let instance = match get_instance_or_error(&mpv_state) {
        Ok(instance) => instance,
        Err(payload) => return Ok(payload),
    };

    match instance.set_option_string(&name, &value) {
        Ok(()) => Ok(ok()),
        Err(e) => Ok(err(e)),
    }
}

/// Get the current track list as a typed JSON object.
#[tauri::command]
pub async fn get_track_list(mpv_state: State<'_, MpvState>) -> Result<serde_json::Value, String> {
    let instance = match get_instance_or_error(&mpv_state) {
        Ok(instance) => instance,
        Err(payload) => return Ok(payload),
    };

    let count: i64 = instance.get_property("track-list/count").unwrap_or(0);
    let mut audio_tracks: Vec<serde_json::Value> = Vec::new();
    let mut sub_tracks: Vec<serde_json::Value> = Vec::new();

    for i in 0..count {
        let id: i64 = instance
            .get_property(&format!("track-list/{i}/id"))
            .unwrap_or(0);
        let title: String = instance
            .get_property(&format!("track-list/{i}/title"))
            .unwrap_or_default();
        let lang: String = instance
            .get_property(&format!("track-list/{i}/lang"))
            .unwrap_or_default();
        let type_: String = instance
            .get_property(&format!("track-list/{i}/type"))
            .unwrap_or_default();
        let selected: bool = instance
            .get_property(&format!("track-list/{i}/selected"))
            .unwrap_or(false);
        let external: bool = instance
            .get_property(&format!("track-list/{i}/external"))
            .unwrap_or(false);
        let src_id: i64 = instance
            .get_property(&format!("track-list/{i}/src-id"))
            .unwrap_or(0);

        let track = json!({
            "id": id,
            "title": title,
            "lang": lang,
            "type": type_,
            "selected": selected,
            "external": external,
            "src_id": src_id,
        });

        if type_ == "audio" {
            audio_tracks.push(track);
        } else if type_ == "sub" {
            sub_tracks.push(track);
        }
    }

    Ok(json!({
        "success": true,
        "audio_tracks": audio_tracks,
        "sub_tracks": sub_tracks,
    }))
}

/// Get the current chapter list.
#[tauri::command]
pub async fn get_chapter_list(mpv_state: State<'_, MpvState>) -> Result<serde_json::Value, String> {
    let instance = match get_instance_or_error(&mpv_state) {
        Ok(instance) => instance,
        Err(payload) => return Ok(payload),
    };

    let count: i64 = instance.get_property("chapter-list/count").unwrap_or(0);
    let mut chapters: Vec<serde_json::Value> = Vec::new();

    for i in 0..count {
        let title: String = instance
            .get_property(&format!("chapter-list/{i}/title"))
            .unwrap_or_default();
        let time: f64 = instance
            .get_property(&format!("chapter-list/{i}/time"))
            .unwrap_or(0.0);

        chapters.push(json!({
            "title": title,
            "time": time,
        }));
    }

    Ok(json!({
        "success": true,
        "chapters": chapters,
    }))
}

/// Navigate to a chapter by index.
#[tauri::command]
pub async fn set_chapter(
    mpv_state: State<'_, MpvState>,
    index: i64,
) -> Result<serde_json::Value, String> {
    let instance = match get_instance_or_error(&mpv_state) {
        Ok(instance) => instance,
        Err(payload) => return Ok(payload),
    };

    match instance.set_chapter(index) {
        Ok(()) => Ok(ok()),
        Err(e) => Ok(err(e)),
    }
}

/// Generic string-valued mpv property getter.
#[tauri::command]
pub async fn get_property(
    mpv_state: State<'_, MpvState>,
    name: String,
) -> Result<serde_json::Value, String> {
    let instance = match get_instance_or_error(&mpv_state) {
        Ok(instance) => instance,
        Err(payload) => return Ok(payload),
    };

    match instance.get_property::<String>(&name) {
        Ok(value) => Ok(json!({
            "success": true,
            "value": value,
        })),
        Err(e) => Ok(err(e)),
    }
}
