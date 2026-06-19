# External Media Launcher Mode

## Context

FYOM Desktop (Tauri + Vue) was originally designed to embed `libmpv` directly into the application window to provide an integrated playback experience. This approach required managing native window handles (`--wid`), OpenGL/Vulkan contexts, and compositor-specific overlays across macOS, Windows, Linux X11, and Linux Wayland.

This embedded model introduced significant, unsolvable engineering friction:
- macOS deprecated OpenGL and requires complex Metal-layer bridging.
- Linux Wayland does not support stable native window ID embedding for mpv.
- WebKitGTK conflicts with GStreamer pipelines.

Furthermore, FYOM's long-term roadmap includes building a dedicated native desktop client using Dart + Flutter, which will handle native playback natively and elegantly. Investing further time in deep C-level rendering integration for the transitional Tauri shell is a misallocation of engineering resources.

## Decision

FYOM Desktop (Tauri) will pivot from an "embedded media player" to a **"Media Library & Launcher"**.

1. **No Bundled Player:** FYOM will no longer bundle, ship, or manage `libmpv` or `mpv` executables.
2. **External Delegation:** When a user clicks "Play", FYOM will resolve the media item to a playable presigned URL and hand it off to the user's operating system or configured external media player (e.g., IINA, PotPlayer, VLC, mpv).
3. **State Degradation:** Playback progress tracking is downgraded. FYOM will no longer track `time-pos` (current timestamp) or synchronize playback state. It will only record a boolean `played: true` state when the user initiates playback.

## Consequences

### Positive
- **Zero Rendering Complexity:** Completely bypasses all X11/Wayland/Metal/OpenGL windowing nightmares.
- **Minimal Bundle Size:** Tauri installer returns to its lean footprint (<10MB) without bundled C libraries, ffmpeg, or MoltenVK.
- **Supply Chain Simplification:** Eliminates the need to maintain `fyom-org/fork-mpv`. Removes GPL licensing friction from the desktop client distribution.
- **Resource Reallocation:** Frees up 100% of desktop development focus to polish the library UI/UX and accelerate the future Flutter native client.

### Negative
- **No In-App Overlays:** Custom HTML/CSS overlays cannot be drawn on top of the video.
- **No State Sync:** If the user pauses, seeks, or closes the external player, FYOM is unaware.
- **Authentication Constraint:** External players generally cannot send custom HTTP headers. Therefore, the FYOM backend **must** support presigned URLs for media streaming.

## Implementation Notes

### Rust Backend
- **Process Spawner:** Implement a simple cross-platform command executor.
  - Default: Use `open` (macOS), `xdg-open` (Linux), or `start` (Windows) to let the OS pick the default player.
  - Custom: Read an optional `external_player_path` from FYOM settings. If set, spawn that binary with the URL as an argument.
- **API Adjustment:**
  - Remove all `libmpv` FFI, IPC sockets, and render surface modules.
  - The `/api/v1/media/:id/progress` endpoint should accept a simple `{ "finished": boolean }` or `{ "played": boolean }` payload, replacing complex timestamp logic.

### Frontend (Vue)
- Remove `PlayerView.vue` rendering surface logic, `attach_render_surface` calls, and `fyom://mpv/*` event listeners.
- The "Play" button now simply calls `invoke('open_external_player', { url })` and triggers a background API call to mark the media as played.
- UI updates to reflect a simple "Watched / Unwatched" badge.
