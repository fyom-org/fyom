# Use external mpv sidecar window for desktop playback

## Context

FYOM Desktop previously attempted to embed mpv into the Tauri WebView window.
This required platform-specific native window integration across macOS,
Windows, Linux X11, and Linux Wayland.

The embedded model introduced significant complexity:

- macOS requires native view and layer lifecycle handling.
- Windows requires HWND, DPI, focus, and resize synchronization.
- Linux X11 supports `--wid`, but still requires window synchronization.
- Linux Wayland does not provide a stable native window id suitable for mpv
  `--wid` embedding.

The Wayland path in particular would require deeper mpv and Wayland protocol
integration, such as parent `wl_surface` / subsurface support. This is outside
FYOM's core product scope.

FYOM's primary role is a media library, metadata system, desktop shell, and
playback controller. mpv is already a mature native media player with
customizable UI, input handling, subtitles, shaders, hardware decoding, and
fullscreen behavior.

## Decision

FYOM Desktop will not embed mpv into the Tauri window.

Instead, FYOM Desktop will launch and control mpv as an external sidecar player.
mpv will own its own native player window on every desktop platform:

- macOS
- Windows
- Linux X11
- Linux Wayland

Tauri and Vue will focus on the library and control UI. Playback control will be
performed through mpv JSON IPC.

## Consequences

### Positive

- Cross-platform playback behavior becomes consistent.
- Linux Wayland no longer requires unsupported native embedding.
- Tauri no longer needs to synchronize native video surfaces.
- FYOM avoids deep platform-specific compositor and windowing complexity.
- Users can browse the library while video plays in a separate window.
- mpv's own UI and configuration ecosystem can be reused and customized.

### Negative

- Vue overlays cannot appear above the mpv video window.
- Playback uses two windows instead of one.
- mpv IPC must be robustly implemented across Unix sockets and Windows named
  pipes.

## Implementation Notes

The desktop playback path must not pass `--wid` to mpv.

The playback controller should:

1. Resolve a media item to a playable path or URL.
2. Spawn mpv with an IPC endpoint if no mpv process is running.
3. Send `loadfile` commands over JSON IPC for subsequent playback.
4. Track process exit and clear playback state.
5. Shut down mpv when FYOM exits.

The old platform render surface modules are deprecated and retained only during
migration.
