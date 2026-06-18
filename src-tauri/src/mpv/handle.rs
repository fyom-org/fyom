//! Thread-safe libmpv facade.
//!
//! This module owns:
//! - the shared `libmpv2::Mpv` handle
//! - the mpv event thread lifecycle
//! - the mpv render thread lifecycle
//!
//! Command handlers must talk to mpv through `MpvInstance` only.

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

    /// Event thread shutdown flag.
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
            // Render API output. The real GL surface is attached later.
            init.set_property("vo", "libmpv")?;

            // UI disabled; frontend owns controls.
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
    // Core playback
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

    pub fn cycle_pause(&self) -> Result<(), String> {
        self.command("cycle", &["pause"])
    }

    pub fn seek(&self, seconds: f64) -> Result<(), String> {
        self.seek_absolute(seconds)
    }

    pub fn seek_absolute(&self, seconds: f64) -> Result<(), String> {
        if !seconds.is_finite() {
            return Err("seek_absolute seconds must be finite".to_string());
        }

        let seconds = seconds.to_string();
        self.command("seek", &[seconds.as_str(), "absolute"])
    }

    pub fn seek_relative(&self, seconds: f64) -> Result<(), String> {
        if !seconds.is_finite() {
            return Err("seek_relative seconds must be finite".to_string());
        }

        let seconds = seconds.to_string();
        self.command("seek", &[seconds.as_str(), "relative"])
    }

    pub fn mpv_keypress(&self, key: &str) -> Result<(), String> {
        if key.trim().is_empty() {
            return Err("keypress key must not be empty".to_string());
        }

        self.command("keypress", &[key])
    }

    // -------------------------------------------------------------------------
    // Subtitle/audio helpers
    // -------------------------------------------------------------------------

    pub fn sub_add(
        &self,
        path: &str,
        mode: &str,
        title: Option<&str>,
        lang: Option<&str>,
    ) -> Result<(), String> {
        if path.trim().is_empty() {
            return Err("subtitle path must not be empty".to_string());
        }

        let mut args = vec![path, mode];

        if title.is_some() || lang.is_some() {
            args.push(title.unwrap_or(""));
        }

        if let Some(lang) = lang {
            args.push(lang);
        }

        self.command("sub-add", &args)
    }

    pub fn sub_remove(&self, track_id: i64) -> Result<(), String> {
        let id = track_id.to_string();
        self.command("sub-remove", &[id.as_str()])
    }

    pub fn sub_reload(&self, track_id: i64) -> Result<(), String> {
        let id = track_id.to_string();
        self.command("sub-reload", &[id.as_str()])
    }

    pub fn audio_add(&self, path: &str, mode: &str) -> Result<(), String> {
        if path.trim().is_empty() {
            return Err("audio path must not be empty".to_string());
        }

        self.command("audio-add", &[path, mode])
    }

    pub fn audio_remove(&self, track_id: i64) -> Result<(), String> {
        let id = track_id.to_string();
        self.command("audio-remove", &[id.as_str()])
    }

    // -------------------------------------------------------------------------
    // Playback adjustments
    // -------------------------------------------------------------------------

    pub fn set_sub_delay(&self, seconds: f64) -> Result<(), String> {
        self.ensure_finite("sub-delay", seconds)?;
        self.set_property("sub-delay", seconds)
    }

    pub fn set_secondary_sub_delay(&self, seconds: f64) -> Result<(), String> {
        self.ensure_finite("secondary-sub-delay", seconds)?;
        self.set_property("secondary-sub-delay", seconds)
    }

    pub fn set_audio_delay(&self, seconds: f64) -> Result<(), String> {
        self.ensure_finite("audio-delay", seconds)?;
        self.set_property("audio-delay", seconds)
    }

    pub fn set_sub_scale(&self, scale: f64) -> Result<(), String> {
        self.ensure_finite("sub-scale", scale)?;

        if scale <= 0.0 {
            return Err("sub-scale must be greater than 0".to_string());
        }

        self.set_property("sub-scale", scale)
    }

    pub fn set_brightness(&self, value: f64) -> Result<(), String> {
        self.ensure_finite("brightness", value)?;
        self.set_property("brightness", value)
    }

    pub fn set_contrast(&self, value: f64) -> Result<(), String> {
        self.ensure_finite("contrast", value)?;
        self.set_property("contrast", value)
    }

    pub fn set_saturation(&self, value: f64) -> Result<(), String> {
        self.ensure_finite("saturation", value)?;
        self.set_property("saturation", value)
    }

    pub fn set_gamma(&self, value: f64) -> Result<(), String> {
        self.ensure_finite("gamma", value)?;
        self.set_property("gamma", value)
    }

    pub fn set_hue(&self, value: f64) -> Result<(), String> {
        self.ensure_finite("hue", value)?;
        self.set_property("hue", value)
    }

    pub fn set_chapter(&self, index: i64) -> Result<(), String> {
        self.set_property("chapter", index)
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

    pub fn set_option_string(&self, key: &str, value: &str) -> Result<(), String> {
        self.set_property(key, value)
    }

    fn ensure_finite(&self, name: &str, value: f64) -> Result<(), String> {
        if value.is_finite() {
            Ok(())
        } else {
            Err(format!("{name} must be finite"))
        }
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

        setlocale(LC_NUMERIC, c"C".as_ptr());
    }
}

// -----------------------------------------------------------------------------
// Safety
// -----------------------------------------------------------------------------

// SAFETY:
// mpv client API is documented as thread-safe for command/property access.
// Event/render contexts are confined to their own threads.
unsafe impl Send for MpvInstance {}
unsafe impl Sync for MpvInstance {}
