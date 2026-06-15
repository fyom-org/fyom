/**
 * Native player availability, failure tracking, and bridge boundary.
 *
 * This module is the single source of truth for native-player UI readiness
 * and fallback decisions. It encapsulates:
 * - state lifecycle (idle → initializing → ready / failed / unavailable)
 * - runtime detection (delegated to the repo's canonical detection utility)
 * - bridge invoke try/catch with typed failure mapping
 *
 * PlayerView.vue consumes this module's state and helpers; it does not
 * contain low-level bridge invoke logic itself.
 */

import { isTauriEnvironment } from '@/lib/runtime/tauri';

/* ── State model ──────────────────────────────────────────────────────── */

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
  | 'bridge'
  | 'unknown';

export interface NativePlayerFailure {
  stage: NativePlayerFailureStage;
  message: string;
}

export interface NativePlayerState {
  status: NativePlayerStatus;
  failure: NativePlayerFailure | null;
  attempted: boolean;
}

/* ── Init params / result ──────────────────────────────────────────────── */

export interface NativePlayerInitParams {
  mediaUrl: string;
  posterUrl?: string;
}

export type NativePlayerInitResult =
  | { ok: true }
  | { ok: false; failure: NativePlayerFailure };

/* ── Factory ───────────────────────────────────────────────────────────── */

export function createInitialNativePlayerState(): NativePlayerState {
  return {
    status: 'idle',
    failure: null,
    attempted: false,
  };
}

/* ── Runtime detection ─────────────────────────────────────────────────── */

/**
 * Check if native playback runtime is available.
 * Delegates to the repo's canonical Tauri environment detection.
 *
 * Semantics: "native playback may legitimately be attempted here."
 * Returns false in normal browser runtime — no native attempt should happen.
 */
export function isNativePlaybackRuntimeAvailable(): boolean {
  return isTauriEnvironment();
}

/* ── Error mapping ─────────────────────────────────────────────────────── */

/**
 * Map a raw error to a typed NativePlayerFailure.
 */
export function mapNativePlayerInitError(err: unknown): NativePlayerFailure {
  const message = err instanceof Error ? err.message : String(err);

  let stage: NativePlayerFailureStage = 'unknown';
  const msgLower = message.toLowerCase();

  if (
    msgLower.includes('rawwindowhandle') ||
    msgLower.includes('raw-window-handle') ||
    msgLower.includes('window handle')
  ) {
    stage = 'raw-window-handle';
  } else if (
    msgLower.includes('wid') ||
    msgLower.includes('window id') ||
    msgLower.includes('injection')
  ) {
    stage = 'wid-injection';
  } else if (
    msgLower.includes('mpv_context') ||
    msgLower.includes('mpv context') ||
    msgLower.includes('context creation') ||
    msgLower.includes('mpv init')
  ) {
    stage = 'mpv-context';
  } else if (
    msgLower.includes('library') ||
    msgLower.includes('dylib') ||
    msgLower.includes('.so') ||
    msgLower.includes('.dll')
  ) {
    stage = 'library-load';
  } else if (
    msgLower.includes('invoke') ||
    msgLower.includes('bridge') ||
    msgLower.includes('command') ||
    msgLower.includes('tauri')
  ) {
    stage = 'bridge';
  }

  return { stage, message };
}

/* ── Bridge function ───────────────────────────────────────────────────── */

/**
 * Attempt to initialize the native player via Tauri invoke.
 *
 * This is the single bridge boundary between frontend and native player.
 * All invoke/catch logic is encapsulated here; PlayerView only sees
 * the typed result.
 *
 * The invoke command name (`play_media`) is a placeholder for the future
 * libmpv backend integration point. When the Tauri backend implements
 * this command, the frontend bridge will work without changes.
 */
export async function tryInitializeNativePlayer(
  params: NativePlayerInitParams,
): Promise<NativePlayerInitResult> {
  if (!isNativePlaybackRuntimeAvailable()) {
    return {
      ok: false,
      failure: {
        stage: 'bridge',
        message: 'Native playback runtime is not available',
      },
    };
  }

  try {
    // @ts-expect-error — __TAURI_INTERNALS__ exists in Tauri runtime
    const { invoke } = window.__TAURI_INTERNALS__.tauri;

    // TODO(phase2): Replace 'play_media' with the actual Tauri command name
    // once the libmpv backend implements it. The command should accept
    // { mediaUrl: string, posterUrl?: string } and return { success: boolean, error?: string }.
    const result = await invoke('play_media', {
      mediaUrl: params.mediaUrl,
      posterUrl: params.posterUrl ?? '',
    });

    if (result && result.success) {
      return { ok: true };
    }

    return {
      ok: false,
      failure: mapNativePlayerInitError(
        new Error(result?.error || 'Unknown native player error'),
      ),
    };
  } catch (err) {
    return {
      ok: false,
      failure: mapNativePlayerInitError(err),
    };
  }
}
