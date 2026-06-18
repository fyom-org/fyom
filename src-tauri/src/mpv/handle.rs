//! `MpvInstance` — a thread-safe wrapper around `libmpv2::Mpv`.
//!
//! PORTED_FROM_TSUKIMI @ v26.6.3 (`src/ui/mpv/tsukimi_mpv.rs::TsukimiMPV`)
//!
//! Adapted for fyom:
//! - GTK-specific pieces are dropped.
//! - The initializer property set and the `Arc<Mpv>` sharing pattern are retained.
//! - The event-pump thread is owned by this instance.
//! - The render thread is owned by this instance and spawned lazily after the frontend
//!   attaches a platform GL surface.
//!
//! The command layer should talk to mpv through this facade only.

use std::sync::atomic::{AtomicU32, Ordering};
use std::sync::{Arc, Mutex};
use std::thread::JoinHandle;

use libmpv2::{GetData, Mpv, SetData};
use tauri::AppHandle;
use tracing::{debug, info, warn};

use crate::mpv::event_loop::{self, ACTIVE, SHUTDOWN};
use crate::mpv::render::{self, RenderSurface, RenderThreadState};

/// Default max volume.
const MAX_VOLUME: i64 = 100;

/// Default cache size in MiB.
const DEFAULT_CACHE_MIB: u64 = 256;

/// Default cache duration in seconds.
const DEFAULT_CACHE_SECS: i64 = 10;

/// Default startup volume.
const DEFAULT_VOLUME: i64 = 80;

/// A thread-safe libmpv instance plus event/render thread lifecycle state.
///
/// `libmpv2::Mpv` wraps a raw `mpv_handle`. mpv's client API is documented as
/// thread-safe for command/property calls, so this wrapper exposes synchronous methods
/// guarded by mpv itself and coordinates only thread lifecycle locally.
pub struct MpvInstance {
    /// Shared libmpv handle.
    pub mpv: Arc<Mpv>,

    /// Event-thread state machine.
    event_thread_alive: Arc<AtomicU32>,

    /// Event-pump thread handle.
    event_handle: Mutex<Option<JoinHandle<()>>>,

    /// Render-thread state. `None` until the platform GL surface is attached.
    render_state: Mutex<Option<RenderThreadState>>,
}

impl std::fmt::Debug for MpvInstance {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("MpvInstance")
            .field(
                "event_thread_alive",
                &self.event_thread_alive.load(Ordering::SeqCst),
            )
            .field(
                "event_handle",
                &self
                    .event_handle
                    .lock()
                    .map(|g| g.is_some())
                    .unwrap_or(false),
            )
            .field(
                "render_state",
                &self
                    .render_state
                    .lock()
                    .map(|g| g.is_some())
                    .unwrap_or(false),
            )
            .finish()
    }
}

impl MpvInstance {
    /// Create and initialize a libmpv instance.
    ///
    /// The event loop is not started here. Call [`Self::spawn_event_loop`] from the Tauri
    /// setup hook after the app handle is available.
    pub fn new() -> Result<Self, String> {
        set_c_numeric_locale();

        let mpv = Mpv::with_initializer(|init| {
            // Render-API driven video output. The OpenGL render context is attached later
            // by the render thread.
            init.set_property("vo", "libmpv")?;
            init.set_property("osc", false)?;
            init.set_property("osd-level", 0)?;

            // Hardware decoding: safe auto fallback.
            init.set_property("hwdec", "auto-safe")?;

            // Cache and sync defaults.
            init.set_property("video-timing-offset", 0)?;
            init.set_property("video-sync", "audio")?;
            init.set_property("demuxer-max-bytes", format!("{}MiB", DEFAULT_CACHE_MIB))?;
            init.set_property("cache-secs", DEFAULT_CACHE_SECS)?;

            // Volume defaults.
            init.set_property("volume-max", MAX_VOLUME)?;
            init.set_property("volume", DEFAULT_VOLUME)?;

            // Input defaults. Webview key events can be forwarded through `mpv_keypress`.
            init.set_property("input-default-bindings", true)?;
            init.set_property("input-vo-keyboard", true)?;

            // Do not loop by default.
            init.set_property("loop", "no")?;

            Ok(())
        })
        .map_err(|e| format!("failed to create libmpv instance: {}", e))?;

        info!("[mpv] instance created and initialized");

        Ok(Self {
            mpv: Arc::new(mpv),
            event_thread_alive: Arc::new(AtomicU32::new(ACTIVE)),
            event_handle: Mutex::new(None),
            render_state: Mutex::new(None),
        })
    }

    // -----------------------------------------------------------------------
    // Event-thread lifecycle.
    // -----------------------------------------------------------------------

    /// Spawn the mpv event-pump thread.
    ///
    /// Idempotent: if already running, this is a no-op.
    pub fn spawn_event_loop(&self, app_handle: AppHandle) {
        let mut guard = match self.event_handle.lock() {
            Ok(guard) => guard,
            Err(e) => {
                warn!(
                    "[mpv] event_handle mutex poisoned; cannot spawn event loop: {}",
                    e
                );
                return;
            }
        };

        if guard.is_some() {
            debug!("[mpv] spawn_event_loop ignored; event loop already running");
            return;
        }

        self.event_thread_alive.store(ACTIVE, Ordering::SeqCst);

        let handle = event_loop::spawn_event_loop(
            Arc::clone(&self.mpv),
            app_handle,
            Arc::clone(&self.event_thread_alive),
        );

        *guard = Some(handle);

        info!("[mpv] event loop thread spawned");
    }

    /// Signal the event-pump thread to shut down and join it.
    ///
    /// Idempotent: if no event thread is running, this is a no-op.
    pub fn shutdown_event_loop(&self) {
        self.event_thread_alive.store(SHUTDOWN, Ordering::SeqCst);

        let handle = match self.event_handle.lock() {
            Ok(mut guard) => guard.take(),
            Err(e) => {
                warn!(
                    "[mpv] event_handle mutex poisoned during shutdown; skipping join: {}",
                    e
                );
                None
            }
        };

        if let Some(handle) = handle {
            if handle.join().is_err() {
                warn!("[mpv] event loop thread panicked during join");
            } else {
                info!("[mpv] event loop thread joined");
            }
        }
    }

    // -----------------------------------------------------------------------
    // Render-thread lifecycle.
    // -----------------------------------------------------------------------

    /// Spawn the mpv render thread.
    ///
    /// The platform GL surface is moved into the render thread and must not be used
    /// elsewhere after this call.
    ///
    /// Idempotent: if the render thread is already running, this returns `Ok(())`.
    pub fn spawn_render_thread(&self, surface: Box<dyn RenderSurface>) -> Result<(), String> {
        let mut guard = self
            .render_state
            .lock()
            .map_err(|e| format!("render_state mutex poisoned: {}", e))?;

        if guard.is_some() {
            warn!("[mpv] spawn_render_thread ignored; render thread is already running");
            return Ok(());
        }

        let state = RenderThreadState::new();

        let handle = render::spawn_render_thread(
            Arc::clone(&self.mpv),
            surface,
            Arc::clone(&state.shutdown),
        )?;

        {
            let mut handle_guard = state
                .handle
                .lock()
                .map_err(|e| format!("render handle mutex poisoned: {}", e))?;
            *handle_guard = Some(handle);
        }

        *guard = Some(state);

        info!("[mpv] render thread spawned");

        Ok(())
    }

    /// Signal the render thread to shut down and join it.
    ///
    /// This should be called before the mpv instance is dropped so libmpv's render context
    /// is destroyed while the underlying mpv handle is still valid.
    ///
    /// Idempotent: if no render thread is running, this is a no-op.
    pub fn shutdown_render_thread(&self) {
        let state = match self.render_state.lock() {
            Ok(mut guard) => guard.take(),
            Err(e) => {
                warn!(
                    "[mpv] render_state mutex poisoned during shutdown; skipping render join: {}",
                    e
                );
                None
            }
        };

        let Some(state) = state else {
            return;
        };

        state.shutdown.store(SHUTDOWN, Ordering::SeqCst);

        // Wake the render loop so it can observe the shutdown flag even if mpv is not
        // producing frames.
        render::wake_render_thread();

        let handle = match state.handle.lock() {
            Ok(mut guard) => guard.take(),
            Err(e) => {
                warn!(
                    "[mpv] render handle mutex poisoned during shutdown; skipping join: {}",
                    e
                );
                None
            }
        };

        if let Some(handle) = handle {
            if handle.join().is_err() {
                warn!("[mpv] render thread panicked during join");
            } else {
                info!("[mpv] render thread joined");
            }
        }
    }

    // -----------------------------------------------------------------------
    // Core command/property facade.
    // -----------------------------------------------------------------------

    /// Load a media URL or local path, replacing the current playlist entry.
    pub fn loadfile(&self, url: &str) -> Result<(), String> {
        info!("[mpv] loadfile: {}", url);

        self.mpv
            .command("loadfile", &[url, "replace"])
            .map_err(|e| format!("mpv loadfile failed: {}", e))
    }

    /// Stop playback and clear the playlist.
    pub fn stop(&self) -> Result<(), String> {
        info!("[mpv] stop");

        self.mpv
            .command("stop", &[])
            .map_err(|e| format!("mpv stop failed: {}", e))
    }

    /// Explicitly pause or resume playback.
    pub fn set_pause(&self, paused: bool) -> Result<(), String> {
        self.set_property("pause", paused)
    }

    /// Toggle pause.
    pub fn cycle_pause(&self) -> Result<(), String> {
        self.mpv
            .command("cycle", &["pause"])
            .map_err(|e| format!("mpv cycle pause failed: {}", e))
    }

    /// Seek by a relative offset in seconds.
    pub fn seek_relative(&self, seconds: f64) -> Result<(), String> {
        let seconds = seconds.to_string();

        self.mpv
            .command("seek", &[seconds.as_str(), "relative"])
            .map_err(|e| format!("mpv seek relative failed: {}", e))
    }

    /// Seek to an absolute position in seconds.
    pub fn seek_absolute(&self, position: f64) -> Result<(), String> {
        let position = position.to_string();

        self.mpv
            .command("seek", &[position.as_str(), "absolute"])
            .map_err(|e| format!("mpv seek absolute failed: {}", e))
    }

    /// Forward an mpv key string, for example `Space`, `Ctrl+Right`, or `Volume+`.
    pub fn mpv_keypress(&self, keystr: &str) -> Result<(), String> {
        self.mpv
            .command("keypress", &[keystr])
            .map_err(|e| format!("mpv keypress failed: {}", e))
    }

    /// Generic mpv command passthrough.
    pub fn command(&self, cmd: &str, args: &[&str]) -> Result<(), String> {
        self.mpv
            .command(cmd, args)
            .map_err(|e| format!("mpv command '{}' failed: {}", cmd, e))
    }

    /// Set an mpv property.
    pub fn set_property<V>(&self, property: &str, value: V) -> Result<(), String>
    where
        V: SetData,
    {
        self.mpv
            .set_property(property, value)
            .map_err(|e| format!("mpv set_property '{}' failed: {}", property, e))
    }

    /// Get an mpv property.
    pub fn get_property<V>(&self, property: &str) -> Result<V, String>
    where
        V: GetData,
    {
        self.mpv
            .get_property(property)
            .map_err(|e| format!("mpv get_property '{}' failed: {}", property, e))
    }

    /// Set an mpv option/property using a string value.
    ///
    /// This is used by power-user command surfaces. Prefer typed setters where possible.
    pub fn set_option_string(&self, name: &str, value: &str) -> Result<(), String> {
        self.mpv
            .set_property(name, value)
            .map_err(|e| format!("mpv set_option_string '{}'='{}' failed: {}", name, value, e))
    }

    // -----------------------------------------------------------------------
    // Subtitle/audio track management.
    // -----------------------------------------------------------------------

    /// Add an external subtitle file.
    ///
    /// mpv command shape:
    /// `sub-add <url> [<flags> [<title> [<lang>]]]`
    pub fn sub_add(
        &self,
        path: &str,
        mode: &str,
        title: Option<&str>,
        lang: Option<&str>,
    ) -> Result<(), String> {
        let mut args: Vec<&str> = Vec::with_capacity(4);
        args.push(path);
        args.push(mode);

        if title.is_some() || lang.is_some() {
            args.push(title.unwrap_or(""));
        }

        if let Some(lang) = lang {
            args.push(lang);
        }

        self.mpv
            .command("sub-add", &args)
            .map_err(|e| format!("mpv sub-add failed: {}", e))
    }

    /// Remove a subtitle track by id.
    pub fn sub_remove(&self, track_id: i64) -> Result<(), String> {
        let id = track_id.to_string();

        self.mpv
            .command("sub-remove", &[id.as_str()])
            .map_err(|e| format!("mpv sub-remove failed: {}", e))
    }

    /// Reload a subtitle track by id.
    pub fn sub_reload(&self, track_id: i64) -> Result<(), String> {
        let id = track_id.to_string();

        self.mpv
            .command("sub-reload", &[id.as_str()])
            .map_err(|e| format!("mpv sub-reload failed: {}", e))
    }

    /// Add an external audio track.
    pub fn audio_add(&self, path: &str, mode: &str) -> Result<(), String> {
        self.mpv
            .command("audio-add", &[path, mode])
            .map_err(|e| format!("mpv audio-add failed: {}", e))
    }

    /// Remove an audio track by id.
    pub fn audio_remove(&self, track_id: i64) -> Result<(), String> {
        let id = track_id.to_string();

        self.mpv
            .command("audio-remove", &[id.as_str()])
            .map_err(|e| format!("mpv audio-remove failed: {}", e))
    }

    // -----------------------------------------------------------------------
    // Playback adjustments.
    // -----------------------------------------------------------------------

    /// Set subtitle delay in seconds.
    pub fn set_sub_delay(&self, seconds: f64) -> Result<(), String> {
        self.set_property("sub-delay", seconds)
    }

    /// Set secondary subtitle delay in seconds.
    pub fn set_secondary_sub_delay(&self, seconds: f64) -> Result<(), String> {
        self.set_property("secondary-sub-delay", seconds)
    }

    /// Set audio delay in seconds.
    pub fn set_audio_delay(&self, seconds: f64) -> Result<(), String> {
        self.set_property("audio-delay", seconds)
    }

    /// Set subtitle scale.
    pub fn set_sub_scale(&self, scale: f64) -> Result<(), String> {
        self.set_property("sub-scale", scale)
    }

    /// Set brightness.
    pub fn set_brightness(&self, value: f64) -> Result<(), String> {
        self.set_property("brightness", value)
    }

    /// Set contrast.
    pub fn set_contrast(&self, value: f64) -> Result<(), String> {
        self.set_property("contrast", value)
    }

    /// Set saturation.
    pub fn set_saturation(&self, value: f64) -> Result<(), String> {
        self.set_property("saturation", value)
    }

    /// Set gamma.
    pub fn set_gamma(&self, value: f64) -> Result<(), String> {
        self.set_property("gamma", value)
    }

    /// Set hue.
    pub fn set_hue(&self, value: f64) -> Result<(), String> {
        self.set_property("hue", value)
    }

    /// Navigate to a chapter by index.
    pub fn set_chapter(&self, index: i64) -> Result<(), String> {
        self.set_property("chapter", index)
    }
}

impl Drop for MpvInstance {
    fn drop(&mut self) {
        // Best-effort cleanup. Normal app shutdown should call these explicitly from the
        // Tauri exit path, but this prevents leaking threads in test/dev paths.
        self.shutdown_render_thread();
        self.shutdown_event_loop();
    }
}

impl Default for MpvInstance {
    fn default() -> Self {
        Self::new().expect("failed to create default MpvInstance")
    }
}

/// Set C numeric locale for mpv numeric parsing.
///
/// mpv expects a C numeric locale so decimal values use `.` consistently.
fn set_c_numeric_locale() {
    // SAFETY: `setlocale` is process-global. This mirrors common mpv integration
    // practice and should run once before creating the mpv handle.
    unsafe {
        use libc::{LC_NUMERIC, setlocale};

        let c_locale = b"C\0";
        setlocale(LC_NUMERIC, c_locale.as_ptr() as *const _);
    }
}

// SAFETY: `libmpv2::Mpv` wraps an `mpv_handle`. mpv's client API is documented as
// thread-safe for command/property access. Event and render contexts are confined to
// their dedicated threads.
unsafe impl Send for MpvInstance {}
unsafe impl Sync for MpvInstance {}
