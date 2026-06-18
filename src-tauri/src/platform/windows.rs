//! Windows GL surface — child `HWND` + WGL context behind the `WebView2` host.
//!
//! PORTED_FROM_SOIA `src-tauri/src/platform/windows.rs` (direction only — soia uses
//! `libsoia_utils`'s closed-source surface; fyom writes the open WGL path. The
//! window-lifecycle logic direction is ported; the surface code is rewritten.)
//!
//! ## Architecture
//! Tauri's main window on Windows contains an `HWND` hosting WebView2. fyom creates a
//! child `HWND` with its own device context and WGL context. The child is placed at the
//! bottom of the sibling z-order, behind the WebView2 host. When the webview root becomes
//! transparent via `.video-mode`, the mpv OpenGL render shows through.
//!
//! The WGL context is created via:
//! - `ChoosePixelFormat`
//! - `SetPixelFormat`
//! - `wglCreateContext`
//! - `wglMakeCurrent`
//!
//! GL function pointers are resolved through:
//! - `wglGetProcAddress` for extension / modern GL symbols
//! - `GetProcAddress(GetModuleHandleA("opengl32.dll"), ...)` for OpenGL 1.1 symbols
//!
//! See `docs/libmpv-assessment.md` §3.3 for the rationale.

use std::ffi::{CString, c_void};
use std::sync::Mutex;

use windows_sys::Win32::Foundation::{HWND, LPARAM, LRESULT, RECT, WPARAM};
use windows_sys::Win32::Graphics::Gdi::{GetDC, HDC, ReleaseDC};
use windows_sys::Win32::Graphics::OpenGL::{
    ChoosePixelFormat, HGLRC, PFD_DOUBLEBUFFER, PFD_DRAW_TO_WINDOW, PFD_MAIN_PLANE,
    PFD_SUPPORT_OPENGL, PFD_TYPE_RGBA, PIXELFORMATDESCRIPTOR, SetPixelFormat, SwapBuffers,
    wglCreateContext, wglDeleteContext, wglGetCurrentContext, wglGetProcAddress, wglMakeCurrent,
};
use windows_sys::Win32::System::LibraryLoader::{
    GetModuleHandleA, GetModuleHandleW, GetProcAddress,
};
use windows_sys::Win32::UI::HiDpi::GetDpiForWindow;
use windows_sys::Win32::UI::WindowsAndMessaging::{
    CS_HREDRAW, CS_OWNDC, CS_VREDRAW, CreateWindowExW, DefWindowProcW, DestroyWindow,
    GetClientRect, HWND_BOTTOM, RegisterClassExW, SWP_NOACTIVATE, SWP_NOMOVE, SWP_NOSIZE,
    SetWindowPos, WNDCLASSEXW, WS_CHILD, WS_CLIPCHILDREN, WS_CLIPSIBLINGS, WS_VISIBLE,
};

use crate::mpv::render::RenderSurface;

// ---------------------------------------------------------------------------
// Constants / helpers.
// ---------------------------------------------------------------------------

/// The custom window class name for fyom's child GL HWND.
const FYOM_MPV_CHILD_CLASS: &[u16] = &[
    b'F' as u16,
    b'y' as u16,
    b'o' as u16,
    b'm' as u16,
    b'M' as u16,
    b'p' as u16,
    b'v' as u16,
    b'C' as u16,
    b'h' as u16,
    b'i' as u16,
    b'l' as u16,
    b'd' as u16,
    0,
];

/// ERROR_CLASS_ALREADY_EXISTS.
const ERROR_CLASS_ALREADY_EXISTS: i32 = 1410;

/// WGL may return sentinel values for failed extension lookups on some drivers.
const INVALID_WGL_PROC_1: usize = 1;
const INVALID_WGL_PROC_2: usize = 2;
const INVALID_WGL_PROC_3: usize = 3;

/// Function pointer shape returned by `wglGetProcAddress` / `GetProcAddress` in
/// `windows-sys 0.59`.
type WinProcAddress = unsafe extern "system" fn() -> isize;

fn last_win32_error(context: &str) -> String {
    let code = std::io::Error::last_os_error().raw_os_error().unwrap_or(0);
    format!("{context} failed (Win32 error {code})")
}

#[inline]
fn null_hwnd() -> HWND {
    std::ptr::null_mut()
}

#[inline]
fn null_hdc() -> HDC {
    std::ptr::null_mut()
}

#[inline]
fn null_hglrc() -> HGLRC {
    std::ptr::null_mut()
}

#[inline]
fn proc_address_to_void_ptr(proc: Option<WinProcAddress>) -> *mut c_void {
    match proc {
        Some(func) => {
            let ptr = func as usize as *mut c_void;
            if is_valid_wgl_proc_address(ptr) {
                ptr
            } else {
                std::ptr::null_mut()
            }
        }
        None => std::ptr::null_mut(),
    }
}

#[inline]
fn is_valid_wgl_proc_address(ptr: *mut c_void) -> bool {
    let value = ptr as usize;

    value != 0
        && value != INVALID_WGL_PROC_1
        && value != INVALID_WGL_PROC_2
        && value != INVALID_WGL_PROC_3
        && value != usize::MAX
}

/// Minimal child window procedure.
///
/// A real WndProc is required for a registered Win32 window class. Delegating everything
/// to `DefWindowProcW` is sufficient for this passive OpenGL child window.
unsafe extern "system" fn fyom_mpv_child_wnd_proc(
    hwnd: HWND,
    msg: u32,
    wparam: WPARAM,
    lparam: LPARAM,
) -> LRESULT {
    // SAFETY: This is the documented default handler for messages we do not process.
    unsafe { DefWindowProcW(hwnd, msg, wparam, lparam) }
}

/// Register the child window class once.
///
/// If the class already exists, this is treated as success.
fn register_child_class() -> Result<(), String> {
    let hinstance = unsafe { GetModuleHandleW(std::ptr::null()) };

    if hinstance.is_null() {
        return Err(last_win32_error("GetModuleHandleW"));
    }

    let wcex = WNDCLASSEXW {
        cbSize: std::mem::size_of::<WNDCLASSEXW>() as u32,
        style: CS_HREDRAW | CS_VREDRAW | CS_OWNDC,
        lpfnWndProc: Some(fyom_mpv_child_wnd_proc),
        cbClsExtra: 0,
        cbWndExtra: 0,
        hInstance: hinstance,
        hIcon: std::ptr::null_mut(),
        hCursor: std::ptr::null_mut(),
        hbrBackground: std::ptr::null_mut(),
        lpszMenuName: std::ptr::null(),
        lpszClassName: FYOM_MPV_CHILD_CLASS.as_ptr(),
        hIconSm: std::ptr::null_mut(),
    };

    let atom = unsafe { RegisterClassExW(&wcex) };

    if atom != 0 {
        return Ok(());
    }

    let err = std::io::Error::last_os_error().raw_os_error().unwrap_or(0);

    if err == ERROR_CLASS_ALREADY_EXISTS {
        return Ok(());
    }

    Err(format!("RegisterClassExW failed (Win32 error {err})"))
}

fn make_pixel_format_descriptor() -> PIXELFORMATDESCRIPTOR {
    let mut pfd: PIXELFORMATDESCRIPTOR = unsafe { std::mem::zeroed() };

    pfd.nSize = std::mem::size_of::<PIXELFORMATDESCRIPTOR>() as u16;
    pfd.nVersion = 1;
    pfd.dwFlags = PFD_DRAW_TO_WINDOW | PFD_SUPPORT_OPENGL | PFD_DOUBLEBUFFER;
    pfd.iPixelType = PFD_TYPE_RGBA as u8;
    pfd.cColorBits = 24;
    pfd.cAlphaBits = 8;
    pfd.cDepthBits = 24;
    pfd.cStencilBits = 8;
    pfd.iLayerType = PFD_MAIN_PLANE as u8;

    pfd
}

fn parent_client_size(parent_hwnd: HWND) -> Result<(i32, i32), String> {
    let mut rect = RECT {
        left: 0,
        top: 0,
        right: 0,
        bottom: 0,
    };

    let ok = unsafe { GetClientRect(parent_hwnd, &mut rect) };

    if ok == 0 {
        return Err(last_win32_error("GetClientRect(parent)"));
    }

    let width = (rect.right - rect.left).max(1);
    let height = (rect.bottom - rect.top).max(1);

    Ok((width, height))
}

// ---------------------------------------------------------------------------
// WinGlSurface.
// ---------------------------------------------------------------------------

/// The Windows GL surface: owns a child `HWND`, its `HDC`, and one WGL context.
///
/// The render thread is the sole GL consumer. `wglMakeCurrent` binds the context to that
/// thread when rendering starts.
pub struct WinGlSurface {
    /// Child HWND created under the Tauri/WebView2 parent window.
    hwnd: HWND,

    /// Device context for the child HWND.
    hdc: HDC,

    /// WGL rendering context.
    hglrc: HGLRC,

    /// Defensive current-context cache.
    make_current_called: Mutex<bool>,
}

// SAFETY: HWND/HDC/HGLRC are OS handles. The render thread is the only thread that issues
// GL commands. WGL context binding is thread-local and done through `wglMakeCurrent`.
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

        if self.hdc.is_null() {
            return Err("HDC is null".to_string());
        }

        if self.hglrc.is_null() {
            return Err("HGLRC is null".to_string());
        }

        let ok = unsafe { wglMakeCurrent(self.hdc, self.hglrc) };

        if ok == 0 {
            return Err(last_win32_error("wglMakeCurrent"));
        }

        *called = true;
        Ok(())
    }

    fn get_proc_address(&self, name: &str) -> *mut c_void {
        let c_name = match CString::new(name) {
            Ok(name) => name,
            Err(_) => return std::ptr::null_mut(),
        };

        let symbol = c_name.as_ptr() as *const u8;

        // Try WGL first. This is required for modern GL / extension symbols.
        let wgl_proc = unsafe { wglGetProcAddress(symbol) };
        let wgl_ptr = proc_address_to_void_ptr(wgl_proc);

        if !wgl_ptr.is_null() {
            return wgl_ptr;
        }

        // Fall back to opengl32.dll for OpenGL 1.1 symbols.
        let opengl32 = unsafe { GetModuleHandleA(b"opengl32.dll\0".as_ptr()) };

        if opengl32.is_null() {
            return std::ptr::null_mut();
        }

        let dll_proc = unsafe { GetProcAddress(opengl32, symbol) };

        proc_address_to_void_ptr(dll_proc)
    }

    fn drawable_size(&self) -> (i32, i32) {
        if self.hwnd.is_null() {
            return (0, 0);
        }

        let mut rect = RECT {
            left: 0,
            top: 0,
            right: 0,
            bottom: 0,
        };

        let ok = unsafe { GetClientRect(self.hwnd, &mut rect) };

        if ok == 0 {
            return (0, 0);
        }

        let logical_w = (rect.right - rect.left).max(0);
        let logical_h = (rect.bottom - rect.top).max(0);

        let dpi = unsafe { GetDpiForWindow(self.hwnd) };

        if dpi == 0 {
            return (logical_w, logical_h);
        }

        let scale = dpi as f64 / 96.0;
        let physical_w = (logical_w as f64 * scale).round() as i32;
        let physical_h = (logical_h as f64 * scale).round() as i32;

        (physical_w.max(0), physical_h.max(0))
    }

    fn swap_buffers(&self) {
        if self.hdc.is_null() {
            return;
        }

        // SAFETY: `hdc` is a valid DC with a double-buffered pixel format.
        unsafe {
            let _ = SwapBuffers(self.hdc);
        }
    }
}

impl Drop for WinGlSurface {
    fn drop(&mut self) {
        unsafe {
            if !self.hglrc.is_null() {
                let current = wglGetCurrentContext();

                if current == self.hglrc {
                    let _ = wglMakeCurrent(null_hdc(), null_hglrc());
                }

                let _ = wglDeleteContext(self.hglrc);
            }

            if !self.hwnd.is_null() && !self.hdc.is_null() {
                let _ = ReleaseDC(self.hwnd, self.hdc);
            }

            if !self.hwnd.is_null() {
                let _ = DestroyWindow(self.hwnd);
            }
        }

        self.hwnd = null_hwnd();
        self.hdc = null_hdc();
        self.hglrc = null_hglrc();
    }
}

// ---------------------------------------------------------------------------
// Factory.
// ---------------------------------------------------------------------------

/// Create the Windows GL surface for the given Tauri window.
///
/// Steps:
/// 1. Register the child window class.
/// 2. Get the parent HWND from the Tauri raw window handle.
/// 3. Create a child HWND sized to the parent client area.
/// 4. Move the child HWND to the bottom of the z-order.
/// 5. Get the child HDC.
/// 6. Choose and set a double-buffered RGBA OpenGL pixel format on the child HDC.
/// 7. Create a WGL context for the child HDC.
pub fn create_surface(window: &tauri::WebviewWindow) -> Result<Box<dyn RenderSurface>, String> {
    use raw_window_handle::{HasWindowHandle, RawWindowHandle};

    register_child_class()?;

    let raw_handle = window
        .window_handle()
        .map_err(|e| format!("failed to get raw window handle: {}", e))?;

    let win32_handle = match raw_handle.as_ref() {
        RawWindowHandle::Win32(handle) => handle,
        _ => return Err("expected Win32 raw window handle on Windows".to_string()),
    };

    let parent_hwnd = win32_handle.hwnd.get() as HWND;

    if parent_hwnd.is_null() {
        return Err("parent HWND is null".to_string());
    }

    let (child_w, child_h) = parent_client_size(parent_hwnd)?;

    let hinstance = unsafe { GetModuleHandleW(std::ptr::null()) };

    if hinstance.is_null() {
        return Err(last_win32_error("GetModuleHandleW"));
    }

    let child_hwnd = unsafe {
        CreateWindowExW(
            0,
            FYOM_MPV_CHILD_CLASS.as_ptr(),
            FYOM_MPV_CHILD_CLASS.as_ptr(),
            WS_CHILD | WS_VISIBLE | WS_CLIPSIBLINGS | WS_CLIPCHILDREN,
            0,
            0,
            child_w,
            child_h,
            parent_hwnd,
            std::ptr::null_mut(),
            hinstance,
            std::ptr::null(),
        )
    };

    if child_hwnd.is_null() {
        return Err(last_win32_error("CreateWindowExW(child)"));
    }

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

    let child_hdc = unsafe { GetDC(child_hwnd) };

    if child_hdc.is_null() {
        unsafe {
            let _ = DestroyWindow(child_hwnd);
        }

        return Err(last_win32_error("GetDC(child)"));
    }

    let pfd = make_pixel_format_descriptor();

    let pixel_format = unsafe { ChoosePixelFormat(child_hdc, &pfd) };

    if pixel_format == 0 {
        unsafe {
            let _ = ReleaseDC(child_hwnd, child_hdc);
            let _ = DestroyWindow(child_hwnd);
        }

        return Err("ChoosePixelFormat returned 0; no matching OpenGL pixel format".to_string());
    }

    let ok = unsafe { SetPixelFormat(child_hdc, pixel_format, &pfd) };

    if ok == 0 {
        unsafe {
            let _ = ReleaseDC(child_hwnd, child_hdc);
            let _ = DestroyWindow(child_hwnd);
        }

        return Err(last_win32_error("SetPixelFormat"));
    }

    let hglrc = unsafe { wglCreateContext(child_hdc) };

    if hglrc.is_null() {
        unsafe {
            let _ = ReleaseDC(child_hwnd, child_hdc);
            let _ = DestroyWindow(child_hwnd);
        }

        return Err(last_win32_error("wglCreateContext"));
    }

    tracing::info!(
        "[platform/windows] child HWND + WGL context created behind WebView2 \
         ({}x{}, pixel format {}, double-buffered RGBA)",
        child_w,
        child_h,
        pixel_format
    );

    Ok(Box::new(WinGlSurface {
        hwnd: child_hwnd,
        hdc: child_hdc,
        hglrc,
        make_current_called: Mutex::new(false),
    }))
}
