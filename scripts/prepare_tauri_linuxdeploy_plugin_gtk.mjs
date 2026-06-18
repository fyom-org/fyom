#!/usr/bin/env node
// PORTED_FROM_SOIA @ 9e22064 (src-tauri/scripts/prepare_tauri_linuxdeploy_plugin_gtk.mjs)
//
// Verbatim port from soia (GPL-3.0-only). Copies the bundled
// linuxdeploy-plugin-gtk.sh into ~/.cache/tauri/ so that Tauri's AppImage
// bundler can find it during `cargo-tauri build --bundles appimage`.
//
// This file is licensed under GPL-3.0-only, inherited from soia.
import { chmodSync, copyFileSync, existsSync, mkdirSync } from "node:fs";
import { homedir } from "node:os";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const scriptDir = dirname(fileURLToPath(import.meta.url));
const repoPluginPath = resolve(scriptDir, "linuxdeploy-plugin-gtk.sh");
const tauriCacheDir = resolve(homedir(), ".cache", "tauri");
const pluginPath = resolve(tauriCacheDir, "linuxdeploy-plugin-gtk.sh");

if (!existsSync(repoPluginPath)) {
  console.error(`[ERROR] repo plugin script not found: ${repoPluginPath}`);
  process.exit(1);
}

mkdirSync(tauriCacheDir, { recursive: true });
copyFileSync(repoPluginPath, pluginPath);
chmodSync(pluginPath, 0o755);
console.log(`[INFO] Prepared linuxdeploy GTK plugin in Tauri cache: ${pluginPath}`);
