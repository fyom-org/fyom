use std::sync::atomic::{AtomicU32, Ordering};use std::JoinHandle;

use libmpv2::{GetData, Mpv, SetData};
use tauri::AppHandle;
use tracing::{debug, error, info, warn};

use crate::mpv::event_loop::{self, ACTIVE, SHUTDOWN};
use crate::mpv::render::{self, RenderSurface};

// ----------------------------------------------------------------------------
// Constants
// ----------------------------------------------------------------------------

const MAX_VOLUME: i64 = 100;
const DEFAULT_VOLUME: i64 = 80;
const DEFAULT_CACHE_MIB: u64 = 256;
const DEFAULT_CACHE_SECS: i64 = 10;

// ----------------------------------------------------------------------------
// Internal states
// ----------------------------------------------------------------------------

struct RenderThread {
    handle: JoinHandle<()>,
    shutdown: Arc<AtomicU32>,
}

// ----------------------------------------------------------------------------
// MpvInstance
// ----------------------------------------------------------------------------

pub struct MpvInstance {
    pub mpv: Arc<Mpv>,

    event_alive: Arc<AtomicU32>,
    event_thread: Mutex<Option<JoinHandle<()>>>,

    render_thread: Mutex<Option<RenderThread>>,
}

impl std::fmt::Debug for MpvInstance {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        let event = self
            .event_thread
            .lock()
            .map(|x| x.is_some())
            .unwrap_or(false);

        let render = self
            .render_thread
            .lock()
            .map(|x| x.is_some())
            .unwrap_or(false);

        f.debug_struct("MpvInstance")
            .field("event_thread_alive", &event)
            .field("render_thread_alive", &render)
            .finish()
    }
}

impl MpvInstance {
    // ------------------------------------------------------------------------
    // Init
    // ------------------------------------------------------------------------

    pub fn new() -> Result<Self, String> {
        set_c_numeric_locale();

        let mpv = Mpv::with_initializer(|init| {
            init.set_property("vo", "libmpv")?;
            init.set_property("osc", false)?;
            init.set_property("osd-level", 0)?;
            init.set_property("hwdec", "auto-safe")?;

            init.set_property("video-sync", "audio")?;
            init.set_property("demuxer-max-bytes", format!("{}MiB", DEFAULT_CACHE_MIB))?;
            init.set_property("cache-secs", DEFAULT_CACHE_SECS)?;

            init.set_property("volume-max", MAX_VOLUME)?;
            init.set_property("volume", DEFAULT_VOLUME)?;

            init.set_property("input-default-bindings", true)?;
            init.set_property("input-vo-keyboard", true)?;

            init.set_property("loop", "no")?;

            Ok(())
        })
        .map_err(|e| format!("mpv init failed: {e}"))?;

        info!("[mpv] instance created");

        Ok(Self {
            mpv: Arc::new(mpv),
            event_alive: Arc::new(AtomicU32::new(ACTIVE)),
            event_thread: Mutex::new(None),
            render_thread: Mutex::new(None),
        })
    }

    // ------------------------------------------------------------------------
    // Event loop
    // ------------------------------------------------------------------------

    pub fn spawn_event_loop(&self, app: AppHandle) {
        let mut guard = match self.event_thread.lock() {
            Ok(g) => g,
            Err(e) => {
                error!("[mpv] event mutex poisoned: {e}");
                return;
            }
        };

        if guard.is_some() {
            debug!("[mpv] event loop already running");
            return;
        }

        self.event_alive.store(ACTIVE, Ordering::SeqCst);

        let handle = event_loop::spawn_event_loop(
            Arc::clone(&self.mpv),
            app,
            Arc::clone(&self.event_alive),
        );

        *guard = Some(handle);

        info!("[mpv] event loop started");
    }

    pub fn shutdown_event_loop(&self) {
        self.event_alive.store(SHUTDOWN, Ordering::SeqCst);

        let handle = match self.event_thread.lock() {
            Ok(mut g) => g.take(),
            Err(_) => None,
        };

        if let Some(h) = handle {
            if h.join().is_err() {
                warn!("[mpv] event thread panic");
            } else {
                info!("[mpv] event thread joined");
            }
        }
    }

    // ------------------------------------------------------------------------
    // Render loop
    // ------------------------------------------------------------------------

    pub fn spawn_render_thread(
        &self,
        surface: Box<dyn RenderSurface>,
    ) -> Result<(), String> {
        let mut guard = self
            .render_thread
            .lock()
            .map_err(|e| format!("render mutex poisoned: {e}"))?;

        if guard.is_some() {
            warn!("[mpv] render already running");
            return Ok(());
        }

        let shutdown = Arc::new(AtomicU32::new(0));

        let handle = render::spawn_render_thread(
            Arc::clone(&self.mpv),
            surface,
            Arc::clone(&shutdown),
        )?;

        *guard = Some(RenderThread { handle, shutdown });

        info!("[mpv] render thread started");

        Ok(())
    }

    pub fn shutdown_render_thread(&self) {
        let thread = match self.render_thread.lock() {
            Ok(mut g) => g.take(),
            Err(_) => None,
        };

        let Some(thread) = thread else { return };

        thread.shutdown.store(SHUTDOWN, Ordering::SeqCst);

        // no wake needed anymore

        if thread.handle.join().is_err() {
            warn!("[mpv] render thread panic");
        } else {
            info!("[mpv] render thread joined");
        }
    }

    // ------------------------------------------------------------------------
    // Playback
    // ------------------------------------------------------------------------

    pub fn loadfile(&self, url: &str) -> Result<(), String> {
        info!("[mpv] loadfile: {}", url);

        self.mpv
            .command("loadfile", &[url, "replace"])
            .map_err(|e| format!("loadfile failed: {e}"))
    }

    pub fn stop(&self) -> Result<(), String> {
        self.mpv
            .command("stop", &[])
            .map_err(|e| format!("stop failed: {e}"))
    }

    pub fn set_pause(&self, v: bool) -> Result<(), String> {
        self.set_property("pause", v)
    }

    pub fn seek(&self, sec: f64) -> Result<(), String> {
        self.mpv
            .command("seek", &[&sec.to_string(), "absolute"])
            .map_err(|e| format!("seek failed: {e}"))
    }

    // ------------------------------------------------------------------------
    // Generic
    // ------------------------------------------------------------------------

    pub fn command(&self, cmd: &str, args: &[&str]) -> Result<(), String> {
        self.mpv
            .command(cmd, args)
            .map_err(|e| format!("{cmd} failed: {e}"))
    }

    pub fn set_property<V: SetData>(&self, key: &str, val: V) -> Result<(), String> {
        self.mpv
            .set_property(key, val)
            .map_err(|e| format!("set_property {key} failed: {e}"))
    }

    pub fn get_property<V: GetData>(&self, key: &str) -> Result<V, String> {
        self.mpv
            .get_property(key)
            .map_err(|e| format!("get_property {key} failed: {e}"))
    }
}

// ----------------------------------------------------------------------------
// Drop
// ----------------------------------------------------------------------------

impl Drop for MpvInstance {
    fn drop(&mut self) {
        // strict order

        self.shutdown_render_thread();
        self.shutdown_event_loop();
    }
}

impl Default for MpvInstance {
    fn default() -> Self {
        Self::new().expect("mpv init failed")
    }
}

// ----------------------------------------------------------------------------
// Locale fix
// ----------------------------------------------------------------------------

fn set_c_numeric_locale() {
    unsafe {
        use libc::{setlocale, LC_NUMERIC};
        setlocale(LC_NUMERIC, b"C\0".as_ptr() as _);
    }
}

// ----------------------------------------------------------------------------
// Safety
// ----------------------------------------------------------------------------

unsafe impl Send for MpvInstance {}
unsafe impl Sync for MpvInstance {}
use std::sync::{Arc, Mutex};
