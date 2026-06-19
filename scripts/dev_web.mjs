#!/usr/bin/env node

import { spawn } from "node:child_process";
import { existsSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const scriptDir = dirname(fileURLToPath(import.meta.url));
const rootDir = resolve(scriptDir, "..");
const webDir = resolve(rootDir, "web");

if (!existsSync(resolve(webDir, "package.json"))) {
  console.error(
    `[dev-web] web package.json not found: ${resolve(webDir, "package.json")}`,
  );
  process.exit(1);
}

const npmCommand = process.platform === "win32" ? "npm.cmd" : "npm";

const child = spawn(
  npmCommand,
  ["run", "dev", "--", "--host", "127.0.0.1", "--port", "5173", "--strictPort"],
  {
    cwd: webDir,
    stdio: "inherit",
    env: process.env,
  },
);

child.on("exit", (code, signal) => {
  if (signal) {
    console.error(`[dev-web] npm dev server terminated by signal: ${signal}`);
    process.exit(1);
  }

  process.exit(code ?? 1);
});

child.on("error", (error) => {
  console.error(`[dev-web] failed to start npm dev server: ${error.message}`);
  process.exit(1);
});
