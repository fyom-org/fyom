use std::ffi::c_void;
use std::sync::atomic::{AtomicU32, Ordering};
use std::sync::{Arc, Mutex};
use std::thread::JoinHandle;
use std::time::Duration;

use flume::{Receiver, RecvTimeoutError, Sender};
use glow::HasContext;
use libmpv2::render::{OpenGLInitParams, RenderContext, RenderParam, RenderParamApiType};
use libmpv2::Mpv;
use tracing::{debug, error, info, warn};

use crate::mpv::event_loop::SHUTDOWN;

// -----------------------------------------------------------------------------
// RenderSurface trait (no change).
// -----------------------------------------------------------------------------

pub trait RenderSurface: Send + 'static {
    fn make_current(&self) -> Result<(), String>;
    fn get_proc_address(&self, name: &str) -> *mut c_void;
    fn drawable_size(&self) -> (i32, i32);
    fn swap_buffers(&self);
}

// -----------------------------------------------------------------------------
// Thread-local surface pointer.
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
        if let Some(ptr) = *slot.borrow() {
            unsafe { (&*ptr).get_proc_address(name) }
        } else {
            std::ptr::null_mut()
        }
    })
}

fn get_proc_address_thunk(_ctx: &(), name: &str) -> *mut c_void {
    get_proc_address(name)
}

// -----------------------------------------------------------------------------
// RenderThreadState.
// -----------------------------------------------------------------------------

pub struct RenderThreadState {
    pub handle: Mutex<Option<JoinHandle<()>>>,
    pub shutdown: Arc<AtomicU32>,
}

impl RenderThreadState {
    pub fn new() -> Self {
        Self {
            handle: Mutex::new(None),
            shutdown: Arc::new(AtomicU32::new(0)),
        }
    }
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
        .map_err(|e| format!("render thread spawn failed: {e}"))
}

// -----------------------------------------------------------------------------
// Core loop
// -----------------------------------------------------------------------------

fn run_render_loop(
    mpv: Arc<Mpv>,
    surface: Box<dyn RenderSurface>,
    shutdown: Arc<AtomicU32>,
) {
    // --- early exit ---
    if shutdown.load(Ordering::SeqCst) == SHUTDOWN {
        return;
    }

    // --- make GL current ---
    if let Err(e) = surface.make_current() {
        error!("[render] make_current failed: {e}");
        return;
    }

    info!("[render] GL context ready");

    // --- thread-local binding ---
    let _guard = SurfaceGuard::install(&*surface);

    // --- build render ctx ---
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
        Err(e) => {
            error!("[render] RenderContext init failed: {e}");
            return;
        }
    };

    info!("[render] mpv RenderContext created");

    // --- private channel (CRITICAL FIX) ---
    let (tx, rx): (Sender<()>, Receiver<()>) = flume::unbounded();

    render_ctx.set_update_callback(move || {
        let _ = tx.send(());
    });

    // --- glow ctx ---
    let gl = unsafe {
        glow::Context::from_loader_function(|name| get_proc_address(name) as *const c_void)
    };

    let mut frames: u64 = 0;

    loop {
        if shutdown.load(Ordering::SeqCst) == SHUTDOWN {
            break;
        }

        // --- wait frame ---
        match rx.recv_timeout(Duration::from_millis(200)) {
            Ok(_) => {}
            Err(RecvTimeoutError::Timeout) => continue,
            Err(_) => break,
        }

        if shutdown.load(Ordering::SeqCst) == SHUTDOWN {
            break;
        }

        // --- size ---
        let (w, h) = surface.drawable_size();
        if w <= 0 || h <= 0 {
            continue;
        }

        // --- fbo ---
        let fbo = unsafe { gl.get_parameter_i32(glow::FRAMEBUFFER_BINDING) };

        // --- render ---
        match unsafe { render_ctx.render::<()>(fbo, w, h, true) } {
            Ok(_) => {
                surface.swap_buffers();

                frames += 1;

                if frames == 1 {
                    info!("[render] first frame {}x{}", w, h);
                } else if frames % 300 == 0 {
                    debug!("[render] frame {} {}x{}", frames, w, h);
                }
            }
            Err(e) => {
                warn!("[render] frame failed: {e}");
            }
        }
    }

    drop(render_ctx);

    info!("[render] exit, total frames={}", frames);
}

// -----------------------------------------------------------------------------
// Shutdown helper
// -----------------------------------------------------------------------------

pub fn wake_render_thread() {
    // no-op now (channel is private)
}
