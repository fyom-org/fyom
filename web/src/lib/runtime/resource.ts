/**
 * Runtime-aware resource URL normalization.
 *
 * In browser mode, relative /api/v1/... paths are kept as-is (same origin).
 * In Tauri desktop mode, they are converted to absolute sidecar URLs.
 */

import { isTauriMode } from './env';

const TAURI_SIDECAR_BASE = 'http://127.0.0.1:27403';

/**
 * Normalize a resource URL for the current runtime environment.
 *
 * - empty input → ''
 * - already absolute (http:// or https://) → unchanged
 * - browser mode → relative paths kept as-is
 * - Tauri mode → /api/v1/... paths become absolute sidecar URLs
 */
export function resolveResourceUrl(raw?: string): string {
  if (!raw) return '';

  // Already absolute.
  if (raw.startsWith('http://') || raw.startsWith('https://')) {
    return raw;
  }

  // Browser mode: keep relative paths as-is.
  if (!isTauriMode()) {
    return raw;
  }

  // Tauri mode: convert /api/v1/... to absolute sidecar URL.
  if (raw.startsWith('/api/v1/')) {
    return `${TAURI_SIDECAR_BASE}${raw}`;
  }

  // Other root-relative paths: normalize against sidecar origin.
  if (raw.startsWith('/')) {
    return `${TAURI_SIDECAR_BASE}${raw}`;
  }

  return raw;
}
