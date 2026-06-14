//! fyom Tauri desktop application

mod commands;
mod sidecar;
mod state;
mod tray;
mod window;

use std::sync::Arc;
use std::sync::atomic::{AtomicBool, Ordering};

use crate::state::SidecarState;
use tauri::Emitter;
use tauri::Manager;

pub const MAIN_WINDOW_LABEL: &str = "main";
pub const SIDECAR_READY_EVENT: &str = "fyom-sidecar-ready";
pub const SIDECAR_ERROR_EVENT: &str = "fyom-sidecar-error";
const SIDECAR_STARTUP_TIMEOUT_SECS: u64 = 30;

#[derive(Clone)]
pub struct AppState {
    pub sidecar_state: Arc<SidecarState>,
    pub shutdown_started: Arc<AtomicBool>,
    pub exit_intent: Arc<AtomicBool>,
    pub desktop_db_path: Arc<String>,
}

impl Default for AppState {
    fn default() -> Self {
        Self {
            sidecar_state: Arc::new(SidecarState::default()),
            shutdown_started: Arc::new(AtomicBool::new(false)),
            exit_intent: Arc::new(AtomicBool::new(false)),
            desktop_db_path: Arc::new(String::new()),
        }
    }
}

impl AppState {
    pub fn mark_shutdown(&self) {
        self.shutdown_started.store(true, Ordering::SeqCst);
    }

    pub fn is_shutting_down(&self) -> bool {
        self.shutdown_started.load(Ordering::SeqCst)
    }

    /// Mark that the user intentionally wants to quit the app
    /// (e.g. tray Quit menu). This distinguishes real exit from
    /// window close (hide-to-tray).
    pub fn mark_exit_intent(&self) {
        self.exit_intent.store(true, Ordering::SeqCst);
    }

    pub fn has_exit_intent(&self) -> bool {
        self.exit_intent.load(Ordering::SeqCst)
    }
}

pub fn run() {
    // Initialize tracing
    tracing_subscriber::fmt()
        .with_env_filter(tracing_subscriber::EnvFilter::from_default_env())
        .init();

    let mut app_state = AppState::default();

    // Resolve the desktop DB path from the main app executable directory.
    // This must be done early, before the sidecar is spawned.
    let exe_path = std::env::current_exe().unwrap_or_default();
    let exe_dir = exe_path.parent().unwrap_or(std::path::Path::new("."));
    let desktop_db_path = exe_dir.join("fyom.db").to_string_lossy().to_string();
    app_state.desktop_db_path = Arc::new(desktop_db_path.clone());

    tracing::info!("Desktop environment:");
    tracing::info!("  app_exe:     {}", exe_path.display());
    tracing::info!("  app_exe_dir: {}", exe_dir.display());
    tracing::info!("  db_path:     {}", desktop_db_path);

    let app = tauri::Builder::default()
        .manage(app_state.clone())
        .setup(move |app| {
            // Setup sidecar
            let app_handle = app.handle().clone();
            let state = app_state.clone();
            tauri::async_runtime::spawn(async move {
                if let Err(e) = sidecar::bootstrap_sidecar(&app_handle, &state).await {
                    tracing::error!("Sidecar bootstrap failed: {}", e);
                    let _ = app_handle.emit(SIDECAR_ERROR_EVENT, e.to_string());
                }
            });

            // Setup tray
            tray::setup_tray(app)?;

            // Setup window
            window::setup_main_window(app)?;

            Ok(())
        })
        .plugin(tauri_plugin_opener::init())
        .plugin(tauri_plugin_process::init())
        .invoke_handler(tauri::generate_handler![
            commands::show_window,
            commands::hide_window,
            commands::quit_app,
            commands::get_sidecar_status,
            commands::playback::get_api_base_url,
            commands::playback::get_playback_backend_info,
        ])
        .build(tauri::generate_context!())
        .expect("error while building tauri application");

    // Handle app run events
    app.run(|app_handle, event| {
        match event {
            tauri::RunEvent::ExitRequested { api, .. } => {
                // Check if this is a real app exit (e.g. tray Quit) or just
                // a window close (should hide to tray).
                let has_exit_intent = app_handle
                    .try_state::<AppState>()
                    .map(|s| s.has_exit_intent())
                    .unwrap_or(false);
                if has_exit_intent {
                    // Real exit: allow the default exit behavior after shutdown.
                    // The shutdown itself is handled by request_quit() which was
                    // called by the tray quit handler.
                    tracing::info!("Exit requested with intent, allowing app exit");
                } else {
                    // Window close without exit intent: hide to tray, don't exit.
                    api.prevent_exit();
                    tracing::debug!("Exit requested without intent, hiding to tray");
                }
            }
            tauri::RunEvent::Exit => {
                // Final cleanup on actual exit.
                tracing::info!("App exiting");
            }
            _ => {}
        }
    });
}

#[derive(serde::Serialize, Clone, Debug, Default)]
#[serde(tag = "status", content = "data")]
pub enum SidecarStatus {
    #[default]
    Starting,
    Ready {
        api_base_url: String,
    },
    Error {
        message: String,
    },
}
