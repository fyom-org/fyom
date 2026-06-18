#!/usr/bin/env node
// PORTED_FROM_SOIA @ 9e22064 (src-tauri/scripts/run_tauri.mjs)
//
// Verbatim port from soia (GPL-3.0-only). Wraps `cargo-tauri dev/build`.
//
// fyom changes:
//   1. Linux Nix dev mode is isolated from bundled runtime libraries.
//   2. Linux Nix dev mode uses Nix-provided GLib/GTK/WebKit/mpv libraries.
//   3. Linux build/bundle mode keeps the existing runtime sync/apply flow.
//   4. macOS and Windows behavior is preserved as much as possible.
//
// This file is licensed under GPL-3.0-only, inherited from soia.

import { existsSync, readdirSync, rmSync } from "node:fs";
import { spawn, spawnSync } from "node:child_process";
import { delimiter, dirname, isAbsolute, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const filename = fileURLToPath(import.meta.url);
const scriptDir = dirname(filename);
const projectRoot = dirname(scriptDir);

const syncScript = resolve(scriptDir, "sync_runtime_libs.mjs");
const applyScript = resolve(scriptDir, "apply_runtime_libs.mjs");

const srcTauriDir = resolve(projectRoot, "src-tauri");
const tauriRuntimeMacConfigPath = resolve(
  srcTauriDir,
  "tauri.runtime.macos.json",
);

const runtimeLibsDir = resolve(srcTauriDir, "libs");
const mpvRuntimeLibsDir = resolve(runtimeLibsDir, "mpv");

const targetDir = resolve(srcTauriDir, "target");
const targetDebugDir = resolve(targetDir, "debug");
const targetReleaseDir = resolve(targetDir, "release");

const devFrameworksDir = resolve(targetDir, "Frameworks");
const devVulkanIcdPath = resolve(
  targetDir,
  "Frameworks",
  "vulkan",
  "icd.d",
  "MoltenVK_icd.json",
);
const libsVulkanIcdPath = resolve(mpvRuntimeLibsDir, "MoltenVK_icd.json");

const tauriArgs = process.argv.slice(2);
const tauriSubcommand =
  tauriArgs.find((arg) => arg === "dev" || arg === "build") ?? "";

const isDevOrBuild = tauriSubcommand === "dev" || tauriSubcommand === "build";
const isDev = tauriSubcommand === "dev";
const isBuild = tauriSubcommand === "build";

const isLinux = process.platform === "linux";
const isDarwin = process.platform === "darwin";
const isWindows = process.platform === "win32";

const isNixShell =
  Boolean(process.env.IN_NIX_SHELL) ||
  Boolean(process.env.NIX_PROFILES) ||
  Boolean(process.env.NIX_STORE) ||
  Boolean(process.env.MPV_LIB_DIR);

const isLinuxNixDev = isLinux && isDev && isNixShell;

const childEnv = {
  ...process.env,
  FYOM_DESKTOP_MODE: tauriSubcommand || process.env.FYOM_DESKTOP_MODE || "",
};

function isPathAbsoluteForCurrentPlatform(value) {
  if (!value) {
    return false;
  }

  if (isWindows) {
    return /^[a-zA-Z]:[\\/]/.test(value) || value.startsWith("\\\\");
  }

  return value.startsWith("/");
}

function configureSidecarEnvironment() {
  const defaultFyomBin = resolve(
    projectRoot,
    "build",
    isWindows ? "fyom.exe" : "fyom",
  );
  const existingFyomBin = childEnv.FYOM_BIN;

  if (!existingFyomBin) {
    childEnv.FYOM_BIN = defaultFyomBin;
    return;
  }

  if (isPathAbsoluteForCurrentPlatform(existingFyomBin)) {
    return;
  }

  // FYOM_BIN must be absolute because cargo-tauri may launch the app with
  // src-tauri or another directory as the process working directory.
  childEnv.FYOM_BIN = resolve(projectRoot, existingFyomBin);
}

configureSidecarEnvironment();

function validateSidecarBinary() {
  if (!isDevOrBuild) {
    return;
  }

  if (!childEnv.FYOM_BIN) {
    console.error("[ERROR] FYOM_BIN is not set.");
    process.exit(1);
  }

  if (!existsSync(childEnv.FYOM_BIN)) {
    console.error(`[ERROR] Missing sidecar binary: ${childEnv.FYOM_BIN}`);
    console.error("[ERROR] Run: task sidecar");
    process.exit(1);
  }

  console.log(`[INFO] FYOM_BIN=${childEnv.FYOM_BIN}`);
}

validateSidecarBinary();

if (isLinuxNixDev) {
  childEnv.FYOM_DESKTOP_MODE = "dev";
  childEnv.FYOM_USE_BUNDLED_RUNTIME = "0";
  childEnv.FYOM_SKIP_APPLY_RUNTIME_LIBS = "1";
}

const userRequestedBundledRuntime = childEnv.FYOM_USE_BUNDLED_RUNTIME === "1";
const userDisabledBundledRuntime = childEnv.FYOM_USE_BUNDLED_RUNTIME === "0";

const shouldUseBundledRuntime =
  !isLinuxNixDev &&
  !userDisabledBundledRuntime &&
  (isBuild || userRequestedBundledRuntime || !isLinux);

const skipApplyRuntimeLibs =
  childEnv.FYOM_SKIP_APPLY_RUNTIME_LIBS === "1" || !shouldUseBundledRuntime;

function runNodeScriptOrExit(commandArgs) {
  const result = spawnSync(process.execPath, commandArgs, {
    cwd: projectRoot,
    stdio: "inherit",
    env: childEnv,
  });

  if (result.status !== 0) {
    process.exit(result.status ?? 1);
  }
}

function hasExecutableOnPath(command) {
  const result = spawnSync(command, ["--version"], {
    cwd: projectRoot,
    stdio: "ignore",
    env: childEnv,
  });

  return result.status === 0;
}

function hasUserConfigArg(args) {
  for (let i = 0; i < args.length; i += 1) {
    const arg = args[i];

    if (arg === "-c" || arg === "--config") {
      return true;
    }

    if (arg.startsWith("--config=")) {
      return true;
    }
  }

  return false;
}

function resolveEnvKey(envKey) {
  if (!isWindows) {
    return envKey;
  }

  const found = Object.keys(childEnv).find(
    (key) => key.toLowerCase() === envKey.toLowerCase(),
  );
  return found ?? envKey;
}

function splitEnvPath(value) {
  return String(value ?? "")
    .split(delimiter)
    .map((entry) => entry.trim())
    .filter(Boolean);
}

function joinEnvPath(entries) {
  return entries.filter(Boolean).join(delimiter);
}

function dedupePreserveOrder(entries) {
  return [...new Set(entries.filter(Boolean))];
}

function prependEnvPath(envKey, extraPaths) {
  const resolvedKey = resolveEnvKey(envKey);
  const existing = childEnv[resolvedKey] ?? "";
  const merged = [...extraPaths, ...splitEnvPath(existing)];
  const deduped = dedupePreserveOrder(merged);

  childEnv[resolvedKey] = joinEnvPath(deduped);

  if (isWindows && resolvedKey.toLowerCase() === "path") {
    for (const key of Object.keys(childEnv)) {
      if (key !== resolvedKey && key.toLowerCase() === "path") {
        delete childEnv[key];
      }
    }
  }
}

function isInside(parent, maybeChild) {
  const relativePath = relative(parent, maybeChild);
  return (
    relativePath === "" ||
    (!relativePath.startsWith("..") && !isAbsolute(relativePath))
  );
}

function normalizePathEntry(entry) {
  if (isAbsolute(entry)) {
    return resolve(entry);
  }

  return resolve(projectRoot, entry);
}

function isAllowedLinuxNixDevRuntimePath(entry) {
  const absoluteEntry = normalizePathEntry(entry);

  if (isInside(runtimeLibsDir, absoluteEntry)) {
    return false;
  }

  if (isInside(mpvRuntimeLibsDir, absoluteEntry)) {
    return false;
  }

  if (isInside(targetDebugDir, absoluteEntry)) {
    return false;
  }

  if (isInside(targetReleaseDir, absoluteEntry)) {
    return false;
  }

  if (isInside(projectRoot, absoluteEntry)) {
    return false;
  }

  if (absoluteEntry.startsWith("/nix/store/")) {
    return true;
  }

  // Keep NixOS OpenGL driver paths when present. They are commonly required for
  // hardware acceleration and should not introduce project-local GLib copies.
  if (absoluteEntry.startsWith("/run/opengl-driver")) {
    return true;
  }

  // Keep profile paths only when they are Nix-managed. This avoids accidentally
  // preserving /usr/local or project-local library directories.
  if (absoluteEntry.startsWith("/nix/var/nix/profiles/")) {
    return true;
  }

  return false;
}

function sanitizeLinuxNixDevPathList(value) {
  return dedupePreserveOrder(
    splitEnvPath(value).filter(isAllowedLinuxNixDevRuntimePath),
  );
}

function removeIfExists(filePath) {
  try {
    rmSync(filePath, {
      force: true,
      recursive: false,
    });
  } catch {
    // Ignore cleanup failures. Runtime validation catches remaining conflicts.
  }
}

function cleanupMatchingLibraries(directory, patterns) {
  if (!existsSync(directory)) {
    return;
  }

  let entries = [];

  try {
    entries = readdirSync(directory);
  } catch {
    return;
  }

  for (const entry of entries) {
    if (patterns.some((pattern) => pattern.test(entry))) {
      removeIfExists(resolve(directory, entry));
    }
  }
}

function cleanupLinuxNixDevForbiddenLibraries() {
  if (!isLinuxNixDev) {
    return;
  }

  const forbiddenLibraryPatterns = [
    /^libglib-2\.0\.so(\..*)?$/,
    /^libgobject-2\.0\.so(\..*)?$/,
    /^libgio-2\.0\.so(\..*)?$/,
    /^libgmodule-2\.0\.so(\..*)?$/,
    /^libgthread-2\.0\.so(\..*)?$/,
  ];

  cleanupMatchingLibraries(targetDebugDir, forbiddenLibraryPatterns);
  cleanupMatchingLibraries(targetReleaseDir, forbiddenLibraryPatterns);
}

function configureLinuxNixDevRuntime() {
  if (!isLinuxNixDev) {
    return;
  }

  cleanupLinuxNixDevForbiddenLibraries();

  // LD_PRELOAD can force old bundled libraries into the process before Nix
  // libraries are resolved. It is not needed in Linux Nix dev mode.
  delete childEnv.LD_PRELOAD;

  childEnv.FYOM_USE_BUNDLED_RUNTIME = "0";
  childEnv.FYOM_SKIP_APPLY_RUNTIME_LIBS = "1";

  const sanitizedLdLibraryPath = sanitizeLinuxNixDevPathList(
    childEnv.LD_LIBRARY_PATH,
  );
  const sanitizedPkgConfigPath = sanitizeLinuxNixDevPathList(
    childEnv.PKG_CONFIG_PATH,
  );

  if (
    process.env.MPV_LIB_DIR &&
    process.env.MPV_LIB_DIR.startsWith("/nix/store/")
  ) {
    childEnv.MPV_LIB_DIR = process.env.MPV_LIB_DIR;
    childEnv.LD_LIBRARY_PATH = joinEnvPath(
      dedupePreserveOrder([process.env.MPV_LIB_DIR, ...sanitizedLdLibraryPath]),
    );
  } else {
    delete childEnv.MPV_LIB_DIR;
    childEnv.LD_LIBRARY_PATH = joinEnvPath(sanitizedLdLibraryPath);
  }

  childEnv.PKG_CONFIG_PATH = joinEnvPath(sanitizedPkgConfigPath);

  console.log("[INFO] Linux Nix dev runtime isolation enabled");
  console.log(
    `[INFO] FYOM_USE_BUNDLED_RUNTIME=${childEnv.FYOM_USE_BUNDLED_RUNTIME}`,
  );
  console.log(
    `[INFO] FYOM_SKIP_APPLY_RUNTIME_LIBS=${childEnv.FYOM_SKIP_APPLY_RUNTIME_LIBS}`,
  );
  console.log(`[INFO] MPV_LIB_DIR=${childEnv.MPV_LIB_DIR ?? ""}`);
}

configureLinuxNixDevRuntime();

if (isDevOrBuild) {
  if (!hasExecutableOnPath("cargo")) {
    console.error("[ERROR] Missing required command: cargo");
    console.error(
      "[ERROR] Install Rust toolchain and ensure cargo is in PATH.",
    );
    console.error("[ERROR] Recommended: https://rustup.rs/");
    process.exit(1);
  }

  if (isDarwin) {
    runNodeScriptOrExit([syncScript, "--platform", "darwin", "--check"]);
  } else if (isLinux || isWindows) {
    if (isLinuxNixDev) {
      console.log("[INFO] Skip sync_runtime_libs in Linux Nix dev mode");
      console.log("[INFO] Skip apply_runtime_libs in Linux Nix dev mode");
    } else {
      runNodeScriptOrExit([
        syncScript,
        "--platform",
        process.platform,
        "--check",
      ]);

      if (!skipApplyRuntimeLibs) {
        if (isDev) {
          runNodeScriptOrExit([
            applyScript,
            "--platform",
            process.platform,
            "--mode",
            "dev",
            "--profile",
            "debug",
          ]);
        } else if (isBuild) {
          const profile = tauriArgs.includes("--debug") ? "debug" : "release";

          runNodeScriptOrExit([
            applyScript,
            "--platform",
            process.platform,
            "--mode",
            "bundle",
            "--profile",
            profile,
          ]);
        }
      } else {
        console.log("[INFO] Skip apply_runtime_libs");
      }
    }
  }
}

if (isDevOrBuild && isDarwin && !hasUserConfigArg(tauriArgs)) {
  if (!existsSync(tauriRuntimeMacConfigPath)) {
    console.error(
      `[ERROR] Missing macOS runtime config: ${tauriRuntimeMacConfigPath}`,
    );
    console.error(
      "[ERROR] Run: node scripts/setup_runtime_libs.mjs --platform darwin",
    );
    process.exit(1);
  }

  tauriArgs.push("--config", tauriRuntimeMacConfigPath);
}

if (isDev && existsSync(runtimeLibsDir)) {
  if (isDarwin) {
    const extra = [runtimeLibsDir];

    if (existsSync(mpvRuntimeLibsDir)) {
      extra.push(mpvRuntimeLibsDir);
    }

    if (existsSync(devFrameworksDir)) {
      extra.push(devFrameworksDir);
    }

    prependEnvPath("DYLD_FALLBACK_LIBRARY_PATH", extra);

    const vulkanIcdCandidates = [];

    if (existsSync(devVulkanIcdPath)) {
      vulkanIcdCandidates.push(devVulkanIcdPath);
    }

    if (existsSync(libsVulkanIcdPath)) {
      vulkanIcdCandidates.push(libsVulkanIcdPath);
    }

    if (vulkanIcdCandidates.length > 0) {
      const dedupedIcdList =
        dedupePreserveOrder(vulkanIcdCandidates).join(delimiter);
      const vkDriverFiles = childEnv.VK_DRIVER_FILES;
      const vkIcdFilenames = childEnv.VK_ICD_FILENAMES;

      childEnv.VK_DRIVER_FILES = vkDriverFiles
        ? `${dedupedIcdList}${delimiter}${vkDriverFiles}`
        : dedupedIcdList;

      childEnv.VK_ICD_FILENAMES = vkIcdFilenames
        ? `${dedupedIcdList}${delimiter}${vkIcdFilenames}`
        : dedupedIcdList;
    }
  } else if (isLinux) {
    if (isLinuxNixDev) {
      // Linux Nix dev must never prepend bundled runtime paths. The bundled mpv
      // directory can contain old GLib-compatible libraries that break Nix
      // libgobject at runtime.
      console.log(
        "[INFO] Do not prepend bundled runtime libs to LD_LIBRARY_PATH in Linux Nix dev",
      );
    } else if (shouldUseBundledRuntime) {
      const extra = [runtimeLibsDir];

      if (existsSync(mpvRuntimeLibsDir)) {
        extra.push(mpvRuntimeLibsDir);
      }

      prependEnvPath("LD_LIBRARY_PATH", extra);
    }
  } else if (isWindows) {
    const extra = [runtimeLibsDir];

    if (existsSync(mpvRuntimeLibsDir)) {
      extra.push(mpvRuntimeLibsDir);
    }

    prependEnvPath("PATH", extra);
  }
}

if (isLinuxNixDev) {
  const forbiddenRuntimePathEntries = splitEnvPath(
    childEnv.LD_LIBRARY_PATH,
  ).filter((entry) => {
    const absoluteEntry = normalizePathEntry(entry);

    return (
      isInside(runtimeLibsDir, absoluteEntry) ||
      isInside(mpvRuntimeLibsDir, absoluteEntry) ||
      isInside(targetDebugDir, absoluteEntry) ||
      isInside(targetReleaseDir, absoluteEntry)
    );
  });

  if (forbiddenRuntimePathEntries.length > 0) {
    console.error("[ERROR] Linux Nix dev runtime isolation failed.");
    console.error("[ERROR] Forbidden LD_LIBRARY_PATH entries:");
    for (const entry of forbiddenRuntimePathEntries) {
      console.error(`[ERROR]   ${entry}`);
    }
    process.exit(1);
  }
}

const tauriCmd = "cargo-tauri";

const child = isWindows
  ? spawn("cmd.exe", ["/d", "/s", "/c", tauriCmd, ...tauriArgs], {
      cwd: projectRoot,
      env: childEnv,
      stdio: "inherit",
    })
  : spawn(tauriCmd, tauriArgs, {
      cwd: projectRoot,
      env: childEnv,
      stdio: "inherit",
    });

child.on("error", (error) => {
  console.error(`[ERROR] Failed to run ${tauriCmd}: ${error.message}`);
  process.exit(1);
});

child.on("exit", (code, signal) => {
  if (signal) {
    process.kill(process.pid, signal);
    return;
  }

  process.exit(code ?? 1);
});
