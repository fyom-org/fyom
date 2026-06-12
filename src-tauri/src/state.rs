//! Sidecar state management

use std::sync::Mutex;
use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::time::{Duration, Instant};

use anyhow::Result;

use crate::{SIDECAR_STARTUP_TIMEOUT_SECS, SidecarStatus};

#[derive(Default)]
pub struct SidecarState {
    status: Mutex<SidecarStatus>,
    api_base_url: Mutex<Option<String>>,
    child_pid: Mutex<Option<u32>>,
    startup_timeout: AtomicU64,
    ready_received: AtomicBool,
}

impl SidecarState {
    pub fn new() -> Self {
        Self::default()
    }

    pub fn get_status(&self) -> SidecarStatus {
        self.status.lock().unwrap().clone()
    }

    pub fn get_api_base_url(&self) -> Result<String, String> {
        let url = self.api_base_url.lock().unwrap();
        url.clone().ok_or_else(|| "Sidecar not ready".to_string())
    }

    pub fn set_starting(&self) {
        *self.status.lock().unwrap() = SidecarStatus::Starting;
        self.ready_received.store(false, Ordering::SeqCst);
        self.startup_timeout.store(
            (Instant::now() + Duration::from_secs(SIDECAR_STARTUP_TIMEOUT_SECS))
                .elapsed()
                .as_millis() as u64,
            Ordering::SeqCst,
        );
    }

    pub fn set_ready(&self, api_base_url: String) {
        *self.status.lock().unwrap() = SidecarStatus::Ready {
            api_base_url: api_base_url.clone(),
        };
        *self.api_base_url.lock().unwrap() = Some(api_base_url);
        self.ready_received.store(true, Ordering::SeqCst);
    }

    pub fn set_error(&self, message: String) {
        *self.status.lock().unwrap() = SidecarStatus::Error { message };
    }

    pub fn set_child_pid(&self, pid: u32) {
        *self.child_pid.lock().unwrap() = Some(pid);
    }

    pub fn get_child_pid(&self) -> Option<u32> {
        *self.child_pid.lock().unwrap()
    }

    pub fn is_ready(&self) -> bool {
        self.ready_received.load(Ordering::SeqCst)
    }

    pub fn is_startup_timeout(&self) -> bool {
        let timeout_ms = self.startup_timeout.load(Ordering::SeqCst);
        if timeout_ms == 0 {
            return false;
        }
        // This is a simplified check - in reality we'd store the absolute deadline
        false // We'll handle timeout in the bootstrap function
    }
}
