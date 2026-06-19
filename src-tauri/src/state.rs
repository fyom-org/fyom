//! Application state.
// std::sync::atomic::{AtomicBool, Ordering};//!
use std::time::{Duration, Instant};

use crate::desktop_config::DesktopConfig;
use crate::{SIDECAR_STARTUP_TIMEOUT_SECS, SidecarStatus};

// -----------------------------------------------------------------------------
// AppState
// -----------------------------------------------------------------------------

/// Global Tauri application state.
///
/// This state is intentionally desktop-local. It may contain local paths,
/// local launcher configuration, and sidecar runtime information.
///
/// The Go backend must not own or interpret desktop-only player configuration.
#[derive(Debug)]
pub struct AppState {
    /// Runtime state for the FYOM server sidecar.
    pub sidecar_state: SidecarState,

    /// Set once shutdown begins to avoid duplicate shutdown flows.
    pub shutdown_started: AtomicBool,

    /// Set when the user explicitly intends to exit the desktop app.
    pub exit_intent: AtomicBool,

    /// Optional desktop database path discovered or selected at runtime.
    pub desktop_db_path: Mutex<Option<PathBuf>>,

    /// Local desktop configuration.
    ///
    /// This includes external player configuration such as:
    /// - FYOM external player mode
    /// - configured mpv binary
    /// - custom player arguments
    ///
    /// This is deliberately separate from the Go backend `fyom.yaml`.
    pub desktop_config: DesktopConfig,
}

impl Default for AppState {
    fn default() -> Self {
        Self {
            sidecar_state: SidecarState::default(),
            shutdown_started: AtomicBool::new(false),
            exit_intent: AtomicBool::new(false),
            desktop_db_path: Mutex::new(None),
            desktop_config: DesktopConfig::load(),
        }
    }
}

impl AppState {
    /// Create a new application state instance.
    pub fn new() -> Self {
        Self::default()
    }

    /// Mark application shutdown as started.
    ///
    /// Returns `true` if this call changed the state from not-started to
    /// started. Returns `false` if shutdown had already started.
    pub fn begin_shutdown(&self) -> bool {
        !self.shutdown_started.swap(true, Ordering::SeqCst)
    }

    /// Returns whether shutdown has already started.
    pub fn is_shutdown_started(&self) -> bool {
        self.shutdown_started.load(Ordering::SeqCst)
    }

    /// Mark that the user explicitly requested app exit.
    pub fn mark_exit_intent(&self) {
        self.exit_intent.store(true, Ordering::SeqCst);
    }

    /// Returns whether the user explicitly requested app exit.
    pub fn has_exit_intent(&self) -> bool {
        self.exit_intent.load(Ordering::SeqCst)
    }

    /// Set the desktop database path.
    pub fn set_desktop_db_path(&self, path: PathBuf) {
        match self.desktop_db_path.lock() {
            Ok(mut desktop_db_path) => {
                *desktop_db_path = Some(path);
            }
            Err(error) => {
                tracing::error!("[app/state] failed to set desktop db path: {error}");
            }
        }
    }

    /// Get the desktop database path.
    pub fn get_desktop_db_path(&self) -> Option<PathBuf> {
        match self.desktop_db_path.lock() {
            Ok(desktop_db_path) => desktop_db_path.clone(),
            Err(error) => {
                tracing::error!("[app/state] failed to get desktop db path: {error}");
                None
            }
        }
    }

    /// Clear the desktop database path.
    pub fn clear_desktop_db_path(&self) {
        match self.desktop_db_path.lock() {
            Ok(mut desktop_db_path) => {
                *desktop_db_path = None;
            }
            Err(error) => {
                tracing::error!("[app/state] failed to clear desktop db path: {error}");
            }
        }
    }
}

// -----------------------------------------------------------------------------
// Sidecar state
// -----------------------------------------------------------------------------

#[derive(Debug)]
pub struct SidecarState {
    status: Mutex<SidecarStatus>,
    api_base_url: Mutex<Option<String>>,
    child_pid: Mutex<Option<u32>>,
    startup_deadline: Mutex<Option<Instant>>,
    ready_received: AtomicBool,
}

impl Default for SidecarState {
    fn default() -> Self {
        Self {
            status: Mutex::new(SidecarStatus::Stopped),
            api_base_url: Mutex::new(None),
            child_pid: Mutex::new(None),
            startup_deadline: Mutex::new(None),
            ready_received: AtomicBool::new(false),
        }
    }
}

impl SidecarState {
    // -------------------------------------------------------------------------
    // Read model
    // -------------------------------------------------------------------------

    pub fn get_status(&self) -> SidecarStatus {
        match self.status.lock() {
            Ok(status) => status.clone(),
            Err(error) => {
                tracing::error!("[sidecar/state] status mutex poisoned: {error}");

                SidecarStatus::Error {
                    message: "sidecar status unavailable".to_string(),
                }
            }
        }
    }

    pub fn get_api_base_url(&self) -> Result<String, String> {
        let status = self.get_status();

        match status {
            SidecarStatus::Ready { .. } => {}
            SidecarStatus::Stopped => {
                return Err("sidecar is stopped".to_string());
            }
            SidecarStatus::Starting => {
                return Err("sidecar is still starting".to_string());
            }
            SidecarStatus::Error { message } => {
                return Err(format!("sidecar is in error state: {message}"));
            }
        }

        let url = self
            .api_base_url
            .lock()
            .map_err(|error| format!("sidecar api_base_url mutex poisoned: {error}"))?;

        url.clone()
            .ok_or_else(|| "sidecar is ready but api_base_url is missing".to_string())
    }

    pub fn get_child_pid(&self) -> Option<u32> {
        match self.child_pid.lock() {
            Ok(child_pid) => *child_pid,
            Err(error) => {
                tracing::error!("[sidecar/state] child_pid mutex poisoned: {error}");
                None
            }
        }
    }

    pub fn is_ready(&self) -> bool {
        matches!(self.get_status(), SidecarStatus::Ready { .. })
            && self.ready_received.load(Ordering::SeqCst)
    }

    pub fn is_starting(&self) -> bool {
        matches!(self.get_status(), SidecarStatus::Starting)
    }

    pub fn is_stopped(&self) -> bool {
        matches!(self.get_status(), SidecarStatus::Stopped)
    }

    pub fn is_error(&self) -> bool {
        matches!(self.get_status(), SidecarStatus::Error { .. })
    }

    pub fn is_startup_timeout(&self) -> bool {
        let deadline = match self.startup_deadline.lock() {
            Ok(deadline) => *deadline,
            Err(error) => {
                tracing::error!("[sidecar/state] startup_deadline mutex poisoned: {error}");
                return false;
            }
        };

        match deadline {
            Some(deadline) => {
                !self.ready_received.load(Ordering::SeqCst) && Instant::now() >= deadline
            }
            None => false,
        }
    }

    // -------------------------------------------------------------------------
    // Transitions
    // -------------------------------------------------------------------------

    pub fn set_starting(&self) {
        let deadline = Instant::now() + Duration::from_secs(SIDECAR_STARTUP_TIMEOUT_SECS);

        self.set_status(SidecarStatus::Starting);
        self.set_api_base_url(None);
        self.set_startup_deadline(Some(deadline));
        self.ready_received.store(false, Ordering::SeqCst);

        tracing::info!(
            "[sidecar/state] status=starting startup_timeout_secs={}",
            SIDECAR_STARTUP_TIMEOUT_SECS
        );
    }

    pub fn set_ready(&self, api_base_url: String) {
        let api_base_url = match normalize_api_base_url(api_base_url) {
            Ok(url) => url,
            Err(error) => {
                tracing::error!("[sidecar/state] invalid ready api_base_url: {error}");

                self.set_error(format!("invalid sidecar api_base_url: {error}"));
                return;
            }
        };

        self.set_status(SidecarStatus::Ready {
            api_base_url: api_base_url.clone(),
        });

        self.set_api_base_url(Some(api_base_url.clone()));
        self.set_startup_deadline(None);
        self.ready_received.store(true, Ordering::SeqCst);

        tracing::info!("[sidecar/state] status=ready api_base_url={api_base_url}");
    }

    pub fn set_error(&self, message: String) {
        tracing::error!("[sidecar/state] status=error message={message}");

        self.set_status(SidecarStatus::Error { message });
        self.set_api_base_url(None);
        self.set_startup_deadline(None);
        self.ready_received.store(false, Ordering::SeqCst);
    }

    pub fn set_stopped(&self) {
        self.set_status(SidecarStatus::Stopped);
        self.set_api_base_url(None);
        self.set_child_pid_inner(None);
        self.set_startup_deadline(None);
        self.ready_received.store(false, Ordering::SeqCst);

        tracing::info!("[sidecar/state] status=stopped");
    }

    pub fn set_child_pid(&self, pid: u32) {
        self.set_child_pid_inner(Some(pid));

        tracing::info!("[sidecar/state] child pid set: {pid}");
    }

    pub fn clear_child_pid(&self) {
        self.set_child_pid_inner(None);

        tracing::debug!("[sidecar/state] child pid cleared");
    }

    // -------------------------------------------------------------------------
    // Internal setters
    // -------------------------------------------------------------------------

    fn set_status(&self, next: SidecarStatus) {
        match self.status.lock() {
            Ok(mut status) => {
                *status = next;
            }
            Err(error) => {
                tracing::error!("[sidecar/state] failed to update status: {error}");
            }
        }
    }

    fn set_api_base_url(&self, next: Option<String>) {
        match self.api_base_url.lock() {
            Ok(mut api_base_url) => {
                *api_base_url = next;
            }
            Err(error) => {
                tracing::error!("[sidecar/state] failed to update api_base_url: {error}");
            }
        }
    }

    fn set_child_pid_inner(&self, next: Option<u32>) {
        match self.child_pid.lock() {
            Ok(mut child_pid) => {
                *child_pid = next;
            }
            Err(error) => {
                tracing::error!("[sidecar/state] failed to update child pid: {error}");
            }
        }
    }

    fn set_startup_deadline(&self, next: Option<Instant>) {
        match self.startup_deadline.lock() {
            Ok(mut startup_deadline) => {
                *startup_deadline = next;
            }
            Err(error) => {
                tracing::error!("[sidecar/state] failed to update startup deadline: {error}");
            }
        }
    }
}

fn normalize_api_base_url(raw: String) -> Result<String, String> {
    let url = raw.trim().trim_end_matches('/').to_string();

    if url.is_empty() {
        return Err("api_base_url must not be empty".to_string());
    }

    if !url.starts_with("http://") && !url.starts_with("https://") {
        return Err(format!(
            "api_base_url must be absolute http(s) URL, got `{url}`"
        ));
    }

    Ok(url)
}
//! This module owns desktop runtime state and sidecar runtime state.

use std::path::PathBuf;
use std::sync::Mutex;
