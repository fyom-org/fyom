/**
 * Reactive store for `fyom://mpv/*` native-playback events.
 *
 * PORTED_FROM_SOIA `src/composables/useAppPlaybackEvents.ts` (GPL-3.0-only).
 *
 * ## Adaptation
 * soia's `useAppPlaybackEvents` is an orchestrator that depends on injected
 * `player` / `history` / `nowPlaying` / `tracks` APIs (which fyom does not have yet —
 * they land in Phase 2.4/2.5). For Phase 2.2, fyom ports the *event-subscription*
 * essence as a lean reactive store: it exposes the raw mpv state (position, duration,
 * paused, volume, speed, tracks, chapters, buffering, …) as Vue refs that PlayerView
 * (and Phase 2.4/2.5 features) can consume. The soia-style orchestration
 * (history.markPlaybackStarted / nowPlaying.update / tracks.applyExternalTracks) is
 * deferred to Phase 2.5, where fyom's Go backend API will stand in for soia's stores.
 *
 * The Phase 9.7 guardrail is honored: this composable is additive — it only subscribes
 * to events; the `invoke('play_media')` / `invoke('stop_media')` contract + the
 * browser `<video>` fallback path are unchanged. Outside the Tauri runtime, `start()`
 * is a no-op (the refs stay at their defaults).
 */

import { onUnmounted, ref, type Ref } from 'vue';

import {
  subscribeMpvEvents,
  type MpvChapter,
  type MpvTrack,
} from '@/lib/player/native-player';

export interface UseAppPlaybackEventsReturn {
  /** Current playback position (seconds). */
  position: Ref<number>;
  /** Total duration (seconds). */
  duration: Ref<number>;
  /** Whether playback is paused. */
  paused: Ref<boolean>;
  /** Current volume (0–100). */
  volume: Ref<number>;
  /** Current playback speed (1.0 = normal). */
  speed: Ref<number>;
  /** Audio tracks reported by mpv. */
  audioTracks: Ref<MpvTrack[]>;
  /** Subtitle tracks reported by mpv. */
  subTracks: Ref<MpvTrack[]>;
  /** Chapter markers. */
  chapters: Ref<MpvChapter[]>;
  /** Whether playback is buffering (paused-for-cache or seeking). */
  buffering: Ref<boolean>;
  /** Whether a file is currently loaded. */
  fileLoaded: Ref<boolean>;
  /** Last mpv error message (null if none). */
  lastError: Ref<string | null>;
  /** Start subscribing (idempotent). Call from `onMounted` or after `play_media`. */
  start: () => Promise<void>;
  /** Stop subscribing + reset state (idempotent). Called automatically on unmount. */
  stop: () => void;
}

/**
 * Subscribe to `fyom://mpv/*` events + expose the state as reactive refs.
 *
 * Auto-cleans up on component unmount. Safe to call in a non-Tauri browser (it no-ops).
 *
 * @example
 * const mpv = useAppPlaybackEvents();
 * onMounted(() => mpv.start());
 * // in template: {{ mpv.position.value }} / {{ mpv.duration.value }}
 */
export function useAppPlaybackEvents(): UseAppPlaybackEventsReturn {
  const position = ref(0);
  const duration = ref(0);
  const paused = ref(true);
  const volume = ref(0);
  const speed = ref(1);
  const audioTracks = ref<MpvTrack[]>([]);
  const subTracks = ref<MpvTrack[]>([]);
  const chapters = ref<MpvChapter[]>([]);
  const buffering = ref(false);
  const fileLoaded = ref(false);
  const lastError = ref<string | null>(null);

  let unlisten: (() => void) | null = null;
  let active = false;

  async function start(): Promise<void> {
    if (active) return;
    active = true;
    unlisten = await subscribeMpvEvents({
      onTimePos: ({ position: p, duration: d }) => {
        position.value = p;
        duration.value = d;
      },
      onDuration: ({ duration: d }) => {
        duration.value = d;
      },
      onPause: ({ paused: p }) => {
        paused.value = p;
      },
      onVolume: ({ volume: v }) => {
        volume.value = v;
      },
      onSpeed: ({ speed: s }) => {
        speed.value = s;
      },
      onTrackList: ({ audio_tracks, sub_tracks }) => {
        audioTracks.value = audio_tracks;
        subTracks.value = sub_tracks;
      },
      onChapterList: ({ chapters: c }) => {
        chapters.value = c;
      },
      onPausedForCache: ({ paused: p }) => {
        buffering.value = p;
      },
      onFileLoaded: () => {
        fileLoaded.value = true;
        lastError.value = null;
      },
      onEndFile: () => {
        fileLoaded.value = false;
      },
      onSeek: () => {
        // Seek started — PlayerView may show a seeking indicator (Phase 2.4).
      },
      onPlaybackRestart: () => {
        // Playback restarted after a seek/load — state is consistent.
      },
      onShutdown: () => {
        fileLoaded.value = false;
      },
      onError: ({ message }) => {
        lastError.value = message;
      },
    });
  }

  function stop(): void {
    if (unlisten) {
      try {
        unlisten();
      } catch {
        // ignore — best-effort teardown
      }
      unlisten = null;
    }
    active = false;
  }

  // Auto-cleanup when the hosting component unmounts.
  onUnmounted(stop);

  return {
    position,
    duration,
    paused,
    volume,
    speed,
    audioTracks,
    subTracks,
    chapters,
    buffering,
    fileLoaded,
    lastError,
    start,
    stop,
  };
}
