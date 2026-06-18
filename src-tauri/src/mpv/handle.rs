//! `MpvInstance` — a thread-safe wrapper around `libmpv2::Mpv`.
//!
//! PORTED_FROM_TSUKIMI @ v26.6.3 (`src/ui/mpv/tsukimi_mpv.rs::TsukimiMPV`)
//! Adapted for fyom: the GTK-specific bits (epoxy loader, `press_key`/`KEYSTRING_MAP`,
//! `SETTINGS` accessors) are dropped; the initializer property set + the `Arc<Mpv>` +
//! `unsafe impl Send/Sync` pattern are retained. The event-pump thread
//! (`process_events`) is Phase 2.2; the render context is Phase 2.3.

use std::sync::Arc;

use libmpv2::{Mpv, SetData};
use tracing::info;

/// Default max volume (matches tsukimi's `MAX_VOLUME`).
const MAX_VOLUME: i64 = 100;

/// Default cache size (MiB) + cache duration (seconds) for the PoC.
/// Tunable via settings in a later phase; hardcoded for the spike.
const DEFAULT_CACHE_MIB: u64 = 256;
const DEFAULT_CACHE_SECS: i64 = 10;
const DEFAULT_VOLUME: i64 = 80;

/// A thread-safe libmpv instance.
///
/// `libmpv2::Mpv` holds a raw `mpv_handle` pointer. mpv's core API (`mpv_command`,
/// `mpv_set_property`, `mpv_get_property`) is thread-safe, so wrapping in `Arc` +
/// declaring `Send + Sync` is sound — this matches tsukimi's proven pattern.
pub struct MpvInstance {
    pub mpv: Arc<Mpv>,
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
    /// context is wired in Phase 2.3), `hwdec=auto-safe`, cache, volume, sub-font, …
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
            // Render-API driven video output. Phase 2.0 has no render context yet, so
            // video is a black frame + audio plays — exactly the PoC exit criterion.
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
            // webview in Phase 2.2 via a `mpv_keypress` command).
            init.set_property("input-default-bindings", true)?;
            init.set_property("input-vo-keyboard", true)?;

            // Don't loop by default.
            init.set_property("loop", "no")?;

            Ok(())
        })
        .map_err(|e| format!("failed to create libmpv instance: {}", e))?;

        info!("[mpv] instance created + initialized (vo=libmpv, hwdec=auto-safe)");

        Ok(Self { mpv: Arc::new(mpv) })
    }

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

    /// Seek by a relative offset (seconds).
    pub fn seek_relative(&self, seconds: f64) -> Result<(), String> {
        self.mpv
            .command("seek", &[&seconds.to_string(), "relative"])
            .map_err(|e| format!("mpv seek failed: {}", e))
    }

    /// Set a property (synchronous — mpv's `mpv_set_property` is a fast, thread-safe C
    /// call). tsukimi spawns this off the UI thread; for the Phase 2.0 PoC a synchronous
    /// call is fine and avoids the `Arc<Mpv>: Send` future-bound question entirely.
    pub fn set_property<V>(&self, property: &str, value: V) -> Result<(), String>
    where
        V: SetData,
    {
        self.mpv
            .set_property(property, value)
            .map_err(|e| format!("mpv set_property '{}' failed: {}", property, e))
    }
}

impl Default for MpvInstance {
    fn default() -> Self {
        Self::new().expect("failed to create default MpvInstance")
    }
}

// SAFETY: `libmpv2::Mpv` holds a raw `mpv_handle`. mpv's core command/property API is
// thread-safe (documented in mpv/client.h). `Arc<Mpv>` sharing across threads is the
// established pattern (tsukimi ships this in production). The render context + event
// context (Phase 2.2/2.3) have their own thread-affinity rules handled separately.
unsafe impl Send for MpvInstance {}
unsafe impl Sync for MpvInstance {}
