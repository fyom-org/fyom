//! FYOM Desktop local configuration.
//!
//! This module is intentionally Tauri/Desktop-only.
//! Do not load external player settings from the Go backend `fyom.yaml`.
//!
//! Configuration priority is handled by the launcher:
//!
//! 1. Environment variables
//! 2. Desktop config file
//! 3. OS default opener

use std::{fs, path::PathBuf};

use serde::{Deserialize, Serialize};

const ENV_DESKTOP_CONFIG: &str = "FYOM_DESKTOP_CONFIG";

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct DesktopConfig {
    #[serde(default)]
    pub external_player: ExternalPlayerConfig,
}

impl Default for DesktopConfig {
    fn default() -> Self {
        Self {
            external_player: ExternalPlayerConfig::default(),
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ExternalPlayerConfig {
    #[serde(default)]
    pub kind: ExternalPlayerKind,

    #[serde(default)]
    pub program: String,

    #[serde(default)]
    pub args: Vec<String>,

    #[serde(default = "default_append_mpv_args")]
    pub append_default_mpv_args: bool,
}

impl Default for ExternalPlayerConfig {
    fn default() -> Self {
        Self {
            kind: ExternalPlayerKind::System,
            program: String::new(),
            args: Vec::new(),
            append_default_mpv_args: true,
        }
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum ExternalPlayerKind {
    System,
    Mpv,
    Custom,
}

impl Default for ExternalPlayerKind {
    fn default() -> Self {
        Self::System
    }
}

fn default_append_mpv_args() -> bool {
    true
}

impl DesktopConfig {
    pub fn load() -> Self {
        match try_load_desktop_config() {
            Ok(Some(config)) => {
                tracing::info!("[desktop-config] loaded desktop config");
                config
            }
            Ok(None) => {
                tracing::info!("[desktop-config] no desktop config found; using defaults");
                Self::default()
            }
            Err(error) => {
                tracing::warn!(
                    "[desktop-config] failed to load desktop config; using defaults; error={}",
                    error
                );
                Self::default()
            }
        }
    }
}

fn try_load_desktop_config() -> Result<Option<DesktopConfig>, String> {
    let Some(path) = find_desktop_config_path() else {
        return Ok(None);
    };

    let raw = fs::read_to_string(&path).map_err(|error| {
        format!(
            "failed to read desktop config `{}`: {error}",
            path.display()
        )
    })?;

    let config = serde_json::from_str::<DesktopConfig>(&raw).map_err(|error| {
        format!(
            "failed to parse desktop config `{}` as JSON: {error}",
            path.display()
        )
    })?;

    Ok(Some(config))
}

fn find_desktop_config_path() -> Option<PathBuf> {
    if let Some(path) = non_empty_env(ENV_DESKTOP_CONFIG).map(PathBuf::from) {
        return Some(path);
    }

    let candidates = [
        PathBuf::from("../configs/fyom-desktop.json"),
        PathBuf::from("configs/fyom-desktop.json"),
    ];

    candidates.into_iter().find(|path| path.exists())
}

fn non_empty_env(name: &str) -> Option<String> {
    std::env::var(name)
        .ok()
        .map(|value| value.trim().to_string())
        .filter(|value| !value.is_empty())
}
