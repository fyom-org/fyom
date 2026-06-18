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

export type NativePlayerStatus = 'idle' | 'initializing' | 'ready' | 'failed' | 'unavailable';

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

export type NativePlayerInitResult = { ok: true } | { ok: false; failure: NativePlayerFailure };

/* ── Common command response shapes ───────────────────────────────────── */

interface NativeCommandResponse {
  success: boolean;
  error?: string;
}

type TauriInvoke = (command: string, args?: Record<string, unknown>) => Promise<unknown>;

interface TauriInternalsWindow extends Window {
  __TAURI_INTERNALS__?: {
    tauri?: {
      invoke?: TauriInvoke;
    };
  };
}

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

/* ── Tauri invoke bridge helpers ───────────────────────────────────────── */

/**
 * Get the Tauri invoke function, or null outside the Tauri runtime.
 *
 * This project intentionally goes through `window.__TAURI_INTERNALS__.tauri.invoke`
 * instead of importing `invoke` directly, so the plain-browser fallback bundle can
 * keep calling these helpers safely.
 */
function getTauriInvoke(): TauriInvoke | null {
  if (!isNativePlaybackRuntimeAvailable()) {
    return null;
  }

  const tauriWindow = window as TauriInternalsWindow;
  const tauriApi = tauriWindow.__TAURI_INTERNALS__?.tauri;
  const invoke = tauriApi?.invoke;

  return typeof invoke === 'function' ? invoke.bind(tauriApi) : null;
}

/**
 * Typed wrapper around Tauri invoke.
 *
 * The raw Tauri invoke function obtained from `window.__TAURI_INTERNALS__` is not
 * typed as a generic function. Do not call `invoke<T>()` directly; call this helper
 * instead so TypeScript sees one safe cast boundary in this file.
 */
async function invokeTauri<T>(command: string, args?: Record<string, unknown>): Promise<T> {
  const invoke = getTauriInvoke();

  if (!invoke) {
    throw new Error('Native playback runtime is not available');
  }

  return (await invoke(command, args)) as T;
}

/**
 * Invoke a command that follows the `{ success: boolean; error?: string }` contract.
 */
async function invokeBooleanCommand(
  command: string,
  args?: Record<string, unknown>
): Promise<boolean> {
  if (!isNativePlaybackRuntimeAvailable()) {
    return false;
  }

  try {
    const result = await invokeTauri<NativeCommandResponse>(command, args);
    return Boolean(result?.success);
  } catch {
    return false;
  }
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
  params: NativePlayerInitParams
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
    const result = await invokeTauri<NativeCommandResponse>('play_media', {
      mediaUrl: params.mediaUrl,
      posterUrl: params.posterUrl ?? '',
    });

    if (result?.success) {
      return { ok: true };
    }

    return {
      ok: false,
      failure: mapNativePlayerInitError(new Error(result?.error || 'Unknown native player error')),
    };
  } catch (err) {
    return {
      ok: false,
      failure: mapNativePlayerInitError(err),
    };
  }
}

/* ── Phase 2.2: `fyom://mpv/*` event subscription ──────────────────────── */

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
export interface MpvTrackListResponse extends NativeCommandResponse {
  audio_tracks: MpvTrackFull[];
  sub_tracks: MpvTrackFull[];
}

/** Response shape from `get_chapter_list` invoke. */
export interface MpvChapterListResponse extends NativeCommandResponse {
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

export interface FindExternalSubtitlesResponse extends NativeCommandResponse {
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
 */
export async function subscribeMpvEvents(handlers: MpvEventHandlers): Promise<() => void> {
  if (!isNativePlaybackRuntimeAvailable()) {
    return () => {};
  }

  const unlistens: Array<() => void> = [];

  const addPayloadListener = async <T>(
    eventName: string,
    handler: ((payload: T) => void) | undefined
  ): Promise<void> => {
    if (!handler) {
      return;
    }

    unlistens.push(
      await listen<T>(eventName, (event) => {
        handler(event.payload);
      })
    );
  };

  const addVoidListener = async (
    eventName: string,
    handler: (() => void) | undefined
  ): Promise<void> => {
    if (!handler) {
      return;
    }

    unlistens.push(
      await listen(eventName, () => {
        handler();
      })
    );
  };

  await addPayloadListener<MpvTimePosEvent>('fyom://mpv/time-pos', handlers.onTimePos);
  await addPayloadListener<MpvPauseEvent>('fyom://mpv/pause', handlers.onPause);
  await addPayloadListener<MpvEndFileEvent>('fyom://mpv/end-file', handlers.onEndFile);
  await addPayloadListener<MpvFileLoadedEvent>('fyom://mpv/file-loaded', handlers.onFileLoaded);
  await addPayloadListener<MpvTrackListEvent>('fyom://mpv/track-list', handlers.onTrackList);
  await addPayloadListener<MpvVolumeEvent>('fyom://mpv/volume', handlers.onVolume);
  await addPayloadListener<MpvDurationEvent>('fyom://mpv/duration', handlers.onDuration);
  await addPayloadListener<MpvSpeedEvent>('fyom://mpv/speed', handlers.onSpeed);
  await addPayloadListener<MpvCacheSpeedEvent>('fyom://mpv/cache-speed', handlers.onCacheSpeed);
  await addPayloadListener<MpvDemuxerCacheTimeEvent>(
    'fyom://mpv/demuxer-cache-time',
    handlers.onDemuxerCacheTime
  );
  await addPayloadListener<MpvPausedForCacheEvent>(
    'fyom://mpv/paused-for-cache',
    handlers.onPausedForCache
  );
  await addPayloadListener<MpvChapterListEvent>('fyom://mpv/chapter-list', handlers.onChapterList);
  await addPayloadListener<MpvErrorEvent>('fyom://mpv/error', handlers.onError);

  await addPayloadListener<MpvHwdecCurrentEvent>(
    'fyom://mpv/hwdec-current',
    handlers.onHwdecCurrent
  );
  await addPayloadListener<MpvAidEvent>('fyom://mpv/aid', handlers.onAid);
  await addPayloadListener<MpvSidEvent>('fyom://mpv/sid', handlers.onSid);
  await addPayloadListener<MpvSubDelayEvent>('fyom://mpv/sub-delay', handlers.onSubDelay);
  await addPayloadListener<MpvAudioDelayEvent>('fyom://mpv/audio-delay', handlers.onAudioDelay);
  await addPayloadListener<MpvColorAdjustmentEvent>('fyom://mpv/brightness', handlers.onBrightness);
  await addPayloadListener<MpvColorAdjustmentEvent>('fyom://mpv/contrast', handlers.onContrast);
  await addPayloadListener<MpvColorAdjustmentEvent>('fyom://mpv/saturation', handlers.onSaturation);
  await addPayloadListener<MpvColorAdjustmentEvent>('fyom://mpv/gamma', handlers.onGamma);
  await addPayloadListener<MpvColorAdjustmentEvent>('fyom://mpv/hue', handlers.onHue);
  await addPayloadListener<MpvChapterEvent>('fyom://mpv/chapter', handlers.onChapter);
  await addPayloadListener<MpvEofReachedEvent>('fyom://mpv/eof-reached', handlers.onEofReached);

  await addVoidListener('fyom://mpv/seek', handlers.onSeek);
  await addVoidListener('fyom://mpv/playback-restart', handlers.onPlaybackRestart);
  await addVoidListener('fyom://mpv/shutdown', handlers.onShutdown);

  return () => {
    for (const unlisten of unlistens) {
      try {
        unlisten();
      } catch {
        // Best-effort teardown during component unmount.
      }
    }
  };
}

/* ── Phase 2.3: GL render surface bridge ───────────────────────────────── */

/**
 * Phase 2.3: attach the platform GL surface (NSOpenGLContext / WGL / GLX) to the main
 * Tauri window + spawn the mpv render thread.
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
    const result = await invokeTauri<NativeCommandResponse>('attach_render_surface');

    if (result?.success) {
      return { ok: true };
    }

    return {
      ok: false,
      failure: mapNativePlayerInitError(
        new Error(result?.error || 'Failed to attach render surface')
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
 */
export async function setVideoMode(enabled: boolean): Promise<void> {
  if (!isNativePlaybackRuntimeAvailable()) {
    return;
  }

  try {
    await invokeTauri<void>('set_video_mode', { enabled });
  } catch {
    // Informational command — ignore errors because CSS is the actual mechanism.
  }
}

/**
 * Phase 2.3: notify the backend of a window resize so it can update the GL surface.
 */
export async function resizeRenderSurface(
  width: number,
  height: number,
  scaleFactor: number
): Promise<void> {
  if (!isNativePlaybackRuntimeAvailable()) {
    return;
  }

  try {
    await invokeTauri<void>('resize_render_surface', {
      width,
      height,
      scaleFactor,
    });
  } catch {
    // Best-effort — ignore errors because the render loop polls dimensions anyway.
  }
}

/* ── Phase 2.4: playback feature bridge ────────────────────────────────── */

/**
 * Phase 2.4: find external subtitle files matching a LOCAL media file.
 *
 * LOCAL-only: returns an empty list for remote (presigned URL / network) media.
 */
export async function findExternalSubtitles(
  mediaPath: string,
  mediaTitle?: string
): Promise<ExternalSubtitleMatch[]> {
  if (!isNativePlaybackRuntimeAvailable()) {
    return [];
  }

  try {
    const result = await invokeTauri<FindExternalSubtitlesResponse>('find_external_subtitles', {
      mediaPath,
      mediaTitle: mediaTitle ?? null,
    });

    if (result?.success) {
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
  lang?: string
): Promise<boolean> {
  return invokeBooleanCommand('sub_add', {
    path,
    mode,
    title: title ?? null,
    lang: lang ?? null,
  });
}

/** Remove an external subtitle track by id. */
export async function subRemove(trackId: number): Promise<boolean> {
  return invokeBooleanCommand('sub_remove', { trackId });
}

/** Reload a subtitle track by id (useful after editing an external .srt). */
export async function subReload(trackId: number): Promise<boolean> {
  return invokeBooleanCommand('sub_reload', { trackId });
}

/** Add an external audio track. */
export async function audioAdd(path: string, mode: 'select' | 'auto' = 'select'): Promise<boolean> {
  return invokeBooleanCommand('audio_add', { path, mode });
}

/** Set the subtitle delay (seconds; negative = earlier, positive = later). */
export async function setSubDelay(seconds: number): Promise<boolean> {
  return invokeBooleanCommand('set_sub_delay', { seconds });
}

/** Set the audio delay (seconds; negative = earlier, positive = later). */
export async function setAudioDelay(seconds: number): Promise<boolean> {
  return invokeBooleanCommand('set_audio_delay', { seconds });
}

/** Set the subtitle font scale (1.0 = default). */
export async function setSubScale(scale: number): Promise<boolean> {
  return invokeBooleanCommand('set_sub_scale', { scale });
}

/**
 * Set a color adjustment.
 *
 * Range: -100..=100.
 */
export async function setColorAdjustment(
  name: 'brightness' | 'contrast' | 'saturation' | 'gamma' | 'hue',
  value: number
): Promise<boolean> {
  return invokeBooleanCommand('set_color_adjustment', {
    name,
    value,
  });
}

/** Generic mpv option-string setter (power-user surface — prefer typed commands above). */
export async function mpvSetOptionString(
  name: string,
  value: string | number | boolean
): Promise<boolean> {
  return invokeBooleanCommand('mpv_set_option_string', {
    name,
    value: String(value),
  });
}

/** Get the current track list (audio + sub) as a typed object. One-shot read. */
export async function getTrackList(): Promise<MpvTrackListResponse | null> {
  if (!isNativePlaybackRuntimeAvailable()) {
    return null;
  }

  try {
    return await invokeTauri<MpvTrackListResponse>('get_track_list');
  } catch {
    return null;
  }
}

/** Get the chapter list (one-shot read). */
export async function getChapterList(): Promise<MpvChapterListResponse | null> {
  if (!isNativePlaybackRuntimeAvailable()) {
    return null;
  }

  try {
    return await invokeTauri<MpvChapterListResponse>('get_chapter_list');
  } catch {
    return null;
  }
}

/** Navigate to a chapter by index. */
export async function setChapter(index: number): Promise<boolean> {
  return invokeBooleanCommand('set_chapter', { index });
}

/** Generic string-valued property getter (one-shot read). */
export async function getProperty(name: string): Promise<string | null> {
  if (!isNativePlaybackRuntimeAvailable()) {
    return null;
  }

  try {
    const result = await invokeTauri<NativeCommandResponse & { value?: string }>('get_property', {
      name,
    });

    if (result?.success) {
      return result.value ?? null;
    }

    return null;
  } catch {
    return null;
  }
}

/* ── Phase 2.4: typed wrappers for Phase 2.2 commands ──────────────────── */

/** Seek to an absolute position (seconds). */
export async function seek(position: number): Promise<boolean> {
  return invokeBooleanCommand('seek', { position });
}

/** Seek by a relative offset (seconds; negative = backward). */
export async function seekRelative(seconds: number): Promise<boolean> {
  return invokeBooleanCommand('seek_relative', { seconds });
}

/** Toggle play/pause. */
export async function togglePause(): Promise<boolean> {
  return invokeBooleanCommand('toggle_pause');
}

/** Explicitly set the pause state (`true` = paused, `false` = playing). */
export async function setPause(paused: boolean): Promise<boolean> {
  return invokeBooleanCommand('set_pause', { paused });
}

/** Set the volume (0–100). */
export async function setVolume(volume: number): Promise<boolean> {
  return invokeBooleanCommand('set_volume', { volume });
}

/** Set the playback speed (1.0 = normal). */
export async function setSpeed(speed: number): Promise<boolean> {
  return invokeBooleanCommand('set_speed', { speed });
}

/** Select the audio track (`null` to disable audio). */
export async function setAudioTrack(trackId: number | null): Promise<boolean> {
  return invokeBooleanCommand('set_audio_track', { trackId });
}

/** Select the subtitle track (`null` to disable subtitles). */
export async function setSubtitleTrack(trackId: number | null): Promise<boolean> {
  return invokeBooleanCommand('set_subtitle_track', { trackId });
}

/** Forward a keypress to mpv (mpv keystr format, e.g. "Space", "Ctrl+Right", "Volume+"). */
export async function mpvKeypress(key: string): Promise<boolean> {
  return invokeBooleanCommand('mpv_keypress', { key });
}

/** Stop playback + clear the playlist. */
export async function stopMedia(): Promise<boolean> {
  return invokeBooleanCommand('stop_media');
}
