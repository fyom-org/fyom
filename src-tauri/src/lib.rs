//! fyom Tauri desktop application

mod commands;
pub(crate) mod desktop_config;
mod sidecar;
mod state;
mod tray;
mod window;

use std::sync::Arc;
use std::sync::atomic::{AtomicBool, Ordering};

use crate::desktop_config::DesktopConfig;
use crate::state::SidecarState;
use tauri::{Emitter, Manager};

pub const MAIN_WINDOW_LABEL: &str = "main";
pub const SIDECAR_READY_EVENT: &str = "fyom-sidecar-ready";
pub const SIDECAR_ERROR_EVENT: &str = "fyom-sidecar-error";

pub(crate) const SIDECAR_STARTUP_TIMEOUT_SECS: u64 = 30;

#[derive(Clone)]
pub struct AppState {
    /// Runtime state for the FYOM server sidecar.
    pub sidecar_state: Arc<SidecarState>,

    /// Set once shutdown begins to avoid duplicate shutdown handling.
    pub shutdown_started: Arc<AtomicBool>,

    /// Set when the user explicitly intends to exit the desktop application.
    pub exit_intent: Arc<AtomicBool>,

    /// Desktop database path used by the local desktop runtime.
    pub desktop_db_path: Arc<String>,

    /// Desktop-local configuration resolved by the `desktop_config` module.
    ///
    /// Resolution order:
    /// 1. `FYOM_DESKTOP_CONFIG` explicit override
    /// 2. Platform user config path
    /// 3. Development fallback `configs/fyom-desktop.json` (debug only)
    ///
    /// This includes external player configuration such as:
    /// - configured mpv binary
    /// - custom external player
    /// - custom player arguments
    ///
    /// This is intentionally separate from the Go backend `fyom.yaml`.
    /// The Go backend must not know or interpret local desktop player paths.
    pub desktop_config: Arc<DesktopConfig>,
}

impl Default for AppState {
    fn default() -> Self {
        Self {
            sidecar_state: Arc::new(SidecarState::default()),
            shutdown_started: Arc::new(AtomicBool::new(false)),
            exit_intent: Arc::new(AtomicBool::new(false)),
            desktop_db_path: Arc::new(String::new()),
            desktop_config: Arc::new(DesktopConfig::load()),
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

    pub fn mark_exit_intent(&self) {
        self.exit_intent.store(true, Ordering::SeqCst);
    }

    pub fn has_exit_intent(&self) -> bool {
        self.exit_intent.load(Ordering::SeqCst)
    }
}

pub fn run() {
    let _ = tracing_subscriber::fmt()
        .with_env_filter(tracing_subscriber::EnvFilter::from_default_env())
        .try_init();

    let mut app_state = AppState::default();

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
            tray::setup_tray(app)?;
            window::setup_main_window(app)?;

            let app_handle = app.handle().clone();
            let state = app_state.clone();

            tauri::async_runtime::spawn(async move {
                if let Err(error) = sidecar::bootstrap_sidecar(&app_handle, &state).await {
                    tracing::error!("[sidecar] bootstrap failed: {}", error);

                    state.sidecar_state.set_error(error.to_string());

                    let _ = app_handle.emit(SIDECAR_ERROR_EVENT, error.to_string());
                }
            });

            Ok(())
        })
        .plugin(tauri_plugin_opener::init())
        .plugin(tauri_plugin_process::init())
        .invoke_handler(tauri::generate_handler![
            commands::show_window,
            commands::hide_window,
            commands::quit_app,
            commands::get_sidecar_status,
            commands::launcher::get_api_base_url,
            commands::launcher::get_external_player_config,
            commands::launcher::open_external_player,
        ])
        .build(tauri::generate_context!())
        .expect("error while building tauri application");

    app.run(|app_handle, event| match event {
        tauri::RunEvent::ExitRequested { api, .. } => {
            let has_exit_intent = app_handle
                .try_state::<AppState>()
                .map(|state| state.has_exit_intent())
                .unwrap_or(false);

            if has_exit_intent {
                tracing::info!("[app] exit requested with explicit exit intent");
            } else {
                api.prevent_exit();
                tracing::debug!("[app] close intercepted; hiding to tray");
            }
        }

        tauri::RunEvent::Exit => {
            if let Some(app_state) = app_handle.try_state::<AppState>() {
                app_state.mark_shutdown();
                app_state.sidecar_state.set_stopped();
            }

            tracing::info!("[app] exited");
        }

        _ => {}
    });
}

#[derive(serde::Serialize, Clone, Debug, Default)]
#[serde(tag = "status", content = "data")]
pub enum SidecarStatus {
    #[default]
    Stopped,

    Starting,

    Ready {
        api_base_url: String,
    },

    Error {
        message: String,
    },
}
