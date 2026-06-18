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

import { listen } from '@tauri-apps/api/event';

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

/* ── Phase 2.2: `fyom://mpv/*` event subscription (additive — no 9.7 change) ──── */

/**
 * The mpv event channel payload shapes (emitted by `src-tauri/src/mpv/event_loop.rs`).
 *
 * These are additive to the Phase 9.7 guardrail: the `invoke('play_media')` /
 * `invoke('stop_media')` contract is unchanged; these events let the frontend observe
 * playback state changes driven by libmpv (position, pause, volume, tracks, …).
 *
 * PORTED_FROM_SOIA `useAppPlaybackEvents.ts` event shapes (renamed `soia://` →
 * `fyom://mpv/`); the orchestration logic (history/nowPlaying) is deferred to Phase 2.5.
 */

export interface MpvTrack {
  id: number;
  title: string;
  lang: string;
  type: string;
  /** Whether this track is currently selected (Phase 2.4). */
  selected?: boolean;
  /** Whether this is an external track (loaded via `sub-add` / `audio-add`) (Phase 2.4). */
  external?: boolean;
  /** Source id (the track's originating file index; 0 for the main file) (Phase 2.4). */
  src_id?: number;
}

export interface MpvChapter {
  title: string;
  time: number;
}

export interface MpvTimePosEvent {
  position: number;
  duration: number;
}
export interface MpvPauseEvent {
  paused: boolean;
}
export interface MpvEndFileEvent {
  /** mpv EndFileReason code: 0=eof, 1=stop, 2=quit, 3=error, 4=redirect. */
  reason: number;
  reason_name: string;
}
export interface MpvFileLoadedEvent {
  /** The path/URL mpv loaded (correlate with the pending play request). */
  path: string | null;
}
export interface MpvTrackListEvent {
  audio_tracks: MpvTrack[];
  sub_tracks: MpvTrack[];
}
export interface MpvVolumeEvent {
  volume: number;
}
export interface MpvDurationEvent {
  duration: number;
}
export interface MpvSpeedEvent {
  speed: number;
}
export interface MpvCacheSpeedEvent {
  speed: number;
}
export interface MpvDemuxerCacheTimeEvent {
  time: number;
}
export interface MpvPausedForCacheEvent {
  paused: boolean;
}
export interface MpvChapterListEvent {
  chapters: MpvChapter[];
}
export interface MpvErrorEvent {
  message: string;
}

// Phase 2.4 additions — typed payloads for the new `fyom://mpv/*` events emitted by
// the extended `event_loop.rs` observer set (hwdec-current, aid, sid, A/V delays, color
// adjustments, chapter, eof-reached).
export interface MpvHwdecCurrentEvent {
  /** Active hwdec backend (e.g. "auto-safe", "vt", "vaapi", "d3d11va", "nv"). */
  hwdec: string;
}
export interface MpvAidEvent {
  /** Current audio track id (0 = none / disabled). */
  id: number;
}
export interface MpvSidEvent {
  /** Current subtitle track id (0 = none / disabled). */
  id: number;
}
export interface MpvSubDelayEvent {
  /** Subtitle delay in seconds (negative = earlier, positive = later). */
  delay: number;
}
export interface MpvAudioDelayEvent {
  /** Audio delay in seconds (negative = earlier, positive = later). */
  delay: number;
}
export interface MpvColorAdjustmentEvent {
  /** Adjustment value in -100..=100 (0 = default). */
  value: number;
}
export interface MpvChapterEvent {
  /** Current chapter index (-1 = no chapter). */
  index: number;
}
export interface MpvEofReachedEvent {
  /** Whether playback has reached end of file. */
  eof: boolean;
}

/**
 * Extended track shape — now identical to `MpvTrack` (Phase 2.4 extended the event
 * payload to include `selected`, `external`, `src_id`). Kept as an alias for backward
 * compatibility with code that references `MpvTrackFull` explicitly.
 */
export type MpvTrackFull = MpvTrack;

/** Response shape from `get_track_list` invoke. */
export interface MpvTrackListResponse {
  success: boolean;
  error?: string;
  audio_tracks: MpvTrackFull[];
  sub_tracks: MpvTrackFull[];
}

/** Response shape from `get_chapter_list` invoke. */
export interface MpvChapterListResponse {
  success: boolean;
  error?: string;
  chapters: MpvChapter[];
}

/** Response shape from `find_external_subtitles` invoke. */
export interface ExternalSubtitleMatch {
  /** Absolute filesystem path to the subtitle file. */
  path: string;
  /** Match score 0.0–100.0 (higher = better). */
  score: number;
  /** Display name (file stem) for the subtitle. */
  label: string;
}

export interface FindExternalSubtitlesResponse {
  success: boolean;
  error?: string;
  matches: ExternalSubtitleMatch[];
}

/**
 * Handler callbacks for each `fyom://mpv/*` event. Only wire the ones you care about;
 * unset handlers are skipped.
 */
export interface MpvEventHandlers {
  onTimePos?: (e: MpvTimePosEvent) => void;
  onPause?: (e: MpvPauseEvent) => void;
  onEndFile?: (e: MpvEndFileEvent) => void;
  onFileLoaded?: (e: MpvFileLoadedEvent) => void;
  onTrackList?: (e: MpvTrackListEvent) => void;
  onVolume?: (e: MpvVolumeEvent) => void;
  onDuration?: (e: MpvDurationEvent) => void;
  onSpeed?: (e: MpvSpeedEvent) => void;
  onCacheSpeed?: (e: MpvCacheSpeedEvent) => void;
  onDemuxerCacheTime?: (e: MpvDemuxerCacheTimeEvent) => void;
  onPausedForCache?: (e: MpvPausedForCacheEvent) => void;
  onChapterList?: (e: MpvChapterListEvent) => void;
  onSeek?: () => void;
  onPlaybackRestart?: () => void;
  onShutdown?: () => void;
  onError?: (e: MpvErrorEvent) => void;
  // Phase 2.4 additions
  onHwdecCurrent?: (e: MpvHwdecCurrentEvent) => void;
  onAid?: (e: MpvAidEvent) => void;
  onSid?: (e: MpvSidEvent) => void;
  onSubDelay?: (e: MpvSubDelayEvent) => void;
  onAudioDelay?: (e: MpvAudioDelayEvent) => void;
  onBrightness?: (e: MpvColorAdjustmentEvent) => void;
  onContrast?: (e: MpvColorAdjustmentEvent) => void;
  onSaturation?: (e: MpvColorAdjustmentEvent) => void;
  onGamma?: (e: MpvColorAdjustmentEvent) => void;
  onHue?: (e: MpvColorAdjustmentEvent) => void;
  onChapter?: (e: MpvChapterEvent) => void;
  onEofReached?: (e: MpvEofReachedEvent) => void;
}

/**
 * Subscribe to all `fyom://mpv/*` events. Returns an `unlisten` function that tears
 * down every listener.
 *
 * No-op (returns a no-op unlisten) outside the Tauri runtime — safe to call from code
 * that also runs in a plain browser; the `<video>` fallback path is unaffected.
 *
 * @example
 * const unlisten = await subscribeMpvEvents({
 *   onTimePos: ({ position, duration }) => updateScrubber(position, duration),
 *   onPause:   ({ paused }) => isPaused.value = paused,
 * });
 * // later:
 * unlisten();
 */
export async function subscribeMpvEvents(
  handlers: MpvEventHandlers,
): Promise<() => void> {
  if (!isNativePlaybackRuntimeAvailable()) {
    // Not in Tauri — nothing to subscribe to (the `<video>` fallback owns playback).
    return () => {};
  }

  const unlistens: Array<() => void> = [];

  // Property-change events (payloads with a typed shape).
  if (handlers.onTimePos) {
    unlistens.push(await listen<MpvTimePosEvent>('fyom://mpv/time-pos', (e) => handlers.onTimePos?.(e.payload)));
  }
  if (handlers.onPause) {
    unlistens.push(await listen<MpvPauseEvent>('fyom://mpv/pause', (e) => handlers.onPause?.(e.payload)));
  }
  if (handlers.onEndFile) {
    unlistens.push(await listen<MpvEndFileEvent>('fyom://mpv/end-file', (e) => handlers.onEndFile?.(e.payload)));
  }
  if (handlers.onFileLoaded) {
    unlistens.push(await listen<MpvFileLoadedEvent>('fyom://mpv/file-loaded', (e) => handlers.onFileLoaded?.(e.payload)));
  }
  if (handlers.onTrackList) {
    unlistens.push(await listen<MpvTrackListEvent>('fyom://mpv/track-list', (e) => handlers.onTrackList?.(e.payload)));
  }
  if (handlers.onVolume) {
    unlistens.push(await listen<MpvVolumeEvent>('fyom://mpv/volume', (e) => handlers.onVolume?.(e.payload)));
  }
  if (handlers.onDuration) {
    unlistens.push(await listen<MpvDurationEvent>('fyom://mpv/duration', (e) => handlers.onDuration?.(e.payload)));
  }
  if (handlers.onSpeed) {
    unlistens.push(await listen<MpvSpeedEvent>('fyom://mpv/speed', (e) => handlers.onSpeed?.(e.payload)));
  }
  if (handlers.onCacheSpeed) {
    unlistens.push(await listen<MpvCacheSpeedEvent>('fyom://mpv/cache-speed', (e) => handlers.onCacheSpeed?.(e.payload)));
  }
  if (handlers.onDemuxerCacheTime) {
    unlistens.push(await listen<MpvDemuxerCacheTimeEvent>('fyom://mpv/demuxer-cache-time', (e) => handlers.onDemuxerCacheTime?.(e.payload)));
  }
  if (handlers.onPausedForCache) {
    unlistens.push(await listen<MpvPausedForCacheEvent>('fyom://mpv/paused-for-cache', (e) => handlers.onPausedForCache?.(e.payload)));
  }
  if (handlers.onChapterList) {
    unlistens.push(await listen<MpvChapterListEvent>('fyom://mpv/chapter-list', (e) => handlers.onChapterList?.(e.payload)));
  }
  if (handlers.onError) {
    unlistens.push(await listen<MpvErrorEvent>('fyom://mpv/error', (e) => handlers.onError?.(e.payload)));
  }

  // Phase 2.4 property-change events.
  if (handlers.onHwdecCurrent) {
    unlistens.push(await listen<MpvHwdecCurrentEvent>('fyom://mpv/hwdec-current', (e) => handlers.onHwdecCurrent?.(e.payload)));
  }
  if (handlers.onAid) {
    unlistens.push(await listen<MpvAidEvent>('fyom://mpv/aid', (e) => handlers.onAid?.(e.payload)));
  }
  if (handlers.onSid) {
    unlistens.push(await listen<MpvSidEvent>('fyom://mpv/sid', (e) => handlers.onSid?.(e.payload)));
  }
  if (handlers.onSubDelay) {
    unlistens.push(await listen<MpvSubDelayEvent>('fyom://mpv/sub-delay', (e) => handlers.onSubDelay?.(e.payload)));
  }
  if (handlers.onAudioDelay) {
    unlistens.push(await listen<MpvAudioDelayEvent>('fyom://mpv/audio-delay', (e) => handlers.onAudioDelay?.(e.payload)));
  }
  if (handlers.onBrightness) {
    unlistens.push(await listen<MpvColorAdjustmentEvent>('fyom://mpv/brightness', (e) => handlers.onBrightness?.(e.payload)));
  }
  if (handlers.onContrast) {
    unlistens.push(await listen<MpvColorAdjustmentEvent>('fyom://mpv/contrast', (e) => handlers.onContrast?.(e.payload)));
  }
  if (handlers.onSaturation) {
    unlistens.push(await listen<MpvColorAdjustmentEvent>('fyom://mpv/saturation', (e) => handlers.onSaturation?.(e.payload)));
  }
  if (handlers.onGamma) {
    unlistens.push(await listen<MpvColorAdjustmentEvent>('fyom://mpv/gamma', (e) => handlers.onGamma?.(e.payload)));
  }
  if (handlers.onHue) {
    unlistens.push(await listen<MpvColorAdjustmentEvent>('fyom://mpv/hue', (e) => handlers.onHue?.(e.payload)));
  }
  if (handlers.onChapter) {
    unlistens.push(await listen<MpvChapterEvent>('fyom://mpv/chapter', (e) => handlers.onChapter?.(e.payload)));
  }
  if (handlers.onEofReached) {
    unlistens.push(await listen<MpvEofReachedEvent>('fyom://mpv/eof-reached', (e) => handlers.onEofReached?.(e.payload)));
  }

  // Void events (no payload).
  if (handlers.onSeek) {
    unlistens.push(await listen('fyom://mpv/seek', () => handlers.onSeek?.()));
  }
  if (handlers.onPlaybackRestart) {
    unlistens.push(await listen('fyom://mpv/playback-restart', () => handlers.onPlaybackRestart?.()));
  }
  if (handlers.onShutdown) {
    unlistens.push(await listen('fyom://mpv/shutdown', () => handlers.onShutdown?.()));
  }

  return () => {
    for (const unlisten of unlistens) {
      try {
        unlisten();
      } catch {
        // ignore — best-effort teardown during component unmount
      }
    }
  };
}

/* ── Phase 2.3: GL render surface bridge ─────────────────────────────────── */

/**
 * Phase 2.3: attach the platform GL surface (NSOpenGLContext / WGL / GLX) to the main
 * Tauri window + spawn the mpv render thread.
 *
 * Called by the frontend after `play_media` succeeds (the main window is ready by then).
 * The backend creates a child GL surface behind the webview + spawns a render thread
 * that hosts `mpv_render_context_create(OpenGL)` on the platform GL context.
 *
 * Returns `{success: boolean, error?: string}` — on failure, the frontend logs the error
 * + the `<video>` fallback takes over (mpv plays audio with a black frame; the 9.7
 * guardrail stays green).
 *
 * No-op outside the Tauri runtime — safe to call from code that also runs in a plain
 * browser; the function returns `{success: false, error: 'Native playback runtime is not available'}`.
 *
 * PORTED_FROM_SOIA `attach_render_surface` pattern (the transparent-overlay + GL context
 * attach direction; the closed-source `libsoia_utils` Metal-layer surface is replaced
 * with fyom's open NSOpenGL / WGL / GLX path).
 */
export async function attachRenderSurface(): Promise<NativePlayerInitResult> {
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

    const result = await invoke('attach_render_surface');

    if (result && result.success) {
      return { ok: true };
    }

    return {
      ok: false,
      failure: mapNativePlayerInitError(
        new Error(result?.error || 'Failed to attach render surface'),
      ),
    };
  } catch (err) {
    return {
      ok: false,
      failure: mapNativePlayerInitError(err),
    };
  }
}

/**
 * Phase 2.3: notify the backend that the webview entered / exited `.video-mode`
 * (transparent background so the mpv GL layer shows through).
 *
 * The CSS class toggle is the actual mechanism — this invoke is informational (the
 * backend logs the transition). Future Phase 2.5+ work may pause/resume the render
 * thread here to save CPU when no video is showing.
 *
 * No-op outside the Tauri runtime.
 */
export async function setVideoMode(enabled: boolean): Promise<void> {
  if (!isNativePlaybackRuntimeAvailable()) {
    return;
  }

  try {
    // @ts-expect-error — __TAURI_INTERNALS__ exists in Tauri runtime
    const { invoke } = window.__TAURI_INTERNALS__.tauri;
    await invoke('set_video_mode', { enabled });
  } catch {
    // Informational command — ignore errors (the CSS is the actual mechanism).
  }
}

/**
 * Phase 2.3: notify the backend of a window resize so it can update the GL surface
 * drawable (e.g. call `NSOpenGLContext::update` on macOS to avoid GL framebuffer
 * corruption after a window resize).
 *
 * **Note**: fyom's `RenderSurface::drawable_size` is polled on every render frame, so
 * the render loop picks up the new dimensions automatically. This invoke is a hook for
 * future platform-specific resize logic (Phase 2.4).
 *
 * No-op outside the Tauri runtime.
 */
export async function resizeRenderSurface(
  width: number,
  height: number,
  scaleFactor: number,
): Promise<void> {
  if (!isNativePlaybackRuntimeAvailable()) {
    return;
  }

  try {
    // @ts-expect-error — __TAURI_INTERNALS__ exists in Tauri runtime
    const { invoke } = window.__TAURI_INTERNALS__.tauri;
    await invoke('resize_render_surface', { width, height, scaleFactor });
  } catch {
    // Best-effort — ignore errors (the render loop polls dimensions anyway).
  }
}

/* ── Phase 2.4: playback feature bridge (subtitles, audio tracks, color
 *    adjustments, chapter nav, generic property get/set). PORTED_FROM_SOIA
 *    `commands/playback.rs` + `useMediaTracks.ts` + `usePlaybackAdjustments.ts`
 *    invoke surface, reimplemented on fyom's `MpvInstance` (soia's
 *    `mpv_run_command`/`mpv_set_option_string` map to fyom's typed commands). ──── */

/**
 * Get the Tauri invoke function, or null outside the Tauri runtime.
 *
 * Internal helper — the Phase 2.4 bridge functions use this to avoid repeating the
 * `__TAURI_INTERNALS__` dance. Returns null in a plain browser (the `<video>` fallback
 * owns playback; the bridge functions return safe defaults).
 */
function getTauriInvoke(): ((command: string, args?: Record<string, unknown>) => Promise<unknown>) | null {
  if (!isNativePlaybackRuntimeAvailable()) {
    return null;
  }
  try {
    // @ts-expect-error — __TAURI_INTERNALS__ exists in Tauri runtime
    const { invoke } = window.__TAURI_INTERNALS__.tauri;
    return typeof invoke === 'function' ? invoke : null;
  } catch {
    return null;
  }
}

/**
 * Phase 2.4: find external subtitle files matching a LOCAL media file (fuzzy match by
 * name, episode key, year). Returns absolute filesystem paths sorted by match score.
 *
 * LOCAL-only: returns an empty list for remote (presigned URL / network) media.
 */
export async function findExternalSubtitles(
  mediaPath: string,
  mediaTitle?: string,
): Promise<ExternalSubtitleMatch[]> {
  const invoke = getTauriInvoke();
  if (!invoke) return [];

  try {
    const result = await invoke<FindExternalSubtitlesResponse>('find_external_subtitles', {
      mediaPath,
      mediaTitle: mediaTitle ?? null,
    });
    if (result && result.success) {
      return result.matches;
    }
    return [];
  } catch {
    return [];
  }
}

/**
 * Phase 2.4: add an external subtitle file to the current playback.
 *
 * @param path Absolute filesystem path to the subtitle file.
 * @param mode `"select"` (activate immediately) or `"auto"` (add but don't activate).
 * @param title Optional display title (shown in track list + subtitle picker).
 * @param lang Optional ISO 639-1 language code (e.g. "en", "zh").
 */
export async function subAdd(
  path: string,
  mode: 'select' | 'auto' = 'select',
  title?: string,
  lang?: string,
): Promise<boolean> {
  const invoke = getTauriInvoke();
  if (!invoke) return false;

  try {
    const result = await invoke<{ success: boolean; error?: string }>('sub_add', {
      path,
      mode,
      title: title ?? null,
      lang: lang ?? null,
    });
    return Boolean(result && result.success);
  } catch {
    return false;
  }
}

/** Remove an external subtitle track by id. */
export async function subRemove(trackId: number): Promise<boolean> {
  const invoke = getTauriInvoke();
  if (!invoke) return false;

  try {
    const result = await invoke<{ success: boolean; error?: string }>('sub_remove', {
      trackId,
    });
    return Boolean(result && result.success);
  } catch {
    return false;
  }
}

/** Reload a subtitle track by id (useful after editing an external .srt). */
export async function subReload(trackId: number): Promise<boolean> {
  const invoke = getTauriInvoke();
  if (!invoke) return false;

  try {
    const result = await invoke<{ success: boolean; error?: string }>('sub_reload', {
      trackId,
    });
    return Boolean(result && result.success);
  } catch {
    return false;
  }
}

/** Add an external audio track. */
export async function audioAdd(
  path: string,
  mode: 'select' | 'auto' = 'select',
): Promise<boolean> {
  const invoke = getTauriInvoke();
  if (!invoke) return false;

  try {
    const result = await invoke<{ success: boolean; error?: string }>('audio_add', {
      path,
      mode,
    });
    return Boolean(result && result.success);
  } catch {
    return false;
  }
}

/** Set the subtitle delay (seconds; negative = earlier, positive = later). */
export async function setSubDelay(seconds: number): Promise<boolean> {
  const invoke = getTauriInvoke();
  if (!invoke) return false;

  try {
    const result = await invoke<{ success: boolean; error?: string }>('set_sub_delay', {
      seconds,
    });
    return Boolean(result && result.success);
  } catch {
    return false;
  }
}

/** Set the audio delay (seconds; negative = earlier, positive = later). */
export async function setAudioDelay(seconds: number): Promise<boolean> {
  const invoke = getTauriInvoke();
  if (!invoke) return false;

  try {
    const result = await invoke<{ success: boolean; error?: string }>('set_audio_delay', {
      seconds,
    });
    return Boolean(result && result.success);
  } catch {
    return false;
  }
}

/** Set the subtitle font scale (1.0 = default). */
export async function setSubScale(scale: number): Promise<boolean> {
  const invoke = getTauriInvoke();
  if (!invoke) return false;

  try {
    const result = await invoke<{ success: boolean; error?: string }>('set_sub_scale', {
      scale,
    });
    return Boolean(result && result.success);
  } catch {
    return false;
  }
}

/** Set a color adjustment (`brightness` / `contrast` / `saturation` / `gamma` / `hue`). Range: -100..=100. */
export async function setColorAdjustment(
  name: 'brightness' | 'contrast' | 'saturation' | 'gamma' | 'hue',
  value: number,
): Promise<boolean> {
  const invoke = getTauriInvoke();
  if (!invoke) return false;

  try {
    const result = await invoke<{ success: boolean; error?: string }>('set_color_adjustment', {
      name,
      value,
    });
    return Boolean(result && result.success);
  } catch {
    return false;
  }
}

/** Generic mpv option-string setter (power-user surface — prefer typed commands above). */
export async function mpvSetOptionString(name: string, value: string | number | boolean): Promise<boolean> {
  const invoke = getTauriInvoke();
  if (!invoke) return false;

  try {
    const result = await invoke<{ success: boolean; error?: string }>('mpv_set_option_string', {
      name,
      value: String(value),
    });
    return Boolean(result && result.success);
  } catch {
    return false;
  }
}

/** Get the current track list (audio + sub) as a typed object. One-shot read. */
export async function getTrackList(): Promise<MpvTrackListResponse | null> {
  const invoke = getTauriInvoke();
  if (!invoke) return null;

  try {
    return await invoke<MpvTrackListResponse>('get_track_list');
  } catch {
    return null;
  }
}

/** Get the chapter list (one-shot read). */
export async function getChapterList(): Promise<MpvChapterListResponse | null> {
  const invoke = getTauriInvoke();
  if (!invoke) return null;

  try {
    return await invoke<MpvChapterListResponse>('get_chapter_list');
  } catch {
    return null;
  }
}

/** Navigate to a chapter by index. */
export async function setChapter(index: number): Promise<boolean> {
  const invoke = getTauriInvoke();
  if (!invoke) return false;

  try {
    const result = await invoke<{ success: boolean; error?: string }>('set_chapter', {
      index,
    });
    return Boolean(result && result.success);
  } catch {
    return false;
  }
}

/** Generic string-valued property getter (one-shot read). */
export async function getProperty(name: string): Promise<string | null> {
  const invoke = getTauriInvoke();
  if (!invoke) return null;

  try {
    const result = await invoke<{ success: boolean; value?: string; error?: string }>('get_property', {
      name,
    });
    if (result && result.success) {
      return result.value ?? null;
    }
    return null;
  } catch {
    return null;
  }
}

/* ── Phase 2.4: typed wrappers for the Phase 2.2 commands (seek, volume, speed,
 *    pause, track selection, keypress, generic command). These existed as Tauri
 *    commands since Phase 2.2 but had no TS bridge — the PlayerView controls now
 *    need them. ───────────────────────────────────────────────────────────── */

/** Seek to an absolute position (seconds). */
export async function seek(position: number): Promise<boolean> {
  const invoke = getTauriInvoke();
  if (!invoke) return false;

  try {
    const result = await invoke<{ success: boolean; error?: string }>('seek', { position });
    return Boolean(result && result.success);
  } catch {
    return false;
  }
}

/** Seek by a relative offset (seconds; negative = backward). */
export async function seekRelative(seconds: number): Promise<boolean> {
  const invoke = getTauriInvoke();
  if (!invoke) return false;

  try {
    const result = await invoke<{ success: boolean; error?: string }>('seek_relative', {
      seconds,
    });
    return Boolean(result && result.success);
  } catch {
    return false;
  }
}

/** Toggle play/pause. */
export async function togglePause(): Promise<boolean> {
  const invoke = getTauriInvoke();
  if (!invoke) return false;

  try {
    const result = await invoke<{ success: boolean; error?: string }>('toggle_pause');
    return Boolean(result && result.success);
  } catch {
    return false;
  }
}

/** Explicitly set the pause state (`true` = paused, `false` = playing). */
export async function setPause(paused: boolean): Promise<boolean> {
  const invoke = getTauriInvoke();
  if (!invoke) return false;

  try {
    const result = await invoke<{ success: boolean; error?: string }>('set_pause', { paused });
    return Boolean(result && result.success);
  } catch {
    return false;
  }
}

/** Set the volume (0–100). */
export async function setVolume(volume: number): Promise<boolean> {
  const invoke = getTauriInvoke();
  if (!invoke) return false;

  try {
    const result = await invoke<{ success: boolean; error?: string }>('set_volume', { volume });
    return Boolean(result && result.success);
  } catch {
    return false;
  }
}

/** Set the playback speed (1.0 = normal). */
export async function setSpeed(speed: number): Promise<boolean> {
  const invoke = getTauriInvoke();
  if (!invoke) return false;

  try {
    const result = await invoke<{ success: boolean; error?: string }>('set_speed', { speed });
    return Boolean(result && result.success);
  } catch {
    return false;
  }
}

/** Select the audio track (`null` to disable audio). */
export async function setAudioTrack(trackId: number | null): Promise<boolean> {
  const invoke = getTauriInvoke();
  if (!invoke) return false;

  try {
    const result = await invoke<{ success: boolean; error?: string }>('set_audio_track', {
      trackId,
    });
    return Boolean(result && result.success);
  } catch {
    return false;
  }
}

/** Select the subtitle track (`null` to disable subtitles). */
export async function setSubtitleTrack(trackId: number | null): Promise<boolean> {
  const invoke = getTauriInvoke();
  if (!invoke) return false;

  try {
    const result = await invoke<{ success: boolean; error?: string }>('set_subtitle_track', {
      trackId,
    });
    return Boolean(result && result.success);
  } catch {
    return false;
  }
}

/** Forward a keypress to mpv (mpv keystr format, e.g. "Space", "Ctrl+Right", "Volume+"). */
export async function mpvKeypress(key: string): Promise<boolean> {
  const invoke = getTauriInvoke();
  if (!invoke) return false;

  try {
    const result = await invoke<{ success: boolean; error?: string }>('mpv_keypress', { key });
    return Boolean(result && result.success);
  } catch {
    return false;
  }
}

/** Stop playback + clear the playlist. */
export async function stopMedia(): Promise<boolean> {
  const invoke = getTauriInvoke();
  if (!invoke) return false;

  try {
    const result = await invoke<{ success: boolean; error?: string }>('stop_media');
    return Boolean(result && result.success);
  } catch {
    return false;
  }
}
