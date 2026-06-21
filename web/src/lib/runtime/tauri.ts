/**
 * Desktop runtime integration for Wails v2 communication.
 *
 * This module handles communication between the Wails frontend and the Go backend.
 * In Wails desktop mode, /api/v1/* requests are handled in-process by Go.
 */

import { getApiBaseUrl as resolveStaticApiBaseUrl } from './env';

// Track initialization state using a Promise to handle concurrent calls safely.
let initPromise: Promise<void> | null = null;

/**
 * Check if we are running in a Wails desktop environment.
 */
export function isDesktopEnvironment(): boolean {
  return typeof window !== 'undefined' && '__WAILS__' in window;
}

/**
 * Initialize Wails desktop event listeners.
 * This is a no-op in browser mode.
 */
export async function initDesktopListeners(): Promise<void> {
  if (!isDesktopEnvironment()) {
    return;
  }

  if (initPromise) {
    return initPromise;
  }

  initPromise = (async () => {
    console.log('[Desktop] Running in Wails desktop mode; API served in-process.');
  })();

  return initPromise;
}

/**
 * Get the current API base URL.
 * In desktop mode, this is always /api/v1 (same origin).
 */
export function getApiBaseUrl(): string {
  return resolveStaticApiBaseUrl();
}

/**
 * Check if we have a dynamic API URL (always false for Wails).
 */
export function hasDynamicApiUrl(): boolean {
  return false;
}

/**
 * Reset the dynamic API URL (no-op for Wails).
 */
export function resetApiBaseUrl(): void {
  // No-op: Wails desktop mode uses same-origin /api/v1/.
}

// Legacy alias for backward compatibility.
export const isTauriEnvironment = isDesktopEnvironment;
export const initTauriListeners = initDesktopListeners;
