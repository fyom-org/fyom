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
//! - The render context is Phase 2.3 (not present here).
//!
//! See `docs/libmpv-assessment.md` §3.2 + §3.4 for the rationale.

use std::sync::Arc;
use std::sync::atomic::{AtomicU32, Ordering};
use std::sync::Mutex;
use std::thread::JoinHandle;

use libmpv2::{GetData, Mpv, SetData};
use tauri::AppHandle;
use tracing::info;

use crate::mpv::event_loop::{self, ACTIVE, SHUTDOWN};

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
}

impl Default for MpvInstance {
    fn default() -> Self {
        Self::new().expect("failed to create default MpvInstance")
    }
}

// SAFETY: `libmpv2::Mpv` holds a raw `mpv_handle`. mpv's core command/property API is
// thread-safe (documented in mpv/client.h). `Arc<Mpv>` sharing across threads is the
// established pattern (tsukimi ships this in production). `Arc<AtomicU32>` + the
// `Mutex<Option<JoinHandle>>` are `Send + Sync` by construction. The render context +
// event context have their own thread-affinity rules handled separately (the event
// thread owns the `EventContext`; the render thread will own the `RenderContext` in 2.3).
unsafe impl Send for MpvInstance {}
unsafe impl Sync for MpvInstance {}
