//! libmpv GL render context + render loop — ports tsukimi's `mpvglarea.rs` pattern.
//!
//! PORTED_FROM_TSUKIMI @ v26.6.3 (`src/ui/mpv/mpvglarea.rs`)
//!
//! Adapted for fyom:
//! - tsukimi's GTK `GLArea` shell (the `MPVGLArea` widget subclass + `WidgetImpl::realize` /
//!   `GLAreaImpl::render` / `glib::spawn_future_local` render-wake loop) is **dropped** —
//!   fyom owns its own per-platform GL surface (see `crate::platform`) + a dedicated render
//!   thread (instead of GTK's `queue_render` future).
//! - The `RenderContext::new` + `set_update_callback` + `render::<C>(fbo, w, h, true)`
//!   pattern is **ported verbatim** (libmpv2 4.1 API, identical to tsukimi's usage). The
//!   `OpenGLInitParams<C>` generic context type is preserved (fyom uses `C = ()` + a
//!   thread-local to find the surface, since the render thread is the sole GL consumer).
//! - `glow::Context::from_loader_function(get_proc_address)` + reading the current FBO via
//!   `glow::get_parameter_i32(glow::FRAMEBUFFER_BINDING)` is **ported verbatim** from
//!   tsukimi's `glow_cxt()` + `render()` body. The `get_proc_address` fn comes from the
//!   platform surface (macOS: dlsym → OpenGL framework; Windows: wglGetProcAddress; Linux:
//!   glXGetProcAddressARB).
//! - `RENDER_UPDATE` flume channel is **ported verbatim** from tsukimi's
//!   `static RENDER_UPDATE: LazyLock<Channel<bool>>` — the update_callback sends `true`,
//!   the render thread wakes on `rx.recv()`.
//! - HiDPI: `width * scale_factor` + `height * scale_factor` (physical pixels) — ported
//!   verbatim from tsukimi's `scale_factor()` multiplication. fyom's `RenderSurface::
//!   drawable_size()` already returns physical pixels.
//!
//! See `docs/libmpv-assessment.md` §3.1 + §3.3 + §3.4 for the rationale + reuse inventory.

use std::ffi::c_void;
use std::sync::Arc;
use std::sync::atomic::{AtomicU32, Ordering};
use std::sync::Mutex;
use std::thread::JoinHandle;

use flume::{Receiver, Sender, unbounded};
use glow::HasContext;
use libmpv2::Mpv;
use libmpv2::render::{OpenGLInitParams, RenderContext, RenderParam, RenderParamApiType};
use tracing::{debug, error, info, warn};

use crate::mpv::event_loop::SHUTDOWN;

// ---------------------------------------------------------------------------
// Render-wake channel (ported verbatim from tsukimi's `RENDER_UPDATE`).
// ---------------------------------------------------------------------------

/// A flume pair signaling "mpv has a new frame ready, please render".
///
/// Ported from tsukimi's `static RENDER_UPDATE: LazyLock<Channel<bool>>`. The sender is
/// fed by `RenderContext::set_update_callback` (fires from mpv's internal threads); the
/// receiver is consumed by fyom's render thread.
struct RenderUpdateChannel {
    /// Fed by the mpv render update callback (fires from mpv's internal threads).
    tx: Sender<bool>,
    /// Consumed by fyom's render thread.
    #[allow(dead_code)] // only read by the render thread, which is spawned lazily
    rx: Receiver<bool>,
}

/// The global render-wake channel. `LazyLock` so the `set_update_callback` closure (which
/// must be `Send + Sync + 'static`) can reference it without capturing context.
static RENDER_UPDATE: std::sync::LazyLock<RenderUpdateChannel> =
    std::sync::LazyLock::new(|| {
        let (tx, rx) = unbounded::<bool>();
        RenderUpdateChannel { tx, rx }
    });

// ---------------------------------------------------------------------------
// RenderSurface — the contract between render.rs + platform/{macos,windows,default}.rs
// ---------------------------------------------------------------------------

/// The platform GL surface contract: provides the GL function-pointer loader + drawable
/// dimensions + buffer swap, owned by the render thread.
///
/// Implemented per-platform in `crate::platform::{macos,windows,default}`:
/// - macOS: `NSOpenGLContext` + `NSOpenGLView` child behind `WKWebView`
/// - Windows: child `HWND` + WGL context (`wglCreateContext` + `wglMakeCurrent`)
/// - Linux: child X11 `Window` + GLX context (`glXCreateContext` + `glXMakeCurrent`),
///   XWayland fallback for v1 (works under both X11 + Wayland via XWayland)
///
/// The render thread calls `make_current` once at startup, then `drawable_size` +
/// `swap_buffers` on each frame. `make_current` is idempotent (the platform impl caches
/// the current-context state to avoid redundant NSOpenGL/WGL/GLX calls).
pub trait RenderSurface: Send + 'static {
    /// Make the GL context current on the calling thread. Called once at render-thread
    /// start. The render thread is the sole consumer of this context — no concurrent
    /// `make_current` from other threads.
    fn make_current(&self) -> Result<(), String>;

    /// Resolve a GL function pointer by name. Used by:
    /// - `libmpv2::render::OpenGLInitParams::get_proc_address` (mpv's internal GL loader,
    ///   which expects `*mut c_void` — matches mpv's C `mpv_opengl_get_proc_address_fn`)
    /// - `glow::Context::from_loader_function` (fyom's GL wrapper for FBO queries; glow
    ///   takes `*const c_void` — the render loop casts `*mut` → `*const`)
    ///
    /// Returns a null pointer if the function is not found (mpv will fail render-context
    /// creation; glow will panic on first use — both are non-recoverable surface-init bugs).
    fn get_proc_address(&self, name: &str) -> *mut c_void;

    /// The current drawable dimensions in physical pixels (post scale-factor multiplication).
    /// Polled on every render frame so resize is handled without an explicit event channel.
    fn drawable_size(&self) -> (i32, i32);

    /// Swap the front/back buffers (present the rendered frame). Called after each
    /// `RenderContext::render`. No-op for single-buffered contexts.
    fn swap_buffers(&self);
}

// ---------------------------------------------------------------------------
// Render-thread state (held by MpvInstance for lifecycle management)
// ---------------------------------------------------------------------------

/// Shared render-thread state. Held by `MpvInstance` so the Tauri command layer can
/// signal shutdown + the RunEvent::Exit handler can join the thread.
pub struct RenderThreadState {
    /// The render-thread handle (`Some` after `spawn_render_thread`, cleared on shutdown).
    pub handle: Mutex<Option<JoinHandle<()>>>,
    /// Shutdown signal — set to `SHUTDOWN` by `shutdown_render_thread`.
    pub shutdown: Arc<AtomicU32>,
}

impl RenderThreadState {
    pub fn new() -> Self {
        Self {
            handle: Mutex::new(None),
            shutdown: Arc::new(AtomicU32::new(0)), // 0 = running; SHUTDOWN = stop
        }
    }
}

impl Default for RenderThreadState {
    fn default() -> Self {
        Self::new()
    }
}

// ---------------------------------------------------------------------------
// Thread-local surface pointer (bridges libmpv2's fn-pointer callback to fyom's method)
// ---------------------------------------------------------------------------

// libmpv2 4.1's `OpenGLInitParams<C>` has `get_proc_address: fn(&C, &str) -> *const c_void`
// (a plain fn pointer, not a closure — matches the C `mpv_opengl_get_proc_address_fn`
// signature). To pass the `RenderSurface` reference through, we use a thread-local: the
// render thread sets it once at startup, + the `get_proc_address` thunk reads it.
//
// SAFETY: The thread-local is set only on the render thread, only during `run_render_loop`,
// + cleared before the thread exits. The surface is owned by `run_render_loop`'s stack
// frame, which outlives the RenderContext (the RenderContext is dropped before the surface).
thread_local! {
    static CURRENT_SURFACE: std::cell::RefCell<Option<*const dyn RenderSurface>> =
        const { std::cell::RefCell::new(None) };
}

/// The `fn(&(), &str) -> *mut c_void` thunk passed to `OpenGLInitParams`.
///
/// Matches tsukimi's `fn get_proc_address(_ctx: &GLContext, name: &str) -> *mut c_void`
/// signature pattern (fyom uses `C = ()` because the surface is found via the thread-local
/// instead of through the ctx argument). Returns `*mut c_void` to match mpv's C
/// `mpv_opengl_get_proc_address_fn` signature (`void* (*get_proc_address)(void*, const char*)`).
fn get_proc_address_thunk(_ctx: &(), name: &str) -> *mut c_void {
    CURRENT_SURFACE.with(|s| {
        let s = s.borrow();
        let surface_ptr = s.expect("CURRENT_SURFACE must be set before get_proc_address");
        // SAFETY: surface_ptr points to the Box<dyn RenderSurface> owned by
        // run_render_loop's stack frame; valid for the duration of this thread.
        unsafe { (**surface_ptr).get_proc_address(name) }
    })
}

// ---------------------------------------------------------------------------
// Render-thread spawn (the core port — tsukimi's `setup_mpv` + `render` + wake loop)
// ---------------------------------------------------------------------------

/// Spawn the mpv render thread.
///
/// The thread:
/// 1. Makes the platform GL context current (sole owner — no concurrent GL calls).
/// 2. Creates `libmpv2::render::RenderContext` with `RenderParamApiType::OpenGl` +
///    `OpenGLInitParams { get_proc_address, ctx: () }` (the tsukimi pattern, with `C = ()`
///    because fyom finds the surface via the thread-local).
/// 3. Registers `set_update_callback(|| { let _ = RENDER_UPDATE.tx.send(true); })` —
///    mpv calls this from its internal threads when a new frame is ready.
/// 4. Loops: `RENDER_UPDATE.rx.recv()` → `glow::Context::from_loader_function` →
///    `glow::get_parameter_i32(FRAMEBUFFER_BINDING)` → `render_ctx.render::<()>(fbo, w, h, true)`
///    → `surface.swap_buffers()`.
/// 5. Exits when `shutdown` is set to `SHUTDOWN` (checked on each wake).
///
/// Returns the `JoinHandle` so the caller can join on shutdown.
///
/// # Errors
/// Returns `Err` only if the OS fails to spawn the thread (rare). `RenderContext::new`
/// failures are logged + the thread exits cleanly (the 9.7 `<video>` fallback stays green).
///
/// # Panics
/// Does not panic — all mpv + GL errors are logged + the thread exits cleanly.
pub fn spawn_render_thread(
    mpv: Arc<Mpv>,
    surface: Box<dyn RenderSurface>,
    shutdown: Arc<AtomicU32>,
) -> Result<JoinHandle<()>, String> {
    let handle = std::thread::Builder::new()
        .name("fyom mpv render loop".into())
        .spawn(move || {
            run_render_loop(mpv, surface, shutdown);
        })
        .map_err(|e| format!("failed to spawn render thread: {}", e))?;

    Ok(handle)
}

/// The render-loop body (runs on the render thread).
///
/// Split out from `spawn_render_thread` so `?` inside doesn't propagate `Err` across the
/// thread boundary (which would lose the error context). All errors are logged + the
/// thread exits cleanly.
fn run_render_loop(mpv: Arc<Mpv>, surface: Box<dyn RenderSurface>, shutdown: Arc<AtomicU32>) {
    // STEP 1: make the GL context current on this thread. All subsequent GL calls
    // (RenderContext::new, glow, render) run on this thread — sole owner.
    if let Err(e) = surface.make_current() {
        error!(
            "[mpv/render] failed to make GL context current: {} — GL rendering disabled \
             (video stays a black frame, audio + <video> fallback unaffected)",
            e
        );
        return;
    }
    info!("[mpv/render] GL context made current on render thread");

    // Set the thread-local so the `get_proc_address` thunk + glow loader can find the
    // surface. This must be set BEFORE `RenderContext::new` (mpv calls get_proc_address
    // during context creation to load GL function pointers).
    //
    // SAFETY: see the thread-local safety note above.
    let surface_ptr: *const dyn RenderSurface = &*surface;
    CURRENT_SURFACE.with(|s| *s.borrow_mut() = Some(surface_ptr));

    // STEP 2: create the mpv render context.
    //
    // PORTED_FROM_TSUKIMI `setup_mpv`:
    //   let mut render_params = vec![
    //       RenderParam::ApiType(RenderParamApiType::OpenGl),
    //       RenderParam::InitParams(OpenGLInitParams { get_proc_address, ctx: gl_context }),
    //   ];
    //   let mut handle = tmpv.mpv.ctx;
    //   let mut ctx = RenderContext::new(unsafe { handle.as_mut() }, render_params)
    //       .expect("Failed creating render context");
    //
    // fyom adaptation: `C = ()` (the surface is found via the thread-local, not the ctx
    // argument); `get_proc_address` is the `get_proc_address_thunk` fn item.
    let render_params = vec![
        RenderParam::ApiType(RenderParamApiType::OpenGl),
        RenderParam::InitParams(OpenGLInitParams {
            get_proc_address: get_proc_address_thunk,
            ctx: (),
        }),
    ];

    let mut handle = mpv.ctx;
    let render_ctx = match unsafe { RenderContext::new(handle.as_mut(), render_params) } {
        Ok(ctx) => ctx,
        Err(e) => {
            error!(
                "[mpv/render] RenderContext::new failed: {} — GL rendering disabled \
                 (video stays a black frame, audio + <video> fallback unaffected)",
                e
            );
            CURRENT_SURFACE.with(|s| *s.borrow_mut() = None);
            return;
        }
    };
    info!("[mpv/render] RenderContext created (OpenGL, libmpv2 4.1)");

    // STEP 3: register the update callback.
    //
    // PORTED_FROM_TSUKIMI:
    //   ctx.set_update_callback(|| { let _ = RENDER_UPDATE.tx.send(true); });
    //
    // The closure is `Send + Sync + 'static` (flume::Sender is Send+Sync; the closure
    // captures only the global `RENDER_UPDATE.tx` via the LazyLock). mpv fires it from
    // internal threads when a new frame is ready.
    render_ctx.set_update_callback(|| {
        let _ = RENDER_UPDATE.tx.send(true);
    });

    // STEP 4: build the glow context (for FBO queries — tsukimi's `glow_cxt()`).
    //
    // PORTED_FROM_TSUKIMI:
    //   fn glow_cxt(&self) -> &glow::Context {
    //       self.ctx.get_or_init(|| unsafe {
    //           glow::Context::from_loader_function(epoxy::get_proc_addr)
    //       })
    //   }
    //
    // fyom uses the thread-local surface's `get_proc_address` instead of `epoxy::get_proc_addr`.
    // glow's `from_loader_function` takes `*const c_void`; our surface returns `*mut c_void`
    // (matching libmpv2's expectation). Cast `*mut` → `*const` (lossless — both are the same
    // pointer, just different mutability qualifiers).
    let glow_ctx = unsafe {
        glow::Context::from_loader_function(|name| {
            CURRENT_SURFACE.with(|s| {
                let s = s.borrow();
                let surface_ptr =
                    s.expect("CURRENT_SURFACE must be set before glow loader calls");
                // SAFETY: see the thread-local safety note above.
                unsafe { (**surface_ptr).get_proc_address(name) as *const c_void }
            })
        })
    };

    // STEP 5: the render loop.
    //
    // PORTED_FROM_TSUKIMI:
    //   glib::spawn_future_local(glib::clone!(#[weak] obj, async move {
    //       while RENDER_UPDATE.rx.recv_async().await.is_ok() {
    //           obj.queue_render();
    //       }
    //   }));
    //
    //   fn render(&self, _context: &GLContext) -> glib::Propagation {
    //       let binding = self.mpv().ctx.borrow();
    //       let Some(ctx) = binding.as_ref() else { return glib::Propagation::Stop; };
    //       let factor = self.obj().scale_factor();
    //       let width = self.obj().width() * factor;
    //       let height = self.obj().height() * factor;
    //       unsafe {
    //           let fbo = self.glow_cxt().get_parameter_i32(glow::FRAMEBUFFER_BINDING);
    //           ctx.render::<GLContext>(fbo, width, height, true).unwrap();
    //       }
    //       glib::Propagation::Stop
    //   }
    //
    // fyom adaptation:
    // - `glib::spawn_future_local` + `obj.queue_render()` → fyom's dedicated render thread
    //   blocking on `RENDER_UPDATE.rx.recv()`.
    // - `glow::get_parameter_i32(FRAMEBUFFER_BINDING)` + `ctx.render::<C>(fbo, w, h, true)`
    //   → ported verbatim (fyom uses `C = ()`).
    // - `width * factor` + `height * factor` → fyom uses `surface.drawable_size()` which
    //   already returns physical pixels.

    let rx = &RENDER_UPDATE.rx;
    let mut frame_count: u64 = 0;

    loop {
        // Check shutdown before blocking on recv (so shutdown is responsive even when
        // mpv isn't producing frames — e.g. when paused or stopped).
        if shutdown.load(Ordering::SeqCst) == SHUTDOWN {
            debug!("[mpv/render] shutdown signal received, exiting render loop");
            break;
        }

        // Block until mpv signals a new frame (or `wake_render_thread` sends a dummy).
        match rx.recv() {
            Ok(_) => {
                // Render one frame.
            }
            Err(flume::RecvError::Disconnected) => {
                debug!("[mpv/render] RENDER_UPDATE channel disconnected, exiting");
                break;
            }
        }

        // Re-check shutdown after wake (a dummy send on shutdown avoids a dangling wake).
        if shutdown.load(Ordering::SeqCst) == SHUTDOWN {
            break;
        }

        // Render the frame.
        let (width, height) = surface.drawable_size();
        if width <= 0 || height <= 0 {
            // Drawable not yet sized (window minimized / not yet mapped). Skip this frame;
            // mpv will signal again when it has a new frame.
            continue;
        }

        // Read the current FBO binding (tsukimi's `glow::get_parameter_i32(FRAMEBUFFER_BINDING)`).
        // For fyom's per-platform contexts, the default framebuffer is 0 (the window's
        // client area), but reading the binding is render-backend-agnostic + matches tsukimi.
        let fbo = unsafe { glow_ctx.get_parameter_i32(glow::FRAMEBUFFER_BINDING) };

        // Render the frame (tsukimi's `ctx.render::<GLContext>(fbo, w, h, true)`).
        //
        // VERIFY(2.3): The `render` method's type parameter `C` is a phantom marker for the
        // GL context type. tsukimi uses `render::<GLContext>` (GTK's gdk::GLContext); fyom
        // uses `render::<()>` (no context type — the surface is found via the thread-local).
        // If libmpv2 4.1's `render<C: RenderCb>` has a trait bound that `()` doesn't satisfy,
        // use a different type (e.g. a unit struct `struct FyomGlCtx;` impl RenderCb). The
        // rest of the call (fbo, width, height, flip=true) matches tsukimi verbatim.
        //
        // SAFETY: the GL context is current on this thread; `fbo` was just read from the
        // GL state; `width`/`height` are positive (checked above).
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
                    "[mpv/render] render failed (non-fatal, will retry next frame): {}",
                    e
                );
            }
        }
    }

    // STEP 6: cleanup. Drop the RenderContext (releases mpv's internal GL resources) +
    // clear the thread-local.
    //
    // SAFETY: `render_ctx` is dropped here (mpv_render_context_free); no concurrent render
    // calls are in flight (we just broke out of the loop).
    drop(render_ctx);
    CURRENT_SURFACE.with(|s| *s.borrow_mut() = None);
    info!(
        "[mpv/render] render thread exited cleanly ({} frames rendered)",
        frame_count
    );
}

// ---------------------------------------------------------------------------
// Shutdown helper — wakes the render thread so it can observe the shutdown signal.
// ---------------------------------------------------------------------------

/// Signal the render thread to wake + observe the shutdown signal.
///
/// Called by `MpvInstance::shutdown_render_thread` after setting `shutdown = SHUTDOWN`.
/// Without this, the render thread would block on `RENDER_UPDATE.rx.recv()` until the next
/// mpv frame (which never comes after mpv stops).
pub fn wake_render_thread() {
    let _ = RENDER_UPDATE.tx.send(false);
}
