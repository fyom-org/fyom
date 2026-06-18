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
    pub fn new() -> Self {
        Self::default()
    }

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

        if let Ok(mut status) = self.status.lock() {
            *status = SidecarStatus::Starting;
        } else {
            tracing::error!("[sidecar/state] failed to set status=Starting");
        }

        if let Ok(mut url) = self.api_base_url.lock() {
            *url = None;
        } else {
            tracing::error!("[sidecar/state] failed to clear api_base_url");
        }

        if let Ok(mut startup_deadline) = self.startup_deadline.lock() {
            *startup_deadline = Some(deadline);
        } else {
            tracing::error!("[sidecar/state] failed to set startup deadline");
        }

        self.ready_received.store(false, Ordering::SeqCst);
    }

    pub fn set_ready(&self, api_base_url: String) {
        if let Ok(mut status) = self.status.lock() {
            *status = SidecarStatus::Ready {
                api_base_url: api_base_url.clone(),
            };
        } else {
            tracing::error!("[sidecar/state] failed to set status=Ready");
        }

        if let Ok(mut url) = self.api_base_url.lock() {
            *url = Some(api_base_url);
        } else {
            tracing::error!("[sidecar/state] failed to set api_base_url");
        }

        if let Ok(mut startup_deadline) = self.startup_deadline.lock() {
            *startup_deadline = None;
        } else {
            tracing::error!("[sidecar/state] failed to clear startup deadline");
        }

        self.ready_received.store(true, Ordering::SeqCst);
    }

    pub fn set_error(&self, message: String) {
        if let Ok(mut status) = self.status.lock() {
            *status = SidecarStatus::Error { message };
        } else {
            tracing::error!("[sidecar/state] failed to set status=Error");
        }

        if let Ok(mut url) = self.api_base_url.lock() {
            *url = None;
        } else {
            tracing::error!("[sidecar/state] failed to clear api_base_url after error");
        }

        if let Ok(mut startup_deadline) = self.startup_deadline.lock() {
            *startup_deadline = None;
        } else {
            tracing::error!("[sidecar/state] failed to clear startup deadline after error");
        }

        self.ready_received.store(false, Ordering::SeqCst);
    }

    pub fn set_stopped(&self) {
        if let Ok(mut status) = self.status.lock() {
            *status = SidecarStatus::Stopped;
        } else {
            tracing::error!("[sidecar/state] failed to set status=Stopped");
        }

        if let Ok(mut url) = self.api_base_url.lock() {
            *url = None;
        } else {
            tracing::error!("[sidecar/state] failed to clear api_base_url after stop");
        }

        if let Ok(mut pid) = self.child_pid.lock() {
            *pid = None;
        } else {
            tracing::error!("[sidecar/state] failed to clear child pid after stop");
        }

        if let Ok(mut startup_deadline) = self.startup_deadline.lock() {
            *startup_deadline = None;
        } else {
            tracing::error!("[sidecar/state] failed to clear startup deadline after stop");
        }

        self.ready_received.store(false, Ordering::SeqCst);
    }

    pub fn set_child_pid(&self, pid: u32) {
        if let Ok(mut child_pid) = self.child_pid.lock() {
            *child_pid = Some(pid);
        } else {
            tracing::error!("[sidecar/state] failed to set child pid");
        }
    }

    pub fn clear_child_pid(&self) {
        if let Ok(mut child_pid) = self.child_pid.lock() {
            *child_pid = None;
        } else {
            tracing::error!("[sidecar/state] failed to clear child pid");
        }
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
        self.ready_received.load(Ordering::SeqCst)
    }

    pub fn is_starting(&self) -> bool {
        matches!(self.get_status(), SidecarStatus::Starting)
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
            Some(deadline) => !self.is_ready() && Instant::now() >= deadline,
            None => false,
        }
    }

    pub fn reset(&self) {
        self.set_stopped();
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

    pub fn is_ready(&self) -> bool {
        self.instance.get().is_some()
    }

    pub fn get_instance(&self) -> Result<&MpvInstance, String> {
        self.instance
            .get()
            .ok_or_else(|| {
                self.init_error
                    .clone()
                    .unwrap_or_else(|| "libmpv not initialized".to_string())
            })
    }
}

impl Default for MpvState {
    fn default() -> Self {
        Self::new()
    }
}
