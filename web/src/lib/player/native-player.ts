/**
 * Native player availability and failure tracking.
 *
 * This module tracks the initialization state of the native (libmpv) player
 * and provides typed failure information for UI fallback decisions.
 */

export type NativePlayerStatus =
  | 'idle'
  | 'initializing'
  | 'ready'
  | 'failed'
  | 'unavailable';

export type NativePlayerFailureStage =
  | 'raw-window-handle'
  | 'wid-injection'
  | 'mpv-context'
  | 'library-load'
  | 'unknown';

export interface NativePlayerFailure {
  stage: NativePlayerFailureStage;
  message: string;
}

export interface NativePlayerState {
  status: NativePlayerStatus;
  failure: NativePlayerFailure | null;
}

/**
 * Create initial native player state.
 */
export function createInitialNativePlayerState(): NativePlayerState {
  return {
    status: 'idle',
    failure: null,
  };
}

/**
 * Map a raw error to a typed NativePlayerFailure.
 */
export function mapNativePlayerInitError(err: unknown): NativePlayerFailure {
  const message = err instanceof Error ? err.message : String(err);

  let stage: NativePlayerFailureStage = 'unknown';
  const msgLower = message.toLowerCase();

  if (msgLower.includes('rawwindowhandle') || msgLower.includes('raw-window-handle') || msgLower.includes('window handle')) {
    stage = 'raw-window-handle';
  } else if (msgLower.includes('wid') || msgLower.includes('window id') || msgLower.includes('injection')) {
    stage = 'wid-injection';
  } else if (msgLower.includes('mpv_context') || msgLower.includes('mpv context') || msgLower.includes('context creation')) {
    stage = 'mpv-context';
  } else if (msgLower.includes('library') || msgLower.includes('dylib') || msgLower.includes('so') || msgLower.includes('dll')) {
    stage = 'library-load';
  }

  return { stage, message };
}

/**
 * Check if native player is available in the current runtime.
 * In browser/non-Tauri mode, native player is unavailable by default.
 */
export function isNativePlayerAvailable(): boolean {
  // Native player is only available in Tauri desktop environment
  return '__TAURI_INTERNALS__' in window;
}
