//! `MpvInstance` — a thread-safe wrapper around `libmpv2::Mpv`.
//!
//! PORTED_FROM_TSUKIMI @ v26.6.3 (`src/ui/mpv/tsukimi_mpv.rs::TsukimiMPV`)
//!
//! Adapted for fyom:
//! - The GTK-specific bits (epoxy loader, `press_key`/`KEYSTRING_MAP`, `SETTINGS`
//!   accessors) are dropped.
//! - The initializer property set + the `Arc<Mpv>` + `unsafe impl Send/Sync` pattern
//!   are retained.
//! - The event-pump thread is owned by the instance (tsukimi stores it in `RefCell` —
//!   fyom uses `Mutex<Option<JoinHandle>>` for interior mutability without `RefCell`'s
//!   runtime borrow checks, since the instance is shared across Tauri command threads).
//! - Phase 2.3: the render thread is owned by the instance (`Mutex<Option<RenderThreadState>>`),
//!   spawned lazily via `spawn_render_thread` (called from the `attach_render_surface`
//!   Tauri command after the frontend has a window handle) + joined on app exit via
//!   `shutdown_render_thread` (called before `shutdown_event_loop` so the render context
//!   is destroyed before the mpv instance). The render context itself is created on the
//!   render thread (sole GL consumer) — see `mpv/render.rs`.
//!
//! See `docs/libmpv-assessment.md` §3.2 + §3.3 + §3.4 for the rationale.

use std::sync::Arc;
use std::sync::atomic::{AtomicU32, Ordering};
use std::sync::Mutex;
use std::thread::JoinHandle;

use libmpv2::{GetData, Mpv, SetData};
use tauri::AppHandle;
use tracing::{info, warn};

use crate::mpv::event_loop::{self, ACTIVE, SHUTDOWN};
use crate::mpv::render::{self, RenderSurface, RenderThreadState};

/// Default max volume (matches tsukimi's `MAX_VOLUME`).
const MAX_VOLUME: i64 = 100;

/// Default cache size (MiB) + cache duration (seconds). Tunable via settings in a
/// later phase; hardcoded for the spike.
const DEFAULT_CACHE_MIB: u64 = 256;
const DEFAULT_CACHE_SECS: i64 = 10;
const DEFAULT_VOLUME: i64 = 80;

/// A thread-safe libmpv instance + its event-pump thread handle.
///
/// `libmpv2::Mpv` holds a raw `mpv_handle` pointer. mpv's core API (`mpv_command`,
/// `mpv_set_property`, `mpv_get_property`) is thread-safe, so wrapping in `Arc` +
/// declaring `Send + Sync` is sound — this matches tsukimi's proven pattern.
///
/// The event-pump thread is spawned lazily via [`MpvInstance::spawn_event_loop`] (called
/// from the Tauri `setup` hook) and joined on app exit via
/// [`MpvInstance::shutdown_event_loop`].
pub struct MpvInstance {
    /// The libmpv instance. `Arc` so the event-pump thread can hold a clone.
    pub mpv: Arc<Mpv>,
    /// Event-thread state machine (`ACTIVE` / `PAUSED` / `SHUTDOWN`).
    event_thread_alive: Arc<AtomicU32>,
    /// The event-pump thread handle (`Some` after `spawn_event_loop`, cleared on
    /// `shutdown_event_loop`).
    event_handle: Mutex<Option<JoinHandle<()>>>,
    /// Phase 2.3: render-thread state (handle + shutdown signal). `None` if the render
    /// thread hasn't been spawned (e.g. `attach_render_surface` not yet called by the
    /// frontend, or platform-surface creation failed).
    render_state: Mutex<Option<RenderThreadState>>,
}

impl std::fmt::Debug for MpvInstance {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("MpvInstance").finish()
    }
}

impl MpvInstance {
    /// Create + initialize a libmpv instance.
    ///
    /// The initializer sets the core playback properties (ported from tsukimi's
    /// `TsukimiMPV::default`): `vo=libmpv` (render-API driven, used once the render
    /// context is wired in Phase 2.3), `hwdec=auto-safe`, cache, volume, …
    ///
    /// The event-pump thread is **not** started here — call
    /// [`MpvInstance::spawn_event_loop`] after this to begin observing properties +
    /// emitting `fyom://mpv/*` events.
    pub fn new() -> Result<Self, String> {
        // mpv requires C locale for numeric parsing (decimal point).
        // SAFETY: setlocale is process-global; calling once at mpv init is the
        // established pattern (tsukimi does the same in TsukimiMPV::default).
        unsafe {
            use libc::{LC_NUMERIC, setlocale};
            let c_str = b"C\0";
            setlocale(LC_NUMERIC, c_str.as_ptr() as *const _);
        }

        let mpv = Mpv::with_initializer(|init| {
            // Render-API driven video output. Phase 2.0/2.2 has no render context yet,
            // so video is a black frame + audio plays — exactly the PoC exit criterion.
            // Phase 2.3 wires `mpv_render_context_create(OpenGL)` which drives frames.
            init.set_property("vo", "libmpv")?;
            init.set_property("osc", false)?;
            init.set_property("osd-level", 0)?;

            // Hardware decoding: auto-safe (falls back to software on failure).
            init.set_property("hwdec", "auto-safe")?;

            // Cache + timing (ported from tsukimi).
            init.set_property("video-timing-offset", 0)?;
            init.set_property("video-sync", "audio")?;
            init.set_property(
                "demuxer-max-bytes",
                format!("{}MiB", DEFAULT_CACHE_MIB),
            )?;
            init.set_property("cache-secs", DEFAULT_CACHE_SECS)?;
            init.set_property("volume-max", MAX_VOLUME)?;
            init.set_property("volume", DEFAULT_VOLUME)?;

            // Input: let mpv handle its own default key bindings (forwarded from the
            // webview in Phase 2.2 via the `mpv_keypress` command).
            init.set_property("input-default-bindings", true)?;
            init.set_property("input-vo-keyboard", true)?;

            // Don't loop by default.
            init.set_property("loop", "no")?;

            Ok(())
        })
        .map_err(|e| format!("failed to create libmpv instance: {}", e))?;

        info!("[mpv] instance created + initialized (vo=libmpv, hwdec=auto-safe)");

        Ok(Self {
            mpv: Arc::new(mpv),
            event_thread_alive: Arc::new(AtomicU32::new(ACTIVE)),
            event_handle: Mutex::new(None),
            render_state: Mutex::new(None),
        })
    }

    /// Spawn the event-pump thread (ports tsukimi's `process_events`).
    ///
    /// Registers the 10 `observe_property` observers + spawns the dedicated
    /// `fyom mpv event loop` thread that loops `wait_event`, decodes events, and emits
    /// `fyom://mpv/*` to the frontend via `AppHandle::emit`. Idempotent: a second call
    /// is a no-op (the thread is already running).
    ///
    /// Called once from the Tauri `setup` hook (see `lib.rs`).
    pub fn spawn_event_loop(&self, app_handle: AppHandle) {
        let mut guard = self
            .event_handle
            .lock()
            .expect("mpv event_handle mutex poisoned");
        if guard.is_some() {
            // Already spawned.
            return;
        }
        let handle =
            event_loop::spawn_event_loop(Arc::clone(&self.mpv), app_handle, self.event_thread_alive.clone());
        *guard = Some(handle);
        info!("[mpv] event loop thread spawned");
    }

    /// Signal the event-pump thread to shut down + join it.
    ///
    /// Called on app exit (see `lib.rs` `RunEvent::Exit`). Idempotent: a second call is
    /// a no-op. Blocks until the event thread has exited (at most one `wait_event`
    /// timeout tick — ~1s).
    pub fn shutdown_event_loop(&self) {
        self.event_thread_alive.store(SHUTDOWN, Ordering::SeqCst);
        // `atomic_wait::wake` would unblock a PAUSED thread immediately; an ACTIVE
        // thread unblocks on the next `wait_event` return (≤1s). Either way, store
        // SHUTDOWN + join.
        let mut guard = self
            .event_handle
            .lock()
            .expect("mpv event_handle mutex poisoned");
        if let Some(handle) = guard.take() {
            let _ = handle.join();
            info!("[mpv] event loop thread joined");
        }
    }

    // -----------------------------------------------------------------------
    // Phase 2.3: render-thread lifecycle.
    // -----------------------------------------------------------------------

    /// Spawn the mpv render thread (ports tsukimi's `mpvglarea.rs` pattern, drop the GTK
    /// `GLArea` shell — fyom owns its own per-platform GL surface).
    ///
    /// Takes a `Box<dyn RenderSurface>` (the platform GL surface created by
    /// `crate::platform::create_platform_surface`). The surface is moved into the render
    /// thread, which is the sole consumer of the GL context.
    ///
    /// Idempotent: a second call is a no-op (the render thread is already running).
    ///
    /// # Errors
    /// Returns `Err` if the OS fails to spawn the thread. `RenderContext::new` failures
    /// are logged inside the thread + the thread exits cleanly (the 9.7 `<video>` fallback
    /// stays green; mpv plays audio with a black frame).
    ///
    /// Called from the `attach_render_surface` Tauri command (invoked by the frontend
    /// after `play_media` succeeds).
    pub fn spawn_render_thread(&self, surface: Box<dyn RenderSurface>) -> Result<(), String> {
        let mut guard = self
            .render_state
            .lock()
            .map_err(|e| format!("render_state mutex poisoned: {}", e))?;
        if guard.is_some() {
            // Already spawned.
            warn!("[mpv] spawn_render_thread called twice — ignoring (render thread already running)");
            return Ok(());
        }
        let state = RenderThreadState::new();
        let handle = render::spawn_render_thread(
            Arc::clone(&self.mpv),
            surface,
            state.shutdown.clone(),
        )?;
        *state.handle.lock().map_err(|e| format!("render handle mutex poisoned: {}", e))? = Some(handle);
        *guard = Some(state);
        info!("[mpv] render thread spawned");
        Ok(())
    }

    /// Signal the render thread to shut down + join it. Called on app exit (after
    /// `shutdown_event_loop` — the render thread must be joined before the event thread
    /// so mpv's render context is destroyed before the mpv instance).
    ///
    /// Idempotent: a second call is a no-op.
    pub fn shutdown_render_thread(&self) {
        let mut guard = match self.render_state.lock() {
            Ok(g) => g,
            Err(_) => {
                warn!("[mpv] render_state mutex poisoned during shutdown — skipping render thread join");
                return;
            }
        };
        let Some(state) = guard.take() else {
            return;
        };
        state.shutdown.store(SHUTDOWN, Ordering::SeqCst);
        // Wake the render thread so it observes the shutdown signal (otherwise it would
        // block on `RENDER_UPDATE.rx.recv()` until the next mpv frame, which never comes
        // after mpv stops).
        render::wake_render_thread();
        let mut handle_guard = match state.handle.lock() {
            Ok(g) => g,
            Err(_) => return,
        };
        if let Some(handle) = handle_guard.take() {
            let _ = handle.join();
            info!("[mpv] render thread joined");
        }
    }

    // -----------------------------------------------------------------------
    // Command / property facade (thin wrappers over `libmpv2::Mpv`).
    // -----------------------------------------------------------------------

    /// Load a media URL (replaces the current file). Honors the Phase 9.7 contract:
    /// `invoke('play_media', { mediaUrl, posterUrl? })`.
    pub fn loadfile(&self, url: &str) -> Result<(), String> {
        info!("[mpv] loadfile: {}", url);
        self.mpv
            .command("loadfile", &[url, "replace"])
            .map_err(|e| format!("mpv loadfile failed: {}", e))
    }

    /// Stop playback + clear the playlist.
    pub fn stop(&self) -> Result<(), String> {
        info!("[mpv] stop");
        self.mpv
            .command("stop", &[])
            .map_err(|e| format!("mpv stop failed: {}", e))
    }

    /// Pause / resume.
    pub fn set_pause(&self, paused: bool) -> Result<(), String> {
        self.set_property("pause", paused)
    }

    /// Cycle the `pause` property (toggle play/pause).
    pub fn cycle_pause(&self) -> Result<(), String> {
        self.mpv
            .command("cycle", &["pause"])
            .map_err(|e| format!("mpv cycle pause failed: {}", e))
    }

    /// Seek by a relative offset (seconds).
    pub fn seek_relative(&self, seconds: f64) -> Result<(), String> {
        self.mpv
            .command("seek", &[&seconds.to_string(), "relative"])
            .map_err(|e| format!("mpv seek failed: {}", e))
    }

    /// Seek to an absolute position (seconds).
    pub fn seek_absolute(&self, position: f64) -> Result<(), String> {
        self.mpv
            .command("seek", &[&position.to_string(), "absolute"])
            .map_err(|e| format!("mpv seek absolute failed: {}", e))
    }

    /// Forward a keypress to mpv (the keystr format mpv accepts, e.g. "Space",
    /// "Ctrl+Right", "Volume+"). Ported from tsukimi's `press_key` (the GTK keyval
    /// translation is dropped — the frontend assembles the keystr).
    pub fn mpv_keypress(&self, keystr: &str) -> Result<(), String> {
        self.mpv
            .command("keypress", &[keystr])
            .map_err(|e| format!("mpv keypress failed: {}", e))
    }

    /// Generic mpv command (passthrough — for power-user commands not covered by the
    /// typed facade). Ported from soia's `mpv_run_command` surface.
    pub fn command(&self, cmd: &str, args: &[&str]) -> Result<(), String> {
        self.mpv
            .command(cmd, args)
            .map_err(|e| format!("mpv command '{}' failed: {}", cmd, e))
    }

    /// Set a property (synchronous — mpv's `mpv_set_property` is a fast, thread-safe C
    /// call). tsukimi spawns this off the UI thread; for fyom's Tauri command surface a
    /// synchronous call is fine and avoids the `Arc<Mpv>: Send` future-bound question
    /// entirely.
    pub fn set_property<V>(&self, property: &str, value: V) -> Result<(), String>
    where
        V: SetData,
    {
        self.mpv
            .set_property(property, value)
            .map_err(|e| format!("mpv set_property '{}' failed: {}", property, e))
    }

    /// Get a property (synchronous — mpv's `mpv_get_property` is thread-safe).
    pub fn get_property<V>(&self, property: &str) -> Result<V, String>
    where
        V: GetData,
    {
        self.mpv
            .get_property(property)
            .map_err(|e| format!("mpv get_property '{}' failed: {}", property, e))
    }

    /// Set an option string (synchronous). Ported from soia's `mpv_set_option_string`
    /// surface — used for runtime-tunable mpv options like `aid`, `sid`, `sub-delay`,
    /// `audio-delay`, `brightness`, `contrast`, `saturation`, `gamma`, `hue`, `speed`,
    /// `sub-scale`, `secondary-sub-delay`.
    ///
    /// Prefer the typed `set_property` facade when the value is a known primitive (bool,
    /// i64, f64, String); this method is for the power-user surface where the frontend
    /// passes the option name + stringified value directly.
    pub fn set_option_string(&self, name: &str, value: &str) -> Result<(), String> {
        self.mpv
            .set_property(name, value)
            .map_err(|e| format!("mpv set_option_string '{}'='{}' failed: {}", name, value, e))
    }

    // -----------------------------------------------------------------------
    // Phase 2.4: subtitle / audio track management (port soia's sub-add /
    // sub-remove / sub-reload + audio-add / audio-remove command surface).
    // -----------------------------------------------------------------------

    /// Add an external subtitle file (`sub-add` command). Ported from soia's
    /// `mpv_run_command(["sub-add", path, mode])`.
    ///
    /// - `mode = "select"`: select the subtitle immediately (the user picked it).
    /// - `mode = "auto"`: add but don't select (auto-discovered subs; mpv picks based on
    ///   `--subs-with-matching-audio` + `--slang`).
    /// - `title`: optional display title (shown in mpv's track list + fyom's subtitle picker).
    /// - `lang`: optional ISO 639-1 language code (e.g. "en", "zh").
    pub fn sub_add(
        &self,
        path: &str,
        mode: &str,
        title: Option<&str>,
        lang: Option<&str>,
    ) -> Result<(), String> {
        // mpv's `sub-add` command takes args: <url> [<flags> [<title> [<lang>]]].
        // `flags` is "select" (activate) or "auto" (don't activate).
        // The command name ("sub-add") is the first arg to `mpv.command()`; the slice
        // is the remaining args (no command name in the slice).
        let mut args: Vec<&str> = vec![path, mode];
        if let Some(t) = title {
            args.push(t);
        } else {
            args.push("");
        }
        if let Some(l) = lang {
            args.push(l);
        }
        self.mpv
            .command("sub-add", &args)
            .map_err(|e| format!("mpv sub-add failed: {}", e))
    }

    /// Remove an external subtitle track by id (`sub-remove` command).
    pub fn sub_remove(&self, track_id: i64) -> Result<(), String> {
        let id_str = track_id.to_string();
        self.mpv
            .command("sub-remove", &[&id_str])
            .map_err(|e| format!("mpv sub-remove failed: {}", e))
    }

    /// Reload a subtitle track by id (`sub-reload` command). Useful after the user edits
    /// an external .srt file.
    pub fn sub_reload(&self, track_id: i64) -> Result<(), String> {
        let id_str = track_id.to_string();
        self.mpv
            .command("sub-reload", &[&id_str])
            .map_err(|e| format!("mpv sub-reload failed: {}", e))
    }

    /// Add an external audio track (`audio-add` command). Ported from soia's
    /// `mpv_run_command(["audio-add", path, mode])`.
    pub fn audio_add(&self, path: &str, mode: &str) -> Result<(), String> {
        self.mpv
            .command("audio-add", &[path, mode])
            .map_err(|e| format!("mpv audio-add failed: {}", e))
    }

    /// Remove an audio track by id (`audio-remove` command).
    pub fn audio_remove(&self, track_id: i64) -> Result<(), String> {
        let id_str = track_id.to_string();
        self.mpv
            .command("audio-remove", &[&id_str])
            .map_err(|e| format!("mpv audio-remove failed: {}", e))
    }

    // -----------------------------------------------------------------------
    // Phase 2.4: convenience setters for color adjustments + A/V delays
    // (ported from soia's `usePlaybackAdjustments.ts` invoke surface).
    // -----------------------------------------------------------------------

    /// Set subtitle delay (seconds; negative = earlier, positive = later).
    pub fn set_sub_delay(&self, seconds: f64) -> Result<(), String> {
        self.set_property("sub-delay", seconds)
    }

    /// Set secondary subtitle delay (seconds; for dual-sub mode).
    pub fn set_secondary_sub_delay(&self, seconds: f64) -> Result<(), String> {
        self.set_property("secondary-sub-delay", seconds)
    }

    /// Set audio delay (seconds; negative = earlier, positive = later).
    pub fn set_audio_delay(&self, seconds: f64) -> Result<(), String> {
        self.set_property("audio-delay", seconds)
    }

    /// Set subtitle scale (font size multiplier; 1.0 = default).
    pub fn set_sub_scale(&self, scale: f64) -> Result<(), String> {
        self.set_property("sub-scale", scale)
    }

    /// Set brightness (-100..=100; 0 = default).
    pub fn set_brightness(&self, value: f64) -> Result<(), String> {
        self.set_property("brightness", value)
    }

    /// Set contrast (-100..=100; 0 = default).
    pub fn set_contrast(&self, value: f64) -> Result<(), String> {
        self.set_property("contrast", value)
    }

    /// Set saturation (-100..=100; 0 = default).
    pub fn set_saturation(&self, value: f64) -> Result<(), String> {
        self.set_property("saturation", value)
    }

    /// Set gamma (-100..=100; 0 = default).
    pub fn set_gamma(&self, value: f64) -> Result<(), String> {
        self.set_property("gamma", value)
    }

    /// Set hue (-100..=100; 0 = default).
    pub fn set_hue(&self, value: f64) -> Result<(), String> {
        self.set_property("hue", value)
    }

    /// Navigate to a chapter by index (`set_property("chapter", n)`).
    pub fn set_chapter(&self, index: i64) -> Result<(), String> {
        self.set_property("chapter", index)
    }
}

impl Default for MpvInstance {
    fn default() -> Self {
        Self::new().expect("failed to create default MpvInstance")
    }
}

// SAFETY: `libmpv2::Mpv` holds a raw `mpv_handle`. mpv's core command/property API is
// thread-safe (documented in mpv/client.h). `Arc<Mpv>` sharing across threads is the
// established pattern (tsukimi ships this in production). `Arc<AtomicU32>` + the
// `Mutex<Option<JoinHandle>>` + `Mutex<Option<RenderThreadState>>` are `Send + Sync` by
// construction. The render context + event context have their own thread-affinity rules
// handled separately (the event thread owns the `EventContext`; the render thread owns
// the `RenderContext` + the platform GL surface — created on the render thread + never
// shared).
unsafe impl Send for MpvInstance {}
unsafe impl Sync for MpvInstance {}
