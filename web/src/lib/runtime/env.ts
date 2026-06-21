/**
 * Runtime environment detection and API base URL resolution.
 *
 * In desktop mode (Wails), the Go backend serves /api/v1/* directly via
 * in-process request interception — no TCP port needed.
 * In browser mode, requests go to the same origin.
 */

export type RuntimeEnv = 'desktop' | 'browser'

/**
 * Detect the current runtime environment.
 */
export function detectRuntimeEnv(): RuntimeEnv {
  const isDesktop =
    typeof window !== 'undefined' &&
    ('__TAURI_INTERNALS__' in window || '__WAILS__' in window)
  return isDesktop ? 'desktop' : 'browser'
}

/**
 * Resolve the API base URL.
 *
 * Desktop mode: /api/v1 (same origin, intercepted by Wails asset server)
 * Browser mode: /api/v1 (same origin)
 */
export function resolveApiBaseUrl(): string {
  return '/api/v1'
}

/**
 * Check if running in desktop mode.
 */
export function isDesktopMode(): boolean {
  return detectRuntimeEnv() === 'desktop'
}

/**
 * Check if running in browser mode.
 */
export function isBrowserMode(): boolean {
  return detectRuntimeEnv() === 'browser'
}
