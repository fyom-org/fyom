//! External player launcher commands.
//!
//! FYOM Desktop delegates playback to an external player instead of embedding
//! mpv into the Tauri window.
//!
//! Launcher priority:
//! 1. Environment variables:
//!    - `FYOM_EXTERNAL_PLAYER`
//!    - `FYOM_EXTERNAL_PLAYER_ARGS`
//!    - `FYOM_MPV_BIN`
//! 2. Desktop config:
//!    - `configs/fyom-desktop.json`
//!    - or `FYOM_DESKTOP_CONFIG`
//! 3. OS default opener:
//!    - macOS: `open`
//!    - Linux: `xdg-open`
//!    - Windows: `rundll32.exe url.dll,FileProtocolHandler`
//!
//! Important architecture boundary:
//! - The Go backend does not know local player configuration.
//! - Player executable paths are local desktop concerns only.
//! - This command must never pass `--wid` or use native embedded rendering.

use std::{
    path::Path,
    process::{Command, Stdio},
};

use serde_json::{Value, json};
use tauri::State;

use crate::{
    AppState,
    desktop_config::{
        ExternalPlayerConfig as DesktopExternalPlayerConfig,
        ExternalPlayerKind as DesktopExternalPlayerKind,
    },
};

// -----------------------------------------------------------------------------
// Environment
// -----------------------------------------------------------------------------

const ENV_EXTERNAL_PLAYER: &str = "FYOM_EXTERNAL_PLAYER";
const ENV_EXTERNAL_PLAYER_ARGS: &str = "FYOM_EXTERNAL_PLAYER_ARGS";
const ENV_MPV_BIN: &str = "FYOM_MPV_BIN";

const URL_PLACEHOLDER: &str = "{url}";

// -----------------------------------------------------------------------------
// Response helpers
// -----------------------------------------------------------------------------

fn ok(resolved_url: &str, launcher: &str) -> Value {
    json!({
        "success": true,
        "resolved_url": resolved_url,
        "launcher": launcher,
    })
}

// -----------------------------------------------------------------------------
// External player configuration
// -----------------------------------------------------------------------------

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum ExternalPlayerKind {
    Generic,
    Mpv,
}

impl ExternalPlayerKind {
    fn as_str(self) -> &'static str {
        match self {
            ExternalPlayerKind::Generic => "custom",
            ExternalPlayerKind::Mpv => "mpv",
        }
    }
}

#[derive(Debug, Clone)]
struct ResolvedExternalPlayerConfig {
    program: String,
    kind: ExternalPlayerKind,
    source: String,
    args: Vec<String>,
    append_default_mpv_args: bool,
}

fn configured_external_player(
    desktop_config: &DesktopExternalPlayerConfig,
) -> Option<ResolvedExternalPlayerConfig> {
    if let Some(program) = non_empty_env(ENV_EXTERNAL_PLAYER) {
        let kind = if looks_like_mpv_binary(&program) {
            ExternalPlayerKind::Mpv
        } else {
            ExternalPlayerKind::Generic
        };

        return Some(ResolvedExternalPlayerConfig {
            program,
            kind,
            source: ENV_EXTERNAL_PLAYER.to_string(),
            args: parse_env_player_args(),
            append_default_mpv_args: kind == ExternalPlayerKind::Mpv,
        });
    }

    if let Some(program) = non_empty_env(ENV_MPV_BIN) {
        return Some(ResolvedExternalPlayerConfig {
            program,
            kind: ExternalPlayerKind::Mpv,
            source: ENV_MPV_BIN.to_string(),
            args: parse_env_player_args(),
            append_default_mpv_args: true,
        });
    }

    match desktop_config.kind {
        DesktopExternalPlayerKind::System => None,

        DesktopExternalPlayerKind::Mpv => {
            let program = if desktop_config.program.trim().is_empty() {
                "mpv".to_string()
            } else {
                desktop_config.program.trim().to_string()
            };

            Some(ResolvedExternalPlayerConfig {
                program,
                kind: ExternalPlayerKind::Mpv,
                source: "desktop-config".to_string(),
                args: desktop_config.args.clone(),
                append_default_mpv_args: desktop_config.append_default_mpv_args,
            })
        }

        DesktopExternalPlayerKind::Custom => {
            let program = desktop_config.program.trim();

            if program.is_empty() {
                tracing::warn!(
                    "[launcher] desktop external player kind=custom but program is empty; falling back to system opener"
                );

                return None;
            }

            Some(ResolvedExternalPlayerConfig {
                program: program.to_string(),
                kind: ExternalPlayerKind::Generic,
                source: "desktop-config".to_string(),
                args: desktop_config.args.clone(),
                append_default_mpv_args: false,
            })
        }
    }
}

fn non_empty_env(name: &str) -> Option<String> {
    std::env::var(name)
        .ok()
        .map(|value| value.trim().to_string())
        .filter(|value| !value.is_empty())
}

fn parse_env_player_args() -> Vec<String> {
    let Some(raw_args) = non_empty_env(ENV_EXTERNAL_PLAYER_ARGS) else {
        return Vec::new();
    };

    // Intentionally simple: this is whitespace-based, not shell parsing.
    //
    // Examples:
    //
    // FYOM_EXTERNAL_PLAYER_ARGS="--profile=fyom {url}"
    // FYOM_EXTERNAL_PLAYER_ARGS="--fullscreen --profile=fyom {url}"
    //
    // For arguments containing spaces, use `configs/fyom-desktop.json` instead:
    //
    // {
    //   "externalPlayer": {
    //     "args": ["--script-opts=foo=a b", "{url}"]
    //   }
    // }
    raw_args
        .split_whitespace()
        .map(ToString::to_string)
        .collect()
}

fn looks_like_mpv_binary(program: &str) -> bool {
    let file_name = Path::new(program)
        .file_name()
        .and_then(|name| name.to_str())
        .unwrap_or(program)
        .to_ascii_lowercase();

    file_name == "mpv" || file_name == "mpv.exe" || file_name.contains("mpv")
}

fn build_configured_player_args(config: &ResolvedExternalPlayerConfig, url: &str) -> Vec<String> {
    let mut args = Vec::new();

    if config.kind == ExternalPlayerKind::Mpv && config.append_default_mpv_args {
        args.extend(default_mpv_args());
    }

    let inserted_url = append_configured_args(&mut args, &config.args, url);

    if !inserted_url {
        if config.kind == ExternalPlayerKind::Mpv {
            args.push("--".to_string());
        }

        args.push(url.to_string());
    }

    args
}

fn append_configured_args(args: &mut Vec<String>, configured_args: &[String], url: &str) -> bool {
    let mut inserted_url = false;

    for arg in configured_args {
        if arg == URL_PLACEHOLDER {
            args.push(url.to_string());
            inserted_url = true;
        } else {
            args.push(arg.to_string());
        }
    }

    inserted_url
}

fn default_mpv_args() -> Vec<String> {
    vec![
        "--force-window=yes".to_string(),
        "--keep-open=yes".to_string(),
        "--player-operation-mode=pseudo-gui".to_string(),
        "--title=FYOM Player".to_string(),
        "--no-terminal".to_string(),
    ]
}

// -----------------------------------------------------------------------------
// Resolve media URL
// -----------------------------------------------------------------------------

fn resolve_media_url(app_state: &AppState, media_url: &str) -> Result<String, String> {
    let media_url = media_url.trim();

    if media_url.is_empty() {
        return Err("media_url must not be empty".to_string());
    }

    if has_url_scheme(media_url) {
        return Ok(media_url.to_string());
    }

    if is_windows_absolute_path(media_url) {
        return Ok(media_url.to_string());
    }

    if is_existing_local_path(media_url) {
        return Ok(media_url.to_string());
    }

    if media_url.starts_with('/') {
        return resolve_sidecar_relative_url(app_state, media_url);
    }

    Ok(media_url.to_string())
}

fn resolve_sidecar_relative_url(app_state: &AppState, media_url: &str) -> Result<String, String> {
    let api_base_url = app_state
        .sidecar_state
        .get_api_base_url()
        .map_err(|error| {
            format!(
                "sidecar API is not ready; cannot resolve relative media URL `{media_url}`: {error}"
            )
        })?;

    let api_base_url = api_base_url.trim_end_matches('/');
    let media_url = media_url.trim_start_matches('/');

    Ok(format!("{api_base_url}/{media_url}"))
}

fn has_url_scheme(input: &str) -> bool {
    let Some(colon_index) = input.find(':') else {
        return false;
    };

    // Avoid treating Windows drive paths like `C:\movie.mkv` as URL schemes.
    if colon_index == 1 {
        let first = input.as_bytes()[0];

        if first.is_ascii_alphabetic() {
            return false;
        }
    }

    let scheme = &input[..colon_index];

    let Some(first) = scheme.as_bytes().first() else {
        return false;
    };

    if !first.is_ascii_alphabetic() {
        return false;
    }

    scheme
        .bytes()
        .all(|byte| byte.is_ascii_alphanumeric() || matches!(byte, b'+' | b'-' | b'.'))
}

fn is_windows_absolute_path(input: &str) -> bool {
    let bytes = input.as_bytes();

    if input.starts_with(r"\\") {
        return true;
    }

    bytes.len() >= 3
        && bytes[0].is_ascii_alphabetic()
        && bytes[1] == b':'
        && matches!(bytes[2], b'\\' | b'/')
}

fn is_existing_local_path(input: &str) -> bool {
    Path::new(input).exists()
}

// -----------------------------------------------------------------------------
// Cross-platform external player launch
// -----------------------------------------------------------------------------

fn open_url_in_external_player(
    desktop_config: &DesktopExternalPlayerConfig,
    url: &str,
) -> Result<String, String> {
    if let Some(config) = configured_external_player(desktop_config) {
        return launch_configured_external_player(&config, url);
    }

    launch_system_default_opener(url)
}

fn launch_configured_external_player(
    config: &ResolvedExternalPlayerConfig,
    url: &str,
) -> Result<String, String> {
    let args = build_configured_player_args(config, url);

    tracing::info!(
        "[launcher] launching configured external player; source={}; kind={}; program={}; args={:?}",
        config.source,
        config.kind.as_str(),
        config.program,
        args
    );

    let mut command = Command::new(&config.program);
    command.args(&args);

    spawn_detached(command, &format!("configured player `{}`", config.program))?;

    Ok(format!("configured:{}", config.program))
}

fn launch_system_default_opener(url: &str) -> Result<String, String> {
    #[cfg(target_os = "macos")]
    {
        let mut command = Command::new("open");
        command.arg(url);

        spawn_detached(command, "`open`")?;

        Ok("system:open".to_string())
    }

    #[cfg(target_os = "linux")]
    {
        let mut command = Command::new("xdg-open");
        command.arg(url);

        spawn_detached(command, "`xdg-open`")?;

        Ok("system:xdg-open".to_string())
    }

    #[cfg(target_os = "windows")]
    {
        let mut command = Command::new("rundll32.exe");
        command.args(["url.dll,FileProtocolHandler", url]);

        spawn_detached(command, "`rundll32.exe url.dll,FileProtocolHandler`")?;

        Ok("system:rundll32".to_string())
    }

    #[cfg(not(any(target_os = "macos", target_os = "linux", target_os = "windows")))]
    {
        let _ = url;

        Err("unsupported target OS: no external player launcher available".to_string())
    }
}

fn spawn_detached(mut command: Command, launcher_name: &str) -> Result<(), String> {
    command
        .stdin(Stdio::null())
        .stdout(Stdio::null())
        .stderr(Stdio::null());

    command.spawn().map_err(|error| {
        format!("failed to launch external player with {launcher_name}: {error}")
    })?;

    Ok(())
}

// -----------------------------------------------------------------------------
// Tauri commands
// -----------------------------------------------------------------------------

/// Open a media URL in an external player.
///
/// Resolution behavior:
///
/// - Absolute URLs are passed through unchanged.
/// - Existing local paths are passed through unchanged.
/// - Relative `/...` URLs are resolved against the FYOM sidecar API base URL.
/// - Other values are passed through as-is.
///
/// Launcher behavior:
///
/// - `FYOM_EXTERNAL_PLAYER` takes priority.
/// - `FYOM_MPV_BIN` is used as an mpv-specific fallback.
/// - `configs/fyom-desktop.json` is used when no environment override exists.
/// - If no configured player is available, the OS default opener is used.
#[tauri::command]
pub async fn open_external_player(
    app_state: State<'_, AppState>,
    media_url: String,
) -> Result<Value, String> {
    let resolved_url = resolve_media_url(&app_state, &media_url)?;

    tracing::info!(
        "[launcher] opening external player; input={} resolved={}",
        media_url,
        resolved_url
    );

    let launcher = open_url_in_external_player(
        &app_state.desktop_config.as_ref().external_player,
        &resolved_url,
    )?;

    Ok(ok(&resolved_url, &launcher))
}

/// Return the effective external player configuration.
///
/// This is intended for the desktop settings UI and diagnostics.
/// It does not query or depend on the Go backend.
#[tauri::command]
pub async fn get_external_player_config(app_state: State<'_, AppState>) -> Result<Value, String> {
    let desktop_player = &app_state.desktop_config.as_ref().external_player;

    let value = match configured_external_player(desktop_player) {
        Some(config) => json!({
            "mode": "configured",
            "source": config.source,
            "kind": config.kind.as_str(),
            "program": config.program,
            "args": config.args,
            "appendDefaultMpvArgs": config.append_default_mpv_args,
            "urlPlaceholder": URL_PLACEHOLDER,
        }),
        None => json!({
            "mode": "system",
            "source": "system-default-opener",
            "kind": "system",
            "program": null,
            "args": [],
            "appendDefaultMpvArgs": false,
            "urlPlaceholder": URL_PLACEHOLDER,
        }),
    };

    Ok(value)
}

/// Get the sidecar API base URL for the current session.
#[tauri::command]
pub async fn get_api_base_url(state: State<'_, AppState>) -> Result<String, String> {
    state
        .sidecar_state
        .get_api_base_url()
        .map_err(|error| error.to_string())
}
