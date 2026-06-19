//! macOS render surface — `NSView` + `CAMetalLayer` for libmpv `--wid` embedding.
//!
//! ## Architecture
//! Tauri's main window on macOS contains an `NSView` content view hosting a `WKWebView`.
//! fyom creates a child `NSView`, explicitly attaches a `CAMetalLayer` to it, and places
//! it behind the webview.
//!
//! When the webview root becomes transparent via `.video-mode`, the mpv render shows through.
//! libmpv is configured with `--wid=<nsview_ptr>` and `--gpu-api=vulkan` (via MoltenVK).
//! mpv handles all Vulkan/Metal initialization and rendering internally on this layer.
//!
//! ## Safety
//! Uses `objc2` `Retained` pointers to ensure Objective-C objects are not prematurely
//! garbage-collected by ARC, preventing EXC_BAD_ACCESS crashes during mpv initialization.

use std::ffi::c_void;
use std::sync::Mutex;

use objc2::rc::Retained;
use objc2::runtime::AnyObject;
use objc2::{class, msg_send, msg_send_id};
use objc2_foundation::NSRect;

use crate::mpv::render::RenderSurface;

// ---------------------------------------------------------------------------
// AppKit Constants
// ---------------------------------------------------------------------------

/// `NSWindowBelow` / `NSWindowOrderingMode`.
const NS_WINDOW_BELOW: isize = 1;

/// `NSViewWidthSizable`.
const NS_VIEW_WIDTH_SIZABLE: usize = 2;

/// `NSViewHeightSizable`.
const NS_VIEW_HEIGHT_SIZABLE: usize = 16;

// ---------------------------------------------------------------------------
// MacosSurface
// ---------------------------------------------------------------------------

/// macOS render surface backed by an `NSView` + `CAMetalLayer`.
///
/// Owns the retained child `NSView`. The render thread does not need to interact
/// with GL contexts directly, as libmpv handles all rendering internally via `--wid`.
pub struct MacosSurface {
    /// Retained child `NSView` containing the `CAMetalLayer`.
    view: Retained<AnyObject>,

    /// Defensive dummy lock to satisfy trait requirements if needed.
    _dummy_lock: Mutex<bool>,
}

impl MacosSurface {
    /// Returns the raw pointer address of the underlying `NSView`.
    /// This value should be passed to libmpv via the `wid` option as a decimal string.
    pub fn wid(&self) -> usize {
        Retained::as_ptr(&self.view) as usize
    }
}

impl RenderSurface for MacosSurface {
    /// In `--wid` mode, libmpv manages its own GL/Vulkan context.
    /// This method is a no-op but required by the trait.
    fn make_current(&self) -> Result<(), String> {
        Ok(())
    }

    /// In `--wid` mode, libmpv resolves GL symbols internally.
    fn get_proc_address(&self, _name: &str) -> *mut c_void {
        std::ptr::null_mut()
    }

    fn drawable_size(&self) -> (i32, i32) {
        unsafe {
            let frame: NSRect = msg_send![&self.view, frame];

            let window: *mut AnyObject = msg_send![&self.view, window];
            let scale: f64 = if window.is_null() {
                1.0
            } else {
                msg_send![window, backingScaleFactor]
            };

            let width = (frame.size.width * scale).round() as i32;
            let height = (frame.size.height * scale).round() as i32;

            (width.max(0), height.max(0))
        }
    }

    /// In `--wid` mode, libmpv handles buffer swapping internally.
    fn swap_buffers(&self) {}
}

// ---------------------------------------------------------------------------
// Factory
// ---------------------------------------------------------------------------

/// Create the macOS Metal-backed surface for the given Tauri window.
///
/// Steps:
/// 1. Get the Tauri window's AppKit `NSView`.
/// 2. Create a generic child `NSView`.
/// 3. Explicitly attach a `CAMetalLayer` to the child view.
/// 4. Add the child view behind the webview (`NSWindowBelow`).
/// 5. Configure autoresizing masks so the video layer follows window resizes.
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

    // Retain the parent view so we can safely send messages to it
    let parent_view: Retained<AnyObject> = unsafe {
        Retained::retain(ns_view_ptr).ok_or_else(|| "Failed to retain parent NSView".to_string())?
    };

    // STEP 2: create generic child NSView
    let child_view: Retained<AnyObject> = unsafe { msg_send_id![class!(NSView), new] };

    if child_view.is_null() {
        return Err("Failed to alloc/init child NSView".to_string());
    }

    // STEP 3: Explicitly attach CAMetalLayer
    // This is the critical fix for macOS 14+ where OpenGL is deprecated and MoltenVK
    // requires a Metal-backed surface to initialize properly.
    unsafe {
        // setWantsLayer: YES
        let _: () = msg_send![&child_view, setWantsLayer: 1i32];

        // Create and set CAMetalLayer
        let metal_layer: Retained<AnyObject> = msg_send_id![class!(CAMetalLayer), layer];
        if metal_layer.is_null() {
            return Err("Failed to create CAMetalLayer instance".to_string());
        }

        let _: () = msg_send![&child_view, setLayer: &*metal_layer];
    }

    // STEP 4: Add child view behind the webview
    unsafe {
        let nil_view: *mut AnyObject = std::ptr::null_mut();

        let _: () = msg_send![
            &parent_view,
            addSubview: &*child_view,
            positioned: NS_WINDOW_BELOW,
            relativeTo: nil_view
        ];

        // Match parent frame initially
        let frame: NSRect = msg_send![&parent_view, frame];
        let _: () = msg_send![&child_view, setFrame: frame];
    }

    // STEP 5: Configure autoresizing
    unsafe {
        let autoresizing_mask = NS_VIEW_WIDTH_SIZABLE | NS_VIEW_HEIGHT_SIZABLE;
        let _: () = msg_send![&child_view, setAutoresizingMask: autoresizing_mask];

        let _: () = msg_send![&child_view, setHidden: 0i32];
    }

    tracing::info!(
        "[platform/macos] NSView + CAMetalLayer created behind WKWebView for libmpv --wid embedding."
    );

    Ok(Box::new(MacosSurface {
        view: child_view,
        _dummy_lock: Mutex::new(false),
    }))
}
