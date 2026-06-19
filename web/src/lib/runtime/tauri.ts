/**
 * Tauri runtime integration for sidecar communication.
 *
 * This module handles communication between the Tauri frontend and the Go sidecar.
 * Updated for Tauri v2 API compatibility.
 */

import { invoke } from '@tauri-apps/api/core';
import { listen, type UnlistenFn } from '@tauri-apps/api/event';
import type { Event } from '@tauri-apps/api/event';
import { resolveApiBaseUrl as resolveStaticApiBaseUrl } from './env';

interface SidecarStatus {
  status: 'starting' | 'ready' | 'error' | string;
  api_base_url?: string;
  error?: string;
}

// Store dynamic API URL
let dynamicApiBaseUrl: string | null = null;

// Store unlisten functions provided by Tauri
let unlistenReady: UnlistenFn | null = null;
let unlistenError: UnlistenFn | null = null;

// Track initialization state using a Promise to handle concurrent calls safely
let initPromise: Promise<void> | null = null;

/**
 * Check if we are running in a Tauri environment.
 * Tauri v2 injects '__TAURI_INTERNALS__' into the window object.
 */
export function isTauriEnvironment(): boolean {
  return typeof window !== 'undefined' && '__TAURI_INTERNALS__' in window;
}

/**
 * Get the sidecar status from Tauri backend.
 * Uses the official Tauri v2 invoke API.
 */
async function getSidecarStatus(): Promise<SidecarStatus | null> {
  if (!isTauriEnvironment()) return null;

  try {
    const result = await invoke<SidecarStatus>('get_sidecar_status');
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
 * Ensures initialization only happens once even if called concurrently.
 */
export async function initTauriListeners(): Promise<void> {
  // Only initialize in Tauri environment
  if (!isTauriEnvironment()) {
    console.log('[Tauri] Not in Tauri environment, skipping event listeners');
    return;
  }

  // Prevent duplicate initialization using a Promise lock
  if (initPromise) {
    return initPromise;
  }

  initPromise = (async () => {
    try {
      console.log('[Tauri] Initializing event listeners...');

      // 1. Try to get the current sidecar status immediately via invoke
      const status = await getSidecarStatus();
      if (status?.status === 'ready' && status.api_base_url) {
        // Ensure we don't double-append /api/v1 if backend already includes it
        dynamicApiBaseUrl = status.api_base_url.endsWith('/api/v1')
          ? status.api_base_url
          : `${status.api_base_url}/api/v1`;
        console.log('[Tauri] Sidecar already ready at:', dynamicApiBaseUrl);
      } else if (status?.status === 'starting') {
        console.log('[Tauri] Sidecar is starting, waiting for ready event...');
      } else if (status?.status === 'error') {
        console.error('[Tauri] Sidecar is in error state:', status.error || 'Unknown error');
      }

      // 2. Listen for sidecar ready event
      // Event name: fyom-sidecar-ready (defined in src-tauri/src/lib.rs)
      unlistenReady = await listen<string>('fyom-sidecar-ready', (event: Event<string>) => {
        const apiUrl = event.payload;
        dynamicApiBaseUrl = apiUrl.endsWith('/api/v1') ? apiUrl : `${apiUrl}/api/v1`;
        console.log('[Tauri] Sidecar ready at:', dynamicApiBaseUrl);
      });

      // 3. Listen for sidecar errors
      unlistenError = await listen<string>('fyom-sidecar-error', (event: Event<string>) => {
        console.error('[Tauri] Sidecar error:', event.payload);
        dynamicApiBaseUrl = null;
      });

      console.log('[Tauri] Event listeners initialized successfully');
    } catch (error) {
      console.error('[Tauri] Failed to setup listeners:', error);
      // Reset initPromise so we can try again later if needed
      initPromise = null;
      // Don't re-throw - this shouldn't break the app
      // The app can still work with the fallback static URL
    }
  })();

  return initPromise;
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
  // Reset the promise so listeners can be re-initialized if needed
  initPromise = null;
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
