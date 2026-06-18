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
//! surfaces are not X11 windows. The fallback is **XWayland**: Tauri's WebView2/GTK
//! window runs under XWayland when the display server is Wayland, so a child X11 window
//! works (R2 in the assessment's risk register). Native Wayland + EGL is Phase 2.5+.
//!
//! See `docs/libmpv-assessment.md` §3.3 + R2 for the rationale.

use std::ffi::{CString, c_void};
use std::sync::Mutex;

use x11_dl::xlib::{
    Display as XDisplay, GLXContext as GlxContext, Window as XWindow,
    XErrorEvent, XIOErrorHandler, XErrorHandler,
};
use x11_dl::xlib::{
    BadAlloc, BadMatch, BadWindow, False, True,
};

use crate::mpv::render::RenderSurface;

// ---------------------------------------------------------------------------
// X11GlxSurface — the RenderSurface impl.
// ---------------------------------------------------------------------------

/// The Linux GL surface: owns a child X11 `Window` + GLX context.
///
/// `Send` because the X11 `Display*` is documented thread-safe (Xlib locks internally for
/// most operations when `XInitThreads` is called — which Tauri/GTK does at startup).
/// GLX is documented thread-safe for `glXMakeCurrent` (the context binds to the calling
/// thread). The render thread is the sole GL consumer.
pub struct X11GlxSurface {
    /// The X11 display connection (`Display*`). Owned by the surface (closed on drop).
    /// In practice this is the same display as Tauri/GTK's, but fyom opens its own for
    /// simplicity (avoids sharing Xlib connection state with GTK).
    display: *mut XDisplay,
    /// The child X11 window (XID). Created as a child of the Tauri window's X11 window.
    /// Destroyed on drop.
    window: XWindow,
    /// The GLX rendering context. Made current on the render thread via
    /// `glXMakeCurrent(display, window, context)`.
    context: GlxContext,
    /// Cached current-context state (defensive — sole caller is the render thread).
    make_current_called: Mutex<bool>,
    /// The `x11_dl::xlib::Xlib` dynamic library handle (owns the function pointers).
    /// Kept alive for the lifetime of the surface (the Xlib + GLX symbols are borrowed).
    _xlib: x11_dl::xlib::Xlib,
    /// The `x11_dl::glx::Glx` dynamic library handle (owns the GLX function pointers).
    _glx: x11_dl::glx::Glx,
}

// SAFETY: The X11 Display* + GLXContext are process-global handles. Xlib's `XInitThreads`
// (called by GTK at startup) makes Xlib thread-safe. GLX's `glXMakeCurrent` is documented
// thread-safe + binds the context to the calling thread. The render thread is the sole
// consumer of all GL calls.
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
        // SAFETY: `display` + `window` + `context` are valid handles. `glXMakeCurrent`
        // binds the GLX context to the calling thread + the window as the drawable.
        let ok = unsafe {
            (self._glx.glXMakeCurrent)(self.display as *mut _, self.window, self.context)
        };
        if ok == False as i32 {
            return Err("glXMakeCurrent failed".to_string());
        }
        *called = true;
        Ok(())
    }

    fn get_proc_address(&self, name: &str) -> *mut c_void {
        let c_name = match CString::new(name) {
            Ok(c) => c,
            Err(_) => return std::ptr::null_mut(),
        };
        // SAFETY: `glXGetProcAddressARB` is the documented GLX API. Returns a function
        // pointer valid for any GLX context, or NULL for unknown names. Thread-safe.
        unsafe {
            let ptr = (self._glx.glXGetProcAddressARB)(c_name.as_ptr() as *const u8);
            ptr as *mut c_void
        }
    }

    fn drawable_size(&self) -> (i32, i32) {
        // GetGeometry returns the window's geometry (x, y, width, height, border, depth).
        // The width/height are in pixels (X11 is DPI-unaware; the compositor scales).
        //
        // PORTED_FROM_TSUKIMI pattern (scale_factor multiplication) — on X11, the window
        // manager handles HiDPI via xrandr scaling, so the drawable size is already
        // physical pixels. Under XWayland, the compositor handles the scale.
        let mut root_return = 0u64;
        let mut x_return = 0;
        let mut y_return = 0;
        let mut width_return = 0u32;
        let mut height_return = 0u32;
        let mut border_width_return = 0u32;
        let mut depth_return = 0u32;
        // SAFETY: `display` + `window` are valid; `XGetGeometry` is the documented API.
        let ok = unsafe {
            (self._xlib.XGetGeometry)(
                self.display as *mut _,
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
        // SAFETY: `display` + `window` are valid; `glXSwapBuffers` is the documented GLX
        // API for double-buffered visuals.
        unsafe {
            (self._glx.glXSwapBuffers)(self.display as *mut _, self.window);
        }
    }
}

impl Drop for X11GlxSurface {
    fn drop(&mut self) {
        // Clear the current context (if this thread has it current) + destroy the context.
        unsafe {
            let current = (self._glx.glXGetCurrentContext)();
            if current == self.context {
                let _ = (self._glx.glXMakeCurrent)(
                    self.display as *mut _,
                    0,
                    std::ptr::null_mut(),
                );
            }
            (self._glx.glXDestroyContext)(self.display as *mut _, self.context);
            (self._xlib.XDestroyWindow)(self.display as *mut _, self.window);
            (self._xlib.XCloseDisplay)(self.display as *mut _);
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
/// 4. Create the child X11 window (a child of the parent window).
/// 5. Position the child behind the webview (`XLowerWindow`).
/// 6. Create the GLX context (`glXCreateContext`).
///
/// On any failure, returns `Err` — the caller logs + continues without GL rendering.
///
/// # Wayland
/// Under Wayland without XWayland, `XOpenDisplay(NULL)` returns NULL (no X server). The
/// caller logs the error + continues without GL rendering (the 9.7 `<video>` fallback
/// stays green). Native Wayland + EGL is Phase 2.5+.
pub fn create_surface(window: &tauri::WebviewWindow) -> Result<Box<dyn RenderSurface>, String> {
    use raw_window_handle::{HasWindowHandle, RawWindowHandle};

    // STEP 1: get the parent X11 window.
    let raw_handle = window
        .window_handle()
        .map_err(|e| format!("failed to get raw window handle: {}", e))?;
    let xlib_handle = match raw_handle.as_ref() {
        RawWindowHandle::Xlib(h) => h,
        _ => {
            return Err(
                "expected Xlib raw window handle on Linux (Wayland native not yet supported — \
                 XWayland fallback required)"
                    .to_string(),
            );
        }
    };
    let parent_window = xlib_handle.window as XWindow;
    if parent_window == 0 {
        return Err("parent X11 window is 0".to_string());
    }

    // Load the Xlib + GLX dynamic libraries.
    let xlib = x11_dl::xlib::Xlib::open()
        .map_err(|e| format!("failed to load libX11: {}", e))?;
    let glx = x11_dl::glx::Glx::open()
        .map_err(|e| format!("failed to load libGL: {}", e))?;

    // STEP 2: open a dedicated X11 display connection.
    // SAFETY: `XOpenDisplay(NULL)` returns the display connection for the default display
    // (from $DISPLAY). Returns NULL if no X server is available (e.g. native Wayland).
    let display = unsafe { (xlib.XOpenDisplay)(std::ptr::null()) };
    if display.is_null() {
        return Err(
            "XOpenDisplay returned null (no X server — native Wayland without XWayland?)"
                .to_string(),
        );
    }

    // Install a forgiving X error handler (so X errors don't abort the process; we'll
    // log them + return Err instead). The default handler calls `exit(1)`.
    //
    // SAFETY: `XSetErrorHandler` is the documented API. The handler must be `extern "C"`.
    // We set a static handler that logs the error + returns 0 (non-fatal).
    unsafe extern "C" fn fyom_x_error_handler(_display: *mut XDisplay, _error: *mut XErrorEvent) -> i32 {
        tracing::warn!("[platform/linux] X11 error received (non-fatal)");
        0
    }
    unsafe {
        (xlib.XSetErrorHandler)(Some(fyom_x_error_handler));
    }

    // STEP 3: choose a double-buffered RGBA GLX visual with alpha + depth.
    //
    // The attribute list is a `Vec<i32>` terminated by `0` (X11's `None` == 0, but typed
    // as `c_int` here to match the GLX API signature `glXChooseVisual(*mut Display, c_int,
    // *mut c_int)`).
    let display_ptr = display as *mut _;
    let mut attributes: Vec<i32> = vec![
        x11_dl::glx::GLX_RGBA as i32,
        x11_dl::glx::GLX_RED_SIZE as i32, 8,
        x11_dl::glx::GLX_GREEN_SIZE as i32, 8,
        x11_dl::glx::GLX_BLUE_SIZE as i32, 8,
        x11_dl::glx::GLX_ALPHA_SIZE as i32, 8,
        x11_dl::glx::GLX_DEPTH_SIZE as i32, 24,
        x11_dl::glx::GLX_DOUBLEBUFFER as i32,
        0, // attribute list terminator (X11's None == 0)
    ];
    // SAFETY: `glXChooseVisual` is the documented GLX API. Returns a `XVisualInfo*` or NULL.
    let visual_info = unsafe {
        (glx.glXChooseVisual)(display_ptr, 0, attributes.as_mut_ptr())
    };
    if visual_info.is_null() {
        unsafe { (xlib.XCloseDisplay)(display_ptr) };
        return Err("glXChooseVisual returned null (no matching GLX visual)".to_string());
    }

    // The XVisualInfo struct contains: visual, visualid, screen, depth, class, red_mask,
    // green_mask, blue_mask, colormap_size, bits_per_rgb.
    //
    // We need the `visual` + `depth` to create the child window with a matching colormap.
    #[repr(C)]
    struct XVisualInfo {
        visual: *mut x11_dl::xlib::Visual,
        visualid: x11_dl::xlib::VisualID,
        screen: i32,
        depth: i32,
        class: i32,
        red_mask: u64,
        green_mask: u64,
        blue_mask: u64,
        colormap_size: i32,
        bits_per_rgb: i32,
    }
    let vi: &XVisualInfo = unsafe { &*(visual_info as *const XVisualInfo) };

    // Get the parent window's attributes (for the colormap + screen).
    let mut parent_attrs: x11_dl::xlib::XWindowAttributes = unsafe { std::mem::zeroed() };
    // SAFETY: `XGetWindowAttributes` is the documented API.
    let ok = unsafe { (xlib.XGetWindowAttributes)(display_ptr, parent_window, &mut parent_attrs) };
    if ok == False as i32 {
        unsafe {
            (xlib.XFree)(visual_info as *mut _);
            (xlib.XCloseDisplay)(display_ptr);
        }
        return Err("XGetWindowAttributes(parent) failed".to_string());
    }

    // Create a colormap matching the visual.
    // SAFETY: `XCreateColormap` is the documented API.
    let colormap = unsafe {
        (xlib.XCreateColormap)(
            display_ptr,
            parent_window,
            vi.visual,
            x11_dl::xlib::AllocNone,
        )
    };

    // STEP 4: create the child X11 window.
    let mut set_window_attrs: x11_dl::xlib::XSetWindowAttributes = unsafe { std::mem::zeroed() };
    set_window_attrs.colormap = colormap;
    set_window_attrs.event_mask = 0; // no events (the render thread polls drawable_size)
    set_window_attrs.override_redirect = False as i32; // respect window manager

    let mut window_attrs = 0u64;
    window_attrs |= x11_dl::xlib::CWColormap;
    window_attrs |= x11_dl::xlib::CWEventMask;

    // SAFETY: `XCreateWindow` is the documented API. The child window is sized to the
    // parent's client area.
    let child_window = unsafe {
        (xlib.XCreateWindow)(
            display_ptr,
            parent_window,
            0,
            0,
            parent_attrs.width.max(1) as u32,
            parent_attrs.height.max(1) as u32,
            0,
            vi.depth,
            x11_dl::xlib::InputOutput as u32,
            vi.visual,
            window_attrs,
            &mut set_window_attrs,
        )
    };
    if child_window == 0 {
        unsafe {
            (xlib.XFreeColormap)(display_ptr, colormap);
            (xlib.XFree)(visual_info as *mut _);
            (xlib.XCloseDisplay)(display_ptr);
        }
        return Err("XCreateWindow for child window failed".to_string());
    }

    // Map the window (make it visible).
    // SAFETY: `XMapWindow` is the documented API.
    unsafe {
        (xlib.XMapWindow)(display_ptr, child_window);
    }

    // STEP 5: lower the child window to the bottom of the stacking order (behind the
    // webview, which is a sibling window).
    // SAFETY: `XLowerWindow` is the documented API.
    unsafe {
        (xlib.XLowerWindow)(display_ptr, child_window);
    }

    // Flush the X connection to ensure the window is created + lowered before we return.
    // SAFETY: `XFlush` is the documented API.
    unsafe {
        (xlib.XFlush)(display_ptr);
    }

    // STEP 6: create the GLX context.
    // SAFETY: `glXCreateContext` is the documented GLX API. Returns a GLXContext or NULL.
    let context = unsafe {
        (glx.glXCreateContext)(
            display_ptr,
            visual_info,
            std::ptr::null_mut(),
            True as i32, // direct rendering
        )
    };
    if context.is_null() {
        unsafe {
            (xlib.XDestroyWindow)(display_ptr, child_window);
            (xlib.XFreeColormap)(display_ptr, colormap);
            (xlib.XFree)(visual_info as *mut _);
            (xlib.XCloseDisplay)(display_ptr);
        }
        return Err("glXCreateContext returned null".to_string());
    }

    // Free the visual info (no longer needed).
    unsafe {
        (xlib.XFree)(visual_info as *mut _);
    }

    tracing::info!(
        "[platform/linux] child X11 window + GLX context created behind webview \
         (depth {}, direct rendering)",
        vi.depth
    );

    Ok(Box::new(X11GlxSurface {
        display,
        window: child_window,
        context,
        make_current_called: Mutex::new(false),
        _xlib: xlib,
        _glx: glx,
    }))
}
