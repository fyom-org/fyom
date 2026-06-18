//! mpv option string matchers — pure integer → mpv-option-string converters.
//!
//! PORTED_FROM_TSUKIMI @ v26.6.3 (`src/ui/mpv/options_matcher.rs`)
//! Verbatim port. Pure functions, no state, no platform coupling. Used by the
//! `MpvInstance` initializer (Phase 2.0 set hardcoded defaults; Phase 2.4 will
//! wire these to fyom's settings store the way tsukimi wires them to `SETTINGS`).
//!
//! License: GPL-3.0-only (tsukimi) → GPL-3.0-only (fyom). See `docs/libmpv-assessment.md`
//! §2.2 for the reuse inventory.

/// Video upscaler → `scale` mpv option.
pub fn match_video_upscale(matcher: i32) -> &'static str {
    match matcher {
        0 => "lanczos",
        1 => "bilinear",
        2 => "ewa_lanczos",
        3 => "mitchell",
        4 => "hermite",
        5 => "oversample",
        6 => "linear",
        7 => "ewa_hanning",
        _ => "ewa_lanczossharp",
    }
}

/// Audio channel layout → `audio-channels` mpv option.
pub fn match_audio_channels(matcher: i32) -> &'static str {
    match matcher {
        1 => "auto-safe",
        2 => "mono",
        3 => "stereo",
        _ => "auto",
    }
}

/// Subtitle border style → `sub-border-style` mpv option.
pub fn match_sub_border_style(matcher: i32) -> &'static str {
    match matcher {
        1 => "opaque-box",
        2 => "background-box",
        _ => "outline-and-shadow",
    }
}

/// Hardware decoder interop → `hwdec` mpv option.
pub fn match_hwdec_interop(matcher: i32) -> &'static str {
    match matcher {
        0 => "no",
        1 => "auto-safe",
        2 => "vaapi",
        _ => "no",
    }
}
