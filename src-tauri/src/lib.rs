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

pub const MAIN_WINDOW_LABEL: &str = "main";
pub const SIDECAR_READY_EVENT: &str = "fyom-sidecar-ready";
pub const SIDECAR_ERROR_EVENT: &str = "fyom-sidecar-error";
const SIDECAR_STARTUP_TIMEOUT_SECS: u64 = 30;

#[derive(Clone)]
pub struct AppState {
    pub sidecar_state: Arc<SidecarState>,
    pub shutdown_started: Arc<AtomicBool>,
    pub desktop_db_path: Arc<String>,
}

impl Default for AppState {
    fn default() -> Self {
        Self {
            sidecar_state: Arc::new(SidecarState::default()),
            shutdown_started: Arc::new(AtomicBool::new(false)),
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
        if let tauri::RunEvent::ExitRequested { api, .. } = event {
            // Prevent default exit behavior, we handle quit via tray
            api.prevent_exit();
            let app_handle = app_handle.clone();
            tauri::async_runtime::spawn(async move {
                let _ = commands::request_quit(app_handle).await;
            });
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
