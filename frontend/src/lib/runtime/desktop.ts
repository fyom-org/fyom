/**
 * Desktop/browser runtime integration.
 *
 * This module intentionally avoids Tauri naming.
 *
 * Runtime policy:
 * - Browser mode: normal web app behavior.
 * - Desktop mode: Wails desktop shell behavior.
 *
 * API strategy:
 * - Browser/dev: resolved from static environment config.
 * - Desktop: normally same-origin /api/v1, resolved by env.ts.
 */

import { getApiBaseUrl as resolveStaticApiBaseUrl } from './env';

type DesktopRuntimeWindow = Window & {
  /**
   * Legacy Wails global used by some Wails versions / runtime bundles.
   */
  __WAILS__?: unknown;

  /**
   * Wails v3 prebuilt runtime bundle may expose APIs on window.wails.
   * Projects using @wailsio/runtime as an npm package may not expose this
   * global, so this check is best-effort only.
   */
  wails?: unknown;
};

let initPromise: Promise<void> | null = null;

function readDesktopRuntimeEnv(): boolean | undefined {
  const value =
    import.meta.env.VITE_FYOM_DESKTOP ?? import.meta.env.VITE_DESKTOP ?? import.meta.env.VITE_WAILS;

  if (value === undefined) {
    return undefined;
  }

  const normalized = String(value).trim().toLowerCase();

  if (['1', 'true', 'yes', 'desktop', 'wails'].includes(normalized)) {
    return true;
  }

  if (['0', 'false', 'no', 'browser', 'web'].includes(normalized)) {
    return false;
  }

  return undefined;
}

/**
 * Check whether the frontend is running inside a desktop shell.
 *
 * Prefer an explicit Vite env flag when possible:
 *
 *   VITE_FYOM_DESKTOP=true
 *
 * Runtime global checks are intentionally best-effort because Wails v3
 * projects using @wailsio/runtime may not always expose a stable global.
 */
export function isDesktopEnvironment(): boolean {
  if (typeof window === 'undefined') {
    return false;
  }

  const envValue = readDesktopRuntimeEnv();

  if (envValue !== undefined) {
    return envValue;
  }

  const runtimeWindow = window as DesktopRuntimeWindow;

  if (runtimeWindow.__WAILS__ !== undefined) {
    return true;
  }

  if (runtimeWindow.wails !== undefined) {
    return true;
  }

  const userAgent = window.navigator?.userAgent ?? '';

  return /\bwails\b/i.test(userAgent);
}

/**
 * Initialize desktop runtime listeners.
 *
 * Currently this is intentionally lightweight. Keep this function as the stable
 * app-level lifecycle hook so callers do not need to know which desktop shell
 * is used underneath.
 */
export async function initDesktopListeners(): Promise<void> {
  if (!isDesktopEnvironment()) {
    return;
  }

  if (initPromise) {
    return initPromise;
  }

  initPromise = Promise.resolve().then(() => {
    console.info('[Desktop] Desktop runtime detected.');
  });

  return initPromise;
}

/**
 * Return the API base URL used by the frontend.
 *
 * In Wails desktop builds, this should normally resolve to same-origin /api/v1.
 * In browser/dev mode, the value may come from Vite env config.
 */
export function getApiBaseUrl(): string {
  return resolveStaticApiBaseUrl();
}

/**
 * The API URL is static for the current app session.
 */
export function hasDynamicApiUrl(): boolean {
  return false;
}

/**
 * No-op because this runtime does not maintain dynamic API state.
 */
export function resetApiBaseUrl(): void {
  // Intentionally empty.
}
