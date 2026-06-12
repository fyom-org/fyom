#!/usr/bin/env node

/**
 * Script to generate Brotli compressed files for static assets
 * This is used as a workaround for vite-plugin-compression brotli issues
 */

import fs from 'node:fs';
import path from 'node:path';
import zlib from 'node:zlib';
import { promisify } from 'node:util';

const brotliCompress = promisify(zlib.brotliCompress);

const DIST_DIR = path.join(process.cwd(), 'dist');
const ASSETS_DIR = path.join(DIST_DIR, 'assets');

// Files to compress (exclude already compressed and source maps)
const EXTENSIONS_TO_COMPRESS = [
  '.js', '.css', '.html', '.json',
  '.png', '.jpg', '.jpeg', '.gif', '.svg',
  '.woff', '.woff2', '.ttf', '.eot'
];

async function compressFile(filePath) {
  const content = await fs.promises.readFile(filePath);
  const compressed = await brotliCompress(content, {
    params: {
      [zlib.constants.BROTLI_PARAM_QUALITY]: 11, // Maximum compression
    },
  });

  const brotliPath = filePath + '.br';
  await fs.promises.writeFile(brotliPath, compressed);

  const originalSize = content.length;
  const compressedSize = compressed.length;
  const ratio = (compressedSize / originalSize * 100).toFixed(2);

  console.log(`Compressed: ${path.relative(DIST_DIR, filePath)} ` +
    `(${formatSize(originalSize)} -> ${formatSize(compressedSize)} - ${ratio}%)`);

  return { originalSize, compressedSize };
}

function formatSize(bytes) {
  if (bytes < 1024) return bytes + ' B';
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(2) + ' KB';
  return (bytes / (1024 * 1024)).toFixed(2) + ' MB';
}

async function findFilesToCompress(dir) {
  const files = [];

  const items = await fs.promises.readdir(dir, { withFileTypes: true });

  for (const item of items) {
    const fullPath = path.join(dir, item.name);

    if (item.isDirectory()) {
      const subFiles = await findFilesToCompress(fullPath);
      files.push(...subFiles);
    } else if (item.isFile()) {
      // Skip already compressed files and source maps
      if (item.name.endsWith('.br') || item.name.endsWith('.gz') || item.name.endsWith('.map')) {
        continue;
      }

      const ext = path.extname(item.name).toLowerCase();
      if (EXTENSIONS_TO_COMPRESS.includes(ext)) {
        files.push(fullPath);
      }
    }
  }

  return files;
}

async function main() {
  console.log('Generating Brotli compressed files...');
  console.log('='.repeat(50));

  try {
    const files = await findFilesToCompress(DIST_DIR);

    if (files.length === 0) {
      console.log('No files to compress');
      return;
    }

    console.log(`Found ${files.length} files to compress`);
    console.log('');

    let totalOriginal = 0;
    let totalCompressed = 0;

    for (const file of files) {
      const result = await compressFile(file);
      totalOriginal += result.originalSize;
      totalCompressed += result.compressedSize;
    }

    console.log('='.repeat(50));
    console.log(`Total: ${formatSize(totalOriginal)} -> ${formatSize(totalCompressed)} ` +
      `(${((totalCompressed / totalOriginal * 100).toFixed(2))}%)`);
    console.log('Brotli compression complete!');

  } catch (error) {
    console.error('Error during compression:', error);
    process.exit(1);
  }
}

main();
