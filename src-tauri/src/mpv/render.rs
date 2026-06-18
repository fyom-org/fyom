//! libmpv GL render context + render loop — ports tsukimi's `mpvglarea.rs` pattern.
//!
//! PORTED_FROM_TSUKIMI @ v26.6.3 (`src/ui/mpv/mpvglarea.rs`)
//!
//! Adapted for fyom:
//! - tsukimi's GTK `GLArea` shell is dropped. fyom owns its own per-platform GL surface
//!   and a dedicated render thread.
//! - The `RenderContext::new` + `set_update_callback` + `render::<C>(fbo, w, h, true)`
//!   pattern is preserved.
//! - `glow::Context::from_loader_function(get_proc_address)` + reading the current FBO via
//!   `glow::get_parameter_i32(glow::FRAMEBUFFER_BINDING)` is preserved.
//! - The update callback wakes a flume channel. The render thread consumes that channel.
//! - fyom's `RenderSurface::drawable_size()` already returns physical pixels.
//!
//! See `docs/libmpv-assessment.md` §3.1 + §3.3 + §3.4 for the rationale + reuse inventory.

use std::ffi::c_void;
use std::sync::atomic::{AtomicU32, Ordering};
use std::sync::{Arc, LazyLock, Mutex};
use std::thread::JoinHandle;
use std::time::Duration;

use flume::{Receiver, RecvTimeoutError, Sender, unbounded};
use glow::HasContext;
use libmpv2::Mpv;
use libmpv2::render::{OpenGLInitParams, RenderContext, RenderParam, RenderParamApiType};
use tracing::{debug, error, info, warn};

use crate::mpv::event_loop::SHUTDOWN;

// ---------------------------------------------------------------------------
// Render-wake channel.
// ---------------------------------------------------------------------------

/// A flume pair signaling "mpv has a new frame ready, please render".
///
/// The sender is fed by `RenderContext::set_update_callback`, which can fire from mpv's
/// internal threads. The receiver is consumed by fyom's render thread.
struct RenderUpdateChannel {
    /// Fed by the mpv render update callback.
    tx: Sender<bool>,
    /// Consumed by fyom's render thread.
    rx: Receiver<bool>,
}

/// Global render wake channel.
///
/// `LazyLock` lets the mpv update callback reference the sender without capturing a
/// non-static render-loop value.
static RENDER_UPDATE: LazyLock<RenderUpdateChannel> = LazyLock::new(|| {
    let (tx, rx) = unbounded::<bool>();
    RenderUpdateChannel { tx, rx }
});

// ---------------------------------------------------------------------------
// RenderSurface — contract between render.rs and platform backends.
// ---------------------------------------------------------------------------

/// The platform GL surface contract.
///
/// Implemented per-platform in `crate::platform::{macos,windows,default}`:
/// - macOS: `NSOpenGLContext` + child view/layer behind `WKWebView`.
/// - Windows: child `HWND` + WGL context.
/// - Linux: child X11 `Window` + GLX context.
///
/// The render thread calls:
/// 1. `make_current()` once at startup.
/// 2. `drawable_size()` on every frame.
/// 3. `swap_buffers()` after every successful mpv render.
pub trait RenderSurface: Send + 'static {
    /// Make the GL context current on the calling thread.
    ///
    /// This is called once at render-thread startup. Platform implementations should make
    /// this idempotent because repeated calls are harmless but unnecessary.
    fn make_current(&self) -> Result<(), String>;

    /// Resolve a GL function pointer by name.
    ///
    /// Used by both libmpv's OpenGL init params and glow's loader function.
    ///
    /// Return null if the symbol cannot be resolved.
    fn get_proc_address(&self, name: &str) -> *mut c_void;

    /// Current drawable dimensions in physical pixels.
    ///
    /// This is polled on every frame so resize does not require a dedicated render event.
    fn drawable_size(&self) -> (i32, i32);

    /// Present the rendered frame.
    ///
    /// No-op for single-buffered contexts.
    fn swap_buffers(&self);
}

// ---------------------------------------------------------------------------
// Render-thread state.
// ---------------------------------------------------------------------------

/// Shared render-thread lifecycle state.
///
/// Held by `MpvInstance` so the command layer can signal shutdown and the application
/// exit path can join the render thread.
pub struct RenderThreadState {
    /// Render-thread handle. `Some` after spawn, cleared by the owner on shutdown.
    pub handle: Mutex<Option<JoinHandle<()>>>,

    /// Shutdown signal. `0` means running; `SHUTDOWN` means stop.
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

impl Default for RenderThreadState {
    fn default() -> Self {
        Self::new()
    }
}

// ---------------------------------------------------------------------------
// Thread-local surface pointer.
// ---------------------------------------------------------------------------

thread_local! {
    /// Render-thread-local pointer to the active platform surface.
    ///
    /// libmpv2's `OpenGLInitParams` accepts a plain function pointer for loading GL
    /// symbols, not a closure. A thread-local pointer gives that function pointer access
    /// to the current `RenderSurface` without global ownership.
    ///
    /// Safety invariant:
    /// - Set only on the render thread.
    /// - Points to the heap allocation owned by `Box<dyn RenderSurface>` in
    ///   `run_render_loop`.
    /// - Cleared before the render thread exits.
    static CURRENT_SURFACE: std::cell::RefCell<Option<*const dyn RenderSurface>> =
        const { std::cell::RefCell::new(None) };
}

/// RAII guard that installs and clears `CURRENT_SURFACE`.
struct CurrentSurfaceGuard;

impl CurrentSurfaceGuard {
    fn install(surface: &dyn RenderSurface) -> Self {
        let surface_ptr = surface as *const dyn RenderSurface;
        CURRENT_SURFACE.with(|slot| {
            *slot.borrow_mut() = Some(surface_ptr);
        });
        Self
    }
}

impl Drop for CurrentSurfaceGuard {
    fn drop(&mut self) {
        CURRENT_SURFACE.with(|slot| {
            *slot.borrow_mut() = None;
        });
    }
}

/// Resolve a GL symbol through the current render-thread surface.
///
/// Returns null instead of panicking if called outside the render thread or after teardown.
fn current_surface_get_proc_address(name: &str) -> *mut c_void {
    CURRENT_SURFACE.with(|slot| {
        let surface_ptr = *slot.borrow();

        match surface_ptr {
            Some(ptr) => {
                // SAFETY: `ptr` is installed by `CurrentSurfaceGuard` and points to the
                // boxed surface owned by `run_render_loop`. It remains valid until the
                // guard is dropped, which happens after the render context is dropped.
                unsafe { (&*ptr).get_proc_address(name) }
            }
            None => std::ptr::null_mut(),
        }
    })
}

/// Function pointer passed to libmpv OpenGL init params.
///
/// libmpv2 expects a plain function pointer. The `ctx` value is unused because fyom
/// resolves the platform surface through `CURRENT_SURFACE`.
fn get_proc_address_thunk(_ctx: &(), name: &str) -> *mut c_void {
    current_surface_get_proc_address(name)
}

// ---------------------------------------------------------------------------
// Render-thread spawn.
// ---------------------------------------------------------------------------

/// Spawn the mpv render thread.
///
/// The thread:
/// 1. Makes the platform GL context current.
/// 2. Creates libmpv's OpenGL render context.
/// 3. Registers an update callback that wakes the render loop.
/// 4. Waits for frame updates and renders into the current framebuffer.
/// 5. Exits when `shutdown` is set to `SHUTDOWN`.
pub fn spawn_render_thread(
    mpv: Arc<Mpv>,
    surface: Box<dyn RenderSurface>,
    shutdown: Arc<AtomicU32>,
) -> Result<JoinHandle<()>, String> {
    std::thread::Builder::new()
        .name("fyom mpv render loop".into())
        .spawn(move || {
            run_render_loop(mpv, surface, shutdown);
        })
        .map_err(|e| format!("failed to spawn render thread: {}", e))
}

/// Render-loop body.
///
/// All errors are logged and terminate the render thread cleanly. The rest of the app can
/// continue using the `<video>` fallback path.
fn run_render_loop(mpv: Arc<Mpv>, surface: Box<dyn RenderSurface>, shutdown: Arc<AtomicU32>) {
    if shutdown.load(Ordering::SeqCst) == SHUTDOWN {
        debug!("[mpv/render] shutdown already requested before render loop start");
        return;
    }

    // STEP 1: make the platform GL context current on this thread.
    if let Err(e) = surface.make_current() {
        error!(
            "[mpv/render] failed to make GL context current: {} — GL rendering disabled",
            e
        );
        return;
    }

    info!("[mpv/render] GL context made current on render thread");

    // Install thread-local surface pointer before creating RenderContext because libmpv may
    // call the GL loader during context creation.
    let _surface_guard = CurrentSurfaceGuard::install(&*surface);

    // STEP 2: create libmpv's OpenGL render context.
    let render_params = vec![
        RenderParam::ApiType(RenderParamApiType::OpenGl),
        RenderParam::InitParams(OpenGLInitParams {
            get_proc_address: get_proc_address_thunk,
            ctx: (),
        }),
    ];

    let mut handle = mpv.ctx;

    // SAFETY: `mpv.ctx` is the live mpv handle owned by the shared `Mpv` instance.
    // The render context is bound to this handle and dropped before the thread exits.
    let mut render_ctx = match unsafe { RenderContext::new(handle.as_mut(), render_params) } {
        Ok(ctx) => ctx,
        Err(e) => {
            error!(
                "[mpv/render] RenderContext::new failed: {} — GL rendering disabled",
                e
            );
            return;
        }
    };

    info!("[mpv/render] RenderContext created (OpenGL)");

    // STEP 3: register mpv's render update callback.
    render_ctx.set_update_callback(|| {
        let _ = RENDER_UPDATE.tx.send(true);
    });

    // STEP 4: create a glow context for querying the current framebuffer binding.
    //
    // glow expects `*const c_void`, while libmpv's GL loader uses `*mut c_void`. The cast is
    // only a mutability qualifier change.
    let glow_ctx = unsafe {
        glow::Context::from_loader_function(|name| {
            current_surface_get_proc_address(name) as *const c_void
        })
    };

    let rx = &RENDER_UPDATE.rx;
    let mut frame_count: u64 = 0;

    loop {
        if shutdown.load(Ordering::SeqCst) == SHUTDOWN {
            debug!("[mpv/render] shutdown signal received before wait");
            break;
        }

        // Wait for mpv to signal a new frame.
        //
        // The timeout is intentional: it prevents a missed dummy wake from keeping the
        // render thread blocked forever during application shutdown.
        let should_render = match rx.recv_timeout(Duration::from_millis(250)) {
            Ok(value) => value,
            Err(RecvTimeoutError::Timeout) => {
                continue;
            }
            Err(RecvTimeoutError::Disconnected) => {
                debug!("[mpv/render] render update channel disconnected");
                break;
            }
        };

        if shutdown.load(Ordering::SeqCst) == SHUTDOWN {
            debug!("[mpv/render] shutdown signal received after wake");
            break;
        }

        // `false` is a dummy wake used by shutdown paths. Do not render a frame for it.
        if !should_render {
            continue;
        }

        let (width, height) = surface.drawable_size();
        if width <= 0 || height <= 0 {
            debug!(
                "[mpv/render] skipping frame because drawable size is invalid: {}x{}",
                width, height
            );
            continue;
        }

        // SAFETY: The GL context is current on this render thread.
        let fbo = unsafe { glow_ctx.get_parameter_i32(glow::FRAMEBUFFER_BINDING) };

        // SAFETY: The GL context is current, `fbo` is read from current GL state, and the
        // dimensions are positive.
        match unsafe { render_ctx.render::<()>(fbo, width, height, true) } {
            Ok(()) => {
                surface.swap_buffers();

                frame_count = frame_count.saturating_add(1);

                if frame_count == 1 {
                    info!("[mpv/render] first frame rendered ({}x{})", width, height);
                } else if frame_count % 300 == 0 {
                    debug!(
                        "[mpv/render] frame {} rendered ({}x{})",
                        frame_count, width, height
                    );
                }
            }
            Err(e) => {
                warn!(
                    "[mpv/render] render failed; will retry on next frame update: {}",
                    e
                );
            }
        }
    }

    // Drop the render context before the surface guard clears the thread-local pointer.
    drop(render_ctx);

    info!(
        "[mpv/render] render thread exited cleanly ({} frames rendered)",
        frame_count
    );
}

// ---------------------------------------------------------------------------
// Shutdown helper.
// ---------------------------------------------------------------------------

/// Wake the render thread so it can observe the shutdown signal.
///
/// This should be called after setting `RenderThreadState.shutdown` to `SHUTDOWN`.
pub fn wake_render_thread() {
    let _ = RENDER_UPDATE.tx.send(false);
}
