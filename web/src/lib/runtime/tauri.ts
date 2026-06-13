/**
 * Tauri runtime integration for sidecar communication.
 *
 * This module handles communication between the Tauri frontend and the Go sidecar.
 */

import { listen } from '@tauri-apps/api/event'
import { resolveApiBaseUrl as resolveStaticApiBaseUrl } from './env'

// 存储动态 API URL
let dynamicApiBaseUrl: string | null = null

/**
 * Initialize Tauri event listeners for sidecar communication.
 * Must be called during app initialization.
 */
export async function initTauriListeners(): Promise<void> {
  // Only initialize in Tauri environment
  if (import.meta.env.MODE !== 'tauri' && !('__TAURI_INTERNALS__' in window)) {
    return
  }

  try {
    // Listen for sidecar ready event
    // Event name: fyom-sidecar-ready (defined in src-tauri/src/lib.rs)
    await listen('fyom-sidecar-ready', (event) => {
      const apiUrl = event.payload as string
      dynamicApiBaseUrl = `${apiUrl}/api/v1`
      console.log('[Tauri] Sidecar ready at:', dynamicApiBaseUrl)
    })

    // Listen for sidecar errors
    await listen('fyom-sidecar-error', (event) => {
      console.error('[Tauri] Sidecar error:', event.payload)
      dynamicApiBaseUrl = null
    })

    console.log('[Tauri] Event listeners initialized')
  } catch (error) {
    console.error('[Tauri] Failed to setup listeners:', error)
  }
}

/**
 * Get the current API base URL, preferring dynamic URL from sidecar.
 * Falls back to static resolution if no dynamic URL is available.
 */
export function getApiBaseUrl(): string {
  if (dynamicApiBaseUrl) {
    return dynamicApiBaseUrl
  }
  return resolveStaticApiBaseUrl()
}

/**
 * Check if we have a dynamic API URL from the sidecar.
 */
export function hasDynamicApiUrl(): boolean {
  return dynamicApiBaseUrl !== null
}

/**
 * Reset the dynamic API URL (useful for testing or reconnection).
 */
export function resetApiBaseUrl(): void {
  dynamicApiBaseUrl = null
}
