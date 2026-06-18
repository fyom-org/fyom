//! macOS GL surface — `NSOpenGLContext` + `NSOpenGLView` child behind `WKWebView`.
//!
//! PORTED_FROM_SOIA `src-tauri/src/platform/macos.rs` (direction only — soia uses
//! `libsoia_utils`'s closed-source Metal-layer surface; fyom writes the open NSOpenGL
//! path. The window-lifecycle / transparency logic direction is ported; the Metal-layer
//! surface code (~400 LOC in soia) is replaced with the simpler NSOpenGL path here.)
//!
//! ## Architecture
//! Tauri's main window on macOS contains an `NSView` content view hosting a `WKWebView`.
//! fyom creates a child `NSOpenGLView` (with a legacy NSOpenGLContext, not Metal) that
//! sits **behind** the `WKWebView` in the layer order. When the webview's root CSS goes
//! `background: transparent !important` (the `.video-mode` class, ported from soia), the
//! mpv GL render shows through.
//!
//! The `NSOpenGLContext` is made current on the render thread via `makeCurrentContext`.
//! `get_proc_address` resolves GL function pointers via `dlsym` to the OpenGL framework
//! (`/System/Library/Frameworks/OpenGL.framework/OpenGL`).
//!
//! ## Why NSOpenGL (not Metal)?
//! - `mpv_render_context_create(MPV_RENDER_API_TYPE_OPENGL)` is the standard, mature path
//!   (mpv's `render_gl.h`); Metal support in mpv is via a third-party fork (not upstream).
//! - NSOpenGL is simpler than Metal for a transparent overlay (no `CAMetalLayer`
//!   geometry sync, no `MTKView` boilerplate).
//! - macOS still fully supports NSOpenGL (deprecated but not removed; the deprecation is
//!   a forward-looking nudge toward Metal, not a functional limitation).
//!
//! ## Implementation note
//! The NSOpenGL classes (`NSOpenGLContext`, `NSOpenGLView`, `NSOpenGLPixelFormat`) are
//! accessed via `objc2::msg_send!` rather than the `objc2-app-kit` high-level wrappers,
//! because the NSOpenGL wrappers in objc2-app-kit 0.3 are behind unstable feature flags
//! that may differ across patch versions. The raw `msg_send!` calls match the Obj-C API
//! exactly (see Apple's `NSOpenGL.h`) and are robust against objc2-app-kit API drift.
//!
//! See `docs/libmpv-assessment.md` §3.3 for the rationale + the soia-vs-fyom LOC comparison.

use std::ffi::{CString, c_void};
use std::sync::Mutex;

use crate::mpv::render::RenderSurface;

// ---------------------------------------------------------------------------
// NSOpenGL attribute constants (from Apple's NSOpenGL.h).
// ---------------------------------------------------------------------------

/// OpenGL Profile selector (followed by the profile value).
const NS_OPENGL_PFA_OPENGL_PROFILE: u32 = 99;
/// Profile value: OpenGL 3.2 Core (modern, supports shaders + FBOs).
const NS_OPENGL_PROFILE_VERSION_3_2_CORE: u32 = 0x3200;
/// Color buffer size (followed by the bit depth).
const NS_OPENGL_PFA_COLOR_SIZE: u32 = 8;
/// Alpha buffer size (followed by the bit depth).
const NS_OPENGL_PFA_ALPHA_SIZE: u32 = 11;
/// Depth buffer size (followed by the bit depth).
const NS_OPENGL_PFA_DEPTH_SIZE: u32 = 12;
/// Double-buffered mode (no value).
const NS_OPENGL_PFA_DOUBLEBUFFER: u32 = 5;
/// Hardware-accelerated renderer only (no value).
const NS_OPENGL_PFA_ACCELERATED: u32 = 73;
/// Attribute list terminator.
const NS_OPENGL_ATTRIBUTE_LIST_TERMINATOR: u32 = 0;

// NSWindowOrderingMode (from NSWindow.h) — used by addSubview:positioned:relativeTo:.
const NS_WINDOW_BELOW: i64 = 1;

// NSViewAutoresizing flags (from NSView.h).
const NS_VIEW_WIDTH_SIZABLE: u64 = 2;
const NS_VIEW_HEIGHT_SIZABLE: u64 = 16;

// ---------------------------------------------------------------------------
// MacGlSurface — the RenderSurface impl.
// ---------------------------------------------------------------------------

/// The macOS GL surface: owns an `NSOpenGLContext` + child `NSOpenGLView` behind the
/// `WKWebView`.
///
/// `Send` because the Obj-C objects are reference-counted + AppKit's NSOpenGLContext /
/// NSOpenGLView are documented thread-safe for the operations fyom uses
/// (makeCurrentContext, setView, flushBuffer). The render thread is the sole GL consumer.
pub struct MacGlSurface {
    /// The NSOpenGLContext (retained). Made current on the render thread via
    /// `makeCurrentContext`. Stored as a raw `*mut AnyObject` because objc2-app-kit's
    /// high-level NSOpenGL wrappers are behind unstable feature flags.
    context: *mut objc2::runtime::AnyObject,
    /// The child NSOpenGLView (retained). Owned by the surface so it's released on drop.
    view: *mut objc2::runtime::AnyObject,
    /// The dlopen handle for the OpenGL framework (`RTLD_LAZY`). Used by `get_proc_address`.
    gl_framework_handle: usize,
    /// Cached current-context state (defensive — sole caller is the render thread).
    make_current_called: Mutex<bool>,
}

// SAFETY: The Obj-C objects are reference-counted (retain/release); we retained them in
// `create_surface` + release them in `Drop`. AppKit's NSOpenGLContext/NSOpenGLView are
// documented thread-safe for the operations fyom uses. The render thread is the sole
// consumer of all GL calls. The dlopen handle is process-global.
unsafe impl Send for MacGlSurface {}

impl RenderSurface for MacGlSurface {
    fn make_current(&self) -> Result<(), String> {
        let mut called = self
            .make_current_called
            .lock()
            .map_err(|e| format!("make_current mutex poisoned: {}", e))?;
        if *called {
            return Ok(());
        }
        // SAFETY: `context` is a valid retained NSOpenGLContext. `makeCurrentContext` is
        // the documented API; binds the context to the calling thread.
        unsafe {
            let _: () = objc2::msg_send![self.context, makeCurrentContext];
        }
        *called = true;
        Ok(())
    }

    fn get_proc_address(&self, name: &str) -> *mut c_void {
        // dlsym the OpenGL framework. macOS GL function pointers are process-global once
        // the framework is loaded (no per-context lookup needed for NSOpenGL, unlike WGL).
        let c_name = match CString::new(name) {
            Ok(c) => c,
            Err(_) => return std::ptr::null_mut(),
        };
        // SAFETY: `gl_framework_handle` was set by `dlopen` in `create_surface`; valid
        // `RTLD_LAZY` handle. `dlsym(handle, name)` is thread-safe.
        unsafe {
            let handle = self.gl_framework_handle as *mut c_void;
            libc::dlsym(handle, c_name.as_ptr())
        }
    }

    fn drawable_size(&self) -> (i32, i32) {
        // Read the NSOpenGLView's frame (in points) + multiply by the window's
        // backingScaleFactor to get physical pixels.
        //
        // PORTED_FROM_TSUKIMI pattern:
        //   let factor = self.obj().scale_factor();
        //   let width = self.obj().width() * factor;
        //   let height = self.obj().height() * factor;
        unsafe {
            // frame → NSRect { origin: NSPoint, size: NSSize { width: f64, height: f64 } }
            let frame: objc2_app_kit::NSRect =
                objc2::msg_send![self.view, frame];
            // window → NSWindow (optional)
            let window: Option<*mut objc2::runtime::AnyObject> =
                objc2::msg_send![self.view, window];
            let scale: f64 = match window {
                Some(w) if !w.is_null() => {
                    objc2::msg_send![w, backingScaleFactor]
                }
                _ => 1.0,
            };
            let width = (frame.size.width * scale) as i32;
            let height = (frame.size.height * scale) as i32;
            (width.max(0), height.max(0))
        }
    }

    fn swap_buffers(&self) {
        // NSOpenGLContext's flushBuffer swaps the back buffer to the front
        // (double-buffered). SAFETY: documented API; context is current on this thread.
        unsafe {
            let _: () = objc2::msg_send![self.context, flushBuffer];
        }
    }
}

impl Drop for MacGlSurface {
    fn drop(&mut self) {
        // Clear the current context (so the render thread doesn't hold a dangling context).
        unsafe {
            let cls = objc2::class!(NSOpenGLContext);
            let _: () = objc2::msg_send![cls, clearCurrentContext];
        }
        // Release the retained Obj-C objects.
        unsafe {
            let _: () = objc2::msg_send![self.context, release];
            let _: () = objc2::msg_send![self.view, release];
        }
        // Close the dlopen handle.
        if self.gl_framework_handle != 0 {
            unsafe {
                let _ = libc::dlclose(self.gl_framework_handle as *mut c_void);
            }
        }
    }
}

// ---------------------------------------------------------------------------
// Factory — create the surface from a Tauri WebviewWindow.
// ---------------------------------------------------------------------------

/// Create the macOS GL surface for the given Tauri window.
///
/// Steps:
/// 1. Get the window's NSView content view (via `raw_window_handle`).
/// 2. Create an `NSOpenGLPixelFormat` with the profile + attributes (alpha, double-buffer,
///    accelerated, color/depth sizes).
/// 3. Create an `NSOpenGLContext` with the pixel format.
/// 4. Create an `NSOpenGLView` with the pixel format, add it as a child of the NSView,
///    order it **behind** the WKWebView (so the webview sits on top in the layer order).
/// 5. Set the context's view to the NSOpenGLView.
/// 6. dlopen the OpenGL framework for `get_proc_address`.
///
/// On any failure, returns `Err` — the caller logs + continues without GL rendering.
pub fn create_surface(window: &tauri::WebviewWindow) -> Result<Box<dyn RenderSurface>, String> {
    use raw_window_handle::{HasWindowHandle, RawWindowHandle};

    // STEP 1: get the NSView content view from the Tauri window.
    let raw_handle = window
        .window_handle()
        .map_err(|e| format!("failed to get raw window handle: {}", e))?;
    let appkit_handle = match raw_handle.as_ref() {
        RawWindowHandle::AppKit(h) => h,
        _ => return Err("expected AppKit raw window handle on macOS".to_string()),
    };
    let ns_view_ptr = appkit_handle.ns_view.as_ptr() as *mut objc2::runtime::AnyObject;
    if ns_view_ptr.is_null() {
        return Err("ns_view is null".to_string());
    }

    // STEP 2: create the NSOpenGLPixelFormat.
    //
    // Attributes: profile 3.2 Core (modern, supports OpenGL 3.2+), alpha size 8, double
    // buffer, hardware accelerated, color size 24, depth size 24.
    let attributes: [u32; 13] = [
        NS_OPENGL_PFA_OPENGL_PROFILE,
        NS_OPENGL_PROFILE_VERSION_3_2_CORE,
        NS_OPENGL_PFA_COLOR_SIZE,
        24,
        NS_OPENGL_PFA_ALPHA_SIZE,
        8,
        NS_OPENGL_PFA_DEPTH_SIZE,
        24,
        NS_OPENGL_PFA_DOUBLEBUFFER,
        NS_OPENGL_PFA_ACCELERATED,
        NS_OPENGL_ATTRIBUTE_LIST_TERMINATOR,
        // Extra terminator padding (paranoid — some Apple docs show a 0,0 double-terminator).
        0,
        0,
    ];

    // SAFETY: NSOpenGLPixelFormat alloc + initWithAttributes: is the documented API.
    // The attribute list is a C array of NSOpenGLPixelFormatAttribute (u32) terminated by 0.
    let pixel_format: *mut objc2::runtime::AnyObject = unsafe {
        let cls = objc2::class!(NSOpenGLPixelFormat);
        let alloc: *mut objc2::runtime::AnyObject = objc2::msg_send![cls, alloc];
        if alloc.is_null() {
            return Err("NSOpenGLPixelFormat alloc returned null".to_string());
        }
        let pf: *mut objc2::runtime::AnyObject = objc2::msg_send![
            alloc,
            initWithAttributes: attributes.as_ptr()
        ];
        if pf.is_null() {
            let _: () = objc2::msg_send![alloc, release];
            return Err(
                "NSOpenGLPixelFormat initWithAttributes returned null (no matching pixel format)"
                    .to_string(),
            );
        }
        pf
    };

    // STEP 3: create the NSOpenGLContext.
    let context: *mut objc2::runtime::AnyObject = unsafe {
        let cls = objc2::class!(NSOpenGLContext);
        let alloc: *mut objc2::runtime::AnyObject = objc2::msg_send![cls, alloc];
        if alloc.is_null() {
            let _: () = objc2::msg_send![pixel_format, release];
            return Err("NSOpenGLContext alloc returned null".to_string());
        }
        let ctx: *mut objc2::runtime::AnyObject = objc2::msg_send![
            alloc,
            initWithFormat: pixel_format,
            shareContext: std::ptr::null::<()>()
        ];
        let _: () = objc2::msg_send![alloc, release];
        if ctx.is_null() {
            let _: () = objc2::msg_send![pixel_format, release];
            return Err("NSOpenGLContext initWithFormat returned null".to_string());
        }
        ctx
    };
    // The pixel format is now retained by the context; release our extra retain.
    unsafe {
        let _: () = objc2::msg_send![pixel_format, release];
    }

    // STEP 4: create the NSOpenGLView + add it as a child of the NSView, behind the WKWebView.
    let view_frame: objc2_app_kit::NSRect = unsafe { objc2::msg_send![ns_view_ptr, frame] };

    let gl_view: *mut objc2::runtime::AnyObject = unsafe {
        let cls = objc2::class!(NSOpenGLView);
        let alloc: *mut objc2::runtime::AnyObject = objc2::msg_send![cls, alloc];
        if alloc.is_null() {
            let _: () = objc2::msg_send![context, release];
            return Err("NSOpenGLView alloc returned null".to_string());
        }
        let v: *mut objc2::runtime::AnyObject = objc2::msg_send![
            alloc,
            initWithFrame: view_frame,
            pixelFormat: pixel_format
        ];
        let _: () = objc2::msg_send![alloc, release];
        if v.is_null() {
            let _: () = objc2::msg_send![context, release];
            return Err("NSOpenGLView initWithFrame returned null".to_string());
        }
        v
    };

    // Add the GL view as a child of the NSView, behind the WKWebView (NSWindowBelow = 1).
    // SAFETY: addSubview:positioned:relativeTo: is the documented NSView API.
    unsafe {
        let _: () = objc2::msg_send![
            ns_view_ptr,
            addSubview: gl_view,
            positioned: NS_WINDOW_BELOW,
            relativeTo: std::ptr::null::<()>()
        ];
        // Enable autoresizing so the GL view resizes with the window.
        let _: () = objc2::msg_send![
            gl_view,
            setAutoresizingMask: NS_VIEW_WIDTH_SIZABLE | NS_VIEW_HEIGHT_SIZABLE
        ];
    }

    // STEP 5: set the context's view to the NSOpenGLView.
    unsafe {
        let _: () = objc2::msg_send![context, setView: gl_view];
    }

    // STEP 6: dlopen the OpenGL framework for `get_proc_address`.
    let framework_path =
        CString::new("/System/Library/Frameworks/OpenGL.framework/OpenGL").map_err(|e| {
            // Clean up the Obj-C objects before returning.
            unsafe {
                let _: () = objc2::msg_send![context, release];
                let _: () = objc2::msg_send![gl_view, release];
            }
            format!("failed to build framework path CString: {}", e)
        })?;
    let gl_framework_handle = unsafe { libc::dlopen(framework_path.as_ptr(), libc::RTLD_LAZY) };
    if gl_framework_handle.is_null() {
        unsafe {
            let _: () = objc2::msg_send![context, release];
            let _: () = objc2::msg_send![gl_view, release];
        }
        return Err("failed to dlopen OpenGL framework".to_string());
    }

    tracing::info!(
        "[platform/macos] NSOpenGLContext + NSOpenGLView created behind WKWebView (OpenGL 3.2 Core, double-buffered, accelerated)"
    );

    Ok(Box::new(MacGlSurface {
        context,
        view: gl_view,
        gl_framework_handle: gl_framework_handle as usize,
        make_current_called: Mutex::new(false),
    }))
}
