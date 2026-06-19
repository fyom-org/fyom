//! Application state.
//!
//! This module owns:
//! - sidecar runtime state
//! - native libmpv playback state
//!
//! Important:
//! mpv must not be initialized eagerly at app startup for native `--wid` embedding.
//!
//! macOS video path requires:
//! 1. create AppKit `NSView`
//! 2. attach `CAMetalLayer`
//! 3. extract native window id (`wid`)
//! 4. initialize mpv with `wid` before `mpv_initialize()`
//!
//! Therefore `MpvState` is intentionally lazy. The playback command layer must
//! initialize it after the platform surface is available.

use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::{Mutex, OnceLock};
use std::time::{Duration, Instant};

use crate::mpv::MpvInstance;
use crate::mpv::handle::MpvStartupConfig;
use crate::{SIDECAR_STARTUP_TIMEOUT_SECS, SidecarStatus};

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

// -----------------------------------------------------------------------------
// libmpv native playback state
// -----------------------------------------------------------------------------

pub struct MpvState {
    /// Lazily initialized native playback instance.
    ///
    /// Do not initialize this at app startup. macOS `--wid` embedding requires
    /// the platform surface to exist first.
    pub instance: OnceLock<MpvInstance>,

    /// Last mpv initialization error.
    ///
    /// This is interior-mutable because initialization now happens after app state
    /// construction.
    init_error: Mutex<Option<String>>,

    /// Serializes mpv initialization attempts.
    init_lock: Mutex<()>,
}

impl std::fmt::Debug for MpvState {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        let initialized = self.instance.get().is_some();
        let init_error = self.init_error();

        f.debug_struct("MpvState")
            .field("initialized", &initialized)
            .field("init_error", &init_error)
            .finish()
    }
}

impl MpvState {
    pub fn new() -> Self {
        tracing::info!("[mpv/state] native playback state created; initialization is lazy");

        Self {
            instance: OnceLock::new(),
            init_error: Mutex::new(None),
            init_lock: Mutex::new(()),
        }
    }

    pub fn is_initialized(&self) -> bool {
        self.instance.get().is_some()
    }

    pub fn init_error(&self) -> Option<String> {
        match self.init_error.lock() {
            Ok(error) => error.clone(),
            Err(error) => {
                tracing::error!("[mpv/state] init_error mutex poisoned: {error}");
                Some("mpv init error unavailable".to_string())
            }
        }
    }

    pub fn clear_init_error(&self) {
        match self.init_error.lock() {
            Ok(mut error) => {
                *error = None;
            }
            Err(error) => {
                tracing::error!("[mpv/state] failed to clear init_error: {error}");
            }
        }
    }

    pub fn get_instance(&self) -> Option<&MpvInstance> {
        self.instance.get()
    }

    pub fn require_instance(&self) -> Result<&MpvInstance, String> {
        self.instance.get().ok_or_else(|| {
            self.init_error()
                .unwrap_or_else(|| "libmpv not initialized".to_string())
        })
    }

    /// Initialize mpv with default native output options.
    ///
    /// This is acceptable for audio-only or non-embedded paths, but macOS video
    /// embedding should prefer `initialize_with_wid`.
    pub fn initialize_default(&self) -> Result<&MpvInstance, String> {
        self.initialize_with_config(MpvStartupConfig::default_native())
    }

    /// Initialize mpv with a native window id.
    ///
    /// This is the preferred macOS video path:
    /// create platform surface -> get wid -> initialize_with_wid(wid).
    pub fn initialize_with_wid(&self, wid: String) -> Result<&MpvInstance, String> {
        if wid.trim().is_empty() {
            return Err("mpv wid must not be empty".to_string());
        }

        self.initialize_with_config(MpvStartupConfig::native_with_wid(wid))
    }

    pub fn initialize_with_config(&self, config: MpvStartupConfig) -> Result<&MpvInstance, String> {
        if let Some(instance) = self.instance.get() {
            return Ok(instance);
        }

        let _init_guard = self
            .init_lock
            .lock()
            .map_err(|error| format!("mpv init_lock mutex poisoned: {error}"))?;

        if let Some(instance) = self.instance.get() {
            return Ok(instance);
        }

        tracing::info!(
            "[mpv/state] initializing native playback; wid_configured={}",
            config.wid.is_some()
        );

        match MpvInstance::new_with_config(config) {
            Ok(mpv) => {
                if self.instance.set(mpv).is_err() {
                    let message = "failed to store MpvInstance in OnceLock".to_string();

                    self.set_init_error(Some(message.clone()));
                    tracing::error!("[mpv/state] {message}");

                    return Err(message);
                }

                self.set_init_error(None);

                tracing::info!("[mpv/state] native playback initialized");

                self.instance
                    .get()
                    .ok_or_else(|| "mpv initialized but instance is unavailable".to_string())
            }

            Err(error) => {
                let message = format!("mpv init failed: {error}");

                self.set_init_error(Some(message.clone()));

                tracing::error!("[mpv/state] native playback disabled; {message}");

                Err(message)
            }
        }
    }

    fn set_init_error(&self, next: Option<String>) {
        match self.init_error.lock() {
            Ok(mut init_error) => {
                *init_error = next;
            }
            Err(error) => {
                tracing::error!("[mpv/state] failed to update init_error: {error}");
            }
        }
    }
}

impl Default for MpvState {
    fn default() -> Self {
        Self::new()
    }
}
