//! fyom Tauri desktop application

mod commands;
mod sidecar;
mod state;
mod tray;
mod window;

use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Arc;

use sidecar::SidecarState;
use tauri::{Emitter, Manager, Runtime};

pub const MAIN_WINDOW_LABEL: &str = "main";
pub const SIDECAR_READY_EVENT: &str = "fyom-sidecar-ready";
pub const SIDECAR_ERROR_EVENT: &str = "fyom-sidecar-error";
const SIDECAR_STARTUP_TIMEOUT_SECS: u64 = 30;

#[derive(Clone)]
pub struct AppState {
    pub sidecar_state: Arc<SidecarState>,
    pub shutdown_started: Arc<AtomicBool>,
}

impl Default for AppState {
    fn default() -> Self {
        Self {
            sidecar_state: Arc::new(SidecarState::default()),
            shutdown_started: Arc::new(AtomicBool::new(false)),
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

    let app_state = AppState::default();

    let app = tauri::Builder::default()
        .manage(app_state.clone())
        .setup(move |app| {
            // Setup sidecar
            let app_handle = app.handle().clone();
            let state = app_state.clone();
            tauri::async_runtime::spawn(async move {
                if let Err(e) = sidecar::bootstrap_sidecar(&app_handle, &state).await {
                    tracing::error!("Sidecar bootstrap failed: {}", e);
                    let _ = app_handle.emit(SIDECAR_ERROR_EVENT, e);
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
            commands::get_api_base_url,
            commands::get_sidecar_status,
            commands::playback::get_playback_backend_info,
        ])
        .build(tauri::generate_context!())
        .expect("error while building tauri application");

    // Handle app run events
    app.run(|app_handle, event| {
        if let tauri::RunEvent::ExitRequested { api, .. } = event {
            // Prevent default exit behavior, we handle quit via tray
            api.prevent_exit();
            let _ = commands::quit_app(app_handle.clone());
        }
    });
}

pub mod commands {
    use super::*;
    use tauri::{AppHandle, State};

    #[tauri::command]
    pub async fn show_window(app: AppHandle) -> Result<(), String> {
        window::show_main_window(&app)
    }

    #[tauri::command]
    pub async fn hide_window(app: AppHandle) -> Result<(), String> {
        window::hide_to_tray(&app)
    }

    #[tauri::command]
    pub async fn quit_app(app: AppHandle) -> Result<(), String> {
        let state: State<'_, AppState> = app.state();
        state.mark_shutdown();

        // Shutdown sidecar
        sidecar::shutdown_sidecar(&app, &state).await?;

        // Exit the app
        app.exit(0);
        Ok(())
    }

    #[tauri::command]
    pub async fn get_api_base_url(state: State<'_, AppState>) -> Result<String, String> {
        state.sidecar_state.get_api_base_url()
    }

    #[tauri::command]
    pub async fn get_sidecar_status(state: State<'_, AppState>) -> Result<SidecarStatus, String> {
        Ok(state.sidecar_state.get_status())
    }
}

#[derive(serde::Serialize, Clone, Debug)]
#[serde(tag = "status", content = "data")]
pub enum SidecarStatus {
    Starting,
    Ready { api_base_url: String },
    Error { message: String },
}