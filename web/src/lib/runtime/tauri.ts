/**
 * Tauri runtime integration for sidecar communication.
 *
 * This module handles communication between the Tauri frontend and the Go sidecar.
 */

import { listen } from '@tauri-apps/api/event';
import { tauri } from '@tauri-apps/api';
import type { Event } from '@tauri-apps/api/event';
import { resolveApiBaseUrl as resolveStaticApiBaseUrl } from './env';

// Store dynamic API URL
let dynamicApiBaseUrl: string | null = null;
// Store unlisten functions
// Tauri's listen() returns UnlistenFn which is () => void
let unlistenReady: (() => void) | null = null;
let unlistenError: (() => void) | null = null;
// Track if already initialized
let initialized = false;

/**
 * Check if we are running in a Tauri environment.
 */
export function isTauriEnvironment(): boolean {
  return '__TAURI_INTERNALS__' in window;
}

/**
 * Get the sidecar status from Tauri backend using invoke.
 */
async function getSidecarStatus(): Promise<{ status: string; api_base_url?: string } | null> {
  try {
    const result = (await tauri.invoke('get_sidecar_status')) as {
      status: string;
      api_base_url?: string;
    };
    return result;
  } catch (error) {
    console.error('[Tauri] Failed to get sidecar status:', error);
    return null;
  }
}

/**
 * Initialize Tauri event listeners for sidecar communication.
 * This function is safe to call - it won't throw errors that break the app.
 * It uses both event listeners and direct invoke to get the sidecar status.
 */
export async function initTauriListeners(): Promise<void> {
  // Only initialize in Tauri environment
  if (!isTauriEnvironment()) {
    console.log('[Tauri] Not in Tauri environment, skipping event listeners');
    return;
  }

  // Prevent duplicate initialization
  if (initialized) {
    console.log('[Tauri] Event listeners already initialized');
    return;
  }
  initialized = true;

  try {
    console.log('[Tauri] Initializing event listeners...');

    // First, try to get the current sidecar status via invoke
    // This works even if the sidecar is already ready
    const status = await getSidecarStatus();
    if (status?.status === 'ready' && status.api_base_url) {
      dynamicApiBaseUrl = status.api_base_url;
      console.log('[Tauri] Sidecar already ready at:', dynamicApiBaseUrl);
    } else if (status?.status === 'starting') {
      console.log('[Tauri] Sidecar is starting, waiting for ready event...');
    } else if (status?.status === 'error') {
      console.error('[Tauri] Sidecar is in error state:', status);
    }

    // Listen for sidecar ready event
    // Event name: fyom-sidecar-ready (defined in src-tauri/src/lib.rs)
    unlistenReady = await listen('fyom-sidecar-ready', (event: Event<string>) => {
      const apiUrl = event.payload;
      dynamicApiBaseUrl = `${apiUrl}/api/v1`;
      console.log('[Tauri] Sidecar ready at:', dynamicApiBaseUrl);
    });

    // Listen for sidecar errors
    unlistenError = await listen('fyom-sidecar-error', (event: Event<string>) => {
      console.error('[Tauri] Sidecar error:', event.payload);
      dynamicApiBaseUrl = null;
    });

    console.log('[Tauri] Event listeners initialized successfully');
  } catch (error) {
    console.error('[Tauri] Failed to setup listeners:', error);
    // Don't re-throw - this shouldn't break the app
    // The app can still work with the fallback static URL
  }
}

/**
 * Cleanup Tauri event listeners.
 */
export function cleanupTauriListeners(): void {
  if (unlistenReady) {
    try {
      unlistenReady();
    } catch (error) {
      console.error('[Tauri] Error cleaning up ready listener:', error);
    }
    unlistenReady = null;
  }
  if (unlistenError) {
    try {
      unlistenError();
    } catch (error) {
      console.error('[Tauri] Error cleaning up error listener:', error);
    }
    unlistenError = null;
  }
  initialized = false;
}

/**
 * Get the current API base URL, preferring dynamic URL from sidecar.
 * Falls back to static resolution if no dynamic URL is available.
 */
export function getApiBaseUrl(): string {
  if (dynamicApiBaseUrl) {
    return dynamicApiBaseUrl;
  }
  return resolveStaticApiBaseUrl();
}

/**
 * Check if we have a dynamic API URL from the sidecar.
 */
export function hasDynamicApiUrl(): boolean {
  return dynamicApiBaseUrl !== null;
}

/**
 * Reset the dynamic API URL (useful for testing or reconnection).
 */
export function resetApiBaseUrl(): void {
  dynamicApiBaseUrl = null;
}
