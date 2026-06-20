# Architecture Migration: Tauri to Wails for fyom

## Background (What)
The `fyom` project is designed as a lightweight, self-hosted media catalog with a dual-mode access pattern: a C/S web UI and a local desktop client. The core technology stack consists of Go (Chi router, business logic) + Vue 3 + SQLite (via `modernc.org`, CGO-free). 

Currently, the desktop client is built with Tauri 2. Because Tauri's native backend is Rust, the Go application runs as a Sidecar process. Tauri merely acts as a process launcher and a WebView wrapper. The Rust layer contains almost no business logic. This Sidecar architecture introduces significant friction: complex process lifecycle management (orphaned processes on crash), local port conflicts (the Go server must bind to a specific TCP port like `:27403`), and unnecessary IPC overhead between the Rust host and the Go process. 

## Decision (What will be)
We will **deprecate Tauri and migrate the desktop client to Wails v2**. 

Wails will make Go the first-class citizen for the desktop backend. We will utilize Wails' `AssetServer` mechanism to embed the Vue frontend and intercept HTTP requests directly in the Go memory space, completely eliminating the local TCP port binding and the Sidecar process model for desktop deployments. The existing Go + Chi + SQLite architecture will be preserved and directly integrated into the Wails application lifecycle.

## Consequences (Why)

### Positive
* **Unified Backend Architecture**: Eliminates the Rust layer entirely. Go handles everything from native window management to business logic and database access. Zero cross-language communication overhead.
* **Elimination of Sidecar Pain Points**: No more orphaned Go processes. No local TCP port conflicts. The desktop app becomes a true single-binary, single-process application.
* **Clear and easy CI Cross-Platform Build**: On the CI runners for their respective target platforms, the build commands for Go + Wails are more concise than those for Rust + Tauri and do not require the Rust toolchain.
* **Dual-Mode Compatibility Preserved**: The same Go codebase can still run as a standalone HTTP server (`./fyom serve`) for headless/C-S deployments, while natively serving the WebView when running as a Wails desktop app.

### Negative
* **Slightly Larger Bundle Size**: The final binary size will increase slightly (estimated 10-15MB) compared to Tauri's ultra-small bundles (3-8MB) due to the inclusion of the Go runtime. This is an acceptable trade-off for a local media catalog application.
* **Loss of Tauri Plugin Ecosystem**: We forfeit access to Tauri's native plugins (e.g., global shortcuts, system tray extensions). If such features are needed in the future, they must be implemented using Go-native libraries or Wails' native APIs.
* **Memory Footprint**: Go's garbage collector will consume more memory at runtime compared to Rust's zero-cost abstraction model.

## Engineering details (How to do)

1. **Remove Tauri Scaffolding**: Delete the `src-tauri` directory and all Tauri-related dependencies from the project.
2. **Initialize Wails Integration**: Run `wails init -n fyom -t vue` in a temporary directory, then merge the generated `main.go` (Wails entry point) and `wails.json` configuration into the existing `fyom` project structure.
3. **Adapt the Entry Point (`cmd/fyom/main.go`)**:
   * Instantiate the existing `internal/config`, `internal/repository`, and `internal/service` components.
   * Instead of immediately calling `http.ListenAndServe`, pass the Chi router to a Wails custom AssetServer handler.
4. **Implement Request Interception**:
   * Use Wails' `assetserver` package. 
   * Register an `http.Handler` (the existing Chi router) to handle requests routed to `/api/v1/*`.
   * Configure the Wails `embed.FS` (containing the Vue `dist/` files) to serve static assets for all other paths.
5. **Application Lifecycle Binding**:
   * Map the `OnStartup` and `OnShutdown` Wails lifecycle hooks to the existing Go server's graceful shutdown and database cleanup logic.
6. **Frontend Adjustments**:
   * The Vue frontend requires zero changes to its API call logic. `fetch('/api/v1/library')` will seamlessly hit the Go Chi router via Wails' internal interception mechanism rather than a network port.

## Key points and boundary (What should do, what should not do)

**What should DO:**
* **Reuse the Chi Router**: Do not rewrite REST endpoints as Wails native Go bindings (e.g., `func (a *App) GetLibrary()`). Keep the REST API contract. This ensures the headless server mode and desktop mode share the exact same routing and middleware logic.
* **Keep CGO Disabled**: Strictly maintain the use of `modernc.org/sqlite`. Do not introduce any C dependencies, ensuring cross-compilation remains trivial.
* **Use Wails AssetServer for API Proxying**: Leverage the `assetserver` middleware to bridge the WebView and the Chi router without opening a local TCP socket.

**What should NOT do:**
* **Do not open local TCP ports in Desktop mode**: When running as a Wails app, the Go HTTP server should *not* bind to `:27403`. Let Wails handle the internal `http.Request` and `http.ResponseWriter` directly.
* **Do not maintain Tauri-specific code**: Completely remove any Rust code, `tauri.conf.json`, or frontend logic specifically written to handle Tauri Sidecar events.
* **Do not bundle SQLite as an external file**: Continue to rely on the single-binary principle. The SQLite database file should be created at runtime in the user's application data directory, managed by Go.

## Implementation Notes (What things left for the future)

* **Native Feature Integration**: If system tray support, global hotkeys, or native file dialog integrations are requested later, they should be implemented using Wails v2's native API bindings in Go, rather than looking back at Tauri.
* **Wails v3 Evaluation**: Wails v3 is currently in alpha. While this migration targets the stable v2, we should monitor v3's release for potential future upgrades, especially regarding its improved routing and native dialog capabilities.

## Additonal:

### External Player Observation Policy

The Wails evaluation phase must not introduce a hard dependency on mpv IPC or any player-specific control protocol.

The desktop shell should support external-player launch as a generic capability:

1. Resolve authorized playback URI.
2. Build player invocation arguments.
3. Launch configured external player.
4. Record launch success or failure.

Precise playback progress synchronization is out of scope for Wails external-player mode. It will be addressed by managed clients, especially future Flutter native clients, through standard backend APIs.
