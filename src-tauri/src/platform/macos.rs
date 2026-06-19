//! macOS render surface — `NSView` + `CAMetalLayer` for libmpv `--wid` embedding.
//!
//! ## Architecture
//! Tauri's main window on macOS contains an AppKit `NSView` hierarchy hosting a `WKWebView`.
//! fyom creates a dedicated child `NSView`, explicitly attaches a `CAMetalLayer` to it,
//! and places it below the webview.
//!
//! When the webview root becomes transparent via `.video-mode`, the mpv render shows through.
//! libmpv is configured with `--wid=<nsview_ptr>` and `--gpu-api=vulkan`.
//! mpv owns the Vulkan/MoltenVK/Metal initialization path internally.
//!
//! ## Important invariant
//! Do not pass WKWebView's own NSView directly to mpv.
//! mpv needs a dedicated NSView whose backing layer is a CAMetalLayer.
//!
//! ## Safety
//! This file uses Objective-C messaging through `objc2`.
//! AppKit objects must be created and mutated on the main thread.
//! The retained child `NSView` and `CAMetalLayer` are kept alive for the lifetime of
//! the render surface to avoid dangling pointers during mpv initialization.

use std::ffi::c_void;
use std::ptr;
use std::sync::Mutex;

use objc2::rc::Retained;
use objc2::runtime::AnyObject;
use objc2::{class, msg_send, msg_send_id};
use objc2_foundation::{NSPoint, NSRect, NSSize};

use crate::mpv::render::RenderSurface;

// -----------------------------------------------------------------------------
// AppKit constants
// -----------------------------------------------------------------------------

/// `NSWindowBelow` / `NSWindowOrderingMode`.
///
/// AppKit values:
/// - NSWindowAbove = 1
/// - NSWindowBelow = -1
/// - NSWindowOut = 0
const NS_WINDOW_BELOW: isize = -1;

/// `NSViewWidthSizable`.
const NS_VIEW_WIDTH_SIZABLE: usize = 2;

/// `NSViewHeightSizable`.
const NS_VIEW_HEIGHT_SIZABLE: usize = 16;

// -----------------------------------------------------------------------------
// MacosSurface
// -----------------------------------------------------------------------------

/// macOS render surface backed by a dedicated `NSView` + `CAMetalLayer`.
///
/// In this architecture, mpv owns the GPU context and renders directly into the
/// native view through `--wid`. `RenderSurface` exists only as a cross-platform
/// abstraction boundary; the mpv render API is intentionally not used here.
pub struct MacosSurface {
    /// Dedicated child/sibling `NSView` passed to libmpv via `--wid`.
    view: Retained<AnyObject>,

    /// Explicitly retained `CAMetalLayer` backing `view`.
    ///
    /// The view also retains its layer, but retaining it here makes lifecycle and
    /// resize synchronization explicit.
    metal_layer: Retained<AnyObject>,

    /// Container view that owns `view` as a subview.
    ///
    /// Usually this is the WKWebView's superview. If no superview is available,
    /// this falls back to the AppKit view returned by the raw window handle.
    container_view: Retained<AnyObject>,

    /// View above the mpv surface.
    ///
    /// Usually the WKWebView's own NSView. Used only to keep the video view ordered
    /// below it.
    overlay_view: Option<Retained<AnyObject>>,

    /// Defensive lock for trait-object Send/Sync compatibility in the existing
    /// architecture. AppKit mutation must still happen on the main thread.
    _dummy_lock: Mutex<()>,
}

/// The existing `RenderSurface` abstraction may be shared across app state.
///
/// AppKit itself is main-thread-bound. This impl is only safe as long as callers
/// do not mutate AppKit views from non-main threads. mpv's `--wid` path does not
/// require calling `make_current`, `get_proc_address`, or `swap_buffers`.
unsafe impl Send for MacosSurface {}
unsafe impl Sync for MacosSurface {}

impl MacosSurface {
    /// Returns the raw pointer address of the dedicated child `NSView`.
    ///
    /// This value must be passed to libmpv via the `wid` option as a decimal string:
    ///
    /// ```text
    /// --wid=<decimal_nsview_pointer>
    /// ```
    pub fn wid(&self) -> usize {
        Retained::as_ptr(&self.view) as usize
    }

    /// Returns the `wid` value formatted exactly as libmpv expects.
    pub fn wid_string(&self) -> String {
        self.wid().to_string()
    }

    /// Returns the raw `NSView` pointer.
    pub fn raw_ns_view(&self) -> *mut c_void {
        Retained::as_ptr(&self.view) as *mut c_void
    }

    /// Synchronize the video surface with the overlay/container view geometry.
    ///
    /// This should be called after creation and whenever the window or video area
    /// changes size.
    ///
    /// AppKit mutation must happen on the main thread.
    pub fn sync_geometry_to_parent(&self) {
        unsafe {
            sync_video_view_geometry(
                &self.view,
                &self.metal_layer,
                &self.container_view,
                self.overlay_view.as_ref(),
            );
        }
    }

    /// Remove the mpv view from the AppKit hierarchy.
    ///
    /// Call this before destroying libmpv or while tearing down the window.
    /// AppKit mutation must happen on the main thread.
    pub fn destroy(&self) {
        unsafe {
            let _: () = msg_send![&*self.view, removeFromSuperview];
        }
    }
}

impl RenderSurface for MacosSurface {
    /// In `--wid` mode, libmpv manages its own Vulkan/MoltenVK/Metal context.
    fn make_current(&self) -> Result<(), String> {
        Ok(())
    }

    /// In `--wid` mode, libmpv resolves GPU symbols internally.
    fn get_proc_address(&self, _name: &str) -> *mut c_void {
        ptr::null_mut()
    }

    /// Returns the drawable size in physical pixels.
    fn drawable_size(&self) -> (i32, i32) {
        unsafe {
            let bounds: NSRect = msg_send![&*self.view, bounds];
            let scale = backing_scale_factor(&self.view);

            let width = (bounds.size.width * scale).round() as i32;
            let height = (bounds.size.height * scale).round() as i32;

            (width.max(0), height.max(0))
        }
    }

    /// In `--wid` mode, libmpv handles buffer presentation internally.
    fn swap_buffers(&self) {}
}

// -----------------------------------------------------------------------------
// Factory
// -----------------------------------------------------------------------------

/// Create a macOS Metal-backed surface for the given Tauri window.
///
/// Steps:
/// 1. Read AppKit `NSView` from raw window handle.
/// 2. Treat that view as the overlay view when it has a superview.
/// 3. Create a dedicated mpv `NSView`.
/// 4. Attach a `CAMetalLayer` explicitly.
/// 5. Insert the mpv view below the overlay view.
/// 6. Sync frame, bounds, drawableSize, autoresizing mask, and backing scale.
pub fn create_surface(window: &tauri::WebviewWindow) -> Result<Box<dyn RenderSurface>, String> {
    use raw_window_handle::{HasWindowHandle, RawWindowHandle};

    let raw_handle = window
        .window_handle()
        .map_err(|e| format!("failed to get raw window handle: {e}"))?;

    let appkit_handle = match raw_handle.as_ref() {
        RawWindowHandle::AppKit(handle) => handle,
        _ => return Err("expected AppKit raw window handle on macOS".to_string()),
    };

    let raw_ns_view = appkit_handle.ns_view.as_ptr() as *mut AnyObject;
    if raw_ns_view.is_null() {
        return Err("AppKit ns_view is null".to_string());
    }

    let overlay_view = unsafe {
        Retained::retain(raw_ns_view)
            .ok_or_else(|| "failed to retain AppKit ns_view".to_string())?
    };

    let raw_superview: *mut AnyObject = unsafe { msg_send![&*overlay_view, superview] };

    let (container_view, overlay_for_ordering) = if raw_superview.is_null() {
        // Fallback:
        // If raw handle already points to a container/content view, place mpv inside it.
        (
            unsafe {
                Retained::retain(raw_ns_view)
                    .ok_or_else(|| "failed to retain AppKit container ns_view".to_string())?
            },
            None,
        )
    } else {
        // Preferred path:
        // raw ns_view is usually the WKWebView. Insert mpv as a sibling below it.
        (
            unsafe {
                Retained::retain(raw_superview)
                    .ok_or_else(|| "failed to retain AppKit superview".to_string())?
            },
            Some(overlay_view),
        )
    };

    let child_view: Retained<AnyObject> = unsafe { msg_send_id![class!(NSView), new] };

    let metal_layer: Retained<AnyObject> = unsafe { msg_send_id![class!(CAMetalLayer), layer] };

    unsafe {
        // Configure dedicated mpv NSView.
        let _: () = msg_send![&*child_view, setWantsLayer: true];
        let _: () = msg_send![&*child_view, setLayer: &*metal_layer];

        let autoresizing_mask = NS_VIEW_WIDTH_SIZABLE | NS_VIEW_HEIGHT_SIZABLE;
        let _: () = msg_send![&*child_view, setAutoresizingMask: autoresizing_mask];
        let _: () = msg_send![&*child_view, setHidden: false];

        // Make sure the video view itself does not draw an opaque AppKit background.
        let _: () = msg_send![&*child_view, setCanDrawSubviewsIntoLayer: true];

        // Insert mpv video surface below the WebView if possible.
        match overlay_for_ordering.as_ref() {
            Some(overlay) => {
                let _: () = msg_send![
                    &*container_view,
                    addSubview: &*child_view,
                    positioned: NS_WINDOW_BELOW,
                    relativeTo: &**overlay
                ];
            }
            None => {
                let nil_view: *mut AnyObject = ptr::null_mut();

                let _: () = msg_send![
                    &*container_view,
                    addSubview: &*child_view,
                    positioned: NS_WINDOW_BELOW,
                    relativeTo: nil_view
                ];
            }
        }

        sync_video_view_geometry(
            &child_view,
            &metal_layer,
            &container_view,
            overlay_for_ordering.as_ref(),
        );
    }

    tracing::info!(
        "[platform/macos] created dedicated NSView + CAMetalLayer for libmpv --wid embedding; wid={}",
        Retained::as_ptr(&child_view) as usize
    );

    Ok(Box::new(MacosSurface {
        view: child_view,
        metal_layer,
        container_view,
        overlay_view: overlay_for_ordering,
        _dummy_lock: Mutex::new(()),
    }))
}

// -----------------------------------------------------------------------------
// Geometry helpers
// -----------------------------------------------------------------------------

unsafe fn sync_video_view_geometry(
    video_view: &Retained<AnyObject>,
    metal_layer: &Retained<AnyObject>,
    container_view: &Retained<AnyObject>,
    overlay_view: Option<&Retained<AnyObject>>,
) {
    let frame = match overlay_view {
        // Sibling mode:
        // mpv view and WKWebView share the same superview, so use WKWebView's frame
        // in the container coordinate space.
        Some(overlay) => {
            let frame: NSRect = msg_send![&**overlay, frame];
            frame
        }

        // Child mode fallback:
        // mpv view is inside the container itself, so use container bounds.
        None => {
            let bounds: NSRect = msg_send![&**container_view, bounds];
            bounds
        }
    };

    let bounds = ns_rect(0.0, 0.0, frame.size.width, frame.size.height);
    let scale = backing_scale_factor(video_view);
    let drawable_size = NSSize::new(frame.size.width * scale, frame.size.height * scale);

    let _: () = msg_send![&**video_view, setFrame: frame];
    let _: () = msg_send![&**video_view, setBounds: bounds];

    let _: () = msg_send![&**metal_layer, setFrame: bounds];
    let _: () = msg_send![&**metal_layer, setBounds: bounds];
    let _: () = msg_send![&**metal_layer, setDrawableSize: drawable_size];
    let _: () = msg_send![&**metal_layer, setContentsScale: scale];

    // Keep mpv below the overlay after geometry changes.
    if let Some(overlay) = overlay_view {
        let _: () = msg_send![
            &**container_view,
            addSubview: &**video_view,
            positioned: NS_WINDOW_BELOW,
            relativeTo: &**overlay
        ];
    }
}

unsafe fn backing_scale_factor(view: &Retained<AnyObject>) -> f64 {
    let window: *mut AnyObject = msg_send![&**view, window];

    if window.is_null() {
        return 1.0;
    }

    let scale: f64 = msg_send![window, backingScaleFactor];

    if scale <= 0.0 { 1.0 } else { scale }
}

fn ns_rect(x: f64, y: f64, width: f64, height: f64) -> NSRect {
    NSRect::new(NSPoint::new(x, y), NSSize::new(width, height))
}
