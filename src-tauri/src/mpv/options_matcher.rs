//! mpv option string matchers.
//!
//! These are pure integer-to-mpv-option converters ported from tsukimi.
//!
//! PORTED_FROM_TSUKIMI @ v26.6.3 (`src/ui/mpv/options_matcher.rs`)
//!
//! ## Why this module may be unused right now
//!
//! Phase 2 currently hardcodes the essential mpv defaults in `MpvInstance::new`.
//! These matchers are intentionally kept for the later settings-store wiring, where UI
//! preferences will be stored as compact integer values and translated into mpv option
//! strings at runtime.
//!
//! Keeping this module now prevents those mappings from being scattered across command
//! handlers later, while keeping them side-effect free and easy to test.
//!
//! License: GPL-3.0-only (tsukimi) -> GPL-3.0-only (fyom). See
//! `docs/libmpv-assessment.md` for the reuse inventory.

#![allow(dead_code)]

/// Default mpv video upscaler used when the stored setting is unknown.
pub const DEFAULT_VIDEO_UPSCALE: &str = "ewa_lanczossharp";

/// Default mpv audio channel layout used when the stored setting is unknown.
pub const DEFAULT_AUDIO_CHANNELS: &str = "auto";

/// Default mpv subtitle border style used when the stored setting is unknown.
pub const DEFAULT_SUB_BORDER_STYLE: &str = "outline-and-shadow";

/// Default mpv hardware decoder interop used when the stored setting is unknown.
pub const DEFAULT_HWDEC_INTEROP: &str = "no";

/// Video upscaler setting values.
///
/// These integer discriminants intentionally match the historical tsukimi settings
/// values. Do not reorder without a migration.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[repr(i32)]
pub enum VideoUpscale {
    Lanczos = 0,
    Bilinear = 1,
    EwaLanczos = 2,
    Mitchell = 3,
    Hermite = 4,
    Oversample = 5,
    Linear = 6,
    EwaHanning = 7,
    EwaLanczosSharp = 8,
}

impl VideoUpscale {
    /// Convert the setting enum to the mpv `scale` option string.
    pub const fn as_mpv_option(self) -> &'static str {
        match self {
            Self::Lanczos => "lanczos",
            Self::Bilinear => "bilinear",
            Self::EwaLanczos => "ewa_lanczos",
            Self::Mitchell => "mitchell",
            Self::Hermite => "hermite",
            Self::Oversample => "oversample",
            Self::Linear => "linear",
            Self::EwaHanning => "ewa_hanning",
            Self::EwaLanczosSharp => DEFAULT_VIDEO_UPSCALE,
        }
    }

    /// Convert a stored integer setting into a typed variant.
    pub const fn from_i32(value: i32) -> Self {
        match value {
            0 => Self::Lanczos,
            1 => Self::Bilinear,
            2 => Self::EwaLanczos,
            3 => Self::Mitchell,
            4 => Self::Hermite,
            5 => Self::Oversample,
            6 => Self::Linear,
            7 => Self::EwaHanning,
            _ => Self::EwaLanczosSharp,
        }
    }
}

/// Audio channel layout setting values.
///
/// These integer discriminants intentionally match the historical tsukimi settings
/// values. Do not reorder without a migration.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[repr(i32)]
pub enum AudioChannels {
    Auto = 0,
    AutoSafe = 1,
    Mono = 2,
    Stereo = 3,
}

impl AudioChannels {
    /// Convert the setting enum to the mpv `audio-channels` option string.
    pub const fn as_mpv_option(self) -> &'static str {
        match self {
            Self::Auto => DEFAULT_AUDIO_CHANNELS,
            Self::AutoSafe => "auto-safe",
            Self::Mono => "mono",
            Self::Stereo => "stereo",
        }
    }

    /// Convert a stored integer setting into a typed variant.
    pub const fn from_i32(value: i32) -> Self {
        match value {
            1 => Self::AutoSafe,
            2 => Self::Mono,
            3 => Self::Stereo,
            _ => Self::Auto,
        }
    }
}

/// Subtitle border style setting values.
///
/// These integer discriminants intentionally match the historical tsukimi settings
/// values. Do not reorder without a migration.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[repr(i32)]
pub enum SubBorderStyle {
    OutlineAndShadow = 0,
    OpaqueBox = 1,
    BackgroundBox = 2,
}

impl SubBorderStyle {
    /// Convert the setting enum to the mpv `sub-border-style` option string.
    pub const fn as_mpv_option(self) -> &'static str {
        match self {
            Self::OutlineAndShadow => DEFAULT_SUB_BORDER_STYLE,
            Self::OpaqueBox => "opaque-box",
            Self::BackgroundBox => "background-box",
        }
    }

    /// Convert a stored integer setting into a typed variant.
    pub const fn from_i32(value: i32) -> Self {
        match value {
            1 => Self::OpaqueBox,
            2 => Self::BackgroundBox,
            _ => Self::OutlineAndShadow,
        }
    }
}

/// Hardware decoder interop setting values.
///
/// These integer discriminants intentionally match the historical tsukimi settings
/// values. Do not reorder without a migration.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[repr(i32)]
pub enum HwdecInterop {
    Disabled = 0,
    AutoSafe = 1,
    Vaapi = 2,
}

impl HwdecInterop {
    /// Convert the setting enum to the mpv `hwdec` option string.
    pub const fn as_mpv_option(self) -> &'static str {
        match self {
            Self::Disabled => DEFAULT_HWDEC_INTEROP,
            Self::AutoSafe => "auto-safe",
            Self::Vaapi => "vaapi",
        }
    }

    /// Convert a stored integer setting into a typed variant.
    pub const fn from_i32(value: i32) -> Self {
        match value {
            1 => Self::AutoSafe,
            2 => Self::Vaapi,
            _ => Self::Disabled,
        }
    }
}

/// Video upscaler -> mpv `scale` option.
///
/// This function preserves the original tsukimi-compatible integer API. Prefer
/// [`VideoUpscale::from_i32`] in new Rust code when a typed value is useful.
pub const fn match_video_upscale(matcher: i32) -> &'static str {
    VideoUpscale::from_i32(matcher).as_mpv_option()
}

/// Audio channel layout -> mpv `audio-channels` option.
///
/// This function preserves the original tsukimi-compatible integer API. Prefer
/// [`AudioChannels::from_i32`] in new Rust code when a typed value is useful.
pub const fn match_audio_channels(matcher: i32) -> &'static str {
    AudioChannels::from_i32(matcher).as_mpv_option()
}

/// Subtitle border style -> mpv `sub-border-style` option.
///
/// This function preserves the original tsukimi-compatible integer API. Prefer
/// [`SubBorderStyle::from_i32`] in new Rust code when a typed value is useful.
pub const fn match_sub_border_style(matcher: i32) -> &'static str {
    SubBorderStyle::from_i32(matcher).as_mpv_option()
}

/// Hardware decoder interop -> mpv `hwdec` option.
///
/// This function preserves the original tsukimi-compatible integer API. Prefer
/// [`HwdecInterop::from_i32`] in new Rust code when a typed value is useful.
pub const fn match_hwdec_interop(matcher: i32) -> &'static str {
    HwdecInterop::from_i32(matcher).as_mpv_option()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn video_upscale_matches_expected_mpv_options() {
        assert_eq!(match_video_upscale(0), "lanczos");
        assert_eq!(match_video_upscale(1), "bilinear");
        assert_eq!(match_video_upscale(2), "ewa_lanczos");
        assert_eq!(match_video_upscale(3), "mitchell");
        assert_eq!(match_video_upscale(4), "hermite");
        assert_eq!(match_video_upscale(5), "oversample");
        assert_eq!(match_video_upscale(6), "linear");
        assert_eq!(match_video_upscale(7), "ewa_hanning");
        assert_eq!(match_video_upscale(8), DEFAULT_VIDEO_UPSCALE);
        assert_eq!(match_video_upscale(-1), DEFAULT_VIDEO_UPSCALE);
        assert_eq!(match_video_upscale(999), DEFAULT_VIDEO_UPSCALE);
    }

    #[test]
    fn audio_channels_match_expected_mpv_options() {
        assert_eq!(match_audio_channels(0), DEFAULT_AUDIO_CHANNELS);
        assert_eq!(match_audio_channels(1), "auto-safe");
        assert_eq!(match_audio_channels(2), "mono");
        assert_eq!(match_audio_channels(3), "stereo");
        assert_eq!(match_audio_channels(-1), DEFAULT_AUDIO_CHANNELS);
        assert_eq!(match_audio_channels(999), DEFAULT_AUDIO_CHANNELS);
    }

    #[test]
    fn sub_border_style_matches_expected_mpv_options() {
        assert_eq!(match_sub_border_style(0), DEFAULT_SUB_BORDER_STYLE);
        assert_eq!(match_sub_border_style(1), "opaque-box");
        assert_eq!(match_sub_border_style(2), "background-box");
        assert_eq!(match_sub_border_style(-1), DEFAULT_SUB_BORDER_STYLE);
        assert_eq!(match_sub_border_style(999), DEFAULT_SUB_BORDER_STYLE);
    }

    #[test]
    fn hwdec_interop_matches_expected_mpv_options() {
        assert_eq!(match_hwdec_interop(0), DEFAULT_HWDEC_INTEROP);
        assert_eq!(match_hwdec_interop(1), "auto-safe");
        assert_eq!(match_hwdec_interop(2), "vaapi");
        assert_eq!(match_hwdec_interop(-1), DEFAULT_HWDEC_INTEROP);
        assert_eq!(match_hwdec_interop(999), DEFAULT_HWDEC_INTEROP);
    }

    #[test]
    fn typed_enums_preserve_integer_mapping() {
        assert_eq!(VideoUpscale::from_i32(0), VideoUpscale::Lanczos);
        assert_eq!(VideoUpscale::from_i32(8), VideoUpscale::EwaLanczosSharp);
        assert_eq!(VideoUpscale::from_i32(99), VideoUpscale::EwaLanczosSharp);

        assert_eq!(AudioChannels::from_i32(0), AudioChannels::Auto);
        assert_eq!(AudioChannels::from_i32(1), AudioChannels::AutoSafe);
        assert_eq!(AudioChannels::from_i32(99), AudioChannels::Auto);

        assert_eq!(
            SubBorderStyle::from_i32(0),
            SubBorderStyle::OutlineAndShadow
        );
        assert_eq!(SubBorderStyle::from_i32(1), SubBorderStyle::OpaqueBox);
        assert_eq!(
            SubBorderStyle::from_i32(99),
            SubBorderStyle::OutlineAndShadow
        );

        assert_eq!(HwdecInterop::from_i32(0), HwdecInterop::Disabled);
        assert_eq!(HwdecInterop::from_i32(1), HwdecInterop::AutoSafe);
        assert_eq!(HwdecInterop::from_i32(99), HwdecInterop::Disabled);
    }
}
