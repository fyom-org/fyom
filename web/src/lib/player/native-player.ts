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

import { invoke } from '@tauri-apps/api/core';
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
 * Typed wrapper around Tauri invoke.
 *
 * Uses the official `invoke` imported from `@tauri-apps/api/core` (Tauri v2).
 * If not in a Tauri environment, it throws safely, which is caught upstream.
 */
async function invokeTauri<T>(command: string, args?: Record<string, unknown>): Promise<T> {
  if (!isNativePlaybackRuntimeAvailable()) {
    throw new Error('Native playback runtime is not available');
  }

  // Directly use the official invoke from Tauri v2
  return await invoke<T>(command, args);
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
      // Pass null instead of empty string for Option<String> in Rust
      posterUrl: params.posterUrl ?? null,
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

export interface MpvTrack {
  id: number;
  title: string;
  lang: string;
  type: string;
  selected?: boolean;
  external?: boolean;
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
  reason: number;
  reason_name: string;
}

export interface MpvFileLoadedEvent {
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

export interface MpvHwdecCurrentEvent {
  hwdec: string;
}

export interface MpvAidEvent {
  id: number;
}

export interface MpvSidEvent {
  id: number;
}

export interface MpvSubDelayEvent {
  delay: number;
}

export interface MpvAudioDelayEvent {
  delay: number;
}

export interface MpvColorAdjustmentEvent {
  value: number;
}

export interface MpvChapterEvent {
  index: number;
}

export interface MpvEofReachedEvent {
  eof: boolean;
}

export type MpvTrackFull = MpvTrack;

export interface MpvTrackListResponse extends NativeCommandResponse {
  audio_tracks: MpvTrackFull[];
  sub_tracks: MpvTrackFull[];
}

export interface MpvChapterListResponse extends NativeCommandResponse {
  chapters: MpvChapter[];
}

export interface ExternalSubtitleMatch {
  path: string;
  score: number;
  label: string;
}

export interface FindExternalSubtitlesResponse extends NativeCommandResponse {
  matches: ExternalSubtitleMatch[];
}

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

export async function subRemove(trackId: number): Promise<boolean> {
  return invokeBooleanCommand('sub_remove', { trackId });
}

export async function subReload(trackId: number): Promise<boolean> {
  return invokeBooleanCommand('sub_reload', { trackId });
}

export async function audioAdd(path: string, mode: 'select' | 'auto' = 'select'): Promise<boolean> {
  return invokeBooleanCommand('audio_add', { path, mode });
}

export async function setSubDelay(seconds: number): Promise<boolean> {
  return invokeBooleanCommand('set_sub_delay', { seconds });
}

export async function setAudioDelay(seconds: number): Promise<boolean> {
  return invokeBooleanCommand('set_audio_delay', { seconds });
}

export async function setSubScale(scale: number): Promise<boolean> {
  return invokeBooleanCommand('set_sub_scale', { scale });
}

export async function setColorAdjustment(
  name: 'brightness' | 'contrast' | 'saturation' | 'gamma' | 'hue',
  value: number
): Promise<boolean> {
  return invokeBooleanCommand('set_color_adjustment', {
    name,
    value,
  });
}

export async function mpvSetOptionString(
  name: string,
  value: string | number | boolean
): Promise<boolean> {
  return invokeBooleanCommand('mpv_set_option_string', {
    name,
    value: String(value),
  });
}

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

export async function setChapter(index: number): Promise<boolean> {
  return invokeBooleanCommand('set_chapter', { index });
}

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

export async function seek(position: number): Promise<boolean> {
  return invokeBooleanCommand('seek', { position });
}

export async function seekRelative(seconds: number): Promise<boolean> {
  return invokeBooleanCommand('seek_relative', { seconds });
}

export async function togglePause(): Promise<boolean> {
  return invokeBooleanCommand('toggle_pause');
}

export async function setPause(paused: boolean): Promise<boolean> {
  return invokeBooleanCommand('set_pause', { paused });
}

export async function setVolume(volume: number): Promise<boolean> {
  return invokeBooleanCommand('set_volume', { volume });
}

export async function setSpeed(speed: number): Promise<boolean> {
  return invokeBooleanCommand('set_speed', { speed });
}

export async function setAudioTrack(trackId: number | null): Promise<boolean> {
  return invokeBooleanCommand('set_audio_track', { trackId });
}

export async function setSubtitleTrack(trackId: number | null): Promise<boolean> {
  return invokeBooleanCommand('set_subtitle_track', { trackId });
}

export async function mpvKeypress(key: string): Promise<boolean> {
  return invokeBooleanCommand('mpv_keypress', { key });
}

export async function stopMedia(): Promise<boolean> {
  return invokeBooleanCommand('stop_media');
}
