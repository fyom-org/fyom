//! Linux X11 GL surface — child X11 `Window` + GLX context behind the WebView host.
//!
//! PORTED_FROM_SOIA `src-tauri/src/platform/default.rs` direction only.
//! SOIA uses a closed-source surface implementation; fyom owns this open GLX path.
//!
//! ## Architecture
//!
//! On Linux/X11, fyom creates a child X11 window under the Tauri/GTK top-level X11
//! window. mpv renders into that child window through a GLX context. The WebView stays
//! above it. When the frontend enables transparent video mode, the mpv render surface
//! becomes visible behind the WebView.
//!
//! ## Important constraints
//!
//! - Native Wayland is not supported here.
//! - Wayland through XWayland is supported only if Tauri/GTK exposes an Xlib raw window
//!   handle.
//! - The file is intentionally named `linux-x11.rs`; because Rust module identifiers
//!   cannot contain `-`, it must be imported with:
//!
//! ```ignore
//! #[path = "linux-x11.rs"]
//! pub mod linux_x11;
//! ```
//!
//! ## GLX lifecycle
//!
//! - `XOpenDisplay(NULL)` opens a dedicated X11 connection for this render surface.
//! - `glXChooseVisual` selects a double-buffered RGBA visual.
//! - `XCreateWindow` creates the child window.
//! - `glXCreateContext` creates the GLX context.
//! - `make_current` binds the context to the current render thread.
//! - `swap_buffers` presents the rendered frame.

use std::ffi::{c_void, CString};
use std::sync::Mutex;
use std::thread::ThreadId;

use x11_dl::glx::GLXContext as GlxContext;
use x11_dl::xlib::{
    AllocNone, Colormap, Display as XDisplay, ExposureMask, False, InputOutput, StructureNotifyMask,
    True, Window as XWindow, XErrorEvent, XSetWindowAttributes, XVisualInfo, XWindowAttributes,
    CWColormap, CWEventMask,
};

use crate::mpv::render::RenderSurface;

// -----------------------------------------------------------------------------
// Helpers.
// -----------------------------------------------------------------------------

#[inline]
fn glx_proc_address_to_void_ptr(proc_addr: Option<unsafe extern "C" fn()>) -> *mut c_void {
    match proc_addr {
        Some(func) => func as *const () as *mut c_void,
        None => std::ptr::null_mut(),
    }
}

#[inline]
unsafe fn free_visual_info(xlib: &x11_dl::xlib::Xlib, visual_info: *mut XVisualInfo) {
    if !visual_info.is_null() {
        // SAFETY: `visual_info` was allocated by Xlib/GLX and must be released with XFree.
        unsafe {
            (xlib.XFree)(visual_info as *mut c_void);
        }
    }
}

#[inline]
unsafe fn close_display(xlib: &x11_dl::xlib::Xlib, display: *mut XDisplay) {
    if !display.is_null() {
        // SAFETY: `display` was opened by XOpenDisplay and is owned by this surface factory.
        unsafe {
            (xlib.XCloseDisplay)(display);
        }
    }
}

#[inline]
unsafe fn destroy_child_window(
    xlib: &x11_dl::xlib::Xlib,
    display: *mut XDisplay,
    window: XWindow,
) {
    if !display.is_null() && window != 0 {
        // SAFETY: `window` was created by this surface factory on `display`.
        unsafe {
            (xlib.XDestroyWindow)(display, window);
        }
    }
}

#[inline]
unsafe fn free_colormap(xlib: &x11_dl::xlib::Xlib, display: *mut XDisplay, colormap: Colormap) {
    if !display.is_null() && colormap != 0 {
        // SAFETY: `colormap` was created by this surface factory on `display`.
        unsafe {
            (xlib.XFreeColormap)(display, colormap);
        }
    }
}

unsafe extern "C" fn fyom_x_error_handler(
    _display: *mut XDisplay,
    error: *mut XErrorEvent,
) -> i32 {
    if error.is_null() {
        tracing::warn!("[platform/linux-x11] X11 error received with null XErrorEvent");
        return 0;
    }

    // SAFETY: Xlib invoked this handler with a valid XErrorEvent pointer.
    let error = unsafe { &*error };

    tracing::warn!(
        "[platform/linux-x11] X11 error received: type={}, error_code={}, request_code={}, minor_code={}, resourceid={}",
        error.type_,
        error.error_code,
        error.request_code,
        error.minor_code,
        error.resourceid
    );

    0
}

// -----------------------------------------------------------------------------
// X11GlxSurface.
// -----------------------------------------------------------------------------

/// Linux/X11 GLX render surface.
///
/// This surface owns:
///
/// - a dedicated X11 display connection,
/// - a child X11 window,
/// - a GLX context,
/// - a colormap compatible with the selected GLX visual.
///
/// The surface is intended to be consumed by the mpv render thread.
pub struct X11GlxSurface {
    display: *mut XDisplay,
    parent_window: XWindow,
    window: XWindow,
    colormap: Colormap,
    context: GlxContext,

    /// GLX current context is thread-local. Caching a single boolean is incorrect because
    /// another thread may call `make_current` later. This tracks the last thread that
    /// successfully made the context current and still allows rebinding on thread changes.
    current_thread: Mutex<Option<ThreadId>>,

    _xlib: x11_dl::xlib::Xlib,
    _glx: x11_dl::glx::Glx,
}

// SAFETY:
// Xlib calls are process/thread safe only after XInitThreads. The factory calls XInitThreads
// before opening the display. The GLX context is expected to be used by the render thread;
// `make_current` explicitly binds the context to the calling thread.
unsafe impl Send for X11GlxSurface {}

impl X11GlxSurface {
    fn resize_to_parent(&self) -> Result<(), String> {
        let mut parent_attrs: XWindowAttributes = unsafe { std::mem::zeroed() };

        // SAFETY: `display` and `parent_window` are valid X11 handles.
        let ok = unsafe {
            (self._xlib.XGetWindowAttributes)(self.display, self.parent_window, &mut parent_attrs)
        };

        if ok == False as i32 {
            return Err("XGetWindowAttributes(parent) failed while resizing child".to_string());
        }

        let width = parent_attrs.width.max(1) as u32;
        let height = parent_attrs.height.max(1) as u32;

        // SAFETY: `window` is the owned child window.
        unsafe {
            (self._xlib.XMoveResizeWindow)(self.display, self.window, 0, 0, width, height);
            (self._xlib.XFlush)(self.display);
        }

        Ok(())
    }
}

impl RenderSurface for X11GlxSurface {
    fn make_current(&self) -> Result<(), String> {
        let current_thread_id = std::thread::current().id();

        {
            let guard = self
                .current_thread
                .lock()
                .map_err(|e| format!("current_thread mutex poisoned: {e}"))?;

            if guard.as_ref() == Some(&current_thread_id) {
                return Ok(());
            }
        }

        // Keep the child window sized to the parent before the first/current binding.
        // This also makes resizing more forgiving if the render loop calls make_current
        // after window size changes.
        if let Err(error) = self.resize_to_parent() {
            tracing::warn!(
                "[platform/linux-x11] failed to resize child window before make_current: {}",
                error
            );
        }

        // SAFETY: `display`, `window`, and `context` are valid handles owned by this surface.
        let ok = unsafe { (self._glx.glXMakeCurrent)(self.display, self.window, self.context) };

        if ok == False as i32 {
            return Err("glXMakeCurrent failed".to_string());
        }

        let mut guard = self
            .current_thread
            .lock()
            .map_err(|e| format!("current_thread mutex poisoned after make_current: {e}"))?;

        *guard = Some(current_thread_id);

        Ok(())
    }

    fn get_proc_address(&self, name: &str) -> *mut c_void {
        let c_name = match CString::new(name) {
            Ok(c_name) => c_name,
            Err(_) => return std::ptr::null_mut(),
        };

        // SAFETY: `glXGetProcAddressARB` is the documented GLX API.
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
        // SAFETY:
        // All resources below are owned by this surface. Destruction order matters:
        // 1. unbind current GLX context if needed,
        // 2. destroy GLX context,
        // 3. destroy child X11 window,
        // 4. free colormap,
        // 5. close display.
        unsafe {
            if self.display.is_null() {
                return;
            }

            let current = (self._glx.glXGetCurrentContext)();

            if current == self.context {
                let _ = (self._glx.glXMakeCurrent)(self.display, 0, std::ptr::null_mut());
            }

            if !self.context.is_null() {
                (self._glx.glXDestroyContext)(self.display, self.context);
                self.context = std::ptr::null_mut();
            }

            destroy_child_window(&self._xlib, self.display, self.window);
            self.window = 0;

            free_colormap(&self._xlib, self.display, self.colormap);
            self.colormap = 0;

            close_display(&self._xlib, self.display);
            self.display = std::ptr::null_mut();
        }
    }
}

// -----------------------------------------------------------------------------
// Factory.
// -----------------------------------------------------------------------------

/// Create the Linux/X11 GLX render surface for a Tauri WebviewWindow.
///
/// This requires Tauri/GTK to expose an Xlib raw window handle. Native Wayland without
/// XWayland is intentionally rejected here.
pub fn create_surface(window: &tauri::WebviewWindow) -> Result<Box<dyn RenderSurface>, String> {
    use raw_window_handle::{HasWindowHandle, RawWindowHandle};

    let raw_handle = window
        .window_handle()
        .map_err(|e| format!("failed to get raw window handle: {e}"))?;

    let parent_window = match raw_handle.as_ref() {
        RawWindowHandle::Xlib(handle) => {
            let window = handle.window as XWindow;

            if window == 0 {
                return Err("parent X11 window is 0".to_string());
            }

            window
        }
        other => {
            return Err(format!(
                "expected Xlib raw window handle on Linux/X11, got {other:?}; native Wayland is not supported by linux-x11 surface"
            ));
        }
    };

    let xlib = x11_dl::xlib::Xlib::open()
        .map_err(|e| format!("failed to load libX11 through x11-dl: {e}"))?;

    let glx = x11_dl::glx::Glx::open()
        .map_err(|e| format!("failed to load libGL/GLX through x11-dl: {e}"))?;

    // SAFETY:
    // XInitThreads must be called before other Xlib functions. GTK/Tauri may already have
    // called it, but calling it again is harmless in practice and returns non-zero on success.
    let xinit_threads_ok = unsafe { (xlib.XInitThreads)() };

    if xinit_threads_ok == 0 {
        tracing::warn!(
            "[platform/linux-x11] XInitThreads returned 0; Xlib threading may be unsafe"
        );
    }

    // SAFETY:
    // XSetErrorHandler is process-global. We install a non-fatal handler so recoverable
    // X errors do not terminate the whole desktop app during GL surface setup/rendering.
    unsafe {
        (xlib.XSetErrorHandler)(Some(fyom_x_error_handler));
    }

    // SAFETY:
    // XOpenDisplay(NULL) opens the display from $DISPLAY. It returns NULL if no X server
    // is available, for example under native Wayland without XWayland.
    let display = unsafe { (xlib.XOpenDisplay)(std::ptr::null()) };

    if display.is_null() {
        return Err(
            "XOpenDisplay returned null; no X11 server is available. Use X11/XWayland or implement native Wayland/EGL surface."
                .to_string(),
        );
    }

    // SAFETY: `display` is valid after XOpenDisplay.
    let screen = unsafe { (xlib.XDefaultScreen)(display) };

    // SAFETY: `display` and `screen` are valid.
    let root_window = unsafe { (xlib.XRootWindow)(display, screen) };

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

    // SAFETY:
    // glXChooseVisual returns an XVisualInfo* or NULL. The pointer must be released with
    // XFree when no longer needed.
    let visual_info = unsafe { (glx.glXChooseVisual)(display, screen, attributes.as_mut_ptr()) };

    if visual_info.is_null() {
        unsafe {
            close_display(&xlib, display);
        }

        return Err("glXChooseVisual returned null; no matching double-buffered RGBA GLX visual".to_string());
    }

    // SAFETY: `visual_info` is non-null and owned until XFree.
    let visual_info_ref = unsafe { &*visual_info };

    let mut parent_attrs: XWindowAttributes = unsafe { std::mem::zeroed() };

    // SAFETY:
    // `parent_window` is an XID from the same X server. We intentionally use a dedicated
    // display connection; XIDs are server-global.
    let ok = unsafe { (xlib.XGetWindowAttributes)(display, parent_window, &mut parent_attrs) };

    if ok == False as i32 {
        unsafe {
            free_visual_info(&xlib, visual_info);
            close_display(&xlib, display);
        }

        return Err("XGetWindowAttributes(parent) failed".to_string());
    }

    let width = parent_attrs.width.max(1) as u32;
    let height = parent_attrs.height.max(1) as u32;

    // SAFETY:
    // Use the root window for colormap creation. This is safer when the selected GLX visual
    // differs from the parent window visual.
    let colormap = unsafe {
        (xlib.XCreateColormap)(display, root_window, visual_info_ref.visual, AllocNone)
    };

    if colormap == 0 {
        unsafe {
            free_visual_info(&xlib, visual_info);
            close_display(&xlib, display);
        }

        return Err("XCreateColormap failed".to_string());
    }

    let mut set_window_attrs: XSetWindowAttributes = unsafe { std::mem::zeroed() };
    set_window_attrs.colormap = colormap;
    set_window_attrs.event_mask = ExposureMask | StructureNotifyMask;
    set_window_attrs.override_redirect = False as i32;

    let value_mask = (CWColormap | CWEventMask) as u64;

    // SAFETY:
    // Create a child X11 window using the GLX-compatible visual and colormap.
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
            free_colormap(&xlib, display, colormap);
            free_visual_info(&xlib, visual_info);
            close_display(&xlib, display);
        }

        return Err("XCreateWindow for child GLX window failed".to_string());
    }

    // SAFETY: `child_window` is valid and owned by this surface.
    unsafe {
        (xlib.XMapWindow)(display, child_window);
        (xlib.XLowerWindow)(display, child_window);
        (xlib.XFlush)(display);
    }

    // SAFETY:
    // visual_info was returned by glXChooseVisual for this display/screen.
    let context =
        unsafe { (glx.glXCreateContext)(display, visual_info, std::ptr::null_mut(), True as i32) };

    if context.is_null() {
        unsafe {
            destroy_child_window(&xlib, display, child_window);
            free_colormap(&xlib, display, colormap);
            free_visual_info(&xlib, visual_info);
            close_display(&xlib, display);
        }

        return Err("glXCreateContext returned null".to_string());
    }

    let visual_depth = visual_info_ref.depth;

    unsafe {
        free_visual_info(&xlib, visual_info);
    }

    tracing::info!(
        "[platform/linux-x11] child X11 window + GLX context created behind webview: size={}x{}, screen={}, depth={}",
        width,
        height,
        screen,
        visual_depth
    );

    Ok(Box::new(X11GlxSurface {
        display,
        parent_window,
        window: child_window,
        colormap,
        context,
        current_thread: Mutex::new(None),
        _xlib: xlib,
        _glx: glx,
    }))
}
