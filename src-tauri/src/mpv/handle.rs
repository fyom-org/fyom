//! Thread-safe libmpv facade.
//!
//! Command handlers must talk to mpv through `MpvInstance` only.
//!
//! This module owns:
//! - the shared `libmpv2::Mpv` handle
//! - the mpv event thread lifecycle
//! - the native video surface lifecycle holder
//!
//! Current rendering architecture:
//! - FYOM does not use `mpv_render_context` for production playback.
//! - mpv owns the GPU context.
//! - macOS uses `NSView` + `CAMetalLayer` + `wid` embedding.
//!
//! Important:
//! `wid` should be configured before the first video output is created.
//! The safest path is to configure it before `mpv_initialize()`. With the current
//! `libmpv2::Mpv::with_initializer` flow, this requires the platform surface to be
//! created before `MpvInstance::new_with_wid(...)`.
//!
//! For the transitional app flow, `configure_embedded_wid(...)` exists so callers
//! can set `wid` immediately after creating the surface and before `loadfile`.

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

const DEFAULT_MPV_LOG_FILE: &str = "/tmp/mpv-fyom.log";

// -----------------------------------------------------------------------------
// Internal thread state
// -----------------------------------------------------------------------------

struct RenderThread {
    handle: JoinHandle<()>,
    shutdown: Arc<AtomicU32>,
}

// -----------------------------------------------------------------------------
// Mpv startup config
// -----------------------------------------------------------------------------

#[derive(Debug, Clone, Default)]
pub struct MpvStartupConfig {
    /// Native window id passed to mpv `wid`.
    ///
    /// macOS expects a decimal `NSView` pointer string.
    pub wid: Option<String>,

    /// Whether to force the native Vulkan path.
    pub force_vulkan: bool,

    /// Optional mpv log file.
    pub log_file: Option<String>,
}

impl MpvStartupConfig {
    pub fn default_native() -> Self {
        Self {
            wid: None,
            force_vulkan: true,
            log_file: Some(DEFAULT_MPV_LOG_FILE.to_string()),
        }
    }

    pub fn native_with_wid(wid: String) -> Self {
        Self {
            wid: Some(wid),
            force_vulkan: true,
            log_file: Some(DEFAULT_MPV_LOG_FILE.to_string()),
        }
    }
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

    /// Native surface lifecycle thread handle + shutdown flag.
    ///
    /// This is intentionally not an mpv render-context thread.
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
            .field("surface_lifecycle_thread_alive", &render_thread_alive)
            .finish()
    }
}

impl MpvInstance {
    // -------------------------------------------------------------------------
    // Init
    // -------------------------------------------------------------------------

    pub fn new() -> Result<Self, String> {
        Self::new_with_config(MpvStartupConfig::default_native())
    }

    pub fn new_with_wid(wid: String) -> Result<Self, String> {
        Self::new_with_config(MpvStartupConfig::native_with_wid(wid))
    }

    pub fn new_with_config(config: MpvStartupConfig) -> Result<Self, String> {
        set_c_numeric_locale();

        let mpv = Mpv::with_initializer(|init| {
            // Native mpv video output.
            //
            // Do not use `vo=libmpv` here. `vo=libmpv` requires mpv_render_context,
            // which FYOM intentionally does not use for the current architecture.
            init.set_property("vo", "gpu")?;

            if let Some(wid) = config.wid.as_deref() {
                init.set_property("wid", wid)?;
            }

            if config.force_vulkan {
                init.set_property("gpu-api", "vulkan")?;

                #[cfg(target_os = "macos")]
                {
                    init.set_property("gpu-context", "macvk")?;
                }
            }

            if let Some(log_file) = config.log_file.as_deref() {
                init.set_property("log-file", log_file)?;
                init.set_property("msg-level", "all=v")?;
            }

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

        info!(
            "[mpv] instance created; render_mode=wid; vo=gpu; vulkan={}",
            config.force_vulkan
        );

        if let Some(wid) = config.wid.as_deref() {
            info!("[mpv] startup wid configured: {wid}");
        } else {
            warn!(
                "[mpv] startup wid not configured; caller must set wid before first loadfile for embedded video"
            );
        }

        Ok(Self {
            mpv: Arc::new(mpv),
            event_alive: Arc::new(AtomicU32::new(ACTIVE)),
            event_thread: Mutex::new(None),
            render_thread: Mutex::new(None),
        })
    }

    // -------------------------------------------------------------------------
    // Native embedding configuration
    // -------------------------------------------------------------------------

    /// Configure the native window id used by mpv video output.
    ///
    /// macOS:
    /// - value must be a decimal `NSView` pointer
    /// - target view must have a `CAMetalLayer`
    ///
    /// This should be called before the first `loadfile`.
    /// Prefer `new_with_wid(...)` when the surface is available before mpv init.
    pub fn configure_embedded_wid(&self, wid: &str) -> Result<(), String> {
        if wid.trim().is_empty() {
            return Err("mpv wid must not be empty".to_string());
        }

        info!("[mpv] configure embedded wid: {wid}");

        self.set_property("wid", wid)
    }

    /// Configure native GPU output options after initialization.
    ///
    /// This is a transitional helper. The preferred path is setting these options
    /// inside `new_with_config(...)` before mpv initialization.
    pub fn configure_native_video_output(&self) -> Result<(), String> {
        self.set_property("vo", "gpu")?;
        self.set_property("gpu-api", "vulkan")?;

        #[cfg(target_os = "macos")]
        {
            self.set_property("gpu-context", "macvk")?;
        }

        self.set_property("log-file", DEFAULT_MPV_LOG_FILE)?;
        self.set_property("msg-level", "all=v")?;

        Ok(())
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

        let handle =
            event_loop::spawn_event_loop(Arc::clone(&self.mpv), app, Arc::clone(&self.event_alive));

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
    // Native surface lifecycle
    // -------------------------------------------------------------------------

    pub fn spawn_render_thread(&self, surface: Box<dyn RenderSurface>) -> Result<(), String> {
        let mut guard = self
            .render_thread
            .lock()
            .map_err(|error| format!("render_thread mutex poisoned: {error}"))?;

        if guard.is_some() {
            debug!("[mpv] native surface lifecycle thread already running");
            return Ok(());
        }

        let shutdown = Arc::new(AtomicU32::new(0));

        let handle =
            render::spawn_render_thread(Arc::clone(&self.mpv), surface, Arc::clone(&shutdown))?;

        *guard = Some(RenderThread { handle, shutdown });

        info!("[mpv] native surface lifecycle thread started");

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
            warn!("[mpv] native surface lifecycle thread panicked during join");
        } else {
            info!("[mpv] native surface lifecycle thread joined");
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
        use libc::{LC_NUMERIC, setlocale};

        let c_locale = b"C\0";
        setlocale(LC_NUMERIC, c_locale.as_ptr() as *const _);
    }
}

// -----------------------------------------------------------------------------
// Safety
// -----------------------------------------------------------------------------

// SAFETY:
// mpv client API is documented as thread-safe for command/property access.
// Event loops are confined to dedicated threads.
// The native surface lifecycle thread only retains the platform surface and does
// not mutate AppKit directly.
unsafe impl Send for MpvInstance {}
unsafe impl Sync for MpvInstance {}
