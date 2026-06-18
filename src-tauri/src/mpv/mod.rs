//! libmpv native playback integration (Phase 2).
//!
//! This module wraps the `libmpv2` crate (the same safe Rust binding tsukimi uses) and
//! exposes a thread-safe `MpvInstance` to the Tauri command layer.
//!
//! ## Phase 2.0 scope (this file + `handle.rs`)
//! - `MpvInstance::new()` — create + initialize a libmpv instance via
//!   `libmpv2::Mpv::with_initializer(...)`, setting the core playback properties
//!   (vo=libmpv, hwdec=auto-safe, cache, volume, sub-font, …) ported from tsukimi's
//!   `TsukimiMPV::default` initializer.
//! - `loadfile(url)`, `stop()`, `pause(bool)`, `set_property`, `command` — thin facade
//!   over `libmpv2::Mpv` for the PoC command surface.
//! - No GL rendering yet (Phase 2.3), no event pump yet (Phase 2.2). The PoC verifies
//!   libmpv loads + plays audio in a transparent Tauri window.
//!
//! ## Attribution
//! The initializer property set + the `Arc<Mpv>` + `unsafe impl Send/Sync` pattern are
//! ported from tsukimi (`tsukinaha/tsukimi`, GPL-3.0) `src/ui/mpv/tsukimi_mpv.rs`.
//! See `docs/libmpv-assessment.md` §2.2 for the reuse inventory.

pub mod handle;

pub use handle::MpvInstance;
