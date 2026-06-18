//! Linux GL surface — child X11 `Window` + GLX context behind the WebView host.
//!
//! PORTED_FROM_SOIA `src-tauri/src/platform/default.rs` (direction only — soia uses
//! `libsoia_utils`'s closed-source surface; fyom writes the open GLX path. The
//! window-lifecycle logic direction is ported; the surface code is rewritten.)
//!
//! ## Architecture
//! Tauri's main window on Linux contains an X11 `Window` (or a Wayland surface via
//! `wl_surface`). fyom creates a child X11 `Window` that sits behind the webview in
//! z-order. When the webview's root CSS goes `background: transparent !important` (the
//! `.video-mode` class, ported from soia), the mpv GL render shows through.
//!
//! The GLX context is created via `glXCreateContext` + `glXMakeCurrent`.
//! `get_proc_address` resolves GL function pointers via `glXGetProcAddressARB`.
//!
//! ## Wayland
//! Under Wayland (without XWayland), fyom cannot create a child X11 window — Wayland
//! surfaces are not X11 windows. The fallback is XWayland: Tauri's WebView2/GTK window
//! runs under XWayland when the display server is Wayland, so a child X11 window works.
//! Native Wayland + EGL is Phase 2.5+.
//!
//! See `docs/libmpv-assessment.md` §3.3 + R2 for the rationale.

use std::ffi::{CString, c_void};
use std::sync::Mutex;

use x11_dl::glx::GLXContext as GlxContext;
use x11_dl::xlib::{
    AllocNone, CWColormap, CWEventMask, Colormap, Display as XDisplay, False, InputOutput, True,
    Window as XWindow, XErrorEvent, XSetWindowAttributes, XVisualInfo, XWindowAttributes,
};

use crate::mpv::render::RenderSurface;

// ---------------------------------------------------------------------------
// Helpers.
// ---------------------------------------------------------------------------

/// Convert the `x11-dl` GLX proc-address result into the `void*` shape expected by
/// libmpv/glow.
///
/// In current `x11-dl`, `glXGetProcAddressARB` returns:
///
/// `Option<unsafe extern "C" fn()>`
///
/// That value is a function pointer wrapped in `Option`, not a raw pointer, so this must
/// not be written as `ptr as *mut c_void`. Instead, unwrap the option and cast the function
/// pointer itself to a raw address.
#[inline]
fn glx_proc_address_to_void_ptr(proc_addr: Option<unsafe extern "C" fn()>) -> *mut c_void {
    match proc_addr {
        Some(func) => func as *const () as *mut c_void,
        None => std::ptr::null_mut(),
    }
}

/// Free an X11 `XVisualInfo*` returned by GLX/Xlib.
///
/// Kept as a helper to make all early-return cleanup paths consistent.
#[inline]
unsafe fn free_visual_info(xlib: &x11_dl::xlib::Xlib, visual_info: *mut XVisualInfo) {
    if !visual_info.is_null() {
        // SAFETY: `visual_info` was allocated by Xlib/GLX and must be released with XFree.
        unsafe {
            (xlib.XFree)(visual_info as *mut c_void);
        }
    }
}

// ---------------------------------------------------------------------------
// X11GlxSurface — the RenderSurface impl.
// ---------------------------------------------------------------------------

/// The Linux GL surface: owns a child X11 `Window` + GLX context.
///
/// `Send` because the X11 `Display*` is documented thread-safe when `XInitThreads` has
/// been called. GTK/Tauri normally initializes Xlib threading before windows are created.
/// GLX's `glXMakeCurrent` binds the context to the calling render thread.
pub struct X11GlxSurface {
    /// The X11 display connection (`Display*`). Owned by the surface and closed on drop.
    ///
    /// In practice this points at the same X server as Tauri/GTK, but fyom opens its own
    /// connection to avoid sharing Xlib connection state with GTK.
    display: *mut XDisplay,

    /// The child X11 window (XID). Created as a child of the Tauri window's X11 window.
    window: XWindow,

    /// Colormap created for the GLX visual. Freed on drop.
    colormap: Colormap,

    /// The GLX rendering context. Made current on the render thread via
    /// `glXMakeCurrent(display, window, context)`.
    context: GlxContext,

    /// Cached current-context state. The intended sole caller is the render thread.
    make_current_called: Mutex<bool>,

    /// The `x11_dl::xlib::Xlib` dynamic library handle. Kept alive for the lifetime of
    /// the surface because the function pointers are owned by this handle.
    _xlib: x11_dl::xlib::Xlib,

    /// The `x11_dl::glx::Glx` dynamic library handle. Kept alive for the lifetime of
    /// the surface because the function pointers are owned by this handle.
    _glx: x11_dl::glx::Glx,
}

// SAFETY: The X11 Display* and GLXContext are process-global handles. With Xlib threading
// initialized by GTK/Tauri, Xlib calls are guarded internally. The render thread is the
// sole GL consumer and binds the GLX context with `glXMakeCurrent`.
unsafe impl Send for X11GlxSurface {}

impl RenderSurface for X11GlxSurface {
    fn make_current(&self) -> Result<(), String> {
        let mut called = self
            .make_current_called
            .lock()
            .map_err(|e| format!("make_current mutex poisoned: {}", e))?;

        if *called {
            return Ok(());
        }

        // SAFETY: `display`, `window`, and `context` are valid handles owned by this
        // surface. `glXMakeCurrent` binds the GLX context to the calling thread.
        let ok = unsafe { (self._glx.glXMakeCurrent)(self.display, self.window, self.context) };

        if ok == False as i32 {
            return Err("glXMakeCurrent failed".to_string());
        }

        *called = true;
        Ok(())
    }

    fn get_proc_address(&self, name: &str) -> *mut c_void {
        let c_name = match CString::new(name) {
            Ok(c_name) => c_name,
            Err(_) => return std::ptr::null_mut(),
        };

        // SAFETY: `glXGetProcAddressARB` is the documented GLX API. In `x11-dl`, the
        // return type is `Option<unsafe extern "C" fn()>`, not a raw pointer.
        unsafe {
            let proc_addr = (self._glx.glXGetProcAddressARB)(c_name.as_ptr() as *const u8);
            glx_proc_address_to_void_ptr(proc_addr)
        }
    }

    fn drawable_size(&self) -> (i32, i32) {
        let mut root_return: XWindow = 0;
        let mut x_return = 0;
        let mut y_return = 0;
        let mut width_return = 0u32;
        let mut height_return = 0u32;
        let mut border_width_return = 0u32;
        let mut depth_return = 0u32;

        // SAFETY: `display` and `window` are valid handles owned by this surface.
        let ok = unsafe {
            (self._xlib.XGetGeometry)(
                self.display,
                self.window,
                &mut root_return,
                &mut x_return,
                &mut y_return,
                &mut width_return,
                &mut height_return,
                &mut border_width_return,
                &mut depth_return,
            )
        };

        if ok == False as i32 {
            return (0, 0);
        }

        (
            width_return.min(i32::MAX as u32) as i32,
            height_return.min(i32::MAX as u32) as i32,
        )
    }

    fn swap_buffers(&self) {
        // SAFETY: `display` and `window` are valid handles owned by this surface.
        unsafe {
            (self._glx.glXSwapBuffers)(self.display, self.window);
        }
    }
}

impl Drop for X11GlxSurface {
    fn drop(&mut self) {
        // SAFETY: All resources here are owned by this surface. Destruction order matters:
        // clear/destroy GLX context, destroy X11 window, free colormap, close display.
        unsafe {
            if !self.display.is_null() {
                let current = (self._glx.glXGetCurrentContext)();

                if current == self.context {
                    let _ = (self._glx.glXMakeCurrent)(self.display, 0, std::ptr::null_mut());
                }

                if !self.context.is_null() {
                    (self._glx.glXDestroyContext)(self.display, self.context);
                }

                if self.window != 0 {
                    (self._xlib.XDestroyWindow)(self.display, self.window);
                }

                if self.colormap != 0 {
                    (self._xlib.XFreeColormap)(self.display, self.colormap);
                }

                (self._xlib.XCloseDisplay)(self.display);
            }
        }
    }
}

// ---------------------------------------------------------------------------
// Factory — create the surface from a Tauri WebviewWindow.
// ---------------------------------------------------------------------------

/// Create the Linux GL surface for the given Tauri window.
///
/// Steps:
/// 1. Get the parent X11 window from the Tauri window's raw window handle.
/// 2. Open a dedicated X11 display connection (`XOpenDisplay(NULL)`).
/// 3. Choose a double-buffered RGBA GLX visual with alpha + depth.
/// 4. Create the child X11 window as a child of the Tauri parent window.
/// 5. Position the child behind the webview (`XLowerWindow`).
/// 6. Create the GLX context (`glXCreateContext`).
///
/// On any failure, returns `Err`; the caller should log and continue without GL rendering.
///
/// # Wayland
/// Under native Wayland without XWayland, `XOpenDisplay(NULL)` returns NULL or the raw
/// handle is not Xlib. The caller logs the error and continues without GL rendering.
/// Native Wayland + EGL is Phase 2.5+.
pub fn create_surface(window: &tauri::WebviewWindow) -> Result<Box<dyn RenderSurface>, String> {
    use raw_window_handle::{HasWindowHandle, RawWindowHandle};

    // STEP 1: get the parent X11 window from Tauri's raw window handle.
    let raw_handle = window
        .window_handle()
        .map_err(|e| format!("failed to get raw window handle: {}", e))?;

    let xlib_handle = match raw_handle.as_ref() {
        RawWindowHandle::Xlib(handle) => handle,
        _ => {
            return Err("expected Xlib raw window handle on Linux \
                 (native Wayland is not supported yet; XWayland fallback required)"
                .to_string());
        }
    };

    let parent_window = xlib_handle.window as XWindow;
    if parent_window == 0 {
        return Err("parent X11 window is 0".to_string());
    }

    // Load the Xlib and GLX dynamic libraries.
    let xlib = x11_dl::xlib::Xlib::open().map_err(|e| format!("failed to load libX11: {}", e))?;
    let glx = x11_dl::glx::Glx::open().map_err(|e| format!("failed to load libGL: {}", e))?;

    // STEP 2: open a dedicated X11 display connection.
    //
    // SAFETY: `XOpenDisplay(NULL)` opens the display from $DISPLAY. It returns NULL when
    // no X server is available, such as native Wayland without XWayland.
    let display = unsafe { (xlib.XOpenDisplay)(std::ptr::null()) };
    if display.is_null() {
        return Err("XOpenDisplay returned null \
             (no X server available; native Wayland without XWayland?)"
            .to_string());
    }

    // Install a forgiving X error handler so async X errors do not terminate the process.
    // The default Xlib handler may abort/exit on some fatal-looking errors. We only log;
    // creation APIs below still return explicit failure where possible.
    unsafe extern "C" fn fyom_x_error_handler(
        _display: *mut XDisplay,
        _error: *mut XErrorEvent,
    ) -> i32 {
        tracing::warn!("[platform/linux] X11 error received (non-fatal)");
        0
    }

    // SAFETY: `XSetErrorHandler` is process-global Xlib API. The handler has the required
    // C ABI and returns 0 to indicate the error was handled.
    unsafe {
        (xlib.XSetErrorHandler)(Some(fyom_x_error_handler));
    }

    // STEP 3: choose a double-buffered RGBA GLX visual with alpha + depth.
    //
    // GLX attribute list is terminated by 0 (`None` in X11 terminology).
    let mut attributes = vec![
        x11_dl::glx::GLX_RGBA as i32,
        x11_dl::glx::GLX_RED_SIZE as i32,
        8,
        x11_dl::glx::GLX_GREEN_SIZE as i32,
        8,
        x11_dl::glx::GLX_BLUE_SIZE as i32,
        8,
        x11_dl::glx::GLX_ALPHA_SIZE as i32,
        8,
        x11_dl::glx::GLX_DEPTH_SIZE as i32,
        24,
        x11_dl::glx::GLX_DOUBLEBUFFER as i32,
        0,
    ];

    // SAFETY: `glXChooseVisual` returns an `XVisualInfo*` or NULL. The pointer must be
    // freed with `XFree` after use.
    let visual_info = unsafe { (glx.glXChooseVisual)(display, 0, attributes.as_mut_ptr()) };
    if visual_info.is_null() {
        unsafe {
            (xlib.XCloseDisplay)(display);
        }
        return Err("glXChooseVisual returned null (no matching GLX visual)".to_string());
    }

    // SAFETY: `visual_info` is non-null and points to a valid `XVisualInfo` returned by GLX.
    let visual_info_ref: &XVisualInfo = unsafe { &*visual_info };

    // Get parent window attributes to size the child window to the current client area.
    let mut parent_attrs: XWindowAttributes = unsafe { std::mem::zeroed() };

    // SAFETY: `display` is valid and `parent_window` is an XID from the same X server.
    let ok = unsafe { (xlib.XGetWindowAttributes)(display, parent_window, &mut parent_attrs) };
    if ok == False as i32 {
        unsafe {
            free_visual_info(&xlib, visual_info);
            (xlib.XCloseDisplay)(display);
        }
        return Err("XGetWindowAttributes(parent) failed".to_string());
    }

    // Create a colormap matching the GLX visual.
    //
    // SAFETY: `visual_info_ref.visual` is the visual selected by GLX.
    let colormap = unsafe {
        (xlib.XCreateColormap)(display, parent_window, visual_info_ref.visual, AllocNone)
    };

    if colormap == 0 {
        unsafe {
            free_visual_info(&xlib, visual_info);
            (xlib.XCloseDisplay)(display);
        }
        return Err("XCreateColormap failed".to_string());
    }

    // STEP 4: create the child X11 window.
    let mut set_window_attrs: XSetWindowAttributes = unsafe { std::mem::zeroed() };
    set_window_attrs.colormap = colormap;
    set_window_attrs.event_mask = 0;
    set_window_attrs.override_redirect = False as i32;

    let value_mask = (CWColormap | CWEventMask) as u64;

    let width = parent_attrs.width.max(1) as u32;
    let height = parent_attrs.height.max(1) as u32;

    // SAFETY: `XCreateWindow` creates a child window using the GLX-compatible visual and
    // colormap. The child is sized to the parent client area.
    let child_window = unsafe {
        (xlib.XCreateWindow)(
            display,
            parent_window,
            0,
            0,
            width,
            height,
            0,
            visual_info_ref.depth,
            InputOutput as u32,
            visual_info_ref.visual,
            value_mask,
            &mut set_window_attrs,
        )
    };

    if child_window == 0 {
        unsafe {
            (xlib.XFreeColormap)(display, colormap);
            free_visual_info(&xlib, visual_info);
            (xlib.XCloseDisplay)(display);
        }
        return Err("XCreateWindow for child window failed".to_string());
    }

    // Map the child window so it becomes visible.
    //
    // SAFETY: `child_window` is a valid X11 window created above.
    unsafe {
        (xlib.XMapWindow)(display, child_window);
    }

    // STEP 5: lower the child window behind the webview/sibling stack.
    //
    // SAFETY: `child_window` is valid.
    unsafe {
        (xlib.XLowerWindow)(display, child_window);
    }

    // Flush the X connection to ensure the window is created and lowered before GLX setup
    // continues.
    //
    // SAFETY: `display` is valid.
    unsafe {
        (xlib.XFlush)(display);
    }

    // STEP 6: create the GLX context.
    //
    // SAFETY: `visual_info` was returned by `glXChooseVisual` for this display.
    let context =
        unsafe { (glx.glXCreateContext)(display, visual_info, std::ptr::null_mut(), True as i32) };

    if context.is_null() {
        unsafe {
            (xlib.XDestroyWindow)(display, child_window);
            (xlib.XFreeColormap)(display, colormap);
            free_visual_info(&xlib, visual_info);
            (xlib.XCloseDisplay)(display);
        }
        return Err("glXCreateContext returned null".to_string());
    }

    let visual_depth = visual_info_ref.depth;

    // `visual_info_ref` must not be used after this point.
    unsafe {
        free_visual_info(&xlib, visual_info);
    }

    tracing::info!(
        "[platform/linux] child X11 window + GLX context created behind webview \
         ({}x{}, depth {}, direct rendering)",
        width,
        height,
        visual_depth
    );

    Ok(Box::new(X11GlxSurface {
        display,
        window: child_window,
        colormap,
        context,
        make_current_called: Mutex::new(false),
        _xlib: xlib,
        _glx: glx,
    }))
}
