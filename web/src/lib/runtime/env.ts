/**
 * Runtime environment detection and API base URL resolution.
 *
 * This module provides runtime-aware API routing so the same frontend build
 * works in both browser mode and Tauri desktop mode.
 */

export type RuntimeEnv = 'browser' | 'tauri'

/**
 * Detect the current runtime environment.
 * Returns 'tauri' if running inside Tauri, 'browser' otherwise.
 */
export function detectRuntimeEnv(): RuntimeEnv {
  const hasTauri =
    typeof window !== 'undefined' &&
    '__TAURI_INTERNALS__' in window
  return hasTauri ? 'tauri' : 'browser'
}

/**
 * Resolve the API base URL based on the current runtime environment.
 *
 * - Tauri mode: http://127.0.0.1:27403/api/v1 (sidecar)
 * - Browser mode: /api/v1 (same origin)
 */
export function resolveApiBaseUrl(): string {
  const env = detectRuntimeEnv()
  if (env === 'tauri') {
    return 'http://127.0.0.1:27403/api/v1'
  }
  return '/api/v1'
}

/**
 * Check if running in Tauri desktop mode.
 */
export function isTauriMode(): boolean {
  return detectRuntimeEnv() === 'tauri'
}

/**
 * Check if running in browser mode.
 */
export function isBrowserMode(): boolean {
  return detectRuntimeEnv() === 'browser'
}
