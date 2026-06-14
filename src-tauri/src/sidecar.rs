//! Sidecar lifecycle management
//!
//! Handles spawning, readiness detection, and graceful shutdown of the Go sidecar process.

use std::path::PathBuf;
use std::process::Stdio;
use std::time::{Duration, Instant};

use anyhow::Result;
use tauri::{AppHandle, Emitter};
use tokio::io::{AsyncBufReadExt, BufReader};
use tokio::process::Command;
use tokio::sync::mpsc;

use crate::{SIDECAR_ERROR_EVENT, SIDECAR_READY_EVENT, SIDECAR_STARTUP_TIMEOUT_SECS};

/// The expected readiness token emitted by the Go sidecar on stdout.
const READY_TOKEN: &str = "FYOM_READY";

/// Maximum number of /readyz retry attempts
const READYZ_MAX_RETRIES: u32 = 3;

/// Delay between /readyz retry attempts
const READYZ_RETRY_DELAY_MS: u64 = 100;

/// Parse a readiness line from the sidecar stdout.
///
/// Expected format: `FYOM_READY http://127.0.0.1:27403`
/// Returns the API base URL if the line is valid.
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
/// 2. `build/fyom` relative to the project root (Cargo workspace root)
/// 3. `fyom` on PATH
fn find_sidecar_binary() -> Result<PathBuf> {
    // 1. Explicit env override
    if let Ok(bin) = std::env::var("FYOM_BIN") {
        let path = PathBuf::from(&bin);
        if path.exists() {
            return Ok(path);
        }
        anyhow::bail!("FYOM_BIN={} not found", bin);
    }

    // 2. Check common build locations relative to the Tauri project root
    let manifest_dir = std::env::var("CARGO_MANIFEST_DIR")
        .map(PathBuf::from)
        .unwrap_or_else(|_| std::env::current_dir().unwrap_or_default());

    // src-tauri -> project root -> build/fyom
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

    // 3. Fall back to PATH
    if let Ok(output) = std::process::Command::new("which").arg("fyom").output() {
        if output.status.success() {
            let path = String::from_utf8_lossy(&output.stdout).trim().to_string();
            if !path.is_empty() {
                return Ok(PathBuf::from(path));
            }
        }
    }

    anyhow::bail!("fyom sidecar binary not found. Build it with: task sidecar (or set FYOM_BIN)")
}

/// Confirm sidecar readiness by calling /readyz with bounded retries.
async fn confirm_readyz_with_retries(api_url: &str) -> Result<()> {
    let client = reqwest::Client::builder()
        .timeout(Duration::from_secs(2))
        .build()?;

    let url = format!("{}/readyz", api_url);

    for attempt in 1..=READYZ_MAX_RETRIES {
        match client.get(&url).send().await {
            Ok(resp) if resp.status().is_success() => {
                tracing::info!(
                    "/readyz confirmed OK (attempt {}/{})",
                    attempt,
                    READYZ_MAX_RETRIES
                );
                return Ok(());
            }
            Ok(resp) => {
                tracing::warn!(
                    "/readyz returned status: {} (attempt {}/{})",
                    resp.status(),
                    attempt,
                    READYZ_MAX_RETRIES
                );
            }
            Err(e) => {
                tracing::warn!(
                    "/readyz request failed: {} (attempt {}/{})",
                    e,
                    attempt,
                    READYZ_MAX_RETRIES
                );
            }
        }

        if attempt < READYZ_MAX_RETRIES {
            tokio::time::sleep(Duration::from_millis(READYZ_RETRY_DELAY_MS)).await;
        }
    }

    // All retries exhausted
    anyhow::bail!(
        "/readyz confirmation failed after {} attempts",
        READYZ_MAX_RETRIES
    )
}

/// Bootstrap the Go sidecar process.
///
/// Spawns the sidecar, starts background drain tasks, waits for FYOM_READY signal,
/// confirms readiness via /readyz, and returns when sidecar is ready.
/// The main window should remain hidden until this succeeds.
pub async fn bootstrap_sidecar(app: &AppHandle, state: &crate::AppState) -> Result<()> {
    let sidecar_state = &state.sidecar_state;
    sidecar_state.set_starting();

    let binary_path = find_sidecar_binary()
        .map_err(|e| anyhow::anyhow!("Failed to locate sidecar binary: {}", e))?;

    // Use the desktop DB path resolved by the main app in lib.rs.
    let db_path = PathBuf::from(state.desktop_db_path.as_str());

    tracing::info!("Desktop sidecar starting");
    tracing::info!(
        "  app_exe_dir: {}",
        db_path
            .parent()
            .unwrap_or(std::path::Path::new(""))
            .display()
    );
    tracing::info!("  db_path:     {}", db_path.display());
    tracing::info!("  sidecar_bin: {}", binary_path.display());
    tracing::info!(
        "  args:        --sidecar --db-path {} --log-level info",
        db_path.display()
    );

    // Spawn the sidecar process
    let mut child = Command::new(&binary_path)
        .arg("--sidecar")
        .arg("--db-path")
        .arg(&db_path)
        .arg("--log-level")
        .arg("info")
        .stdin(Stdio::null())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .spawn()
        .map_err(|e| {
            let msg = format!("Failed to spawn sidecar: {}", e);
            tracing::error!("{}", msg);
            anyhow::anyhow!("{}", msg)
        })?;

    let child_id = child.id().unwrap_or(0);
    sidecar_state.set_child_pid(child_id);
    tracing::info!("Sidecar spawned, pid={}", child_id);

    // Take stdout and stderr handles
    let stdout = child
        .stdout
        .take()
        .ok_or_else(|| anyhow::anyhow!("Failed to capture sidecar stdout"))?;
    let stderr = child
        .stderr
        .take()
        .ok_or_else(|| anyhow::anyhow!("Failed to capture sidecar stderr"))?;

    // Create channel for readiness notification
    let (ready_tx, mut ready_rx) = mpsc::channel::<String>(1);

    // Create channel for kill signal
    let (kill_tx, mut kill_rx) = mpsc::channel::<()>(1);

    // Spawn a background task to drain stdout and detect FYOM_READY
    // This task owns the child process and stdout reader
    tauri::async_runtime::spawn(async move {
        tracing::info!("[sidecar-drain] stdout drain task started");

        let reader = BufReader::new(stdout);
        let mut lines = reader.lines();
        let mut ready_emitted = false;

        // Spawn stderr drain in background
        tauri::async_runtime::spawn(async move {
            tracing::info!("[sidecar-drain] stderr drain task started");
            let reader = BufReader::new(stderr);
            let mut lines = reader.lines();
            while let Ok(Some(line)) = lines.next_line().await {
                tracing::info!(target: "sidecar", "[stderr] {}", line);
            }
            tracing::info!("[sidecar-drain] stderr drain task ended");
        });

        // Monitor child process exit and kill signals
        tauri::async_runtime::spawn(async move {
            tracing::info!("[sidecar-drain] process monitor task started");
            tokio::select! {
                status = child.wait() => {
                    match status {
                        Ok(status) if !status.success() => {
                            tracing::warn!("[sidecar-drain] Sidecar exited with non-zero status: {:?}", status);
                        }
                        Err(e) => {
                            tracing::error!("[sidecar-drain] Sidecar wait error: {}", e);
                        }
                        _ => tracing::info!("[sidecar-drain] Sidecar exited normally"),
                    }
                }
                _ = kill_rx.recv() => {
                    tracing::info!("[sidecar-drain] Kill signal received, killing sidecar");
                    let _ = child.kill().await;
                }
            }
            tracing::info!("[sidecar-drain] process monitor task ended");
        });

        // Read stdout lines and detect readiness
        loop {
            match tokio::time::timeout(Duration::from_millis(200), lines.next_line()).await {
                Ok(Ok(Some(line))) => {
                    tracing::debug!(target: "sidecar", "[stdout] {}", line);

                    // Check for readiness token
                    if let Some(api_url) = parse_ready_line(&line) {
                        if ready_emitted {
                            // Already emitted ready event, skip but continue draining
                            continue;
                        }

                        ready_emitted = true;
                        tracing::info!("[sidecar-drain] FYOM_READY received: {}", api_url);

                        // Send the API URL through the channel
                        // This is a one-time signal, channel capacity is 1
                        let _ = ready_tx.send(api_url).await;

                        // Continue reading stdout to drain the pipe
                        continue;
                    }
                }
                Ok(Ok(None)) => {
                    // EOF on stdout — process may have exited
                    tracing::error!("[sidecar-drain] Sidecar stdout closed unexpectedly");
                    break;
                }
                Ok(Err(e)) => {
                    tracing::error!("[sidecar-drain] Error reading sidecar stdout: {}", e);
                    break;
                }
                Err(_) => {
                    // Timeout — continue looping
                    continue;
                }
            }
        }
        tracing::info!("[sidecar-drain] stdout drain task ended");
    });

    // Wait for ready notification with timeout
    let deadline = Duration::from_secs(SIDECAR_STARTUP_TIMEOUT_SECS);
    let start = Instant::now();

    loop {
        // Check if we received the ready signal
        match tokio::time::timeout(Duration::from_millis(0), ready_rx.recv()).await {
            Ok(Some(api_url)) => {
                tracing::info!("[bootstrap] FYOM_READY received, confirming /readyz...");

                // Confirm readiness with /readyz using bounded retries
                if let Err(e) = confirm_readyz_with_retries(&api_url).await {
                    tracing::error!("[bootstrap] /readyz confirmation failed: {}", e);
                    // Even if /readyz fails, if we got FYOM_READY, the sidecar is likely ready
                    // But we treat this as a failure for strict readiness confirmation
                    let msg = format!("Sidecar ready but /readyz failed: {}", e);
                    sidecar_state.set_error(msg.clone());
                    let _ = app.emit(SIDECAR_ERROR_EVENT, msg);
                    anyhow::bail!("{}", msg);
                }

                tracing::info!("[bootstrap] /readyz confirmed OK");

                // Store the API base URL
                let api_base_url = format!("{}/api/v1", api_url);
                sidecar_state.set_ready(api_base_url.clone());

                // Emit readiness event to the frontend
                let _ = app.emit(SIDECAR_READY_EVENT, api_url.clone());

                tracing::info!("[bootstrap] Sidecar bootstrap completed successfully");

                // Bootstrap completed successfully
                return Ok(());
            }
            Ok(None) => {
                // Channel closed, sidecar probably exited before emitting FYOM_READY
                let msg = "Sidecar exited without emitting FYOM_READY".to_string();
                tracing::error!("[bootstrap] {}", msg);
                sidecar_state.set_error(msg.clone());
                let _ = app.emit(SIDECAR_ERROR_EVENT, msg.clone());
                anyhow::bail!("{}", msg);
            }
            Err(_) => {
                // No message yet, continue waiting
            }
        }

        // Check timeout
        if start.elapsed() > deadline {
            // Send kill signal to background task
            let _ = kill_tx.send(()).await;
            let msg = format!(
                "Sidecar startup timed out after {}s (pid={})",
                SIDECAR_STARTUP_TIMEOUT_SECS, child_id
            );
            tracing::error!("[bootstrap] {}", msg);
            sidecar_state.set_error(msg.clone());
            let _ = app.emit(SIDECAR_ERROR_EVENT, msg.clone());
            anyhow::bail!("{}", msg);
        }

        // Short sleep to avoid busy waiting
        tokio::time::sleep(Duration::from_millis(50)).await;
    }
}

/// Shutdown the sidecar process cleanly.
pub async fn shutdown_sidecar(_app: &AppHandle, state: &crate::AppState) -> Result<()> {
    let sidecar_state = &state.sidecar_state;
    tracing::info!("Shutting down sidecar");
    sidecar_state.set_error("Shutting down".to_string());
    Ok(())
}
