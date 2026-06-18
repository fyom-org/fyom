//! fyom Tauri desktop application

mod commands;
mod mpv;
mod platform;
mod sidecar;
mod state;
mod subtitles;
mod tray;
mod window;

use std::sync::Arc;
use std::sync::atomic::{AtomicBool, Ordering};

use crate::state::{MpvState, SidecarState};
use tauri::{Emitter, Manager};

pub const MAIN_WINDOW_LABEL: &str = "main";
pub const SIDECAR_READY_EVENT: &str = "fyom-sidecar-ready";
pub const SIDECAR_ERROR_EVENT: &str = "fyom-sidecar-error";

pub(crate) const SIDECAR_STARTUP_TIMEOUT_SECS: u64 = 30;

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

            let mpv_state = MpvState::new();

            if let Some(instance) = mpv_state.instance.get() {
                tracing::info!("[mpv] native playback ready; spawning event loop");
                instance.spawn_event_loop(app.handle().clone());
            } else if let Some(error) = &mpv_state.init_error {
                tracing::warn!("[mpv] native playback disabled: {}", error);
            }

            app.manage(mpv_state);

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
            commands::playback::get_api_base_url,
            commands::playback::get_playback_backend_info,
            commands::playback::play_media,
            commands::playback::stop_media,
            commands::playback::play_test_media,
            commands::playback::seek,
            commands::playback::seek_relative,
            commands::playback::toggle_pause,
            commands::playback::set_pause,
            commands::playback::set_volume,
            commands::playback::set_speed,
            commands::playback::set_audio_track,
            commands::playback::set_subtitle_track,
            commands::playback::mpv_keypress,
            commands::playback::mpv_command,
            commands::playback::attach_render_surface,
            commands::playback::set_video_mode,
            commands::playback::resize_render_surface,
            commands::playback::find_external_subtitles,
            commands::playback::sub_add,
            commands::playback::sub_remove,
            commands::playback::sub_reload,
            commands::playback::audio_add,
            commands::playback::audio_remove,
            commands::playback::set_sub_delay,
            commands::playback::set_secondary_sub_delay,
            commands::playback::set_audio_delay,
            commands::playback::set_sub_scale,
            commands::playback::set_color_adjustment,
            commands::playback::mpv_set_option_string,
            commands::playback::get_track_list,
            commands::playback::get_chapter_list,
            commands::playback::set_chapter,
            commands::playback::get_property,
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

            if let Some(mpv_state) = app_handle.try_state::<MpvState>() {
                if let Some(instance) = mpv_state.instance.get() {
                    instance.shutdown_render_thread();
                    instance.shutdown_event_loop();
                }
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
