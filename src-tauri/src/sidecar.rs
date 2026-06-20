//! Sidecar lifecycle management.
//!
//! Owns spawning, readiness detection, stdout/stderr draining,
//! readiness confirmation, global runtime retention, and shutdown.

use std::path::{Path, PathBuf};
use std::process::{ExitStatus, Stdio};
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::{Arc, OnceLock};
use std::time::{Duration, Instant};

use anyhow::{Result, anyhow, bail};
use tauri::async_runtime::JoinHandle;
use tauri::{AppHandle, Emitter};
use tokio::io::{AsyncBufReadExt, BufReader};
use tokio::process::Command;
use tokio::sync::{Mutex as AsyncMutex, oneshot};

use crate::{AppState, SIDECAR_ERROR_EVENT, SIDECAR_READY_EVENT, SIDECAR_STARTUP_TIMEOUT_SECS};

const READY_TOKEN: &str = "FYOM_READY";

const READYZ_CONNECT_TIMEOUT_MS: u64 = 300;
const READYZ_REQUEST_TIMEOUT_SECS: u64 = 1;
const READYZ_RETRY_DELAY_MS: u64 = 200;
const READYZ_INITIAL_GRACE_MS: u64 = 200;
const TASK_JOIN_TIMEOUT_SECS: u64 = 3;

// -----------------------------------------------------------------------------
// Global runtime slot
// -----------------------------------------------------------------------------

static RUNNING_SIDECAR: OnceLock<AsyncMutex<Option<RunningSidecar>>> = OnceLock::new();

fn runtime_slot() -> &'static AsyncMutex<Option<RunningSidecar>> {
    RUNNING_SIDECAR.get_or_init(|| AsyncMutex::new(None))
}

async fn take_running_sidecar() -> Option<RunningSidecar> {
    runtime_slot().lock().await.take()
}

async fn install_running_sidecar(runtime: RunningSidecar) {
    let mut slot = runtime_slot().lock().await;

    if let Some(existing) = slot.take() {
        tracing::warn!(
            "[sidecar] replacing existing runtime, old_pid={}",
            existing.pid
        );

        drop(slot);

        existing.shutdown("replaced by new sidecar runtime").await;

        slot = runtime_slot().lock().await;
    }

    *slot = Some(runtime);
}

// -----------------------------------------------------------------------------
// Runtime handle
// -----------------------------------------------------------------------------

struct RunningSidecar {
    pid: u32,
    shutdown_tx: Option<oneshot::Sender<()>>,
    stdout_task: JoinHandle<()>,
    stderr_task: JoinHandle<()>,
    monitor_task: JoinHandle<()>,
    stop_requested: Arc<AtomicBool>,
}

impl RunningSidecar {
    fn new(
        pid: u32,
        shutdown_tx: oneshot::Sender<()>,
        stdout_task: JoinHandle<()>,
        stderr_task: JoinHandle<()>,
        monitor_task: JoinHandle<()>,
        stop_requested: Arc<AtomicBool>,
    ) -> Self {
        Self {
            pid,
            shutdown_tx: Some(shutdown_tx),
            stdout_task,
            stderr_task,
            monitor_task,
            stop_requested,
        }
    }

    async fn shutdown(mut self, reason: &str) {
        tracing::info!(
            "[sidecar] shutdown requested, pid={}, reason={}",
            self.pid,
            reason
        );

        self.stop_requested.store(true, Ordering::SeqCst);

        if let Some(tx) = self.shutdown_tx.take() {
            let _ = tx.send(());
        }

        await_task("monitor", &mut self.monitor_task).await;
        await_task("stdout", &mut self.stdout_task).await;
        await_task("stderr", &mut self.stderr_task).await;
    }
}

async fn await_task(name: &str, handle: &mut JoinHandle<()>) {
    match tokio::time::timeout(Duration::from_secs(TASK_JOIN_TIMEOUT_SECS), &mut *handle).await {
        Ok(join_result) => {
            if let Err(error) = join_result {
                tracing::warn!("[sidecar] {name} task join failed: {error}");
            }
        }
        Err(_) => {
            tracing::warn!("[sidecar] timed out waiting for {name} task; aborting");
            handle.abort();
        }
    }
}

// -----------------------------------------------------------------------------
// Monitor
// -----------------------------------------------------------------------------

#[derive(Debug)]
enum MonitorOutcome {
    Exited(ExitStatus),
    WaitError(String),
    KilledByRequest(Option<ExitStatus>),
}

fn format_exit_status(status: &ExitStatus) -> String {
    #[cfg(unix)]
    {
        use std::os::unix::process::ExitStatusExt;

        if let Some(code) = status.code() {
            return format!("exit code {code}");
        }

        if let Some(signal) = status.signal() {
            return format!("signal {signal}");
        }
    }

    #[cfg(not(unix))]
    {
        if let Some(code) = status.code() {
            return format!("exit code {code}");
        }
    }

    format!("{status:?}")
}

fn spawn_process_monitor_task(
    mut child: tokio::process::Child,
    mut shutdown_rx: oneshot::Receiver<()>,
    exit_tx: oneshot::Sender<MonitorOutcome>,
    stop_requested: Arc<AtomicBool>,
) -> JoinHandle<()> {
    tauri::async_runtime::spawn(async move {
        tracing::info!("[sidecar] process monitor started");

        let outcome = tokio::select! {
            status = child.wait() => {
                match status {
                    Ok(status) => {
                        if status.success() {
                            tracing::info!(
                                "[sidecar] process exited normally: {}",
                                format_exit_status(&status)
                            );
                        } else {
                            tracing::warn!(
                                "[sidecar] process exited abnormally: {}",
                                format_exit_status(&status)
                            );
                        }

                        MonitorOutcome::Exited(status)
                    }
                    Err(error) => {
                        tracing::error!("[sidecar] process wait failed: {error}");
                        MonitorOutcome::WaitError(error.to_string())
                    }
                }
            }

            signal = &mut shutdown_rx => {
                match signal {
                    Ok(()) => {
                        stop_requested.store(true, Ordering::SeqCst);

                        tracing::info!("[sidecar] killing process after shutdown signal");

                        if let Err(error) = child.kill().await {
                            tracing::warn!("[sidecar] failed to kill process: {error}");
                        }

                        match child.wait().await {
                            Ok(status) => {
                                tracing::info!(
                                    "[sidecar] process terminated after shutdown: {}",
                                    format_exit_status(&status)
                                );

                                MonitorOutcome::KilledByRequest(Some(status))
                            }
                            Err(error) => {
                                tracing::warn!(
                                    "[sidecar] process kill issued but wait failed: {error}"
                                );

                                MonitorOutcome::WaitError(error.to_string())
                            }
                        }
                    }

                    Err(_) => {
                        tracing::debug!(
                            "[sidecar] shutdown sender dropped; waiting for natural process exit"
                        );

                        match child.wait().await {
                            Ok(status) => MonitorOutcome::Exited(status),
                            Err(error) => MonitorOutcome::WaitError(error.to_string()),
                        }
                    }
                }
            }
        };

        let _ = exit_tx.send(outcome);

        tracing::info!("[sidecar] process monitor ended");
    })
}

// -----------------------------------------------------------------------------
// Stream drains
// -----------------------------------------------------------------------------

pub fn parse_ready_line(line: &str) -> Option<String> {
    let line = line.trim();

    let rest = line.strip_prefix(READY_TOKEN)?;
    let url = rest.trim();

    if url.starts_with("http://") || url.starts_with("https://") {
        Some(url.to_string())
    } else {
        None
    }
}

fn log_stream_eof(stream: &str, ready_seen: bool, stop_requested: bool) {
    if stop_requested {
        tracing::info!("[sidecar] {stream} closed after shutdown");
    } else if ready_seen {
        tracing::warn!("[sidecar] {stream} closed after readiness");
    } else {
        tracing::error!("[sidecar] {stream} closed before readiness");
    }
}

fn spawn_stdout_drain_task(
    stdout: tokio::process::ChildStdout,
    ready_tx: oneshot::Sender<String>,
    ready_seen: Arc<AtomicBool>,
    stop_requested: Arc<AtomicBool>,
) -> JoinHandle<()> {
    tauri::async_runtime::spawn(async move {
        tracing::info!("[sidecar] stdout drain started");

        let reader = BufReader::new(stdout);
        let mut lines = reader.lines();
        let mut ready_tx = Some(ready_tx);

        loop {
            match lines.next_line().await {
                Ok(Some(line)) => {
                    tracing::debug!(target: "sidecar", "[stdout] {line}");

                    let Some(api_url) = parse_ready_line(&line) else {
                        continue;
                    };

                    if ready_seen.swap(true, Ordering::SeqCst) {
                        tracing::debug!("[sidecar] duplicate FYOM_READY ignored: {api_url}");
                        continue;
                    }

                    tracing::info!("[sidecar] FYOM_READY received: {api_url}");

                    if let Some(tx) = ready_tx.take() {
                        let _ = tx.send(api_url);
                    }
                }

                Ok(None) => {
                    log_stream_eof(
                        "stdout",
                        ready_seen.load(Ordering::SeqCst),
                        stop_requested.load(Ordering::SeqCst),
                    );
                    break;
                }

                Err(error) => {
                    tracing::error!("[sidecar] stdout read failed: {error}");
                    break;
                }
            }
        }

        tracing::info!("[sidecar] stdout drain ended");
    })
}

fn spawn_stderr_drain_task(
    stderr: tokio::process::ChildStderr,
    ready_seen: Arc<AtomicBool>,
    stop_requested: Arc<AtomicBool>,
) -> JoinHandle<()> {
    tauri::async_runtime::spawn(async move {
        tracing::info!("[sidecar] stderr drain started");

        let reader = BufReader::new(stderr);
        let mut lines = reader.lines();

        loop {
            match lines.next_line().await {
                Ok(Some(line)) => {
                    tracing::info!(target: "sidecar", "[stderr] {line}");
                }

                Ok(None) => {
                    log_stream_eof(
                        "stderr",
                        ready_seen.load(Ordering::SeqCst),
                        stop_requested.load(Ordering::SeqCst),
                    );
                    break;
                }

                Err(error) => {
                    tracing::error!("[sidecar] stderr read failed: {error}");
                    break;
                }
            }
        }

        tracing::info!("[sidecar] stderr drain ended");
    })
}

// -----------------------------------------------------------------------------
// Binary discovery
// -----------------------------------------------------------------------------

fn find_sidecar_binary() -> Result<PathBuf> {
    if let Ok(value) = std::env::var("FYOM_BIN") {
        let path = PathBuf::from(&value);

        if path.exists() {
            return Ok(path);
        }

        bail!("FYOM_BIN={value} does not exist");
    }

    let manifest_dir = std::env::var("CARGO_MANIFEST_DIR")
        .map(PathBuf::from)
        .unwrap_or_else(|_| std::env::current_dir().unwrap_or_default());

    let candidates = [
        manifest_dir.join("../../build/fyom"),
        manifest_dir.join("../build/fyom"),
        manifest_dir.join("build/fyom"),
        manifest_dir.join("../../target/debug/fyom"),
        manifest_dir.join("../target/debug/fyom"),
        manifest_dir.join("target/debug/fyom"),
    ];

    for candidate in candidates {
        if candidate.exists() {
            return Ok(candidate);
        }
    }

    if let Ok(output) = std::process::Command::new("which").arg("fyom").output()
        && output.status.success()
    {
        let path = String::from_utf8_lossy(&output.stdout).trim().to_string();

        if !path.is_empty() {
            return Ok(PathBuf::from(path));
        }
    }

    bail!("fyom sidecar binary not found; run `task sidecar` or set FYOM_BIN")
}

// -----------------------------------------------------------------------------
// Readyz
// -----------------------------------------------------------------------------

async fn confirm_readyz_once(api_url: &str) -> Result<()> {
    let client = reqwest::Client::builder()
        .connect_timeout(Duration::from_millis(READYZ_CONNECT_TIMEOUT_MS))
        .timeout(Duration::from_secs(READYZ_REQUEST_TIMEOUT_SECS))
        .build()?;

    let url = format!("{}/readyz", api_url.trim_end_matches('/'));

    let response = client.get(&url).send().await?;

    if response.status().is_success() {
        Ok(())
    } else {
        bail!("/readyz returned {}", response.status())
    }
}

fn should_warn_readyz_retry(attempt: u32) -> bool {
    attempt == 1 || attempt == 5 || attempt.is_multiple_of(10)
}

async fn cleanup_failed_bootstrap(runtime: RunningSidecar, reason: &str) {
    tracing::warn!("[sidecar] cleaning failed bootstrap: {reason}");
    runtime.shutdown(reason).await;
}

// -----------------------------------------------------------------------------
// Bootstrap
// -----------------------------------------------------------------------------

pub async fn bootstrap_sidecar(app: &AppHandle, state: &AppState) -> Result<()> {
    let sidecar_state = &state.sidecar_state;

    sidecar_state.set_starting();

    if let Some(existing) = take_running_sidecar().await {
        tracing::warn!(
            "[sidecar] existing runtime found, pid={}; shutting down before bootstrap",
            existing.pid
        );

        existing.shutdown("restart before bootstrap").await;
    }

    let binary_path = find_sidecar_binary()
        .map_err(|error| anyhow!("failed to locate sidecar binary: {error}"))?;

    let db_path = PathBuf::from(state.desktop_db_path.as_str());
    let app_exe_dir = db_path.parent().unwrap_or(Path::new(""));

    tracing::info!("[sidecar] starting");
    tracing::info!("[sidecar] app_exe_dir={}", app_exe_dir.display());
    tracing::info!("[sidecar] db_path={}", db_path.display());
    tracing::info!("[sidecar] binary={}", binary_path.display());
    tracing::info!(
        "[sidecar] args=--sidecar --db-path {} --log-level info",
        db_path.display()
    );

    let mut child = Command::new(&binary_path)
        .arg("--sidecar")
        .arg("--db-path")
        .arg(&db_path)
        .arg("--log-level")
        .arg("info")
        .stdin(Stdio::null())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .kill_on_drop(true)
        .spawn()
        .map_err(|error| anyhow!("failed to spawn sidecar: {error}"))?;

    let pid = child.id().unwrap_or(0);

    sidecar_state.set_child_pid(pid);

    tracing::info!("[sidecar] spawned, pid={pid}");

    let stdout = child
        .stdout
        .take()
        .ok_or_else(|| anyhow!("failed to capture sidecar stdout"))?;

    let stderr = child
        .stderr
        .take()
        .ok_or_else(|| anyhow!("failed to capture sidecar stderr"))?;

    let ready_seen = Arc::new(AtomicBool::new(false));
    let stop_requested = Arc::new(AtomicBool::new(false));

    let (ready_tx, mut ready_rx) = oneshot::channel::<String>();
    let (shutdown_tx, shutdown_rx) = oneshot::channel::<()>();
    let (exit_tx, mut exit_rx) = oneshot::channel::<MonitorOutcome>();

    let stdout_task = spawn_stdout_drain_task(
        stdout,
        ready_tx,
        Arc::clone(&ready_seen),
        Arc::clone(&stop_requested),
    );

    let stderr_task =
        spawn_stderr_drain_task(stderr, Arc::clone(&ready_seen), Arc::clone(&stop_requested));

    let monitor_task =
        spawn_process_monitor_task(child, shutdown_rx, exit_tx, Arc::clone(&stop_requested));

    let runtime = RunningSidecar::new(
        pid,
        shutdown_tx,
        stdout_task,
        stderr_task,
        monitor_task,
        Arc::clone(&stop_requested),
    );

    let startup_deadline = Instant::now() + Duration::from_secs(SIDECAR_STARTUP_TIMEOUT_SECS);

    let mut runtime = Some(runtime);
    let mut ready_api_url: Option<String> = None;
    let mut readyz_attempts: u32 = 0;
    let mut readyz_not_before: Option<Instant> = None;

    loop {
        if Instant::now() >= startup_deadline {
            let message = format!(
                "sidecar startup timed out after {}s, pid={pid}",
                SIDECAR_STARTUP_TIMEOUT_SECS
            );

            if let Some(runtime) = runtime.take() {
                cleanup_failed_bootstrap(runtime, &message).await;
            }

            sidecar_state.set_error(message.clone());
            let _ = app.emit(SIDECAR_ERROR_EVENT, message.clone());

            bail!("{message}");
        }

        tokio::select! {
            exit_result = &mut exit_rx => {
                let message = match exit_result {
                    Ok(MonitorOutcome::Exited(status)) => {
                        format!(
                            "sidecar exited before bootstrap completed: {}",
                            format_exit_status(&status)
                        )
                    }

                    Ok(MonitorOutcome::KilledByRequest(status)) => {
                        match status {
                            Some(status) => {
                                format!(
                                    "sidecar was killed during bootstrap: {}",
                                    format_exit_status(&status)
                                )
                            }
                            None => "sidecar was killed during bootstrap".to_string(),
                        }
                    }

                    Ok(MonitorOutcome::WaitError(error)) => {
                        format!("sidecar monitor wait error during bootstrap: {error}")
                    }

                    Err(_) => {
                        "sidecar monitor channel closed during bootstrap".to_string()
                    }
                };

                if let Some(runtime) = runtime.take() {
                    cleanup_failed_bootstrap(runtime, &message).await;
                }

                sidecar_state.set_error(message.clone());
                let _ = app.emit(SIDECAR_ERROR_EVENT, message.clone());

                bail!("{message}");
            }

            ready_result = &mut ready_rx, if ready_api_url.is_none() => {
                match ready_result {
                    Ok(api_url) => {
                        tracing::info!(
                            "[sidecar] FYOM_READY accepted; confirming /readyz after {}ms",
                            READYZ_INITIAL_GRACE_MS
                        );

                        ready_api_url = Some(api_url);
                        readyz_not_before = Some(
                            Instant::now() + Duration::from_millis(READYZ_INITIAL_GRACE_MS)
                        );
                    }

                    Err(_) => {
                        let message = format!(
                            "sidecar stdout ended before FYOM_READY, pid={pid}"
                        );

                        if let Some(runtime) = runtime.take() {
                            cleanup_failed_bootstrap(runtime, &message).await;
                        }

                        sidecar_state.set_error(message.clone());
                        let _ = app.emit(SIDECAR_ERROR_EVENT, message.clone());

                        bail!("{message}");
                    }
                }
            }

            _ = tokio::time::sleep(Duration::from_millis(READYZ_RETRY_DELAY_MS)), if ready_api_url.is_some() => {
                if let Some(not_before) = readyz_not_before
                    && Instant::now() < not_before
                {
                    continue;
                }

                let api_url = ready_api_url
                    .as_ref()
                    .expect("ready_api_url checked by select guard");

                readyz_attempts = readyz_attempts.saturating_add(1);

                match confirm_readyz_once(api_url).await {
                    Ok(()) => {
                        tracing::info!(
                            "[sidecar] /readyz confirmed, attempt={readyz_attempts}"
                        );

                        let api_base_url = format!(
                            "{}/api/v1",
                            api_url.trim_end_matches('/')
                        );

                        sidecar_state.set_ready(api_base_url.clone());

                        if let Some(runtime) = runtime.take() {
                            install_running_sidecar(runtime).await;
                        }

                        let _ = app.emit(SIDECAR_READY_EVENT, api_base_url);

                        tracing::info!("[sidecar] bootstrap completed");
                        return Ok(());
                    }

                    Err(error) => {
                        if Instant::now() >= startup_deadline {
                            let message = format!(
                                "sidecar emitted FYOM_READY but /readyz did not confirm before timeout: {error}"
                            );

                            if let Some(runtime) = runtime.take() {
                                cleanup_failed_bootstrap(runtime, &message).await;
                            }

                            sidecar_state.set_error(message.clone());
                            let _ = app.emit(SIDECAR_ERROR_EVENT, message.clone());

                            bail!("{message}");
                        }

                        if should_warn_readyz_retry(readyz_attempts) {
                            tracing::warn!(
                                "[sidecar] /readyz failed: {error}, attempt={readyz_attempts}"
                            );
                        } else {
                            tracing::debug!(
                                "[sidecar] /readyz failed: {error}, attempt={readyz_attempts}"
                            );
                        }
                    }
                }
            }
        }
    }
}

// -----------------------------------------------------------------------------
// Shutdown
// -----------------------------------------------------------------------------

pub async fn shutdown_sidecar(state: &AppState) -> Result<()> {
    let sidecar_state = &state.sidecar_state;

    if let Some(runtime) = take_running_sidecar().await {
        tracing::info!("[sidecar] shutting down, pid={}", runtime.pid);
        runtime.shutdown("shutdown_sidecar").await;
    } else {
        tracing::debug!("[sidecar] no running sidecar to shut down");
    }

    sidecar_state.set_stopped();

    Ok(())
}
