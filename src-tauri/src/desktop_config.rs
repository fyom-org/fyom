//! fyom Desktop local configuration.
//!
//! This module is intentionally Tauri/Desktop-only.
//! Do not load external player settings from the Go backend `fyom.yaml`.
//!
//! Player selection priority is handled by the desktop launcher:
//!
//! 1. Player environment overrides
//! 2. Desktop config file
//! 3. OS default opener
//!
//! Desktop config file resolution order:
//!
//! 1. `FYOM_DESKTOP_CONFIG` explicit override
//! 2. Platform user config path
//! 3. Development fallback `configs/fyom-desktop.json` (debug builds only)
//! 4. No config
//!
//! Important path behavior:
//!
//! - Explicit env override paths are interpreted by the operating system as-is.
//!   Relative paths therefore resolve relative to the current working directory.
//! - The debug development fallback never depends on the current working
//!   directory. It is derived from `CARGO_MANIFEST_DIR`.

use std::{
    env,
    ffi::OsString,
    fmt, fs,
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

    #[cfg(debug_assertions)]
    DevFallback,
}

impl fmt::Display for DesktopConfigSource {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::ExplicitEnv => formatter.write_str("explicit-env"),
            Self::PlatformUser => formatter.write_str("platform-user"),

            #[cfg(debug_assertions)]
            Self::DevFallback => formatter.write_str("dev-fallback"),
        }
    }
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct DesktopConfigPath {
    pub path: PathBuf,
    pub source: DesktopConfigSource,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum DesktopConfigPathError {
    ExplicitOverrideMissing {
        path: PathBuf,
        cwd: Option<PathBuf>,
    },
    ExplicitOverrideUnreadable {
        path: PathBuf,
        reason: String,
        cwd: Option<PathBuf>,
    },
    PlatformUnsupported,
}

impl fmt::Display for DesktopConfigPathError {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::ExplicitOverrideMissing { path, cwd } => {
                write!(
                    formatter,
                    "explicit desktop config override `{}` points to a missing file: {}",
                    ENV_DESKTOP_CONFIG,
                    path.display()
                )?;

                if let Some(cwd) = cwd {
                    write!(formatter, "; cwd={}", cwd.display())?;
                }

                Ok(())
            }

            Self::ExplicitOverrideUnreadable { path, reason, cwd } => {
                write!(
                    formatter,
                    "desktop config file `{}` is not readable: {}",
                    path.display(),
                    reason
                )?;

                if let Some(cwd) = cwd {
                    write!(formatter, "; cwd={}", cwd.display())?;
                }

                Ok(())
            }

            Self::PlatformUnsupported => formatter.write_str(
                "platform user config path is unsupported or required environment variables are missing",
            ),
        }
    }
}

impl std::error::Error for DesktopConfigPathError {}

pub trait EnvProvider {
    fn var_os(&self, key: &str) -> Option<OsString>;
}

pub struct ProcessEnv;

impl EnvProvider for ProcessEnv {
    fn var_os(&self, key: &str) -> Option<OsString> {
        env::var_os(key)
    }
}

/// Supported platform targets for desktop config path construction.
///
/// This enum intentionally contains variants for all supported desktop
/// platforms even when compiling on only one host OS. Tests use these variants
/// to verify deterministic cross-platform path behavior.
#[allow(dead_code)]
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

#[derive(Debug, Clone, PartialEq, Eq)]
enum ConfigFileStatus {
    File,
    Missing,
    NotFile,
    Unreadable(String),
}

trait ConfigFileProbe {
    fn status(&self, path: &Path) -> ConfigFileStatus;
}

struct ProcessConfigFileProbe;

impl ConfigFileProbe for ProcessConfigFileProbe {
    fn status(&self, path: &Path) -> ConfigFileStatus {
        match fs::metadata(path) {
            Ok(metadata) if metadata.is_file() => ConfigFileStatus::File,
            Ok(_) => ConfigFileStatus::NotFile,
            Err(error) if error.kind() == std::io::ErrorKind::NotFound => ConfigFileStatus::Missing,
            Err(error) => ConfigFileStatus::Unreadable(error.to_string()),
        }
    }
}

pub fn resolve_desktop_config_path() -> Result<Option<DesktopConfigPath>, DesktopConfigPathError> {
    resolve_desktop_config_path_with_env(&ProcessEnv, current_platform())
}

pub fn resolve_desktop_config_path_with_env<E: EnvProvider>(
    env: &E,
    platform: Platform,
) -> Result<Option<DesktopConfigPath>, DesktopConfigPathError> {
    resolve_desktop_config_path_with_env_and_probe(env, platform, &ProcessConfigFileProbe)
}

fn resolve_desktop_config_path_with_env_and_probe<E: EnvProvider, F: ConfigFileProbe>(
    env: &E,
    platform: Platform,
    files: &F,
) -> Result<Option<DesktopConfigPath>, DesktopConfigPathError> {
    // 1. Explicit env override: FYOM_DESKTOP_CONFIG.
    //
    // Empty or whitespace-only values are treated as unset.
    // Non-empty values are explicit and must not silently fall back.
    if let Some(path) = explicit_config_path_from_env(env) {
        return resolve_explicit_config_path(path, files);
    }

    // 2. Platform user config path.
    //
    // Missing platform environment variables should not crash desktop startup.
    // A missing or invalid platform config means "no platform config".
    match platform_user_config_path_with_env(env, platform) {
        Ok(platform_path) => {
            if matches!(files.status(&platform_path), ConfigFileStatus::File) {
                return Ok(Some(DesktopConfigPath {
                    path: platform_path,
                    source: DesktopConfigSource::PlatformUser,
                }));
            }
        }
        Err(DesktopConfigPathError::PlatformUnsupported) => {
            // Continue to dev fallback before giving up.
        }
        Err(error) => return Err(error),
    }

    // 3. Dev fallback (debug builds only).
    //
    // This must never rely on process current working directory.
    #[cfg(debug_assertions)]
    {
        let dev_path = dev_fallback_config_path();

        if matches!(files.status(&dev_path), ConfigFileStatus::File) {
            return Ok(Some(DesktopConfigPath {
                path: dev_path,
                source: DesktopConfigSource::DevFallback,
            }));
        }
    }

    // 4. No config found.
    Ok(None)
}

fn explicit_config_path_from_env<E: EnvProvider>(env: &E) -> Option<PathBuf> {
    non_blank_env(env, ENV_DESKTOP_CONFIG).map(PathBuf::from)
}

fn resolve_explicit_config_path<F: ConfigFileProbe>(
    path: PathBuf,
    files: &F,
) -> Result<Option<DesktopConfigPath>, DesktopConfigPathError> {
    match files.status(&path) {
        ConfigFileStatus::File => Ok(Some(DesktopConfigPath {
            path,
            source: DesktopConfigSource::ExplicitEnv,
        })),

        ConfigFileStatus::Missing => Err(DesktopConfigPathError::ExplicitOverrideMissing {
            path,
            cwd: process_cwd(),
        }),

        ConfigFileStatus::NotFile => Err(DesktopConfigPathError::ExplicitOverrideUnreadable {
            path,
            reason: "path is not a regular file".to_string(),
            cwd: process_cwd(),
        }),

        ConfigFileStatus::Unreadable(reason) => {
            Err(DesktopConfigPathError::ExplicitOverrideUnreadable {
                path,
                reason,
                cwd: process_cwd(),
            })
        }
    }
}

pub fn platform_user_config_path_with_env<E: EnvProvider>(
    env: &E,
    platform: Platform,
) -> Result<PathBuf, DesktopConfigPathError> {
    match platform {
        Platform::Windows => {
            let appdata =
                non_blank_env(env, "APPDATA").ok_or(DesktopConfigPathError::PlatformUnsupported)?;

            Ok(PathBuf::from(appdata)
                .join("fyom")
                .join("fyom-desktop.json"))
        }

        Platform::Macos => {
            let home =
                non_blank_env(env, "HOME").ok_or(DesktopConfigPathError::PlatformUnsupported)?;

            Ok(PathBuf::from(home)
                .join("Library")
                .join("Application Support")
                .join("fyom")
                .join("fyom-desktop.json"))
        }

        Platform::Linux => {
            if let Some(xdg_config_home) = non_blank_env(env, "XDG_CONFIG_HOME") {
                return Ok(PathBuf::from(xdg_config_home)
                    .join("fyom")
                    .join("fyom-desktop.json"));
            }

            let home =
                non_blank_env(env, "HOME").ok_or(DesktopConfigPathError::PlatformUnsupported)?;

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
    // CARGO_MANIFEST_DIR points to src-tauri, where this crate's Cargo.toml lives.
    // The repo-local config is at <repo>/configs/fyom-desktop.json.
    PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .parent()
        .expect("src-tauri must have a repository parent")
        .join("configs")
        .join("fyom-desktop.json")
}

fn process_cwd() -> Option<PathBuf> {
    env::current_dir().ok()
}

fn non_blank_env<E: EnvProvider>(env: &E, key: &str) -> Option<OsString> {
    env.var_os(key).filter(|value| !os_string_is_blank(value))
}

fn os_string_is_blank(value: &OsString) -> bool {
    value.to_string_lossy().trim().is_empty()
}

// ---------------------------------------------------------------------------
// Config loading
// ---------------------------------------------------------------------------

impl DesktopConfig {
    pub fn load() -> Self {
        match try_load_desktop_config() {
            Ok(Some(config)) => config,
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
    let Some(selected) = resolve_desktop_config_path().map_err(|error| error.to_string())? else {
        return Ok(None);
    };

    tracing::info!(
        "[desktop-config] selected desktop config source={} path={}",
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

    tracing::info!(
        "[desktop-config] loaded desktop config source={} path={}",
        selected.source,
        selected.path.display()
    );

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

    #[derive(Default)]
    struct FakeConfigFileProbe {
        statuses: HashMap<PathBuf, ConfigFileStatus>,
    }

    impl FakeConfigFileProbe {
        fn with_file<P: Into<PathBuf>>(mut self, path: P) -> Self {
            self.statuses.insert(path.into(), ConfigFileStatus::File);
            self
        }

        fn with_status<P: Into<PathBuf>>(mut self, path: P, status: ConfigFileStatus) -> Self {
            self.statuses.insert(path.into(), status);
            self
        }
    }

    impl ConfigFileProbe for FakeConfigFileProbe {
        fn status(&self, path: &Path) -> ConfigFileStatus {
            self.statuses
                .get(path)
                .cloned()
                .unwrap_or(ConfigFileStatus::Missing)
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
    fn windows_empty_appdata_returns_error() {
        let env = FakeEnv::default().with("APPDATA", "");

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
    fn macos_empty_home_returns_error() {
        let env = FakeEnv::default().with("HOME", "");

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

    #[test]
    fn linux_empty_home_returns_error_when_xdg_missing() {
        let env = FakeEnv::default().with("HOME", "");

        let result = platform_user_config_path_with_env(&env, Platform::Linux);

        assert!(matches!(
            result,
            Err(DesktopConfigPathError::PlatformUnsupported)
        ));
    }

    // --- Priority tests ---

    #[test]
    fn explicit_env_wins_over_platform_config() {
        let explicit_path = PathBuf::from("/custom/path/fyom-desktop.json");
        let platform_path = PathBuf::from("/home/test")
            .join(".config")
            .join("fyom")
            .join("fyom-desktop.json");

        let env = FakeEnv::default()
            .with(ENV_DESKTOP_CONFIG, explicit_path.to_string_lossy().as_ref())
            .with("HOME", "/home/test");

        let files = FakeConfigFileProbe::default()
            .with_file(explicit_path.clone())
            .with_file(platform_path);

        let result = resolve_desktop_config_path_with_env_and_probe(&env, Platform::Linux, &files)
            .expect("explicit config should resolve")
            .expect("explicit config should be selected");

        assert_eq!(result.source, DesktopConfigSource::ExplicitEnv);
        assert_eq!(result.path, explicit_path);
    }

    #[test]
    fn empty_explicit_env_is_ignored() {
        let platform_path = PathBuf::from("/home/test")
            .join(".config")
            .join("fyom")
            .join("fyom-desktop.json");

        let env = FakeEnv::default()
            .with(ENV_DESKTOP_CONFIG, "")
            .with("HOME", "/home/test");

        let files = FakeConfigFileProbe::default().with_file(platform_path.clone());

        let result = resolve_desktop_config_path_with_env_and_probe(&env, Platform::Linux, &files)
            .expect("empty explicit env should not error")
            .expect("platform config should be selected");

        assert_eq!(result.source, DesktopConfigSource::PlatformUser);
        assert_eq!(result.path, platform_path);
    }

    #[test]
    fn whitespace_explicit_env_is_ignored() {
        let platform_path = PathBuf::from("/home/test")
            .join(".config")
            .join("fyom")
            .join("fyom-desktop.json");

        let env = FakeEnv::default()
            .with(ENV_DESKTOP_CONFIG, "   ")
            .with("HOME", "/home/test");

        let files = FakeConfigFileProbe::default().with_file(platform_path.clone());

        let result = resolve_desktop_config_path_with_env_and_probe(&env, Platform::Linux, &files)
            .expect("whitespace explicit env should not error")
            .expect("platform config should be selected");

        assert_eq!(result.source, DesktopConfigSource::PlatformUser);
        assert_eq!(result.path, platform_path);
    }

    #[test]
    fn missing_explicit_override_does_not_silently_fall_back() {
        let explicit_path = PathBuf::from("/nonexistent/fyom-desktop.json");
        let platform_path = PathBuf::from("/home/test")
            .join(".config")
            .join("fyom")
            .join("fyom-desktop.json");

        let env = FakeEnv::default()
            .with(ENV_DESKTOP_CONFIG, explicit_path.to_string_lossy().as_ref())
            .with("HOME", "/home/test");

        let files = FakeConfigFileProbe::default().with_file(platform_path);

        let result = resolve_desktop_config_path_with_env_and_probe(&env, Platform::Linux, &files);

        assert!(matches!(
            result,
            Err(DesktopConfigPathError::ExplicitOverrideMissing { path, .. })
            if path == explicit_path
        ));
    }

    #[test]
    fn relative_missing_explicit_override_reports_cwd() {
        let explicit_path = PathBuf::from("configs/fyom-desktop.json");

        let env = FakeEnv::default()
            .with(ENV_DESKTOP_CONFIG, explicit_path.to_string_lossy().as_ref())
            .with("HOME", "/home/test");

        let files = FakeConfigFileProbe::default();

        let result = resolve_desktop_config_path_with_env_and_probe(&env, Platform::Linux, &files);

        let Err(error) = result else {
            panic!("relative missing explicit override should fail");
        };

        let rendered = error.to_string();

        assert!(rendered.contains("configs/fyom-desktop.json"));
        assert!(rendered.contains("cwd="));
    }

    #[test]
    fn explicit_override_directory_is_error() {
        let explicit_path = PathBuf::from("/tmp/fyom-config-dir");

        let env = FakeEnv::default()
            .with(ENV_DESKTOP_CONFIG, explicit_path.to_string_lossy().as_ref())
            .with("HOME", "/home/test");

        let files = FakeConfigFileProbe::default()
            .with_status(explicit_path.clone(), ConfigFileStatus::NotFile);

        let result = resolve_desktop_config_path_with_env_and_probe(&env, Platform::Linux, &files);

        assert!(matches!(
            result,
            Err(DesktopConfigPathError::ExplicitOverrideUnreadable { path, reason, .. })
            if path == explicit_path && reason == "path is not a regular file"
        ));
    }

    #[test]
    fn explicit_override_unreadable_is_error() {
        let explicit_path = PathBuf::from("/tmp/fyom-desktop.json");

        let env = FakeEnv::default()
            .with(ENV_DESKTOP_CONFIG, explicit_path.to_string_lossy().as_ref())
            .with("HOME", "/home/test");

        let files = FakeConfigFileProbe::default().with_status(
            explicit_path.clone(),
            ConfigFileStatus::Unreadable("permission denied".to_string()),
        );

        let result = resolve_desktop_config_path_with_env_and_probe(&env, Platform::Linux, &files);

        assert!(matches!(
            result,
            Err(DesktopConfigPathError::ExplicitOverrideUnreadable { path, reason, .. })
            if path == explicit_path && reason == "permission denied"
        ));
    }

    #[test]
    fn platform_config_is_selected_when_override_absent_and_file_exists() {
        let platform_path = PathBuf::from("/home/test")
            .join(".config")
            .join("fyom")
            .join("fyom-desktop.json");

        let env = FakeEnv::default().with("HOME", "/home/test");
        let files = FakeConfigFileProbe::default().with_file(platform_path.clone());

        let result = resolve_desktop_config_path_with_env_and_probe(&env, Platform::Linux, &files)
            .expect("platform config should resolve")
            .expect("platform config should be selected");

        assert_eq!(result.source, DesktopConfigSource::PlatformUser);
        assert_eq!(result.path, platform_path);
    }

    #[test]
    fn platform_config_missing_falls_through_to_no_config() {
        let env = FakeEnv::default().with("HOME", "/home/test");
        let files = FakeConfigFileProbe::default();

        let result = resolve_desktop_config_path_with_env_and_probe(&env, Platform::Linux, &files)
            .expect("missing config should not error");

        assert_eq!(result, None);
    }

    #[test]
    fn platform_config_directory_is_ignored() {
        let platform_path = PathBuf::from("/home/test")
            .join(".config")
            .join("fyom")
            .join("fyom-desktop.json");

        let env = FakeEnv::default().with("HOME", "/home/test");

        let files =
            FakeConfigFileProbe::default().with_status(platform_path, ConfigFileStatus::NotFile);

        let result = resolve_desktop_config_path_with_env_and_probe(&env, Platform::Linux, &files)
            .expect("invalid platform config should be ignored");

        assert_eq!(result, None);
    }

    #[test]
    fn platform_config_unreadable_is_ignored() {
        let platform_path = PathBuf::from("/home/test")
            .join(".config")
            .join("fyom")
            .join("fyom-desktop.json");

        let env = FakeEnv::default().with("HOME", "/home/test");

        let files = FakeConfigFileProbe::default().with_status(
            platform_path,
            ConfigFileStatus::Unreadable("permission denied".to_string()),
        );

        let result = resolve_desktop_config_path_with_env_and_probe(&env, Platform::Linux, &files)
            .expect("unreadable platform config should be ignored");

        assert_eq!(result, None);
    }

    #[cfg(debug_assertions)]
    #[test]
    fn dev_fallback_is_selected_in_debug_when_file_exists() {
        let env = FakeEnv::default().with("HOME", "/home/test");
        let dev_path = dev_fallback_config_path();

        let files = FakeConfigFileProbe::default().with_file(dev_path.clone());

        let result = resolve_desktop_config_path_with_env_and_probe(&env, Platform::Linux, &files)
            .expect("dev fallback should not error")
            .expect("dev fallback should be selected");

        assert_eq!(result.source, DesktopConfigSource::DevFallback);
        assert_eq!(result.path, dev_path);
    }

    #[cfg(debug_assertions)]
    #[test]
    fn platform_config_wins_over_dev_fallback() {
        let platform_path = PathBuf::from("/home/test")
            .join(".config")
            .join("fyom")
            .join("fyom-desktop.json");

        let dev_path = dev_fallback_config_path();

        let env = FakeEnv::default().with("HOME", "/home/test");

        let files = FakeConfigFileProbe::default()
            .with_file(platform_path.clone())
            .with_file(dev_path);

        let result = resolve_desktop_config_path_with_env_and_probe(&env, Platform::Linux, &files)
            .expect("config resolution should succeed")
            .expect("platform config should be selected");

        assert_eq!(result.source, DesktopConfigSource::PlatformUser);
        assert_eq!(result.path, platform_path);
    }

    #[test]
    fn unsupported_platform_falls_through_to_no_config_when_no_dev_fallback() {
        let env = FakeEnv::default();
        let files = FakeConfigFileProbe::default();

        let result =
            resolve_desktop_config_path_with_env_and_probe(&env, Platform::Unsupported, &files)
                .expect("unsupported platform should fall through to no config");

        assert_eq!(result, None);
    }

    #[test]
    fn display_error_reads_missing_error_fields() {
        let missing = DesktopConfigPathError::ExplicitOverrideMissing {
            path: PathBuf::from("/missing/fyom-desktop.json"),
            cwd: Some(PathBuf::from("/workdir")),
        };

        let rendered = missing.to_string();

        assert!(rendered.contains("/missing/fyom-desktop.json"));
        assert!(rendered.contains("/workdir"));
    }

    #[test]
    fn display_error_reads_unreadable_error_fields() {
        let unreadable = DesktopConfigPathError::ExplicitOverrideUnreadable {
            path: PathBuf::from("/unreadable/fyom-desktop.json"),
            reason: "permission denied".to_string(),
            cwd: Some(PathBuf::from("/workdir")),
        };

        let rendered = unreadable.to_string();

        assert!(rendered.contains("/unreadable/fyom-desktop.json"));
        assert!(rendered.contains("permission denied"));
        assert!(rendered.contains("/workdir"));
    }

    #[test]
    fn desktop_config_deserializes_minimal_json() {
        let config = serde_json::from_str::<DesktopConfig>(r#"{}"#)
            .expect("minimal desktop config should deserialize");

        assert_eq!(config.external_player.kind, ExternalPlayerKind::System);
        assert_eq!(config.external_player.program, "");
        assert!(config.external_player.args.is_empty());
        assert!(config.external_player.append_default_mpv_args);
    }

    #[test]
    fn desktop_config_deserializes_external_player_json() {
        let config = serde_json::from_str::<DesktopConfig>(
            r#"
            {
              "externalPlayer": {
                "kind": "mpv",
                "program": "mpv",
                "args": ["--force-window=yes"],
                "appendDefaultMpvArgs": false
              }
            }
            "#,
        )
        .expect("desktop config should deserialize");

        assert_eq!(config.external_player.kind, ExternalPlayerKind::Mpv);
        assert_eq!(config.external_player.program, "mpv");
        assert_eq!(config.external_player.args, vec!["--force-window=yes"]);
        assert!(!config.external_player.append_default_mpv_args);
    }
}
