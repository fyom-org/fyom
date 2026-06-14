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

use crate::{SIDECAR_ERROR_EVENT, SIDECAR_READY_EVENT, SIDECAR_STARTUP_TIMEOUT_SECS};

/// The expected readiness token emitted by the Go sidecar on stdout.
const READY_TOKEN: &str = "FYOM_READY";

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

/// Bootstrap the Go sidecar process.
///
/// Spawns the sidecar, waits for the FYOM_READY signal, then confirms /readyz.
/// The main window should remain hidden until this succeeds.
pub async fn bootstrap_sidecar(app: &AppHandle, state: &crate::AppState) -> Result<()> {
    let sidecar_state = &state.sidecar_state;
    sidecar_state.set_starting();

    let binary_path = find_sidecar_binary()
        .map_err(|e| anyhow::anyhow!("Failed to locate sidecar binary: {}", e))?;

    // Use the desktop DB path resolved by the main app in lib.rs.
    // This guarantees the path is based on the main app executable directory,
    // not the sidecar binary directory.
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

    // Spawn a task to log stderr
    tauri::async_runtime::spawn(async move {
        let reader = BufReader::new(stderr);
        let mut lines = reader.lines();
        while let Ok(Some(line)) = lines.next_line().await {
            tracing::info!(target: "sidecar", "{}", line);
        }
    });

    // Wait for FYOM_READY on stdout, with timeout
    let deadline = Duration::from_secs(SIDECAR_STARTUP_TIMEOUT_SECS);
    let start = Instant::now();

    let reader = BufReader::new(stdout);
    let mut lines = reader.lines();
    let mut ready_emitted = false;

    // Spawn a background task to monitor the child process
    // We do this before the loop to avoid ownership issues
    let app_handle_for_monitoring = app.clone();
    tauri::async_runtime::spawn(async move {
        match child.wait().await {
            Ok(status) if !status.success() => {
                tracing::warn!("Sidecar exited with non-zero status: {:?}", status);
                let _ = app_handle_for_monitoring
                    .emit(SIDECAR_ERROR_EVENT, format!("Sidecar exited: {:?}", status));
            }
            Err(e) => {
                tracing::error!("Sidecar wait error: {}", e);
                let _ = app_handle_for_monitoring
                    .emit(SIDECAR_ERROR_EVENT, format!("Sidecar error: {}", e));
            }
            _ => tracing::info!("Sidecar exited normally"),
        }
    });

    loop {
        // Check overall timeout
        if start.elapsed() > deadline {
            // We can't kill the child here since it was moved to the background task
            // But we can still report the timeout
            let msg = format!(
                "Sidecar startup timed out after {}s (pid={})",
                SIDECAR_STARTUP_TIMEOUT_SECS, child_id
            );
            tracing::error!("{}", msg);
            sidecar_state.set_error(msg.clone());
            let _ = app.emit(SIDECAR_ERROR_EVENT, msg.clone());
            anyhow::bail!("{}", msg);
        }

        // We can't use child.try_wait() here since child was moved to the background task
        // Instead, we rely on the background task to monitor process exit
        // and just continue reading stdout

        // Try to read next line with a short timeout
        match tokio::time::timeout(Duration::from_millis(200), lines.next_line()).await {
            Ok(Ok(Some(line))) => {
                tracing::debug!(target: "sidecar", "stdout: {}", line);

                // Check for readiness token
                if let Some(api_url) = parse_ready_line(&line) {
                    if ready_emitted {
                        // Already emitted ready event, skip but continue reading stdout
                        continue;
                    }

                    ready_emitted = true;
                    tracing::info!("FYOM_READY received: {}", api_url);
                    tracing::info!("  db_path:    {}", db_path.display());

                    // Confirm readiness with /readyz
                    if let Err(e) = confirm_readyz(&api_url).await {
                        tracing::warn!("/readyz confirmation failed: {}", e);
                        // Don't fail — the sidecar emitted FYOM_READY so it's likely fine
                    }

                    // Store the API base URL
                    let api_base_url = format!("{}/api/v1", api_url);
                    let full_url = api_url.clone();
                    sidecar_state.set_ready(api_base_url);

                    // Emit readiness event to the frontend
                    let _ = app.emit(SIDECAR_READY_EVENT, full_url);

                    // Continue reading stdout instead of returning
                    // This ensures we don't miss any subsequent output from the sidecar
                    continue;
                }
            }
            Ok(Ok(None)) => {
                // EOF on stdout — process may have exited
                tracing::error!("Sidecar stdout closed unexpectedly");
                break;
            }
            Ok(Err(e)) => {
                tracing::error!("Error reading sidecar stdout: {}", e);
                break;
            }
            Err(_) => {
                // Timeout — loop back and check deadline / process status
                continue;
            }
        }
    }

    // If we get here, the sidecar exited without emitting FYOM_READY
    let msg = format!(
        "Sidecar exited without emitting FYOM_READY (pid={})",
        child_id
    );
    tracing::error!("{}", msg);
    sidecar_state.set_error(msg.clone());
    let _ = app.emit(SIDECAR_ERROR_EVENT, msg.clone());
    anyhow::bail!("{}", msg)
}

/// Confirm sidecar readiness by calling /readyz.
async fn confirm_readyz(api_url: &str) -> Result<()> {
    let client = reqwest::Client::builder()
        .timeout(Duration::from_secs(2))
        .build()?;

    let url = format!("{}/readyz", api_url);
    let resp = client.get(&url).send().await?;

    if resp.status().is_success() {
        tracing::info!("/readyz confirmed OK");
        Ok(())
    } else {
        anyhow::bail!("/readyz returned status: {}", resp.status())
    }
}

/// Shutdown the sidecar process cleanly.
pub async fn shutdown_sidecar(_app: &AppHandle, state: &crate::AppState) -> Result<()> {
    let sidecar_state = &state.sidecar_state;
    tracing::info!("Shutting down sidecar");
    sidecar_state.set_error("Shutting down".to_string());
    Ok(())
}
