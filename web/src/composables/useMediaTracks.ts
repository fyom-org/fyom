/**
 * useMediaTracks — audio/subtitle track management composable.
 *
 * PORTED_FROM_SOIA @ <2025-Q4> (`src/composables/useMediaTracks.ts`, GPL-3.0-only)
 *
 * Adapted for fyom:
 * - soia's dual-subtitle support (`useSubtitleState` + `secondarySubId` +
 *   `setDualSubEnabled`) is **deferred** to Phase 2.6+ (fyom v1 ships single-sub only;
 *   the mpv `secondary-sid` property + dual-sub UI is a Phase 2.6 nicety). The composable
 *   exposes a single `currentSubId` + `selectSubTrack` instead.
 * - soia's `HistoryApi` (which records external tracks per media key in soia's store) is
 *   **dropped** — fyom uses the Go backend + SQLite for watch progress (Phase 2.5 will
 *   wire external-track recording via the backend API if needed; for v1, external tracks
 *   are re-discovered on each play via `findExternalSubtitles`).
 * - soia's `invoke("mpv_run_command", { args: ["sub-add", path, mode] })` is replaced
 *   with fyom's typed `subAdd(path, mode, title?, lang?)` bridge.
 * - soia's `invoke("mpv_set_option_string", { name: "aid", value: track.id })` is
 *   replaced with fyom's typed `setAudioTrack(trackId)` bridge.
 * - The background subtitle queue + `waitForNextTracksUpdate` + `requestAnimationFrame`
 *   batching is **ported verbatim** — it's the core algorithm that prevents mpv's
 *   `track-list` observer from thrashing when multiple subs are added in rapid
 *   succession (each `sub-add` triggers a track-list update; without batching the UI
 *   would flicker).
 * - The `applyExternalTracksForUrl` method calls fyom's `findExternalSubtitles(path,
 *   title)` (LOCAL-only — returns [] for remote media).
 */

import { ref } from 'vue';

import {
  findExternalSubtitles,
  setAudioTrack,
  setSubtitleTrack,
  subAdd,
  type MpvTrackFull,
} from '@/lib/player/native-player';

/** A single track (audio or subtitle) in the fyom track list. */
export interface FyomMediaTrack {
  id: number;
  title: string;
  lang: string;
  /** `"audio"` or `"sub"`. */
  type: 'audio' | 'sub';
  /** Whether this track is currently selected. */
  selected: boolean;
  /** Whether this is an external track (loaded via `sub-add` / `audio-add`). */
  external: boolean;
}

/** The shape of the `fyom://mpv/track-list` event payload (Phase 2.2 contract). */
export interface TrackListPayload {
  audio_tracks: MpvTrackFull[];
  sub_tracks: MpvTrackFull[];
}

const TRACK_UPDATE_WAIT_TIMEOUT_MS = 700;
const BACKGROUND_TRACK_ADD_GAP_MS = 40;
const VISIBLE_MENU_TRACK_ADD_GAP_MS = 180;

interface BackgroundSubtitleItem {
  path: string;
  mode: 'select' | 'auto';
  mediaKey: string;
}

interface TrackUpdateWaiter {
  resolve: () => void;
  timeoutId: ReturnType<typeof setTimeout>;
}

export function useMediaTracks(getCurrentMediaUrl?: () => string) {
  const audioTracks = ref<FyomMediaTrack[]>([]);
  const subTracks = ref<FyomMediaTrack[]>([]);

  const showAudioMenu = ref(false);
  const showSubMenu = ref(false);

  /** The "None (Off)" pseudo-track id for subtitles (matches soia's convention). */
  const SUB_OFF_ID = 0;

  let pendingTracksUpdate: TrackListPayload | null = null;
  let tracksUpdateFrame: number | null = null;
  let backgroundSubtitleQueue: BackgroundSubtitleItem[] = [];
  let isAddingBackgroundSubtitle = false;
  let backgroundSubtitleGeneration = 0;
  let trackUpdateWaiters: TrackUpdateWaiter[] = [];

  const getMediaKey = (explicitUrl?: string): string | null => {
    const url = (explicitUrl ?? getCurrentMediaUrl?.() ?? '').trim();
    if (!url) return null;
    return url;
  };

  const waitForBackgroundTrackGap = (): Promise<void> => {
    return new Promise((resolve) => {
      const gapMs =
        showAudioMenu.value || showSubMenu.value
          ? VISIBLE_MENU_TRACK_ADD_GAP_MS
          : BACKGROUND_TRACK_ADD_GAP_MS;
      setTimeout(resolve, gapMs);
    });
  };

  const waitForNextTracksUpdate = (timeoutMs = TRACK_UPDATE_WAIT_TIMEOUT_MS) => {
    let waiter: TrackUpdateWaiter | null = null;
    const promise = new Promise<void>((resolve) => {
      const finish = () => {
        if (!waiter) return;
        trackUpdateWaiters = trackUpdateWaiters.filter((item) => item !== waiter);
        clearTimeout(waiter.timeoutId);
        waiter = null;
        resolve();
      };
      waiter = {
        resolve: finish,
        timeoutId: setTimeout(finish, timeoutMs),
      };
      trackUpdateWaiters.push(waiter);
    });
    return {
      promise,
      cancel: () => {
        if (!waiter) return;
        trackUpdateWaiters = trackUpdateWaiters.filter((item) => item !== waiter);
        clearTimeout(waiter.timeoutId);
        waiter = null;
      },
    };
  };

  const notifyTrackUpdateWaiters = () => {
    const waiters = trackUpdateWaiters;
    trackUpdateWaiters = [];
    waiters.forEach((waiter) => waiter.resolve());
  };

  const clearBackgroundSubtitleQueue = () => {
    backgroundSubtitleGeneration += 1;
    backgroundSubtitleQueue = [];
    notifyTrackUpdateWaiters();
  };

  const runNextBackgroundSubtitle = async (): Promise<void> => {
    if (isAddingBackgroundSubtitle) return;
    if (!backgroundSubtitleQueue.length) return;
    const generation = backgroundSubtitleGeneration;
    const next = backgroundSubtitleQueue.shift();
    if (!next) return;
    if (getMediaKey() !== next.mediaKey) {
      clearBackgroundSubtitleQueue();
      return;
    }
    isAddingBackgroundSubtitle = true;
    const trackUpdateWaiter = waitForNextTracksUpdate();
    const added = await subAdd(next.path, next.mode);
    if (generation !== backgroundSubtitleGeneration) {
      trackUpdateWaiter.cancel();
      isAddingBackgroundSubtitle = false;
      void runNextBackgroundSubtitle();
      return;
    }
    if (added) {
      await trackUpdateWaiter.promise;
    } else {
      trackUpdateWaiter.cancel();
    }
    isAddingBackgroundSubtitle = false;
    if (generation !== backgroundSubtitleGeneration) {
      void runNextBackgroundSubtitle();
      return;
    }
    await waitForBackgroundTrackGap();
    void runNextBackgroundSubtitle();
  };

  const enqueueBackgroundSubtitles = (items: BackgroundSubtitleItem[]): void => {
    if (!items.length) return;
    backgroundSubtitleQueue.push(...items);
    void runNextBackgroundSubtitle();
  };

  /**
   * Process a `fyom://mpv/track-list` event payload. Batches updates via
   * `requestAnimationFrame` (ported verbatim from soia) so rapid track-list changes
   * (e.g. when adding multiple external subs) don't thrash the UI.
   */
  const handleTracksUpdate = (payload: TrackListPayload): void => {
    pendingTracksUpdate = payload;
    if (tracksUpdateFrame != null) return;

    const flushTracksUpdate = () => {
      tracksUpdateFrame = null;
      const latest = pendingTracksUpdate;
      pendingTracksUpdate = null;
      if (latest) {
        processTracksUpdate(latest);
      }
    };

    if (typeof window === 'undefined') {
      setTimeout(flushTracksUpdate, 0);
      return;
    }
    tracksUpdateFrame = window.requestAnimationFrame(flushTracksUpdate);
  };

  /** Apply a track-list payload to the refs (split into audio/sub + add the "Off" pseudo-track). */
  const processTracksUpdate = (payload: TrackListPayload): void => {
    const audioAll: FyomMediaTrack[] = payload.audio_tracks.map((t) => ({
      id: t.id,
      title: t.title || `Track ${t.id}`,
      lang: t.lang,
      type: 'audio' as const,
      selected: Boolean(t.selected),
      external: Boolean(t.external),
    }));
    audioTracks.value = audioAll;

    const subsAll: FyomMediaTrack[] = payload.sub_tracks.map((t) => ({
      id: t.id,
      title: t.title || `Track ${t.id}`,
      lang: t.lang,
      type: 'sub' as const,
      selected: Boolean(t.selected),
      external: Boolean(t.external),
    }));
    // Prepend the "None (Off)" pseudo-track (id 0) — matches soia's convention.
    subTracks.value = [
      {
        id: SUB_OFF_ID,
        title: 'Off',
        lang: '',
        type: 'sub' as const,
        selected: !subsAll.some((t) => t.selected),
        external: false,
      },
      ...subsAll,
    ];
    notifyTrackUpdateWaiters();
  };

  /** Select an audio track by id. */
  const selectAudio = async (track: FyomMediaTrack): Promise<void> => {
    showAudioMenu.value = false;
    await setAudioTrack(track.id);
  };

  /** Select a subtitle track by id (id 0 = off). */
  const selectSubTrack = async (track: FyomMediaTrack): Promise<void> => {
    showSubMenu.value = false;
    if (track.id === SUB_OFF_ID) {
      await setSubtitleTrack(null);
      return;
    }
    await setSubtitleTrack(track.id);
  };

  /**
   * Discover + auto-add external subtitles for the current media. Called on
   * `fyom://mpv/file-loaded` when the media is a LOCAL file.
   *
   * The best match is added with `mode="select"` (activated immediately); the rest are
   * added with `mode="auto"` (available in the subtitle picker but not activated).
   */
  const applyExternalSubtitlesForUrl = async (
    url: string,
    mediaTitle?: string,
  ): Promise<void> => {
    const mediaKey = getMediaKey(url);
    if (!mediaKey) return;
    clearBackgroundSubtitleQueue();

    const matches = await findExternalSubtitles(url, mediaTitle);
    if (getMediaKey() !== mediaKey) return;
    if (!matches.length) return;

    enqueueBackgroundSubtitles(
      matches.map((match, index) => ({
        path: match.path,
        mode: index === 0 ? 'select' : 'auto',
        mediaKey,
      })),
    );
  };

  /** Reset all track state (call on media change / unmount). */
  const resetTracks = (): void => {
    clearBackgroundSubtitleQueue();
    if (tracksUpdateFrame != null && typeof window !== 'undefined') {
      window.cancelAnimationFrame(tracksUpdateFrame);
    }
    tracksUpdateFrame = null;
    pendingTracksUpdate = null;
    audioTracks.value = [];
    subTracks.value = [];
    showAudioMenu.value = false;
    showSubMenu.value = false;
  };

  return {
    audioTracks,
    subTracks,
    showAudioMenu,
    showSubMenu,
    handleTracksUpdate,
    selectAudio,
    selectSubTrack,
    applyExternalSubtitlesForUrl,
    resetTracks,
    SUB_OFF_ID,
  };
}
