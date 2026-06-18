//! macOS GL surface — `NSOpenGLContext` + `NSOpenGLView` child behind `WKWebView`.
//!
//! PORTED_FROM_SOIA `src-tauri/src/platform/macos.rs` (direction only — soia uses
//! `libsoia_utils`'s closed-source Metal-layer surface; fyom writes the open NSOpenGL
//! path. The window-lifecycle / transparency logic direction is ported; the Metal-layer
//! surface code is replaced with the simpler NSOpenGL path here.)
//!
//! ## Architecture
//! Tauri's main window on macOS contains an `NSView` content view hosting a `WKWebView`.
//! fyom creates a child `NSOpenGLView` with a legacy `NSOpenGLContext` that sits behind
//! the webview. When the webview root becomes transparent via `.video-mode`, the mpv GL
//! render shows through.
//!
//! The `NSOpenGLContext` is made current on the render thread via `makeCurrentContext`.
//! `get_proc_address` resolves GL symbols via `dlsym` from:
//!
//! `/System/Library/Frameworks/OpenGL.framework/OpenGL`
//!
//! ## objc2 compatibility notes
//! This file intentionally uses raw `objc2::msg_send!` for NSOpenGL types because the
//! high-level NSOpenGL wrappers in `objc2-app-kit` vary by feature set/version.
//!
//! With `objc2 0.6.x`:
//! - Geometry types such as `NSRect` live in `objc2_foundation`, not `objc2_app_kit`.
//! - Objective-C `nil` object arguments must be typed as `*mut AnyObject`, not `*const ()`.
//! - Returning `Option<*mut AnyObject>` from `msg_send!` is invalid because raw pointers do
//!   not implement `OptionEncode`; use raw `*mut AnyObject` and test for null.

use std::ffi::{CString, c_void};
use std::sync::Mutex;

use objc2::runtime::AnyObject;
use objc2_foundation::NSRect;

use crate::mpv::render::RenderSurface;

// ---------------------------------------------------------------------------
// NSOpenGL attribute constants.
// ---------------------------------------------------------------------------

/// OpenGL profile selector, followed by the profile value.
const NS_OPENGL_PFA_OPENGL_PROFILE: u32 = 99;

/// Profile value: OpenGL 3.2 Core.
const NS_OPENGL_PROFILE_VERSION_3_2_CORE: u32 = 0x3200;

/// Color buffer size, followed by bit depth.
const NS_OPENGL_PFA_COLOR_SIZE: u32 = 8;

/// Alpha buffer size, followed by bit depth.
const NS_OPENGL_PFA_ALPHA_SIZE: u32 = 11;

/// Depth buffer size, followed by bit depth.
const NS_OPENGL_PFA_DEPTH_SIZE: u32 = 12;

/// Double-buffered mode.
const NS_OPENGL_PFA_DOUBLEBUFFER: u32 = 5;

/// Hardware-accelerated renderer only.
const NS_OPENGL_PFA_ACCELERATED: u32 = 73;

/// Attribute list terminator.
const NS_OPENGL_ATTRIBUTE_LIST_TERMINATOR: u32 = 0;

/// `NSWindowBelow` / `NSWindowOrderingMode`.
const NS_WINDOW_BELOW: isize = 1;

/// `NSViewWidthSizable`.
const NS_VIEW_WIDTH_SIZABLE: usize = 2;

/// `NSViewHeightSizable`.
const NS_VIEW_HEIGHT_SIZABLE: usize = 16;

// ---------------------------------------------------------------------------
// Small Obj-C helpers.
// ---------------------------------------------------------------------------

#[inline]
fn nil_object() -> *mut AnyObject {
    std::ptr::null_mut::<AnyObject>()
}

#[inline]
unsafe fn release_object(object: *mut AnyObject) {
    if !object.is_null() {
        // SAFETY: `object` is an Objective-C object retained/owned by this code path.
        unsafe {
            let _: () = objc2::msg_send![object, release];
        }
    }
}

#[inline]
unsafe fn remove_from_superview(view: *mut AnyObject) {
    if !view.is_null() {
        // SAFETY: `view` is a valid NSView. `removeFromSuperview` is idempotent enough for
        // our teardown path; if the view has no superview, AppKit simply does nothing.
        unsafe {
            let _: () = objc2::msg_send![view, removeFromSuperview];
        }
    }
}

// ---------------------------------------------------------------------------
// MacGlSurface.
// ---------------------------------------------------------------------------

/// macOS OpenGL render surface.
///
/// Owns:
/// - one retained `NSOpenGLContext`
/// - one retained child `NSOpenGLView`
/// - one `dlopen` handle for the OpenGL framework
///
/// The render thread is the sole GL consumer.
pub struct MacGlSurface {
    /// Retained `NSOpenGLContext`.
    context: *mut AnyObject,

    /// Retained child `NSOpenGLView`.
    view: *mut AnyObject,

    /// `dlopen` handle for OpenGL.framework.
    gl_framework_handle: usize,

    /// Defensive current-context cache.
    make_current_called: Mutex<bool>,
}

// SAFETY: The Obj-C objects are reference-counted and retained by this struct. The render
// thread is the sole GL consumer. AppKit view hierarchy mutation is done during creation
// and teardown only; GL rendering itself is confined to the render thread.
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

        if self.context.is_null() {
            return Err("NSOpenGLContext is null".to_string());
        }

        // SAFETY: `context` is a valid retained NSOpenGLContext. `makeCurrentContext`
        // binds it to the calling render thread.
        unsafe {
            let _: () = objc2::msg_send![self.context, makeCurrentContext];
        }

        *called = true;
        Ok(())
    }

    fn get_proc_address(&self, name: &str) -> *mut c_void {
        if self.gl_framework_handle == 0 {
            return std::ptr::null_mut();
        }

        let c_name = match CString::new(name) {
            Ok(name) => name,
            Err(_) => return std::ptr::null_mut(),
        };

        // SAFETY: `gl_framework_handle` is a valid handle returned by `dlopen`.
        unsafe { libc::dlsym(self.gl_framework_handle as *mut c_void, c_name.as_ptr()) }
    }

    fn drawable_size(&self) -> (i32, i32) {
        if self.view.is_null() {
            return (0, 0);
        }

        // Read the view frame in points, then multiply by the window backing scale.
        //
        // objc2 0.6.x geometry types live in `objc2_foundation`, not `objc2_app_kit`.
        unsafe {
            let frame: NSRect = objc2::msg_send![self.view, frame];

            // Return raw pointer. Do not use `Option<*mut AnyObject>` here: raw pointers do
            // not implement `OptionEncode` in objc2 0.6.x.
            let window: *mut AnyObject = objc2::msg_send![self.view, window];

            let scale: f64 = if window.is_null() {
                1.0
            } else {
                objc2::msg_send![window, backingScaleFactor]
            };

            let width = (frame.size.width * scale).round() as i32;
            let height = (frame.size.height * scale).round() as i32;

            (width.max(0), height.max(0))
        }
    }

    fn swap_buffers(&self) {
        if self.context.is_null() {
            return;
        }

        // SAFETY: `context` is a valid NSOpenGLContext. The render loop calls this after
        // making the context current on the same thread.
        unsafe {
            let _: () = objc2::msg_send![self.context, flushBuffer];
        }
    }
}

impl Drop for MacGlSurface {
    fn drop(&mut self) {
        unsafe {
            if !self.context.is_null() {
                // Detach drawable before tearing down the NSOpenGLView.
                let _: () = objc2::msg_send![self.context, clearDrawable];

                let cls = objc2::class!(NSOpenGLContext);
                let _: () = objc2::msg_send![cls, clearCurrentContext];
            }

            // `addSubview:` retains the view. Remove it from the hierarchy before
            // releasing our own retain.
            remove_from_superview(self.view);

            release_object(self.context);
            release_object(self.view);

            if self.gl_framework_handle != 0 {
                let _ = libc::dlclose(self.gl_framework_handle as *mut c_void);
            }
        }

        self.context = std::ptr::null_mut();
        self.view = std::ptr::null_mut();
        self.gl_framework_handle = 0;
    }
}

// ---------------------------------------------------------------------------
// Factory.
// ---------------------------------------------------------------------------

/// Create the macOS GL surface for the given Tauri window.
///
/// Steps:
/// 1. Get the Tauri window's AppKit `NSView`.
/// 2. Create `NSOpenGLPixelFormat`.
/// 3. Create `NSOpenGLContext`.
/// 4. Create `NSOpenGLView`.
/// 5. Add the GL view behind the webview.
/// 6. Attach the context to the GL view.
/// 7. `dlopen` OpenGL.framework for GL symbol resolution.
pub fn create_surface(window: &tauri::WebviewWindow) -> Result<Box<dyn RenderSurface>, String> {
    use raw_window_handle::{HasWindowHandle, RawWindowHandle};

    // STEP 1: get AppKit NSView from the raw window handle.
    let raw_handle = window
        .window_handle()
        .map_err(|e| format!("failed to get raw window handle: {}", e))?;

    let appkit_handle = match raw_handle.as_ref() {
        RawWindowHandle::AppKit(handle) => handle,
        _ => return Err("expected AppKit raw window handle on macOS".to_string()),
    };

    let ns_view_ptr = appkit_handle.ns_view.as_ptr() as *mut AnyObject;
    if ns_view_ptr.is_null() {
        return Err("AppKit ns_view is null".to_string());
    }

    // STEP 2: create NSOpenGLPixelFormat.
    //
    // The list is terminated by 0. The final extra zeros are harmless padding and match
    // common Apple sample-code style.
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
        0,
        0,
    ];

    let pixel_format: *mut AnyObject = unsafe {
        let cls = objc2::class!(NSOpenGLPixelFormat);
        let alloc: *mut AnyObject = objc2::msg_send![cls, alloc];

        if alloc.is_null() {
            return Err("NSOpenGLPixelFormat alloc returned null".to_string());
        }

        let pixel_format: *mut AnyObject = objc2::msg_send![
            alloc,
            initWithAttributes: attributes.as_ptr()
        ];

        if pixel_format.is_null() {
            release_object(alloc);
            return Err(
                "NSOpenGLPixelFormat initWithAttributes returned null; no matching pixel format"
                    .to_string(),
            );
        }

        pixel_format
    };

    // STEP 3: create NSOpenGLContext.
    let context: *mut AnyObject = unsafe {
        let cls = objc2::class!(NSOpenGLContext);
        let alloc: *mut AnyObject = objc2::msg_send![cls, alloc];

        if alloc.is_null() {
            release_object(pixel_format);
            return Err("NSOpenGLContext alloc returned null".to_string());
        }

        // `shareContext:` takes an Objective-C object pointer or nil. Do not pass
        // `std::ptr::null::<()>`; objc2 requires an Obj-C-encodable pointer type.
        let context: *mut AnyObject = objc2::msg_send![
            alloc,
            initWithFormat: pixel_format,
            shareContext: nil_object()
        ];

        release_object(alloc);

        if context.is_null() {
            release_object(pixel_format);
            return Err("NSOpenGLContext initWithFormat returned null".to_string());
        }

        context
    };

    // STEP 4: create NSOpenGLView.
    let view_frame: NSRect = unsafe { objc2::msg_send![ns_view_ptr, frame] };

    let gl_view: *mut AnyObject = unsafe {
        let cls = objc2::class!(NSOpenGLView);
        let alloc: *mut AnyObject = objc2::msg_send![cls, alloc];

        if alloc.is_null() {
            release_object(context);
            release_object(pixel_format);
            return Err("NSOpenGLView alloc returned null".to_string());
        }

        let gl_view: *mut AnyObject = objc2::msg_send![
            alloc,
            initWithFrame: view_frame,
            pixelFormat: pixel_format
        ];

        release_object(alloc);

        if gl_view.is_null() {
            release_object(context);
            release_object(pixel_format);
            return Err("NSOpenGLView initWithFrame:pixelFormat: returned null".to_string());
        }

        gl_view
    };

    // Pixel format is now retained by the context/view as needed. Release our ownership.
    unsafe {
        release_object(pixel_format);
    }

    // STEP 5: add GL view behind the webview and configure resizing.
    unsafe {
        // `relativeTo:` takes an NSView object pointer or nil. Use typed nil.
        let _: () = objc2::msg_send![
            ns_view_ptr,
            addSubview: gl_view,
            positioned: NS_WINDOW_BELOW,
            relativeTo: nil_object()
        ];

        let autoresizing_mask = NS_VIEW_WIDTH_SIZABLE | NS_VIEW_HEIGHT_SIZABLE;

        let _: () = objc2::msg_send![
            gl_view,
            setAutoresizingMask: autoresizing_mask
        ];

        let _: () = objc2::msg_send![gl_view, setHidden: false];
    }

    // STEP 6: attach context to the GL view.
    unsafe {
        let _: () = objc2::msg_send![context, setView: gl_view];
    }

    // STEP 7: dlopen OpenGL.framework for GL proc lookup.
    let framework_path = CString::new("/System/Library/Frameworks/OpenGL.framework/OpenGL")
        .map_err(|e| {
            unsafe {
                remove_from_superview(gl_view);
                release_object(context);
                release_object(gl_view);
            }
            format!("failed to build OpenGL framework path CString: {}", e)
        })?;

    let gl_framework_handle = unsafe { libc::dlopen(framework_path.as_ptr(), libc::RTLD_LAZY) };

    if gl_framework_handle.is_null() {
        unsafe {
            remove_from_superview(gl_view);
            release_object(context);
            release_object(gl_view);
        }

        return Err("failed to dlopen OpenGL.framework".to_string());
    }

    tracing::info!(
        "[platform/macos] NSOpenGLContext + NSOpenGLView created behind WKWebView \
         (OpenGL 3.2 Core, double-buffered, accelerated)"
    );

    Ok(Box::new(MacGlSurface {
        context,
        view: gl_view,
        gl_framework_handle: gl_framework_handle as usize,
        make_current_called: Mutex::new(false),
    }))
}
