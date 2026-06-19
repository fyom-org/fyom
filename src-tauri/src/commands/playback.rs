//! Playback commands — native libmpv command surface.
//!
//! This module is the Tauri command boundary for desktop playback.
//!
//! Keep this layer thin:
//! - validate command input
//! - resolve `MpvInstance`
//! - call mpv facade
//! - return stable JSON payloads
//!
//! Important:
//! The current FYOM video path uses native `--wid` embedding.
//! This command module must not create or drive `mpv_render_context`.
//!
//! macOS production path:
//! - create a dedicated `NSView`
//! - attach `CAMetalLayer`
//! - expose the `NSView` pointer as mpv `wid`
//! - initialize mpv with that `wid`
//! - retain the platform surface for the playback lifetime
//! - let mpv own Vulkan/MoltenVK/Metal rendering internally

use serde_json::{Value, json};
use tauri::State;

use crate::mpv::MpvInstance;
use crate::state::MpvState;

// -----------------------------------------------------------------------------
// Response helpers
// -----------------------------------------------------------------------------

fn ok() -> Value {
    json!({ "success": true })
}

fn ok_with(extra: Value) -> Value {
    let mut base = json!({ "success": true });

    if let (Some(base), Some(extra)) = (base.as_object_mut(), extra.as_object()) {
        for (key, value) in extra {
            base.insert(key.clone(), value.clone());
        }
    }

    base
}

fn err(message: impl Into<String>) -> Value {
    json!({
        "success": false,
        "error": message.into(),
    })
}

fn no_instance_error(mpv_state: &MpvState) -> Value {
    err(mpv_state
        .init_error()
        .unwrap_or_else(|| "libmpv not initialized; attach render surface first".to_string()))
}

fn get_instance<'a>(mpv_state: &'a MpvState) -> Result<&'a MpvInstance, Value> {
    mpv_state
        .get_instance()
        .ok_or_else(|| no_instance_error(mpv_state))
}

fn require_instance<'a>(mpv_state: &'a MpvState) -> Result<&'a MpvInstance, Value> {
    mpv_state.require_instance().map_err(err)
}

fn normalize_mpv_version(raw: Option<String>) -> Option<String> {
    raw.map(|value| value.trim().to_string())
        .filter(|value| !value.is_empty())
        .map(|value| value.strip_prefix("mpv ").unwrap_or(&value).to_string())
}

fn normalize_generic_version(raw: Option<String>) -> Option<String> {
    raw.map(|value| value.trim().to_string())
        .filter(|value| !value.is_empty())
}

fn ensure_finite(name: &str, value: f64) -> Result<(), Value> {
    if value.is_finite() {
        Ok(())
    } else {
        Err(err(format!("{name} must be finite")))
    }
}

fn ensure_non_empty(name: &str, value: &str) -> Result<(), Value> {
    if value.trim().is_empty() {
        Err(err(format!("{name} must not be empty")))
    } else {
        Ok(())
    }
}

fn track_selection(track_id: Option<i64>) -> String {
    match track_id {
        Some(id) => id.to_string(),
        None => "no".to_string(),
    }
}

fn restart_from_eof_if_needed(instance: &MpvInstance) -> Result<(), String> {
    let eof: bool = instance.get_property("eof-reached").unwrap_or(false);

    if eof {
        instance.command("seek", &["0", "absolute", "exact"])?;
        instance.set_property("pause", false)?;
    }

    Ok(())
}

fn command_result(result: Result<(), String>) -> Result<Value, String> {
    Ok(match result {
        Ok(()) => ok(),
        Err(error) => err(error),
    })
}

fn resolve_media_url_for_mpv(
    app_state: &crate::AppState,
    media_url: &str,
) -> Result<String, String> {
    let media_url = media_url.trim();

    if media_url.is_empty() {
        return Err("media_url must not be empty".to_string());
    }

    if media_url.starts_with("http://")
        || media_url.starts_with("https://")
        || media_url.starts_with("file://")
        || media_url.starts_with("lavfi://")
    {
        return Ok(media_url.to_string());
    }

    if media_url.starts_with('/') {
        let api_base_url = app_state.sidecar_state.get_api_base_url().map_err(|error| {
            format!(
                "sidecar API is not ready; cannot resolve relative media URL `{media_url}`: {error}"
            )
        })?;

        let api_base_url = api_base_url.trim_end_matches('/');
        let media_url = media_url.trim_start_matches('/');

        return Ok(format!("{api_base_url}/{media_url}"));
    }

    Ok(media_url.to_string())
}

// -----------------------------------------------------------------------------
// Backend info
// -----------------------------------------------------------------------------

#[tauri::command]
pub async fn get_playback_backend_info(mpv_state: State<'_, MpvState>) -> Result<Value, String> {
    let Ok(instance) = get_instance(&mpv_state) else {
        return Ok(json!({
            "backend": mpv_state
                .init_error()
                .map(|error| format!("libmpv (init failed: {error})"))
                .unwrap_or_else(|| "none".to_string()),
            "version": "0.1.0",
            "capabilities": [],
            "native_playback": false,
            "ready": false,
            "render_mode": "none",
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
        "capabilities": [
            "audio",
            "video",
            "subtitles",
            "hardware-decode",
            "native-wid-embedding"
        ],
        "native_playback": true,
        "ready": true,
        "render_mode": "wid",
        "hwdec": "auto-safe",
        "hwdec_current": hwdec_current,
        "volume": volume,
        "paused": paused,
    }))
}

// -----------------------------------------------------------------------------
// Core playback contract
// -----------------------------------------------------------------------------

#[tauri::command]
pub async fn play_media(
    mpv_state: State<'_, MpvState>,
    app_state: State<'_, crate::AppState>,
    media_url: String,
    poster_url: Option<String>,
) -> Result<Value, String> {
    let _ = poster_url;

    if let Err(payload) = ensure_non_empty("media_url", &media_url) {
        return Ok(payload);
    }

    let instance = match require_instance(&mpv_state) {
        Ok(instance) => instance,
        Err(payload) => return Ok(payload),
    };

    let resolved_media_url = match resolve_media_url_for_mpv(&app_state, &media_url) {
        Ok(url) => url,
        Err(error) => return Ok(err(error)),
    };

    tracing::info!(
        "[mpv/playback] play_media resolved url: input={} resolved={}",
        media_url,
        resolved_media_url
    );

    command_result(instance.loadfile(&resolved_media_url))
}

#[tauri::command]
pub async fn stop_media(mpv_state: State<'_, MpvState>) -> Result<Value, String> {
    let instance = match require_instance(&mpv_state) {
        Ok(instance) => instance,
        Err(payload) => return Ok(payload),
    };

    command_result(instance.stop())
}

// -----------------------------------------------------------------------------
// Playback controls
// -----------------------------------------------------------------------------

#[tauri::command]
pub async fn seek(mpv_state: State<'_, MpvState>, position: f64) -> Result<Value, String> {
    if let Err(payload) = ensure_finite("position", position) {
        return Ok(payload);
    }

    let instance = match require_instance(&mpv_state) {
        Ok(instance) => instance,
        Err(payload) => return Ok(payload),
    };

    let position = position.to_string();

    match instance.command("seek", &[position.as_str(), "absolute"]) {
        Ok(()) => {
            if let Err(error) = restart_from_eof_if_needed(instance) {
                return Ok(err(error));
            }

            Ok(ok())
        }
        Err(error) => Ok(err(error)),
    }
}

#[tauri::command]
pub async fn seek_relative(mpv_state: State<'_, MpvState>, seconds: f64) -> Result<Value, String> {
    if let Err(payload) = ensure_finite("seconds", seconds) {
        return Ok(payload);
    }

    let instance = match require_instance(&mpv_state) {
        Ok(instance) => instance,
        Err(payload) => return Ok(payload),
    };

    let seconds = seconds.to_string();

    command_result(instance.command("seek", &[seconds.as_str(), "relative"]))
}

#[tauri::command]
pub async fn toggle_pause(mpv_state: State<'_, MpvState>) -> Result<Value, String> {
    let instance = match require_instance(&mpv_state) {
        Ok(instance) => instance,
        Err(payload) => return Ok(payload),
    };

    if let Err(error) = restart_from_eof_if_needed(instance) {
        return Ok(err(error));
    }

    command_result(instance.command("cycle", &["pause"]))
}

#[tauri::command]
pub async fn set_pause(mpv_state: State<'_, MpvState>, paused: bool) -> Result<Value, String> {
    let instance = match require_instance(&mpv_state) {
        Ok(instance) => instance,
        Err(payload) => return Ok(payload),
    };

    command_result(instance.set_pause(paused))
}

#[tauri::command]
pub async fn set_volume(mpv_state: State<'_, MpvState>, volume: i64) -> Result<Value, String> {
    let instance = match require_instance(&mpv_state) {
        Ok(instance) => instance,
        Err(payload) => return Ok(payload),
    };

    let volume = volume.clamp(0, 100);

    command_result(instance.set_property("volume", volume))
}

#[tauri::command]
pub async fn set_speed(mpv_state: State<'_, MpvState>, speed: f64) -> Result<Value, String> {
    if let Err(payload) = ensure_finite("speed", speed) {
        return Ok(payload);
    }

    if speed <= 0.0 {
        return Ok(err("speed must be greater than 0"));
    }

    let instance = match require_instance(&mpv_state) {
        Ok(instance) => instance,
        Err(payload) => return Ok(payload),
    };

    command_result(instance.set_property("speed", speed))
}

#[tauri::command]
pub async fn set_audio_track(
    mpv_state: State<'_, MpvState>,
    track_id: Option<i64>,
) -> Result<Value, String> {
    let instance = match require_instance(&mpv_state) {
        Ok(instance) => instance,
        Err(payload) => return Ok(payload),
    };

    let selection = track_selection(track_id);

    command_result(instance.set_property("aid", selection))
}

#[tauri::command]
pub async fn set_subtitle_track(
    mpv_state: State<'_, MpvState>,
    track_id: Option<i64>,
) -> Result<Value, String> {
    let instance = match require_instance(&mpv_state) {
        Ok(instance) => instance,
        Err(payload) => return Ok(payload),
    };

    let selection = track_selection(track_id);

    command_result(instance.set_property("sid", selection))
}

#[tauri::command]
pub async fn mpv_keypress(mpv_state: State<'_, MpvState>, key: String) -> Result<Value, String> {
    if let Err(payload) = ensure_non_empty("key", &key) {
        return Ok(payload);
    }

    let instance = match require_instance(&mpv_state) {
        Ok(instance) => instance,
        Err(payload) => return Ok(payload),
    };

    command_result(instance.command("keypress", &[key.as_str()]))
}

#[tauri::command]
pub async fn mpv_command(
    mpv_state: State<'_, MpvState>,
    args: Vec<String>,
) -> Result<Value, String> {
    let instance = match require_instance(&mpv_state) {
        Ok(instance) => instance,
        Err(payload) => return Ok(payload),
    };

    let Some((cmd, rest)) = args.split_first() else {
        return Ok(err("mpv_command requires at least the command name"));
    };

    if let Err(payload) = ensure_non_empty("command", cmd) {
        return Ok(payload);
    }

    let rest: Vec<&str> = rest.iter().map(String::as_str).collect();

    command_result(instance.command(cmd, &rest))
}

// -----------------------------------------------------------------------------
// Test / diagnostics
// -----------------------------------------------------------------------------

#[tauri::command]
pub async fn play_test_media(mpv_state: State<'_, MpvState>) -> Result<Value, String> {
    let instance = match require_instance(&mpv_state) {
        Ok(instance) => instance,
        Err(payload) => return Ok(payload),
    };

    let media = std::env::var("FYOM_MPV_TEST_MEDIA")
        .unwrap_or_else(|_| "lavfi://sine=frequency=440:duration=10".to_string());

    match instance.loadfile(&media) {
        Ok(()) => Ok(ok_with(json!({ "media": media }))),
        Err(error) => Ok(err(error)),
    }
}

// -----------------------------------------------------------------------------
// Native video surface
// -----------------------------------------------------------------------------

/// Attach the native video surface used by mpv `--wid` embedding.
///
/// This command name is intentionally kept as `attach_render_surface` for frontend
/// compatibility, but it no longer means "initialize mpv render API".
///
/// Current behavior:
/// - create platform surface on the main thread
/// - read platform native window id
/// - initialize mpv with `wid`
/// - start mpv event loop
/// - retain the platform surface through `MpvInstance::spawn_render_thread`
/// - do not initialize `mpv_render_context`
#[tauri::command]
pub async fn attach_render_surface(
    mpv_state: State<'_, MpvState>,
    app_handle: tauri::AppHandle,
) -> Result<Value, String> {
    use tauri::Manager;

    let Some(window) = app_handle.get_webview_window(crate::MAIN_WINDOW_LABEL) else {
        return Ok(err("main window not found"));
    };

    let (tx, rx) = tokio::sync::oneshot::channel();

    let run_result = app_handle.run_on_main_thread(move || {
        let result = crate::platform::create_platform_surface(&window);
        let _ = tx.send(result);
    });

    if let Err(error) = run_result {
        tracing::warn!("[mpv/playback] failed to schedule surface creation: {error}");
        return Ok(err(format!(
            "failed to schedule platform surface creation: {error}"
        )));
    }

    let surface = match rx.await {
        Ok(Ok(surface)) => surface,
        Ok(Err(error)) => {
            tracing::warn!("[mpv/playback] platform surface creation failed: {error}");
            return Ok(err(format!("platform surface creation failed: {error}")));
        }
        Err(_) => {
            tracing::warn!("[mpv/playback] platform surface creation task dropped");
            return Ok(err("main thread surface creation task dropped"));
        }
    };

    let backend = surface.backend_name();
    let (width, height) = surface.drawable_size();

    let Some(wid) = surface.native_window_id() else {
        tracing::warn!(
            "[mpv/playback] platform surface does not expose native window id; backend={backend}"
        );

        return Ok(err(
            "platform surface does not expose a native window id for mpv wid embedding",
        ));
    };

    if wid.trim().is_empty() {
        tracing::warn!("[mpv/playback] platform surface returned empty native window id");

        return Ok(err(
            "platform surface returned an empty native window id for mpv wid embedding",
        ));
    }

    tracing::info!(
        "[mpv/playback] native wid surface created; backend={backend}; wid={wid}; drawable={}x{}",
        width,
        height
    );

    let instance = match mpv_state.initialize_with_wid(wid.clone()) {
        Ok(instance) => instance,
        Err(error) => {
            tracing::warn!("[mpv/playback] mpv initialize_with_wid failed: {error}");
            return Ok(err(error));
        }
    };

    instance.spawn_event_loop(app_handle.clone());

    match instance.spawn_render_thread(surface) {
        Ok(()) => Ok(ok_with(json!({
            "render_mode": "wid",
            "backend": backend,
            "wid": wid,
            "drawable_width": width,
            "drawable_height": height,
            "mpv_initialized": true,
        }))),
        Err(error) => {
            tracing::warn!("[mpv/playback] native surface lifecycle spawn failed: {error}");
            Ok(err(error))
        }
    }
}

#[tauri::command]
pub async fn set_video_mode(
    _mpv_state: State<'_, MpvState>,
    enabled: bool,
) -> Result<Value, String> {
    tracing::info!(
        "[mpv/playback] video mode {}",
        if enabled { "enabled" } else { "disabled" }
    );

    Ok(ok())
}

#[tauri::command]
pub async fn resize_render_surface(
    _mpv_state: State<'_, MpvState>,
    width: u32,
    height: u32,
    scale_factor: f64,
) -> Result<Value, String> {
    if width == 0 || height == 0 {
        return Ok(err("render surface size must be non-zero"));
    }

    if let Err(payload) = ensure_finite("scale_factor", scale_factor) {
        return Ok(payload);
    }

    tracing::debug!(
        "[mpv/playback] resize surface request: {}x{} scale={}",
        width,
        height,
        scale_factor
    );

    Ok(ok())
}

#[tauri::command]
pub async fn get_api_base_url(state: tauri::State<'_, crate::AppState>) -> Result<String, String> {
    state
        .sidecar_state
        .get_api_base_url()
        .map_err(|error| error.to_string())
}

// -----------------------------------------------------------------------------
// Subtitles / audio tracks
// -----------------------------------------------------------------------------

#[tauri::command]
pub async fn find_external_subtitles(
    _mpv_state: State<'_, MpvState>,
    media_path: String,
    media_title: Option<String>,
) -> Result<Value, String> {
    if let Err(payload) = ensure_non_empty("media_path", &media_path) {
        return Ok(payload);
    }

    let payload = crate::subtitles::ExternalSubtitleMatchesPayloadResolved {
        media_path,
        media_title,
    };

    let matches = crate::subtitles::find_external_subtitles_impl(payload).await?;

    Ok(ok_with(json!({ "matches": matches })))
}

#[tauri::command]
pub async fn sub_add(
    mpv_state: State<'_, MpvState>,
    path: String,
    mode: Option<String>,
    title: Option<String>,
    lang: Option<String>,
) -> Result<Value, String> {
    if let Err(payload) = ensure_non_empty("path", &path) {
        return Ok(payload);
    }

    let instance = match require_instance(&mpv_state) {
        Ok(instance) => instance,
        Err(payload) => return Ok(payload),
    };

    let mode = mode.unwrap_or_else(|| "select".to_string());

    let mut args: Vec<&str> = vec![path.as_str(), mode.as_str()];

    if title.is_some() || lang.is_some() {
        args.push(title.as_deref().unwrap_or(""));
    }

    if let Some(lang) = lang.as_deref() {
        args.push(lang);
    }

    command_result(instance.command("sub-add", &args))
}

#[tauri::command]
pub async fn sub_remove(mpv_state: State<'_, MpvState>, track_id: i64) -> Result<Value, String> {
    let instance = match require_instance(&mpv_state) {
        Ok(instance) => instance,
        Err(payload) => return Ok(payload),
    };

    let id = track_id.to_string();

    command_result(instance.command("sub-remove", &[id.as_str()]))
}

#[tauri::command]
pub async fn sub_reload(mpv_state: State<'_, MpvState>, track_id: i64) -> Result<Value, String> {
    let instance = match require_instance(&mpv_state) {
        Ok(instance) => instance,
        Err(payload) => return Ok(payload),
    };

    let id = track_id.to_string();

    command_result(instance.command("sub-reload", &[id.as_str()]))
}

#[tauri::command]
pub async fn audio_add(
    mpv_state: State<'_, MpvState>,
    path: String,
    mode: Option<String>,
) -> Result<Value, String> {
    if let Err(payload) = ensure_non_empty("path", &path) {
        return Ok(payload);
    }

    let instance = match require_instance(&mpv_state) {
        Ok(instance) => instance,
        Err(payload) => return Ok(payload),
    };

    let mode = mode.unwrap_or_else(|| "select".to_string());

    command_result(instance.command("audio-add", &[path.as_str(), mode.as_str()]))
}

#[tauri::command]
pub async fn audio_remove(mpv_state: State<'_, MpvState>, track_id: i64) -> Result<Value, String> {
    let instance = match require_instance(&mpv_state) {
        Ok(instance) => instance,
        Err(payload) => return Ok(payload),
    };

    let id = track_id.to_string();

    command_result(instance.command("audio-remove", &[id.as_str()]))
}

// -----------------------------------------------------------------------------
// Playback adjustments
// -----------------------------------------------------------------------------

#[tauri::command]
pub async fn set_sub_delay(mpv_state: State<'_, MpvState>, seconds: f64) -> Result<Value, String> {
    if let Err(payload) = ensure_finite("seconds", seconds) {
        return Ok(payload);
    }

    let instance = match require_instance(&mpv_state) {
        Ok(instance) => instance,
        Err(payload) => return Ok(payload),
    };

    command_result(instance.set_property("sub-delay", seconds))
}

#[tauri::command]
pub async fn set_secondary_sub_delay(
    mpv_state: State<'_, MpvState>,
    seconds: f64,
) -> Result<Value, String> {
    if let Err(payload) = ensure_finite("seconds", seconds) {
        return Ok(payload);
    }

    let instance = match require_instance(&mpv_state) {
        Ok(instance) => instance,
        Err(payload) => return Ok(payload),
    };

    command_result(instance.set_property("secondary-sub-delay", seconds))
}

#[tauri::command]
pub async fn set_audio_delay(
    mpv_state: State<'_, MpvState>,
    seconds: f64,
) -> Result<Value, String> {
    if let Err(payload) = ensure_finite("seconds", seconds) {
        return Ok(payload);
    }

    let instance = match require_instance(&mpv_state) {
        Ok(instance) => instance,
        Err(payload) => return Ok(payload),
    };

    command_result(instance.set_property("audio-delay", seconds))
}

#[tauri::command]
pub async fn set_sub_scale(mpv_state: State<'_, MpvState>, scale: f64) -> Result<Value, String> {
    if let Err(payload) = ensure_finite("scale", scale) {
        return Ok(payload);
    }

    if scale <= 0.0 {
        return Ok(err("scale must be greater than 0"));
    }

    let instance = match require_instance(&mpv_state) {
        Ok(instance) => instance,
        Err(payload) => return Ok(payload),
    };

    command_result(instance.set_property("sub-scale", scale))
}

#[tauri::command]
pub async fn set_color_adjustment(
    mpv_state: State<'_, MpvState>,
    name: String,
    value: f64,
) -> Result<Value, String> {
    if let Err(payload) = ensure_finite("value", value) {
        return Ok(payload);
    }

    let instance = match require_instance(&mpv_state) {
        Ok(instance) => instance,
        Err(payload) => return Ok(payload),
    };

    let property = match name.as_str() {
        "brightness" => "brightness",
        "contrast" => "contrast",
        "saturation" => "saturation",
        "gamma" => "gamma",
        "hue" => "hue",
        other => return Ok(err(format!("unknown color adjustment: {other}"))),
    };

    command_result(instance.set_property(property, value))
}

#[tauri::command]
pub async fn set_chapter(mpv_state: State<'_, MpvState>, index: i64) -> Result<Value, String> {
    let instance = match require_instance(&mpv_state) {
        Ok(instance) => instance,
        Err(payload) => return Ok(payload),
    };

    command_result(instance.set_property("chapter", index))
}

// -----------------------------------------------------------------------------
// mpv option / property
// -----------------------------------------------------------------------------

#[tauri::command]
pub async fn mpv_set_option_string(
    mpv_state: State<'_, MpvState>,
    name: String,
    value: String,
) -> Result<Value, String> {
    if let Err(payload) = ensure_non_empty("name", &name) {
        return Ok(payload);
    }

    let instance = match require_instance(&mpv_state) {
        Ok(instance) => instance,
        Err(payload) => return Ok(payload),
    };

    command_result(instance.set_property(name.as_str(), value))
}

#[tauri::command]
pub async fn get_property(mpv_state: State<'_, MpvState>, name: String) -> Result<Value, String> {
    if let Err(payload) = ensure_non_empty("name", &name) {
        return Ok(payload);
    }

    let instance = match require_instance(&mpv_state) {
        Ok(instance) => instance,
        Err(payload) => return Ok(payload),
    };

    match instance.get_property::<String>(&name) {
        Ok(value) => Ok(ok_with(json!({ "value": value }))),
        Err(error) => Ok(err(error)),
    }
}

// -----------------------------------------------------------------------------
// Read models
// -----------------------------------------------------------------------------

#[tauri::command]
pub async fn get_track_list(mpv_state: State<'_, MpvState>) -> Result<Value, String> {
    let instance = match require_instance(&mpv_state) {
        Ok(instance) => instance,
        Err(payload) => return Ok(payload),
    };

    let count: i64 = instance.get_property("track-list/count").unwrap_or(0);
    let mut audio_tracks = Vec::new();
    let mut sub_tracks = Vec::new();

    for index in 0..count {
        let id: i64 = instance
            .get_property(&format!("track-list/{index}/id"))
            .unwrap_or(0);
        let title: String = instance
            .get_property(&format!("track-list/{index}/title"))
            .unwrap_or_default();
        let lang: String = instance
            .get_property(&format!("track-list/{index}/lang"))
            .unwrap_or_default();
        let type_: String = instance
            .get_property(&format!("track-list/{index}/type"))
            .unwrap_or_default();
        let selected: bool = instance
            .get_property(&format!("track-list/{index}/selected"))
            .unwrap_or(false);
        let external: bool = instance
            .get_property(&format!("track-list/{index}/external"))
            .unwrap_or(false);
        let src_id: i64 = instance
            .get_property(&format!("track-list/{index}/src-id"))
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

        match type_.as_str() {
            "audio" => audio_tracks.push(track),
            "sub" => sub_tracks.push(track),
            _ => {}
        }
    }

    Ok(ok_with(json!({
        "audio_tracks": audio_tracks,
        "sub_tracks": sub_tracks,
    })))
}

#[tauri::command]
pub async fn get_chapter_list(mpv_state: State<'_, MpvState>) -> Result<Value, String> {
    let instance = match require_instance(&mpv_state) {
        Ok(instance) => instance,
        Err(payload) => return Ok(payload),
    };

    let count: i64 = instance.get_property("chapter-list/count").unwrap_or(0);
    let mut chapters = Vec::new();

    for index in 0..count {
        let title: String = instance
            .get_property(&format!("chapter-list/{index}/title"))
            .unwrap_or_default();
        let time: f64 = instance
            .get_property(&format!("chapter-list/{index}/time"))
            .unwrap_or(0.0);

        chapters.push(json!({
            "title": title,
            "time": time,
        }));
    }

    Ok(ok_with(json!({
        "chapters": chapters,
    })))
}
