//! Application state.
//!
//! This module owns:
//! - sidecar runtime state
//! - native libmpv playback state

use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::{Mutex, OnceLock};
use std::time::{Duration, Instant};

use crate::mpv::MpvInstance;
use crate::{SidecarStatus, SIDECAR_STARTUP_TIMEOUT_SECS};

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
        let url = self
            .api_base_url
            .lock()
            .map_err(|error| format!("sidecar api_base_url mutex poisoned: {error}"))?;

        url.clone()
            .ok_or_else(|| "sidecar is not ready".to_string())
    }

    pub fn set_starting(&self) {
        let deadline = Instant::now() + Duration::from_secs(SIDECAR_STARTUP_TIMEOUT_SECS);

        self.set_status(SidecarStatus::Starting);
        self.set_api_base_url(None);
        self.set_startup_deadline(Some(deadline));
        self.ready_received.store(false, Ordering::SeqCst);
    }

    pub fn set_ready(&self, api_base_url: String) {
        self.set_status(SidecarStatus::Ready {
            api_base_url: api_base_url.clone(),
        });

        self.set_api_base_url(Some(api_base_url));
        self.set_startup_deadline(None);
        self.ready_received.store(true, Ordering::SeqCst);
    }

    pub fn set_error(&self, message: String) {
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
    }

    pub fn set_child_pid(&self, pid: u32) {
        self.set_child_pid_inner(Some(pid));
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

// -----------------------------------------------------------------------------
// libmpv native playback state
// -----------------------------------------------------------------------------

pub struct MpvState {
    pub instance: OnceLock<MpvInstance>,
    pub init_error: Option<String>,
}

impl MpvState {
    pub fn new() -> Self {
        let instance = OnceLock::new();

        match MpvInstance::new() {
            Ok(mpv) => {
                if instance.set(mpv).is_err() {
                    let message = "failed to store MpvInstance in OnceLock".to_string();

                    tracing::error!("[mpv/state] {message}");

                    return Self {
                        instance: OnceLock::new(),
                        init_error: Some(message),
                    };
                }

                tracing::info!("[mpv/state] native playback initialized");

                Self {
                    instance,
                    init_error: None,
                }
            }

            Err(error) => {
                tracing::error!(
                    "[mpv/state] native playback disabled; mpv init failed: {}",
                    error
                );

                Self {
                    instance,
                    init_error: Some(error),
                }
            }
        }
    }
}

impl Default for MpvState {
    fn default() -> Self {
        Self::new()
    }
}
