/**
 * usePlaybackSeekActions — seek + loading-state coordination composable.
 *
 * PORTED_FROM_SOIA @ <2025-Q4> (`src/composables/usePlaybackSeekActions.ts`, GPL-3.0-only)
 *
 * Adapted for fyom:
 * - soia's `PlayerApi` (a full controller abstraction with `state.media.url`,
 *   `state.media.isLivePlayback`, `state.playback.downloadSpeedBps`) is replaced with
 *   a minimal `SeekActionsOptions` that takes refs for the loading state + a `seek` /
 *   `seekRelative` function pair. fyom's PlayerView owns the loading state directly.
 * - soia's `isLivePlayback` check (which gates seek for HLS/DASH live streams) is
 *   deferred to Phase 2.6 (fyom doesn't yet detect live playback — mpv's
 *   `seekable` property is the right gate, but wiring it requires another observer).
 *   For now, all seeks are allowed; mpv rejects seeks on unseekable streams and the
 *   bridge returns false (the loading state is reset on failure).
 * - The `beginSeekLoading` / `endSeekLoading` pattern is ported verbatim — it shows a
 *   loading indicator while mpv processes the seek (mpv seeks are async; the
 *   `fyom://mpv/playback-restart` event signals seek completion).
 */

import type { Ref } from 'vue';

import { seek as bridgeSeek, seekRelative as bridgeSeekRelative } from '@/lib/player/native-player';

export interface SeekActionsOptions {
  /** The loading-state ref (true while a seek is in progress). */
  isLoading: Ref<boolean>;
  /** The loading-url ref (set to the current media URL while seeking — used to detect
   *  media changes during a seek). */
  loadingUrl: Ref<string>;
  /** The current media URL (used to populate `loadingUrl` on seek start). */
  getCurrentMediaUrl: () => string;
}

export function usePlaybackSeekActions(options: SeekActionsOptions) {
  const { isLoading, loadingUrl, getCurrentMediaUrl } = options;

  /** Show the loading indicator + record the media URL being seeked within. */
  function beginSeekLoading(): boolean {
    isLoading.value = true;
    loadingUrl.value = getCurrentMediaUrl();
    return true;
  }

  /** Hide the loading indicator + clear the loading URL (seek completed or failed). */
  function endSeekLoading(): void {
    isLoading.value = false;
    loadingUrl.value = '';
  }

  /** Seek to an absolute position (seconds). */
  async function onSeek(position: number): Promise<void> {
    if (!beginSeekLoading()) return;
    const ok = await bridgeSeek(position);
    if (!ok) endSeekLoading();
  }

  /** Seek by a relative offset (seconds; negative = backward). */
  async function onSeekRelative(seconds: number): Promise<void> {
    if (!beginSeekLoading()) return;
    const ok = await bridgeSeekRelative(seconds);
    if (!ok) endSeekLoading();
  }

  return {
    onSeek,
    onSeekRelative,
    endSeekLoading,
  };
}
