//! Thread-safe libmpv facade.//! Thread
//!
//! Command handlers must talk to mpv through `MpvInstance` only.
//!
//! This module owns:
//! - the shared `libmpv2::Mpv` handle
//! - the mpv event thread lifecycle


use std::sync::atomic::{AtomicU32, Ordering};
use std::sync::{Arc, Mutex};
use std::thread::JoinHandle;

use libmpv2::{GetData, Mpv, SetData};
use tauri::AppHandle;
use tracing::{debug, error, info, warn};

use crate::mpv::event_loop::{self, ACTIVE, SHUTDOWN};
use crate::mpv::render::{self, RenderSurface};

// -----------------------------------------------------------------------------
// Defaults
// -----------------------------------------------------------------------------

const MAX_VOLUME: i64 = 100;
const DEFAULT_VOLUME: i64 = 80;
const DEFAULT_CACHE_MIB: u64 = 256;
const DEFAULT_CACHE_SECS: i64 = 10;

// -----------------------------------------------------------------------------
// Internal thread state
// -----------------------------------------------------------------------------

struct RenderThread {
    handle: JoinHandle<()>,
    shutdown: Arc<AtomicU32>,
}

// -----------------------------------------------------------------------------
// MpvInstance
// -----------------------------------------------------------------------------

pub struct MpvInstance {
    /// Shared libmpv handle.
    pub mpv: Arc<Mpv>,

    /// Event thread state flag.
    event_alive: Arc<AtomicU32>,

    /// Event thread handle.
    event_thread: Mutex<Option<JoinHandle<()>>>,

    /// Render thread handle + shutdown flag.
    render_thread: Mutex<Option<RenderThread>>,
}

impl std::fmt::Debug for MpvInstance {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        let event_thread_alive = self
            .event_thread
            .lock()
            .map(|guard| guard.is_some())
            .unwrap_or(false);

        let render_thread_alive = self
            .render_thread
            .lock()
            .map(|guard| guard.is_some())
            .unwrap_or(false);

        f.debug_struct("MpvInstance")
            .field("event_thread_alive", &event_thread_alive)
            .field("render_thread_alive", &render_thread_alive)
            .finish()
    }
}

impl MpvInstance {
    // -------------------------------------------------------------------------
    // Init
    // -------------------------------------------------------------------------

    pub fn new() -> Result<Self, String> {
        set_c_numeric_locale();

        let mpv = Mpv::with_initializer(|init| {
            // Render API output. The real GL surface is attached later by render.rs.
            init.set_property("vo", "libmpv")?;

            // Frontend owns UI and controls.
            init.set_property("osc", false)?;
            init.set_property("osd-level", 0)?;

            // Conservative hardware decode.
            init.set_property("hwdec", "auto-safe")?;

            // Playback/cache defaults.
            init.set_property("video-sync", "audio")?;
            init.set_property("demuxer-max-bytes", format!("{}MiB", DEFAULT_CACHE_MIB))?;
            init.set_property("cache-secs", DEFAULT_CACHE_SECS)?;

            // Audio defaults.
            init.set_property("volume-max", MAX_VOLUME)?;
            init.set_property("volume", DEFAULT_VOLUME)?;

            // Input defaults.
            init.set_property("input-default-bindings", true)?;
            init.set_property("input-vo-keyboard", true)?;

            // No implicit loop.
            init.set_property("loop", "no")?;

            Ok(())
        })
        .map_err(|error| format!("mpv init failed: {error}"))?;

        info!("[mpv] instance created");

        Ok(Self {
            mpv: Arc::new(mpv),
            event_alive: Arc::new(AtomicU32::new(ACTIVE)),
            event_thread: Mutex::new(None),
            render_thread: Mutex::new(None),
        })
    }

    // -------------------------------------------------------------------------
    // Event thread lifecycle
    // -------------------------------------------------------------------------

    pub fn spawn_event_loop(&self, app: AppHandle) {
        let mut guard = match self.event_thread.lock() {
            Ok(guard) => guard,
            Err(error) => {
                error!("[mpv] event_thread mutex poisoned: {error}");
                return;
            }
        };

        if guard.is_some() {
            debug!("[mpv] event loop already running");
            return;
        }

        self.event_alive.store(ACTIVE, Ordering::SeqCst);

        let handle = event_loop::spawn_event_loop(
            Arc::clone(&self.mpv),
            app,
            Arc::clone(&self.event_alive),
        );

        *guard = Some(handle);

        info!("[mpv] event loop started");
    }

    pub fn shutdown_event_loop(&self) {
        self.event_alive.store(SHUTDOWN, Ordering::SeqCst);

        let handle = match self.event_thread.lock() {
            Ok(mut guard) => guard.take(),
            Err(error) => {
                warn!("[mpv] event_thread mutex poisoned during shutdown: {error}");
                None
            }
        };

        let Some(handle) = handle else {
            return;
        };

        if handle.join().is_err() {
            warn!("[mpv] event thread panicked during join");
        } else {
            info!("[mpv] event thread joined");
        }
    }

    // -------------------------------------------------------------------------
    // Render thread lifecycle
    // -------------------------------------------------------------------------

    pub fn spawn_render_thread(&self, surface: Box<dyn RenderSurface>) -> Result<(), String> {
        let mut guard = self
            .render_thread
            .lock()
            .map_err(|error| format!("render_thread mutex poisoned: {error}"))?;

        if guard.is_some() {
            debug!("[mpv] render thread already running");
            return Ok(());
        }

        let shutdown = Arc::new(AtomicU32::new(0));

        let handle = render::spawn_render_thread(
            Arc::clone(&self.mpv),
            surface,
            Arc::clone(&shutdown),
        )?;

        *guard = Some(RenderThread { handle, shutdown });

        info!("[mpv] render thread started");

        Ok(())
    }

    pub fn shutdown_render_thread(&self) {
        let render_thread = match self.render_thread.lock() {
            Ok(mut guard) => guard.take(),
            Err(error) => {
                warn!("[mpv] render_thread mutex poisoned during shutdown: {error}");
                None
            }
        };

        let Some(render_thread) = render_thread else {
            return;
        };

        render_thread.shutdown.store(SHUTDOWN, Ordering::SeqCst);

        if render_thread.handle.join().is_err() {
            warn!("[mpv] render thread panicked during join");
        } else {
            info!("[mpv] render thread joined");
        }
    }

    // -------------------------------------------------------------------------
    // Core playback facade
    // -------------------------------------------------------------------------

    pub fn loadfile(&self, url: &str) -> Result<(), String> {
        if url.trim().is_empty() {
            return Err("loadfile url must not be empty".to_string());
        }

        info!("[mpv] loadfile: {url}");

        self.command("loadfile", &[url, "replace"])
    }

    pub fn stop(&self) -> Result<(), String> {
        info!("[mpv] stop");

        self.command("stop", &[])
    }

    pub fn set_pause(&self, paused: bool) -> Result<(), String> {
        self.set_property("pause", paused)
    }

    // -------------------------------------------------------------------------
    // Generic mpv facade
    // -------------------------------------------------------------------------

    pub fn command(&self, cmd: &str, args: &[&str]) -> Result<(), String> {
        if cmd.trim().is_empty() {
            return Err("mpv command must not be empty".to_string());
        }

        self.mpv
            .command(cmd, args)
            .map_err(|error| format!("mpv command `{cmd}` failed: {error}"))
    }

    pub fn set_property<V>(&self, key: &str, value: V) -> Result<(), String>
    where
        V: SetData,
    {
        if key.trim().is_empty() {
            return Err("mpv property name must not be empty".to_string());
        }

        self.mpv
            .set_property(key, value)
            .map_err(|error| format!("mpv set_property `{key}` failed: {error}"))
    }

    pub fn get_property<V>(&self, key: &str) -> Result<V, String>
    where
        V: GetData,
    {
        if key.trim().is_empty() {
            return Err("mpv property name must not be empty".to_string());
        }

        self.mpv
            .get_property(key)
            .map_err(|error| format!("mpv get_property `{key}` failed: {error}"))
    }
}

// -----------------------------------------------------------------------------
// Drop
// -----------------------------------------------------------------------------

impl Drop for MpvInstance {
    fn drop(&mut self) {
        self.shutdown_render_thread();
        self.shutdown_event_loop();
    }
}

impl Default for MpvInstance {
    fn default() -> Self {
        Self::new().expect("mpv init failed")
    }
}

// -----------------------------------------------------------------------------
// Locale
// -----------------------------------------------------------------------------

fn set_c_numeric_locale() {
    // SAFETY:
    // mpv expects LC_NUMERIC=C so decimal parsing is stable.
    unsafe {
        use libc::{setlocale, LC_NUMERIC};

        let c_locale = b"C\0";
        setlocale(LC_NUMERIC, c_locale.as_ptr() as *const _);
    }
}

// -----------------------------------------------------------------------------
// Safety
// -----------------------------------------------------------------------------

// SAFETY:
// mpv client API is documented as thread-safe for command/property access.
// Event/render contexts are confined to dedicated threads.
unsafe impl Send for MpvInstance {}
unsafe impl Sync for MpvInstance {}
