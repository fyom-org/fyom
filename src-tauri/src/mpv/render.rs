//! mpv rendering abstraction.
//!
//! FYOM currently uses native window embedding for the production playback path.
//!
//! macOS path:
//! - create a dedicated `NSView`
//! - attach a `CAMetalLayer`
//! - pass the `NSView` pointer to mpv through the `wid` option before `mpv_initialize()`
//! - let mpv own the Vulkan/MoltenVK/Metal context
//!
//! This module intentionally does not initialize `mpv_render_context` for the
//! normal embedded playback path.
//!
//! `mpv_render_context` is only appropriate for a future texture-sharing architecture
//! where FYOM owns the GPU context and asks mpv to render into host-managed targets.
//! That is not the current architecture.

use std::ffi::c_void;
use std::sync::Arc;
use std::sync::atomic::{AtomicU32, Ordering};
use std::thread::JoinHandle;
use std::time::Duration;

use libmpv2::Mpv;
use tracing::{debug, info};

use crate::mpv::event_loop::SHUTDOWN;

// -----------------------------------------------------------------------------
// RenderSurface
// -----------------------------------------------------------------------------

/// Platform render target abstraction.
///
/// In the current `--wid` architecture, this is not a real mpv render context.
/// It represents the native platform surface that mpv renders into by itself.
///
/// Important:
/// Do not use this trait as a reason to call `mpv_render_context_create()`.
/// In `--wid` mode, mpv owns the GPU context and presentation pipeline.
pub trait RenderSurface: Send + 'static {
    /// Make the platform GPU context current.
    ///
    /// In `--wid` mode this is a no-op because mpv owns the GPU context.
    fn make_current(&self) -> Result<(), String>;

    /// Resolve platform GPU function pointers.
    ///
    /// In `--wid` mode this returns null because mpv resolves GPU symbols internally.
    fn get_proc_address(&self, name: &str) -> *mut c_void;

    /// Drawable size in physical pixels.
    ///
    /// This can still be useful for diagnostics and layout validation.
    fn drawable_size(&self) -> (i32, i32);

    /// Swap platform buffers.
    ///
    /// In `--wid` mode this is a no-op because mpv presents internally.
    fn swap_buffers(&self);
}

// -----------------------------------------------------------------------------
// Render mode
// -----------------------------------------------------------------------------

/// FYOM's active mpv rendering architecture.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum MpvRenderMode {
    /// Native window embedding.
    ///
    /// This is the current production path.
    ///
    /// macOS:
    /// `NSView` + `CAMetalLayer` + mpv `wid`.
    Wid,

    /// Future texture-sharing path.
    ///
    /// This must not be used unless FYOM explicitly owns the GPU context and
    /// imports mpv-rendered textures into its own compositor.
    RenderApi,
}

impl Default for MpvRenderMode {
    fn default() -> Self {
        Self::Wid
    }
}

/// Guard against accidental use of mpv's render API in the current architecture.
pub fn ensure_wid_mode(mode: MpvRenderMode) -> Result<(), String> {
    match mode {
        MpvRenderMode::Wid => Ok(()),
        MpvRenderMode::RenderApi => Err(
            "mpv render API is disabled for FYOM's current --wid embedding architecture"
                .to_string(),
        ),
    }
}

// -----------------------------------------------------------------------------
// Spawn
// -----------------------------------------------------------------------------

/// Spawn the render lifecycle thread.
///
/// In the old architecture this function initialized `mpv_render_context` and drove
/// an OpenGL render loop. That is now intentionally disabled for the default path.
///
/// In the current `--wid` architecture:
/// - mpv renders directly into the native child view
/// - mpv owns the GPU context
/// - no OpenGL context is made current by FYOM
/// - no `mpv_render_context` is created
///
/// This thread exists only to retain the platform surface for as long as playback
/// needs it, and to preserve the existing caller contract while the playback path
/// is being migrated to explicit `wid` startup options.
pub fn spawn_render_thread(
    mpv: Arc<Mpv>,
    surface: Box<dyn RenderSurface>,
    shutdown: Arc<AtomicU32>,
) -> Result<JoinHandle<()>, String> {
    std::thread::Builder::new()
        .name("fyom-mpv-surface-lifecycle".into())
        .spawn(move || run_surface_lifecycle_loop(mpv, surface, shutdown))
        .map_err(|error| format!("surface lifecycle thread spawn failed: {error}"))
}

// -----------------------------------------------------------------------------
// Lifecycle loop
// -----------------------------------------------------------------------------

fn run_surface_lifecycle_loop(
    mpv: Arc<Mpv>,
    surface: Box<dyn RenderSurface>,
    shutdown: Arc<AtomicU32>,
) {
    let _mpv = mpv;

    if shutdown.load(Ordering::SeqCst) == SHUTDOWN {
        return;
    }

    let (width, height) = surface.drawable_size();

    info!(
        "[render] wid embedding active; mpv_render_context disabled; initial drawable={}x{}",
        width, height
    );

    loop {
        if shutdown.load(Ordering::SeqCst) == SHUTDOWN {
            break;
        }

        let (width, height) = surface.drawable_size();

        debug!(
            "[render] surface lifecycle heartbeat; drawable={}x{}",
            width, height
        );

        std::thread::sleep(Duration::from_millis(500));
    }

    drop(surface);

    info!("[render] surface lifecycle exit");
}
