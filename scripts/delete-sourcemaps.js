#!/usr/bin/env node

const fs = require("fs");
const path = require("path");

const targetDir = process.argv[2] || "dist";

let deleted = 0;

function walk(dir) {
  if (!fs.existsSync(dir)) {
    return;
  }

  const entries = fs.readdirSync(dir, { withFileTypes: true });

  for (const entry of entries) {
    const fullPath = path.join(dir, entry.name);

    if (entry.isDirectory()) {
      walk(fullPath);
      continue;
    }

    if (entry.isFile() && fullPath.endsWith(".map")) {
      fs.unlinkSync(fullPath);
      deleted += 1;
      console.log(`Deleted sourcemap: ${fullPath}`);
    }
  }
}

walk(targetDir);

console.log(`Deleted ${deleted} sourcemap file(s) from ${targetDir}`);
