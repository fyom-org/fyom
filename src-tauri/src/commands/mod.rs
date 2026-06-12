//! Tauri command handlers
//!
//! This module contains all Tauri invoke handlers.
//! Playback commands will be added in Phase 2 when native playback is integrated.

pub mod playback;

/// Re-export commands for easier access
pub use playback::*;
