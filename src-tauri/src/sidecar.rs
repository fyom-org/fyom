//! Sidecar lifecycle management
//!
//! Handles spawning, readiness detection, long-running stdout/stderr draining,
//! readiness confirmation, and graceful shutdown of the Go sidecar process.

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

/// Global runtime slot for the currently running sidecar.
///
/// This keeps the running sidecar handle alive after `bootstrap_sidecar()`
/// returns successfully, without requiring an immediate wider refactor of
/// `AppState`.
static RUNNING_SIDECAR: OnceLock<AsyncMutex<Option<RunningSidecar>>> = OnceLock::new();

fn runtime_slot() -> &'static AsyncMutex<Option<RunningSidecar>> {
    RUNNING_SIDECAR.get_or_init(|| AsyncMutex::new(None))
}

fn should_warn_readyz_retry(attempt: u32) -> bool {
    attempt == 1 || attempt == 5 || attempt % 10 == 0
}

/// Runtime handle for the running sidecar process.
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
            "Shutting down sidecar runtime, pid={}, reason={}",
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
    match tokio::time::timeout(Duration::from_secs(3), &mut *handle).await {
        Ok(join_result) => {
            if let Err(e) = join_result {
                tracing::warn!("Sidecar {} task join failed: {}", name, e);
            }
        }
        Err(_) => {
            tracing::warn!("Timed out waiting for sidecar {} task to finish", name);
            handle.abort();
        }
    }
}

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
            return format!("exit code {}", code);
        }
        if let Some(sig) = status.signal() {
            return format!("signal {}", sig);
        }
    }

    #[cfg(not(unix))]
    {
        if let Some(code) = status.code() {
            return format!("exit code {}", code);
        }
    }

    format!("{:?}", status)
}

/// Parse a readiness line from sidecar stdout.
///
/// Expected format:
/// `FYOM_READY http://127.0.0.1:27403`
pub fn parse_ready_line(line: &str) -> Option<String> {
    let line = line.trim();
    if let Some(rest) = line.strip_prefix(READY_TOKEN) {
        let url = rest.trim();
        if url.starts_with("http://") {
            return Some(url.to_string());
        }
    }
    None
}

/// Find the fyom sidecar binary.
///
/// Search order:
/// 1. `FYOM_BIN` environment variable
/// 2. common build paths relative to the Tauri project root
/// 3. `fyom` on PATH
fn find_sidecar_binary() -> Result<PathBuf> {
    if let Ok(bin) = std::env::var("FYOM_BIN") {
        let path = PathBuf::from(&bin);
        if path.exists() {
            return Ok(path);
        }
        bail!("FYOM_BIN={} not found", bin);
    }

    let manifest_dir = std::env::var("CARGO_MANIFEST_DIR")
        .map(PathBuf::from)
        .unwrap_or_else(|_| std::env::current_dir().unwrap_or_default());

    let candidates = [
        manifest_dir.join("../../build/fyom"),
        manifest_dir.join("../build/fyom"),
        manifest_dir.join("build/fyom"),
    ];

    for candidate in &candidates {
        if candidate.exists() {
            return Ok(candidate.clone());
        }
    }

    if let Ok(output) = std::process::Command::new("which").arg("fyom").output() {
        if output.status.success() {
            let path = String::from_utf8_lossy(&output.stdout).trim().to_string();
            if !path.is_empty() {
                return Ok(PathBuf::from(path));
            }
        }
    }

    bail!("fyom sidecar binary not found. Build it with: task sidecar (or set FYOM_BIN)")
}

async fn take_running_sidecar() -> Option<RunningSidecar> {
    runtime_slot().lock().await.take()
}

async fn install_running_sidecar(runtime: RunningSidecar) {
    let mut slot = runtime_slot().lock().await;
    if let Some(existing) = slot.take() {
        tracing::warn!(
            "Replacing existing sidecar runtime handle for pid={}; shutting old one down first",
            existing.pid
        );
        drop(slot);
        existing.shutdown("replaced by new runtime").await;
        slot = runtime_slot().lock().await;
    }
    *slot = Some(runtime);
}

fn log_stream_eof(stream_name: &str, ready_seen: bool, stop_requested: bool) {
    if stop_requested {
        tracing::info!(
            "[sidecar-drain] Sidecar {} closed after shutdown",
            stream_name
        );
    } else if ready_seen {
        tracing::warn!(
            "[sidecar-drain] Sidecar {} closed after readiness",
            stream_name
        );
    } else {
        tracing::error!(
            "[sidecar-drain] Sidecar {} closed before readiness",
            stream_name
        );
    }
}

fn spawn_stdout_drain_task(
    stdout: tokio::process::ChildStdout,
    ready_tx: oneshot::Sender<String>,
    ready_seen: Arc<AtomicBool>,
    stop_requested: Arc<AtomicBool>,
) -> JoinHandle<()> {
    tauri::async_runtime::spawn(async move {
        tracing::info!("[sidecar-drain] stdout drain task started");

        let reader = BufReader::new(stdout);
        let mut lines = reader.lines();
        let mut ready_tx = Some(ready_tx);

        loop {
            match lines.next_line().await {
                Ok(Some(line)) => {
                    tracing::debug!(target: "sidecar", "[stdout] {}", line);

                    if let Some(api_url) = parse_ready_line(&line) {
                        if ready_seen.swap(true, Ordering::SeqCst) {
                            tracing::debug!(
                                "[sidecar-drain] Duplicate FYOM_READY ignored: {}",
                                api_url
                            );
                            continue;
                        }

                        tracing::info!("[sidecar-drain] FYOM_READY received: {}", api_url);

                        if let Some(tx) = ready_tx.take() {
                            let _ = tx.send(api_url);
                        }

                        continue;
                    }
                }
                Ok(None) => {
                    let ready = ready_seen.load(Ordering::SeqCst);
                    let stop = stop_requested.load(Ordering::SeqCst);
                    log_stream_eof("stdout", ready, stop);
                    break;
                }
                Err(e) => {
                    tracing::error!("[sidecar-drain] Error reading sidecar stdout: {}", e);
                    break;
                }
            }
        }

        tracing::info!("[sidecar-drain] stdout drain task ended");
    })
}

fn spawn_stderr_drain_task(
    stderr: tokio::process::ChildStderr,
    ready_seen: Arc<AtomicBool>,
    stop_requested: Arc<AtomicBool>,
) -> JoinHandle<()> {
    tauri::async_runtime::spawn(async move {
        tracing::info!("[sidecar-drain] stderr drain task started");

        let reader = BufReader::new(stderr);
        let mut lines = reader.lines();

        loop {
            match lines.next_line().await {
                Ok(Some(line)) => {
                    tracing::info!(target: "sidecar", "[stderr] {}", line);
                }
                Ok(None) => {
                    let ready = ready_seen.load(Ordering::SeqCst);
                    let stop = stop_requested.load(Ordering::SeqCst);
                    log_stream_eof("stderr", ready, stop);
                    break;
                }
                Err(e) => {
                    tracing::error!("[sidecar-drain] Error reading sidecar stderr: {}", e);
                    break;
                }
            }
        }

        tracing::info!("[sidecar-drain] stderr drain task ended");
    })
}

fn spawn_process_monitor_task(
    mut child: tokio::process::Child,
    mut shutdown_rx: oneshot::Receiver<()>,
    exit_tx: oneshot::Sender<MonitorOutcome>,
    stop_requested: Arc<AtomicBool>,
) -> JoinHandle<()> {
    tauri::async_runtime::spawn(async move {
        tracing::info!("[sidecar-drain] process monitor task started");

        let outcome = tokio::select! {
            status = child.wait() => {
                match status {
                    Ok(status) => {
                        if status.success() {
                            tracing::info!(
                                "[sidecar-drain] Sidecar exited normally: {}",
                                format_exit_status(&status)
                            );
                        } else {
                            tracing::warn!(
                                "[sidecar-drain] Sidecar exited with non-zero status: {}",
                                format_exit_status(&status)
                            );
                        }
                        MonitorOutcome::Exited(status)
                    }
                    Err(e) => {
                        tracing::error!("[sidecar-drain] Sidecar wait error: {}", e);
                        MonitorOutcome::WaitError(e.to_string())
                    }
                }
            }
            signal = &mut shutdown_rx => {
                match signal {
                    Ok(()) => {
                        stop_requested.store(true, Ordering::SeqCst);
                        tracing::info!("[sidecar-drain] Explicit shutdown signal received, killing sidecar");

                        if let Err(e) = child.kill().await {
                            tracing::warn!("[sidecar-drain] Failed to kill sidecar: {}", e);
                        }

                        match child.wait().await {
                            Ok(status) => {
                                tracing::info!(
                                    "[sidecar-drain] Sidecar terminated after shutdown request: {}",
                                    format_exit_status(&status)
                                );
                                MonitorOutcome::KilledByRequest(Some(status))
                            }
                            Err(e) => {
                                tracing::warn!(
                                    "[sidecar-drain] Sidecar kill issued, but wait failed: {}",
                                    e
                                );
                                MonitorOutcome::WaitError(e.to_string())
                            }
                        }
                    }
                    Err(_) => {
                        tracing::debug!(
                            "[sidecar-drain] Shutdown sender dropped without explicit signal; monitor will keep waiting for natural exit"
                        );

                        match child.wait().await {
                            Ok(status) => {
                                if status.success() {
                                    tracing::info!(
                                        "[sidecar-drain] Sidecar exited normally after shutdown sender drop: {}",
                                        format_exit_status(&status)
                                    );
                                } else {
                                    tracing::warn!(
                                        "[sidecar-drain] Sidecar exited with non-zero status after shutdown sender drop: {}",
                                        format_exit_status(&status)
                                    );
                                }
                                MonitorOutcome::Exited(status)
                            }
                            Err(e) => {
                                tracing::error!("[sidecar-drain] Sidecar wait error: {}", e);
                                MonitorOutcome::WaitError(e.to_string())
                            }
                        }
                    }
                }
            }
        };

        let _ = exit_tx.send(outcome);
        tracing::info!("[sidecar-drain] process monitor task ended");
    })
}

async fn confirm_readyz_once(api_url: &str) -> Result<()> {
    let client = reqwest::Client::builder()
        .connect_timeout(Duration::from_millis(READYZ_CONNECT_TIMEOUT_MS))
        .timeout(Duration::from_secs(READYZ_REQUEST_TIMEOUT_SECS))
        .build()?;

    let url = format!("{}/readyz", api_url);
    let resp = client.get(&url).send().await?;
    if resp.status().is_success() {
        return Ok(());
    }

    bail!("/readyz returned unexpected status {}", resp.status())
}

async fn cleanup_failed_bootstrap(runtime: RunningSidecar, reason: &str) {
    tracing::warn!("Cleaning up failed sidecar bootstrap: {}", reason);
    runtime.shutdown(reason).await;
}

/// Bootstrap the Go sidecar process.
///
/// Success path:
/// 1. spawn sidecar
/// 2. start long-running stdout/stderr drain tasks
/// 3. wait for first FYOM_READY
/// 4. confirm /readyz within the startup timeout budget
/// 5. store runtime handle globally
/// 6. emit ready event and return success
pub async fn bootstrap_sidecar(app: &AppHandle, state: &AppState) -> Result<()> {
    let sidecar_state = &state.sidecar_state;
    sidecar_state.set_starting();

    if let Some(existing) = take_running_sidecar().await {
        tracing::warn!(
            "A sidecar runtime is already installed (pid={}); shutting it down before restarting",
            existing.pid
        );
        existing.shutdown("restart before bootstrap").await;
    }

    let binary_path =
        find_sidecar_binary().map_err(|e| anyhow!("Failed to locate sidecar binary: {}", e))?;

    let db_path = PathBuf::from(state.desktop_db_path.as_str());
    let app_exe_dir = db_path.parent().unwrap_or(Path::new(""));

    tracing::info!("Desktop sidecar starting");
    tracing::info!("  app_exe_dir: {}", app_exe_dir.display());
    tracing::info!("  db_path:     {}", db_path.display());
    tracing::info!("  sidecar_bin: {}", binary_path.display());
    tracing::info!(
        "  args:        --sidecar --db-path {} --log-level info",
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
        .map_err(|e| anyhow!("Failed to spawn sidecar: {}", e))?;

    let child_id = child.id().unwrap_or(0);
    sidecar_state.set_child_pid(child_id);
    tracing::info!("Sidecar spawned, pid={}", child_id);

    let stdout = child
        .stdout
        .take()
        .ok_or_else(|| anyhow!("Failed to capture sidecar stdout"))?;
    let stderr = child
        .stderr
        .take()
        .ok_or_else(|| anyhow!("Failed to capture sidecar stderr"))?;

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
        child_id,
        shutdown_tx,
        stdout_task,
        stderr_task,
        monitor_task,
        Arc::clone(&stop_requested),
    );

    let startup_deadline = Instant::now() + Duration::from_secs(SIDECAR_STARTUP_TIMEOUT_SECS);
    let mut ready_api_url: Option<String> = None;
    let mut readyz_attempts: u32 = 0;
    let mut readyz_not_before: Option<Instant> = None;

    let mut runtime = Some(runtime);

    loop {
        if Instant::now() >= startup_deadline {
            let msg = format!(
                "Sidecar startup timed out after {}s (pid={})",
                SIDECAR_STARTUP_TIMEOUT_SECS, child_id
            );

            if let Some(rt) = runtime.take() {
                cleanup_failed_bootstrap(rt, &msg).await;
            }

            tracing::error!("[bootstrap] {}", msg);
            sidecar_state.set_error(msg.clone());
            let _ = app.emit(SIDECAR_ERROR_EVENT, msg.clone());
            bail!("{}", msg);
        }

        tokio::select! {
            exit_result = &mut exit_rx => {
                let msg = match exit_result {
                    Ok(MonitorOutcome::Exited(status)) => {
                        format!(
                            "Sidecar exited before bootstrap completed: {}",
                            format_exit_status(&status)
                        )
                    }
                    Ok(MonitorOutcome::KilledByRequest(status)) => {
                        if let Some(status) = status {
                            format!(
                                "Sidecar was killed during bootstrap: {}",
                                format_exit_status(&status)
                            )
                        } else {
                            "Sidecar was killed during bootstrap".to_string()
                        }
                    }
                    Ok(MonitorOutcome::WaitError(err)) => {
                        format!("Sidecar monitor wait error during bootstrap: {}", err)
                    }
                    Err(_) => {
                        "Sidecar monitor channel closed unexpectedly during bootstrap".to_string()
                    }
                };

                if let Some(rt) = runtime.take() {
                    cleanup_failed_bootstrap(rt, &msg).await;
                }

                tracing::error!("[bootstrap] {}", msg);
                sidecar_state.set_error(msg.clone());
                let _ = app.emit(SIDECAR_ERROR_EVENT, msg.clone());
                bail!("{}", msg);
            }

            ready_result = &mut ready_rx, if ready_api_url.is_none() => {
                match ready_result {
                    Ok(api_url) => {
                        tracing::info!(
                            "[bootstrap] FYOM_READY received, waiting {}ms before confirming /readyz...",
                            READYZ_INITIAL_GRACE_MS
                        );
                        ready_api_url = Some(api_url);
                        readyz_not_before = Some(Instant::now() + Duration::from_millis(READYZ_INITIAL_GRACE_MS));
                    }
                    Err(_) => {
                        let msg = format!("Sidecar stdout ended before emitting FYOM_READY (pid={})", child_id);

                        if let Some(rt) = runtime.take() {
                            cleanup_failed_bootstrap(rt, &msg).await;
                        }

                        tracing::error!("[bootstrap] {}", msg);
                        sidecar_state.set_error(msg.clone());
                        let _ = app.emit(SIDECAR_ERROR_EVENT, msg.clone());
                        bail!("{}", msg);
                    }
                }
            }

            _ = tokio::time::sleep(Duration::from_millis(READYZ_RETRY_DELAY_MS)), if ready_api_url.is_some() => {
                if let Some(not_before) = readyz_not_before {
                    if Instant::now() < not_before {
                        continue;
                    }
                }

                let api_url = ready_api_url.as_ref().expect("checked is_some");
                readyz_attempts += 1;

                match confirm_readyz_once(api_url).await {
                    Ok(()) => {
                        tracing::info!("/readyz confirmed OK (attempt {})", readyz_attempts);
                        tracing::info!("[bootstrap] /readyz confirmed OK");

                        let api_base_url = format!("{}/api/v1", api_url);
                        sidecar_state.set_ready(api_base_url);

                        if let Some(rt) = runtime.take() {
                            install_running_sidecar(rt).await;
                        }

                        let _ = app.emit(SIDECAR_READY_EVENT, api_url.clone());
                        tracing::info!("[bootstrap] Sidecar bootstrap completed successfully");
                        return Ok(());
                    }
                    Err(e) => {
                        let now = Instant::now();
                        if now >= startup_deadline {
                            let msg = format!(
                                "Sidecar emitted FYOM_READY but /readyz never confirmed before timeout: {}",
                                e
                            );

                            if let Some(rt) = runtime.take() {
                                cleanup_failed_bootstrap(rt, &msg).await;
                            }

                            tracing::error!("[bootstrap] {}", msg);
                            sidecar_state.set_error(msg.clone());
                            let _ = app.emit(SIDECAR_ERROR_EVENT, msg.clone());
                            bail!("{}", msg);
                        } else {
                            if should_warn_readyz_retry(readyz_attempts) {
                                tracing::warn!(
                                    "[bootstrap] /readyz request failed after FYOM_READY: {} (attempt {}, retrying until timeout budget is exhausted)",
                                    e,
                                    readyz_attempts,
                                );
                            } else {
                                tracing::debug!(
                                    "[bootstrap] /readyz request failed after FYOM_READY: {} (attempt {})",
                                    e,
                                    readyz_attempts,
                                );
                            }
                        }
                    }
                }
            }
        }
    }
}

/// Shutdown the sidecar process cleanly.
pub async fn shutdown_sidecar(state: &AppState) -> Result<()> {
    let sidecar_state = &state.sidecar_state;

    if let Some(runtime) = take_running_sidecar().await {
        tracing::info!("Shutting down sidecar (pid={})", runtime.pid);
        runtime.shutdown("shutdown_sidecar called").await;
    } else {
        tracing::debug!("No running sidecar to shut down (already stopped or never started)");
    }

    sidecar_state.set_error("Shutting down".to_string());
    Ok(())
}
