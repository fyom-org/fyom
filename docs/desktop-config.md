# fyom Desktop Config

fyom Desktop uses a Tauri-local desktop config file named `fyom-desktop.json`.

This file is only for desktop shell behavior such as external player selection.
It is intentionally separate from the Go backend `fyom.yaml`.

## Resolution order

1. `FYOM_DESKTOP_CONFIG`
2. Platform user config path
3. Development fallback `configs/fyom-desktop.json`
4. No config; use environment player overrides or OS default opener

## Platform user config paths

- Windows: `%APPDATA%\fyom\fyom-desktop.json`
- macOS: `~/Library/Application Support/fyom/fyom-desktop.json`
- Linux: `${XDG_CONFIG_HOME:-~/.config}/fyom/fyom-desktop.json`

## Player override priority

1. `FYOM_EXTERNAL_PLAYER`, `FYOM_EXTERNAL_PLAYER_ARGS`, `FYOM_MPV_BIN`
2. resolved `fyom-desktop.json`
3. OS default opener

## Important boundary

Do not move desktop-local player configuration into Go backend `fyom.yaml`.
