#!/usr/bin/env node
// PORTED_FROM_SOIA @ 9e22064 (src-tauri/scripts/sync_version.mjs)
//
// Adapted port from soia (GPL-3.0-only). fyom differs from soia in that the
// authoritative `package.json` lives under `web/` (fyom has no root
// package.json — the Go module is the root project). The version is sourced
// from `web/package.json` and propagated to:
//   - src-tauri/Cargo.toml ([package].version)
//   - src-tauri/tauri.conf.json (.version)
//
// Usage:
//   node scripts/sync_version.mjs           # write
//   node scripts/sync_version.mjs --check   # check only (exit 1 on drift)
//
// This file is licensed under GPL-3.0-only, inherited from soia.
import fs from "node:fs";
import path from "node:path";
import process from "node:process";

const root = process.cwd();
const isCheckMode = process.argv.includes("--check");

const packageJsonPath = path.join(root, "web", "package.json");
const cargoTomlPath = path.join(root, "src-tauri", "Cargo.toml");
const tauriConfPath = path.join(root, "src-tauri", "tauri.conf.json");

function readUtf8(filePath) {
  return fs.readFileSync(filePath, "utf8");
}

function writeUtf8(filePath, content) {
  fs.writeFileSync(filePath, content, "utf8");
}

function extractCargoPackageVersion(cargoToml) {
  const match = cargoToml.match(/(\[package\][\s\S]*?\nversion\s*=\s*")([^"]+)(")/m);
  if (!match) {
    throw new Error("Could not find [package].version in src-tauri/Cargo.toml");
  }
  return match[2];
}

function replaceCargoPackageVersion(cargoToml, version) {
  const re = /(\[package\][\s\S]*?\nversion\s*=\s*")([^"]+)(")/m;
  if (!re.test(cargoToml)) {
    throw new Error("Could not replace [package].version in src-tauri/Cargo.toml");
  }
  return cargoToml.replace(re, `$1${version}$3`);
}

function main() {
  const packageJson = JSON.parse(readUtf8(packageJsonPath));
  const sourceVersion = packageJson.version;
  if (!sourceVersion || typeof sourceVersion !== "string") {
    throw new Error("web/package.json version is missing or invalid");
  }

  const tauriConfRaw = readUtf8(tauriConfPath);
  const tauriConf = JSON.parse(tauriConfRaw);
  const cargoTomlRaw = readUtf8(cargoTomlPath);

  const mismatches = [];
  const writes = [];

  if (tauriConf.version !== sourceVersion) {
    mismatches.push(
      `src-tauri/tauri.conf.json: ${tauriConf.version ?? "<missing>"} -> ${sourceVersion}`
    );
    if (!isCheckMode) {
      tauriConf.version = sourceVersion;
      writes.push(() => writeUtf8(tauriConfPath, `${JSON.stringify(tauriConf, null, 2)}\n`));
    }
  }

  const cargoVersion = extractCargoPackageVersion(cargoTomlRaw);
  if (cargoVersion !== sourceVersion) {
    mismatches.push(`src-tauri/Cargo.toml: ${cargoVersion} -> ${sourceVersion}`);
    if (!isCheckMode) {
      const nextCargo = replaceCargoPackageVersion(cargoTomlRaw, sourceVersion);
      writes.push(() => writeUtf8(cargoTomlPath, nextCargo));
    }
  }

  if (isCheckMode) {
    if (mismatches.length > 0) {
      console.error("Version mismatch detected (source: web/package.json):");
      for (const line of mismatches) {
        console.error(`- ${line}`);
      }
      process.exit(1);
    }
    console.log("Version check passed.");
    return;
  }

  if (writes.length === 0) {
    console.log("Versions already in sync.");
    return;
  }

  for (const write of writes) {
    write();
  }
  console.log(`Version synced to ${sourceVersion}.`);
}

main();
