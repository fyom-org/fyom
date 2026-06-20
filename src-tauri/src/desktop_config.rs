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
//!
//! Desktop config file resolution order:
//!
//! 1. `FYOM_DESKTOP_CONFIG` explicit override
//! 2. Platform user config path
//! 3. Development fallback `configs/fyom-desktop.json` (debug builds only)
//! 4. No config

use std::{
    env,
    ffi::OsString,
    fs,
    path::{Path, PathBuf},
};

use serde::{Deserialize, Serialize};

const ENV_DESKTOP_CONFIG: &str = "FYOM_DESKTOP_CONFIG";

// ---------------------------------------------------------------------------
// Config file schema
// ---------------------------------------------------------------------------

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct DesktopConfig {
    #[serde(default)]
    pub external_player: ExternalPlayerConfig,
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
            append_default_mpv_args: default_append_mpv_args(),
        }
    }
}

#[derive(Debug, Clone, Copy, Default, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum ExternalPlayerKind {
    #[default]
    System,
    Mpv,
    Custom,
}

fn default_append_mpv_args() -> bool {
    true
}

// ---------------------------------------------------------------------------
// Config path resolution
// ---------------------------------------------------------------------------

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum DesktopConfigSource {
    ExplicitEnv,
    PlatformUser,
    DevFallback,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct DesktopConfigPath {
    pub path: PathBuf,
    pub source: DesktopConfigSource,
}

#[derive(Debug)]
pub enum DesktopConfigPathError {
    ExplicitOverrideMissing(PathBuf),
    ExplicitOverrideUnreadable { path: PathBuf, reason: String },
    PlatformUnsupported,
}

pub trait EnvProvider {
    fn var_os(&self, key: &str) -> Option<OsString>;
}

pub struct ProcessEnv;

impl EnvProvider for ProcessEnv {
    fn var_os(&self, key: &str) -> Option<OsString> {
        env::var_os(key)
    }
}

#[derive(Debug, Copy, Clone, PartialEq, Eq)]
pub enum Platform {
    Windows,
    Macos,
    Linux,
    Unsupported,
}

pub fn current_platform() -> Platform {
    #[cfg(target_os = "windows")]
    {
        return Platform::Windows;
    }

    #[cfg(target_os = "macos")]
    {
        return Platform::Macos;
    }

    #[cfg(target_os = "linux")]
    {
        return Platform::Linux;
    }

    #[allow(unreachable_code)]
    Platform::Unsupported
}

pub fn resolve_desktop_config_path() -> Result<Option<DesktopConfigPath>, DesktopConfigPathError> {
    resolve_desktop_config_path_with_env(&ProcessEnv, current_platform())
}

pub fn resolve_desktop_config_path_with_env<E: EnvProvider>(
    env: &E,
    platform: Platform,
) -> Result<Option<DesktopConfigPath>, DesktopConfigPathError> {
    // 1. Explicit env override: FYOM_DESKTOP_CONFIG
    if let Some(value) = env.var_os(ENV_DESKTOP_CONFIG) {
        let value = value.to_string_lossy().trim().to_string();

        if value.is_empty() {
            // Empty string: treat as unset, continue to platform paths.
        } else {
            let path = PathBuf::from(&value);

            return match path_exists_and_readable(&path) {
                Ok(true) => Ok(Some(DesktopConfigPath {
                    path,
                    source: DesktopConfigSource::ExplicitEnv,
                })),
                Ok(false) => Err(DesktopConfigPathError::ExplicitOverrideMissing(path)),
                Err(err) => Err(err),
            };
        }
    }

    // 2. Platform user config path
    match platform_user_config_path_with_env(env, platform) {
        Ok(platform_path) => {
            if platform_path.is_file() {
                return Ok(Some(DesktopConfigPath {
                    path: platform_path,
                    source: DesktopConfigSource::PlatformUser,
                }));
            }
        }
        Err(DesktopConfigPathError::PlatformUnsupported) => {
            // Continue to dev fallback before giving up.
        }
        Err(err) => return Err(err),
    }

    // 3. Dev fallback (debug builds only)
    #[cfg(debug_assertions)]
    {
        let dev_path = dev_fallback_config_path();
        if dev_path.is_file() {
            return Ok(Some(DesktopConfigPath {
                path: dev_path,
                source: DesktopConfigSource::DevFallback,
            }));
        }
    }

    // 4. No config found
    Ok(None)
}

pub fn platform_user_config_path_with_env<E: EnvProvider>(
    env: &E,
    platform: Platform,
) -> Result<PathBuf, DesktopConfigPathError> {
    match platform {
        Platform::Windows => {
            let appdata = env
                .var_os("APPDATA")
                .ok_or(DesktopConfigPathError::PlatformUnsupported)?;
            Ok(PathBuf::from(appdata)
                .join("fyom")
                .join("fyom-desktop.json"))
        }

        Platform::Macos => {
            let home = env
                .var_os("HOME")
                .ok_or(DesktopConfigPathError::PlatformUnsupported)?;
            Ok(PathBuf::from(home)
                .join("Library")
                .join("Application Support")
                .join("fyom")
                .join("fyom-desktop.json"))
        }

        Platform::Linux => {
            // Prefer XDG_CONFIG_HOME, fall back to $HOME/.config
            if let Some(xdg) = env
                .var_os("XDG_CONFIG_HOME")
                .filter(|v| !v.to_string_lossy().trim().is_empty())
            {
                return Ok(PathBuf::from(xdg).join("fyom").join("fyom-desktop.json"));
            }

            let home = env
                .var_os("HOME")
                .ok_or(DesktopConfigPathError::PlatformUnsupported)?;

            Ok(PathBuf::from(home)
                .join(".config")
                .join("fyom")
                .join("fyom-desktop.json"))
        }

        Platform::Unsupported => Err(DesktopConfigPathError::PlatformUnsupported),
    }
}

#[cfg(debug_assertions)]
pub fn dev_fallback_config_path() -> PathBuf {
    // CARGO_MANIFEST_DIR points to src-tauri (where this crate's Cargo.toml lives).
    // The repo-local config is at <repo>/configs/fyom-desktop.json.
    PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .parent()
        .expect("src-tauri must have a repository parent")
        .join("configs")
        .join("fyom-desktop.json")
}

pub fn path_exists_and_readable(path: &Path) -> Result<bool, DesktopConfigPathError> {
    match fs::metadata(path) {
        Ok(metadata) => {
            if metadata.is_file() {
                Ok(true)
            } else {
                Err(DesktopConfigPathError::ExplicitOverrideUnreadable {
                    path: path.to_path_buf(),
                    reason: "path is not a regular file".to_string(),
                })
            }
        }
        Err(err) if err.kind() == std::io::ErrorKind::NotFound => Ok(false),
        Err(err) => Err(DesktopConfigPathError::ExplicitOverrideUnreadable {
            path: path.to_path_buf(),
            reason: err.to_string(),
        }),
    }
}

// ---------------------------------------------------------------------------
// Config loading
// ---------------------------------------------------------------------------

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
    let Some(selected) = resolve_desktop_config_path().map_err(|err| format!("{err:?}"))? else {
        return Ok(None);
    };

    tracing::info!(
        "[desktop-config] selected desktop config source={:?} path={}",
        selected.source,
        selected.path.display()
    );

    let raw = fs::read_to_string(&selected.path).map_err(|error| {
        format!(
            "failed to read desktop config `{}`: {error}",
            selected.path.display()
        )
    })?;

    let config = serde_json::from_str::<DesktopConfig>(&raw).map_err(|error| {
        format!(
            "failed to parse desktop config `{}` as JSON: {error}",
            selected.path.display()
        )
    })?;

    Ok(Some(config))
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

#[cfg(test)]
mod tests {
    use super::*;
    use std::{collections::HashMap, ffi::OsString};

    #[derive(Default)]
    struct FakeEnv {
        vars: HashMap<String, OsString>,
    }

    impl FakeEnv {
        fn with(mut self, key: &str, value: &str) -> Self {
            self.vars.insert(key.to_string(), OsString::from(value));
            self
        }
    }

    impl EnvProvider for FakeEnv {
        fn var_os(&self, key: &str) -> Option<OsString> {
            self.vars.get(key).cloned()
        }
    }

    // --- Platform path tests ---

    #[test]
    fn windows_platform_config_path_uses_appdata() {
        let env = FakeEnv::default().with("APPDATA", r"C:\Users\test\AppData\Roaming");

        let path = platform_user_config_path_with_env(&env, Platform::Windows)
            .expect("windows path should resolve");

        assert_eq!(
            path,
            PathBuf::from(r"C:\Users\test\AppData\Roaming")
                .join("fyom")
                .join("fyom-desktop.json")
        );
    }

    #[test]
    fn macos_platform_config_path_uses_application_support() {
        let env = FakeEnv::default().with("HOME", "/Users/test");

        let path = platform_user_config_path_with_env(&env, Platform::Macos)
            .expect("macos path should resolve");

        assert_eq!(
            path,
            PathBuf::from("/Users/test")
                .join("Library")
                .join("Application Support")
                .join("fyom")
                .join("fyom-desktop.json")
        );
    }

    #[test]
    fn linux_platform_config_path_uses_xdg_config_home() {
        let env = FakeEnv::default().with("XDG_CONFIG_HOME", "/home/test/.xdg-config");

        let path = platform_user_config_path_with_env(&env, Platform::Linux)
            .expect("linux path should resolve");

        assert_eq!(
            path,
            PathBuf::from("/home/test/.xdg-config")
                .join("fyom")
                .join("fyom-desktop.json")
        );
    }

    #[test]
    fn linux_platform_config_path_falls_back_to_home_dot_config() {
        let env = FakeEnv::default().with("HOME", "/home/test");

        let path = platform_user_config_path_with_env(&env, Platform::Linux)
            .expect("linux path should resolve");

        assert_eq!(
            path,
            PathBuf::from("/home/test")
                .join(".config")
                .join("fyom")
                .join("fyom-desktop.json")
        );
    }

    #[test]
    fn unsupported_platform_returns_error() {
        let env = FakeEnv::default();

        let result = platform_user_config_path_with_env(&env, Platform::Unsupported);

        assert!(matches!(
            result,
            Err(DesktopConfigPathError::PlatformUnsupported)
        ));
    }

    #[test]
    fn windows_missing_appdata_returns_error() {
        let env = FakeEnv::default();

        let result = platform_user_config_path_with_env(&env, Platform::Windows);

        assert!(matches!(
            result,
            Err(DesktopConfigPathError::PlatformUnsupported)
        ));
    }

    #[test]
    fn macos_missing_home_returns_error() {
        let env = FakeEnv::default();

        let result = platform_user_config_path_with_env(&env, Platform::Macos);

        assert!(matches!(
            result,
            Err(DesktopConfigPathError::PlatformUnsupported)
        ));
    }

    #[test]
    fn linux_missing_all_returns_error() {
        let env = FakeEnv::default();

        let result = platform_user_config_path_with_env(&env, Platform::Linux);

        assert!(matches!(
            result,
            Err(DesktopConfigPathError::PlatformUnsupported)
        ));
    }

    // --- Priority tests ---

    #[test]
    fn explicit_env_wins_over_platform_config() {
        let env = FakeEnv::default()
            .with("FYOM_DESKTOP_CONFIG", "/custom/path/fyom-desktop.json")
            .with("HOME", "/home/test");

        // We can't test file existence without real files, so test that
        // the explicit env var is checked first by verifying it returns
        // an error for a missing explicit path (not falling back to platform).
        let result = resolve_desktop_config_path_with_env(&env, Platform::Linux);

        assert!(matches!(
            result,
            Err(DesktopConfigPathError::ExplicitOverrideMissing(p))
            if p == PathBuf::from("/custom/path/fyom-desktop.json")
        ));
    }

    #[test]
    fn empty_explicit_env_is_ignored() {
        let env = FakeEnv::default()
            .with("FYOM_DESKTOP_CONFIG", "")
            .with("HOME", "/home/test");

        // Empty FYOM_DESKTOP_CONFIG should be treated as unset.
        // Platform path won't exist on disk, so falls through to dev fallback.
        let result = resolve_desktop_config_path_with_env(&env, Platform::Linux)
            .expect("should not error with empty env override");

        // Dev fallback may exist in this repo (configs/fyom-desktop.json).
        // If it does, it should be returned as DevFallback, not silently skipped.
        if let Some(ref selected) = result {
            assert_eq!(selected.source, DesktopConfigSource::DevFallback);
        }
    }

    #[test]
    fn missing_explicit_override_does_not_silently_fall_back() {
        let env = FakeEnv::default()
            .with("FYOM_DESKTOP_CONFIG", "/nonexistent/fyom-desktop.json")
            .with("HOME", "/home/test");

        // Explicit override that doesn't exist should return an error,
        // NOT silently fall back to platform config.
        let result = resolve_desktop_config_path_with_env(&env, Platform::Linux);

        assert!(matches!(
            result,
            Err(DesktopConfigPathError::ExplicitOverrideMissing(p))
            if p == PathBuf::from("/nonexistent/fyom-desktop.json")
        ));
    }

    #[test]
    fn no_config_returns_none_when_override_absent() {
        let env = FakeEnv::default().with("HOME", "/home/test");

        // No FYOM_DESKTOP_CONFIG, platform path doesn't exist on disk.
        let result =
            resolve_desktop_config_path_with_env(&env, Platform::Linux).expect("should not error");

        // Dev fallback may or may not exist depending on the build environment.
        // In CI it likely won't exist, so result is None.
        // We just verify no error occurred; the exact result depends on filesystem.
        // If dev fallback exists, it returns DevFallback; otherwise None.
        if let Some(ref selected) = result {
            assert_eq!(selected.source, DesktopConfigSource::DevFallback);
        }
    }

    #[test]
    fn explicit_env_whitespace_only_is_ignored() {
        let env = FakeEnv::default()
            .with("FYOM_DESKTOP_CONFIG", "   ")
            .with("HOME", "/home/test");

        // Whitespace-only should be treated as empty/unset.
        let result = resolve_desktop_config_path_with_env(&env, Platform::Linux)
            .expect("should not error with whitespace-only env override");

        // Same as empty: falls through to platform, then dev fallback.
        if let Some(ref selected) = result {
            assert_eq!(selected.source, DesktopConfigSource::DevFallback);
        }
    }

    #[test]
    fn linux_empty_xdg_config_home_falls_back_to_home() {
        let env = FakeEnv::default()
            .with("XDG_CONFIG_HOME", "")
            .with("HOME", "/home/test");

        let path = platform_user_config_path_with_env(&env, Platform::Linux)
            .expect("linux path should resolve via HOME fallback");

        assert_eq!(
            path,
            PathBuf::from("/home/test")
                .join(".config")
                .join("fyom")
                .join("fyom-desktop.json")
        );
    }

    #[test]
    fn linux_whitespace_xdg_config_home_falls_back_to_home() {
        let env = FakeEnv::default()
            .with("XDG_CONFIG_HOME", "  ")
            .with("HOME", "/home/test");

        let path = platform_user_config_path_with_env(&env, Platform::Linux)
            .expect("linux path should resolve via HOME fallback");

        assert_eq!(
            path,
            PathBuf::from("/home/test")
                .join(".config")
                .join("fyom")
                .join("fyom-desktop.json")
        );
    }
}
