use std::ffi::c_void;
use std::sync::Arc;
use std::sync::atomic::{AtomicU32, Ordering};
use std::thread::JoinHandle;
use std::time::Duration;

use flume::{Receiver, RecvTimeoutError, Sender, bounded};
use glow::HasContext;
use libmpv2::Mpv;
use libmpv2::render::{OpenGLInitParams, RenderContext, RenderParam, RenderParamApiType};
use tracing::{debug, error, info, warn};

use crate::mpv::event_loop::SHUTDOWN;

// -----------------------------------------------------------------------------
// RenderSurface
// -----------------------------------------------------------------------------

pub trait RenderSurface: Send + 'static {
    fn make_current(&self) -> Result<(), String>;
    fn get_proc_address(&self, name: &str) -> *mut c_void;
    fn drawable_size(&self) -> (i32, i32);
    fn swap_buffers(&self);
}

// -----------------------------------------------------------------------------
// Thread-local surface pointer
// -----------------------------------------------------------------------------

thread_local! {
    static CURRENT_SURFACE: std::cell::RefCell<Option<*const dyn RenderSurface>> =
        const { std::cell::RefCell::new(None) };
}

struct SurfaceGuard;

impl SurfaceGuard {
    fn install(surface: &dyn RenderSurface) -> Self {
        CURRENT_SURFACE.with(|slot| {
            *slot.borrow_mut() = Some(surface as *const _);
        });

        Self
    }
}

impl Drop for SurfaceGuard {
    fn drop(&mut self) {
        CURRENT_SURFACE.with(|slot| {
            *slot.borrow_mut() = None;
        });
    }
}

fn get_proc_address(name: &str) -> *mut c_void {
    CURRENT_SURFACE.with(|slot| {
        let Some(ptr) = *slot.borrow() else {
            return std::ptr::null_mut();
        };

        // SAFETY:
        // The pointer is installed by `SurfaceGuard` on the render thread and remains
        // valid until the render context is dropped and the guard is cleared.
        unsafe { (&*ptr).get_proc_address(name) }
    })
}

fn get_proc_address_thunk(_ctx: &(), name: &str) -> *mut c_void {
    get_proc_address(name)
}

// -----------------------------------------------------------------------------
// Spawn
// -----------------------------------------------------------------------------

pub fn spawn_render_thread(
    mpv: Arc<Mpv>,
    surface: Box<dyn RenderSurface>,
    shutdown: Arc<AtomicU32>,
) -> Result<JoinHandle<()>, String> {
    std::thread::Builder::new()
        .name("fyom-mpv-render".into())
        .spawn(move || run_render_loop(mpv, surface, shutdown))
        .map_err(|error| format!("render thread spawn failed: {error}"))
}

// -----------------------------------------------------------------------------
// Core loop
// -----------------------------------------------------------------------------

fn run_render_loop(mpv: Arc<Mpv>, surface: Box<dyn RenderSurface>, shutdown: Arc<AtomicU32>) {
    if shutdown.load(Ordering::SeqCst) == SHUTDOWN {
        return;
    }

    if let Err(error) = surface.make_current() {
        error!("[render] make_current failed: {error}");
        return;
    }

    info!("[render] GL context ready");

    let _surface_guard = SurfaceGuard::install(&*surface);

    let params = vec![
        RenderParam::ApiType(RenderParamApiType::OpenGl),
        RenderParam::InitParams(OpenGLInitParams {
            get_proc_address: get_proc_address_thunk,
            ctx: (),
        }),
    ];

    let mut handle = mpv.ctx;

    let mut render_ctx = match unsafe { RenderContext::new(handle.as_mut(), params) } {
        Ok(ctx) => ctx,
        Err(error) => {
            error!("[render] RenderContext init failed: {error}");
            return;
        }
    };

    info!("[render] mpv RenderContext created");

    // Keep only one pending render wake.
    //
    // mpv can emit update callbacks faster than we render. A bounded(1) channel coalesces
    // redundant wakeups and prevents unbounded memory growth. The callback must never block.
    let (tx, rx): (Sender<()>, Receiver<()>) = bounded(1);

    render_ctx.set_update_callback(move || {
        let _ = tx.try_send(());
    });

    // SAFETY:
    // The GL context is current on this render thread. glow only stores function pointers
    // resolved through the current platform surface.
    let gl = unsafe {
        glow::Context::from_loader_function(|name| get_proc_address(name) as *const c_void)
    };

    let mut frames: u64 = 0;

    loop {
        if shutdown.load(Ordering::SeqCst) == SHUTDOWN {
            break;
        }

        match rx.recv_timeout(Duration::from_millis(200)) {
            Ok(()) => {}
            Err(RecvTimeoutError::Timeout) => continue,
            Err(RecvTimeoutError::Disconnected) => break,
        }

        if shutdown.load(Ordering::SeqCst) == SHUTDOWN {
            break;
        }

        let (width, height) = surface.drawable_size();

        if width <= 0 || height <= 0 {
            debug!("[render] skip frame with invalid drawable size: {width}x{height}");
            continue;
        }

        // SAFETY:
        // The GL context is current on this render thread.
        let fbo = unsafe { gl.get_parameter_i32(glow::FRAMEBUFFER_BINDING) };

        match render_ctx.render::<()>(fbo, width, height, true) {
            Ok(()) => {
                surface.swap_buffers();

                frames = frames.saturating_add(1);

                if frames == 1 {
                    info!("[render] first frame {width}x{height}");
                } else if frames % 300 == 0 {
                    debug!("[render] frame {frames} {width}x{height}");
                }
            }
            Err(error) => {
                warn!("[render] frame failed: {error}");
            }
        }
    }

    drop(render_ctx);

    info!("[render] exit, total frames={frames}");
}
