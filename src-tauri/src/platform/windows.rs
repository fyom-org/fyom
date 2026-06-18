//! Windows GL surface — child `HWND` + WGL context behind the `WebView2` host.
//!
//! PORTED_FROM_SOIA `src-tauri/src/platform/windows.rs` (direction only — soia uses
//! `libsoia_utils`'s closed-source surface; fyom writes the open WGL path. The
//! window-lifecycle logic direction is ported; the surface code is rewritten.)
//!
//! ## Architecture
//! Tauri's main window on Windows contains an `HWND` hosting a `WebView2`. fyom creates
//! a child `HWND` (a separate `WS_CHILD` window, registered with a custom window class
//! `L"FyomMpvChild"`) that sits behind the WebView2 in z-order. When the webview's root
//! CSS goes `background: transparent !important` (the `.video-mode` class, ported from
//! soia), the mpv GL render shows through.
//!
//! The WGL context is created via `wglCreateContext` + `wglMakeCurrent`. `get_proc_address`
//! resolves GL function pointers via `wglGetProcAddress` (for GL 1.2+ functions) +
//! `GetProcAddress(GetModuleHandle("opengl32.dll"), ...)` (for GL 1.1 functions like
//! `glClear`, `glGetIntegerv` — `wglGetProcAddress` returns null for these).
//!
//! See `docs/libmpv-assessment.md` §3.3 for the rationale.

use std::ffi::{CString, c_void};
use std::sync::Mutex;

use windows_sys::Win32::Foundation::HWND;
use windows_sys::Win32::Graphics::Gdi::HDC;
use windows_sys::Win32::Graphics::OpenGL::{
    wglCreateContext, wglDeleteContext, wglGetCurrentContext, wglGetProcAddress, wglMakeCurrent,
};
use windows_sys::Win32::UI::WindowsAndMessaging::{
    CreateWindowExW, DestroyWindow, HWND_BOTTOM, SetWindowPos, SWP_NOACTIVATE, SWP_NOMOVE,
    SWP_NOSIZE, WS_CHILD, WS_VISIBLE,
};

use crate::mpv::render::RenderSurface;

// ---------------------------------------------------------------------------
// Child window class registration (idempotent).
// ---------------------------------------------------------------------------

/// The custom window class name for fyom's child GL HWND.
const FYOM_MPV_CHILD_CLASS: &[u16] = &[
    b'F' as u16, b'y' as u16, b'o' as u16, b'm' as u16, b'M' as u16, b'p' as u16,
    b'C' as u16, b'h' as u16, b'i' as u16, b'l' as u16, b'd' as u16, 0, // UTF-16 null terminator
];

/// Register the child window class once (idempotent — checks if already registered).
///
/// SAFETY: calls RegisterClassExW with a WNDCLASSEXW struct. Idempotent because we check
/// the return value for ERROR_CLASS_ALREADY_EXISTS.
fn register_child_class() -> Result<(), String> {
    use windows_sys::Win32::UI::WindowsAndMessaging::{
        RegisterClassExW, WNDCLASSEXW, CS_HREDRAW, CS_VREDRAW, CS_OWNDC,
    };

    let class_name_ptr = FYOM_MPV_CHILD_CLASS.as_ptr();
    let wcex = WNDCLASSEXW {
        cbSize: std::mem::size_of::<WNDCLASSEXW>() as u32,
        style: CS_HREDRAW | CS_VREDRAW | CS_OWNDC,
        lpfnWndProc: None, // use DefWindowProcW
        cbClsExtra: 0,
        cbWndExtra: 0,
        hInstance: std::ptr::null_mut(),
        hIcon: std::ptr::null_mut(),
        hCursor: std::ptr::null_mut(),
        hbrBackground: std::ptr::null_mut(),
        lpszMenuName: std::ptr::null(),
        lpszClassName: class_name_ptr,
        hIconSm: std::ptr::null_mut(),
    };
    // SAFETY: RegisterClassExW is the documented API. Returns 0 on failure (check
    // GetLastError for ERROR_CLASS_ALREADY_EXISTS → treat as success).
    let atom = unsafe { RegisterClassExW(&wcex) };
    if atom != 0 {
        return Ok(());
    }
    let err = std::io::Error::last_os_error().raw_os_error().unwrap_or(0);
    // ERROR_CLASS_ALREADY_EXISTS = 1410 — idempotent registration is OK.
    if err == 1410 {
        return Ok(());
    }
    Err(format!("RegisterClassExW failed (Win32 error {})", err))
}

// ---------------------------------------------------------------------------
// WinGlSurface — the RenderSurface impl.
// ---------------------------------------------------------------------------

/// The Windows GL surface: owns a child `HWND` + `HDC` + WGL context.
///
/// `Send` because the HWND + HDC + HGLRC are process-global handles (not thread-affine);
/// `wglMakeCurrent` binds the context to the calling thread. The render thread is the
/// sole GL consumer.
pub struct WinGlSurface {
    /// The child HWND (a WS_CHILD window behind the WebView2). Destroyed on drop.
    hwnd: isize,
    /// The HDC for the HWND's client area (the GL drawable).
    hdc: isize,
    /// The WGL rendering context (`HGLRC`). Made current on the render thread via
    /// `wglMakeCurrent(hdc, hglrc)`.
    hglrc: isize,
    /// Cached current-context state (defensive — sole caller is the render thread).
    make_current_called: Mutex<bool>,
}

// SAFETY: HWND + HDC + HGLRC are process-global handles; WGL is documented thread-safe
// for the operations fyom uses (wglMakeCurrent, wglGetProcAddress, SwapBuffers). The
// render thread is the sole consumer of all GL calls.
unsafe impl Send for WinGlSurface {}

impl RenderSurface for WinGlSurface {
    fn make_current(&self) -> Result<(), String> {
        let mut called = self
            .make_current_called
            .lock()
            .map_err(|e| format!("make_current mutex poisoned: {}", e))?;
        if *called {
            return Ok(());
        }
        // SAFETY: `hdc` + `hglrc` are valid handles created in `create_surface`.
        // `wglMakeCurrent(hdc, hglrc)` binds the context to the calling thread.
        let ok = unsafe { wglMakeCurrent(self.hdc as HDC, self.hglrc as isize) };
        if ok == 0 {
            return Err(format!(
                "wglMakeCurrent failed (Win32 error {})",
                std::io::Error::last_os_error().raw_os_error().unwrap_or(0)
            ));
        }
        *called = true;
        Ok(())
    }

    fn get_proc_address(&self, name: &str) -> *mut c_void {
        // WGL resolves GL 1.2+ functions via wglGetProcAddress, but GL 1.1 functions
        // (glClear, glGetIntegerv, etc.) are in opengl32.dll + must be resolved via
        // GetProcAddress. We try wgl first, then fall back to opengl32.dll.
        let c_name = match CString::new(name) {
            Ok(c) => c,
            Err(_) => return std::ptr::null_mut(),
        };
        // SAFETY: `wglGetProcAddress` is the documented WGL API. Returns a function
        // pointer valid for the current WGL context, or NULL for GL 1.1 functions /
        // unknown names. Thread-safe.
        let ptr = unsafe { wglGetProcAddress(c_name.as_ptr()) };
        if !ptr.is_null() {
            return ptr as *mut c_void;
        }
        // Fall back to opengl32.dll for GL 1.1 functions.
        // SAFETY: `GetModuleHandleA` returns the already-loaded opengl32.dll handle.
        unsafe {
            let lib = windows_sys::Win32::System::LibraryLoader::GetModuleHandleA(
                b"opengl32.dll\0".as_ptr(),
            );
            if lib.is_null() {
                return std::ptr::null_mut();
            }
            windows_sys::Win32::System::LibraryLoader::GetProcAddress(lib, c_name.as_ptr())
                .map_or(std::ptr::null_mut(), |f| f as *mut c_void)
        }
    }

    fn drawable_size(&self) -> (i32, i32) {
        // GetClientRect returns the client area in logical pixels. Multiply by the
        // window's DPI scale to get physical pixels (HiDPI).
        //
        // PORTED_FROM_TSUKIMI pattern:
        //   let factor = self.obj().scale_factor();
        //   let width = self.obj().width() * factor;
        //   let height = self.obj().height() * factor;
        let mut rect = windows_sys::Win32::Foundation::RECT {
            left: 0,
            top: 0,
            right: 0,
            bottom: 0,
        };
        // SAFETY: `hwnd` is a valid HWND; `GetClientRect` is the documented API.
        let ok = unsafe {
            windows_sys::Win32::UI::WindowsAndMessaging::GetClientRect(
                self.hwnd as HWND,
                &mut rect,
            )
        };
        if ok == 0 {
            return (0, 0);
        }
        let logical_w = (rect.right - rect.left).max(0);
        let logical_h = (rect.bottom - rect.top).max(0);
        // Get the DPI scale via GetDpiForWindow (Windows 10 1607+).
        let dpi = unsafe { windows_sys::Win32::UI::HiDpi::GetDpiForWindow(self.hwnd as HWND) };
        if dpi == 0 {
            return (logical_w, logical_h);
        }
        let scale = dpi as f64 / 96.0;
        let physical_w = (logical_w as f64 * scale) as i32;
        let physical_h = (logical_h as f64 * scale) as i32;
        (physical_w.max(0), physical_h.max(0))
    }

    fn swap_buffers(&self) {
        // SAFETY: `hdc` is a valid HDC; `SwapBuffers` is the documented GDI API for
        // double-buffered pixel formats. WGL requires this (not wglSwapBuffers).
        unsafe {
            windows_sys::Win32::Graphics::Gdi::SwapBuffers(self.hdc as HDC);
        }
    }
}

impl Drop for WinGlSurface {
    fn drop(&mut self) {
        // Clear the current context (if this thread has it current) + delete the context.
        unsafe {
            if wglGetCurrentContext() == self.hglrc as isize {
                let _ = wglMakeCurrent(std::ptr::null_mut(), std::ptr::null_mut());
            }
            let _ = wglDeleteContext(self.hglrc as isize);
            // Release the DC + destroy the child window.
            let _ = windows_sys::Win32::Graphics::Gdi::ReleaseDC(
                self.hwnd as HWND,
                self.hdc as HDC,
            );
            let _ = DestroyWindow(self.hwnd as HWND);
        }
    }
}

// ---------------------------------------------------------------------------
// Factory — create the surface from a Tauri WebviewWindow.
// ---------------------------------------------------------------------------

/// Create the Windows GL surface for the given Tauri window.
///
/// Steps:
/// 1. Register the child window class (idempotent).
/// 2. Get the parent HWND from the Tauri window's raw window handle.
/// 3. Get the parent's client rect (to size the child).
/// 4. Get the parent's HDC (for pixel-format negotiation).
/// 5. Choose + set a double-buffered RGBA pixel format with alpha + depth.
/// 6. Create the child HWND (`WS_CHILD` + `WS_VISIBLE`, sized to the parent's client area).
/// 7. Position the child behind the WebView2 (`SetWindowPos` with `HWND_BOTTOM`).
/// 8. Get the child's HDC (it inherits the parent's pixel format).
/// 9. Create the WGL context (`wglCreateContext`).
///
/// On any failure, returns `Err` — the caller logs + continues without GL rendering.
pub fn create_surface(window: &tauri::WebviewWindow) -> Result<Box<dyn RenderSurface>, String> {
    use raw_window_handle::{HasWindowHandle, RawWindowHandle};
    use windows_sys::Win32::Graphics::Gdi::{
        ChoosePixelFormat, GetDC, GetClientRect, PIXELFORMATDESCRIPTOR, PFD_DOUBLEBUFFER,
        PFD_DRAW_TO_WINDOW, PFD_MAIN_PLANE, PFD_SUPPORT_OPENGL, PFD_TYPE_RGBA, ReleaseDC,
        SetPixelFormat,
    };

    // STEP 1: register the child window class (idempotent).
    register_child_class()?;

    // STEP 2: get the parent HWND.
    let raw_handle = window
        .window_handle()
        .map_err(|e| format!("failed to get raw window handle: {}", e))?;
    let win32_handle = match raw_handle.as_ref() {
        RawWindowHandle::Win32(h) => h,
        _ => return Err("expected Win32 raw window handle on Windows".to_string()),
    };
    let parent_hwnd = win32_handle.hwnd.get() as HWND;
    if parent_hwnd.is_null() {
        return Err("parent HWND is null".to_string());
    }

    // STEP 3: get the parent's client rect.
    let mut parent_rect = windows_sys::Win32::Foundation::RECT {
        left: 0,
        top: 0,
        right: 0,
        bottom: 0,
    };
    // SAFETY: `parent_hwnd` is valid; `GetClientRect` is the documented API.
    if unsafe { GetClientRect(parent_hwnd, &mut parent_rect) } == 0 {
        return Err("GetClientRect(parent) failed".to_string());
    }
    let child_w = (parent_rect.right - parent_rect.left).max(1);
    let child_h = (parent_rect.bottom - parent_rect.top).max(1);

    // STEP 4: get the parent's HDC.
    let parent_hdc = unsafe { GetDC(parent_hwnd) };
    if parent_hdc.is_null() {
        return Err("GetDC(parent) returned null".to_string());
    }

    // STEP 5: choose + set the pixel format on the parent HDC (the child inherits it).
    let pfd = PIXELFORMATDESCRIPTOR {
        nSize: std::mem::size_of::<PIXELFORMATDESCRIPTOR>() as u16,
        nVersion: 1,
        dwFlags: PFD_DRAW_TO_WINDOW | PFD_SUPPORT_OPENGL | PFD_DOUBLEBUFFER,
        iPixelType: PFD_TYPE_RGBA as u8,
        cColorBits: 24,
        cRedBits: 0,
        cRedShift: 0,
        cGreenBits: 0,
        cGreenShift: 0,
        cBlueBits: 0,
        cBlueShift: 0,
        cAlphaBits: 8,
        cAlphaShift: 0,
        cAccumBits: 0,
        cAccumRedBits: 0,
        cAccumGreenBits: 0,
        cAccumBlueBits: 0,
        cAccumAlphaBits: 0,
        cDepthBits: 24,
        cStencilBits: 8,
        cAuxBuffers: 0,
        iLayerType: PFD_MAIN_PLANE as u8,
        bReserved: 0,
        dwLayerMask: 0,
        dwVisibleMask: 0,
        dwDamageMask: 0,
    };
    let pixel_format = unsafe { ChoosePixelFormat(parent_hdc as HDC, &pfd) };
    if pixel_format == 0 {
        unsafe { ReleaseDC(parent_hwnd, parent_hdc as HDC) };
        return Err("ChoosePixelFormat returned 0 (no matching pixel format)".to_string());
    }
    if unsafe { SetPixelFormat(parent_hdc as HDC, pixel_format, &pfd) } == 0 {
        unsafe { ReleaseDC(parent_hwnd, parent_hdc as HDC) };
        return Err("SetPixelFormat failed".to_string());
    }

    // STEP 6: create the child HWND.
    // SAFETY: CreateWindowExW with WS_CHILD creates a child window. The class name must
    // have been registered (step 1).
    let child_hwnd = unsafe {
        CreateWindowExW(
            0, // dwExStyle
            FYOM_MPV_CHILD_CLASS.as_ptr(),
            FYOM_MPV_CHILD_CLASS.as_ptr(), // window title (ignored for WS_CHILD)
            WS_CHILD | WS_VISIBLE,
            0,
            0,
            child_w,
            child_h,
            parent_hwnd,
            std::ptr::null_mut(),
            std::ptr::null_mut(),
            std::ptr::null(),
        )
    };
    if child_hwnd.is_null() {
        unsafe { ReleaseDC(parent_hwnd, parent_hdc as HDC) };
        return Err("CreateWindowExW for child HWND failed".to_string());
    }

    // STEP 7: position the child behind the WebView2 (which is a sibling HWND).
    // SAFETY: SetWindowPos with HWND_BOTTOM sends the window to the bottom of the z-order.
    unsafe {
        let _ = SetWindowPos(
            child_hwnd,
            HWND_BOTTOM,
            0,
            0,
            0,
            0,
            SWP_NOMOVE | SWP_NOSIZE | SWP_NOACTIVATE,
        );
    }

    // STEP 8: get the child's HDC.
    let child_hdc = unsafe { GetDC(child_hwnd) };
    if child_hdc.is_null() {
        unsafe {
            let _ = DestroyWindow(child_hwnd);
            ReleaseDC(parent_hwnd, parent_hdc as HDC);
        }
        return Err("GetDC(child) returned null".to_string());
    }

    // STEP 9: create the WGL context.
    let hglrc = unsafe { wglCreateContext(child_hdc as HDC) };
    if hglrc.is_null() {
        unsafe {
            ReleaseDC(child_hwnd, child_hdc as HDC);
            let _ = DestroyWindow(child_hwnd);
            ReleaseDC(parent_hwnd, parent_hdc as HDC);
        }
        return Err("wglCreateContext returned null".to_string());
    }

    // Release the parent's DC (no longer needed).
    unsafe { ReleaseDC(parent_hwnd, parent_hdc as HDC) };

    tracing::info!(
        "[platform/windows] child HWND + WGL context created behind WebView2 \
         (pixel format {}, double-buffered RGBA)",
        pixel_format
    );

    Ok(Box::new(WinGlSurface {
        hwnd: child_hwnd as isize,
        hdc: child_hdc as isize,
        hglrc: hglrc as isize,
        make_current_called: Mutex::new(false),
    }))
}
