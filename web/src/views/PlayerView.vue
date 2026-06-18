<template>
  <main class="player-view" :class="{ 'video-mode': isVideoModeActive }">
    <PlayerFallbackNotice
      v-if="showFallbackBanner"
      :message="$t('player.nativeUnavailable')"
      class="fallback-banner"
    />

    <section v-if="error" class="error-state" role="alert">
      <h1>{{ $t('player.unableToPlay') }}</h1>
      <p>{{ error }}</p>

      <div class="error-actions">
        <button type="button" class="error-btn" @click="reloadCurrentMedia">
          {{ $t('player.retry') }}
        </button>

        <router-link v-if="mediaId" :to="`/media/${mediaId}`" class="error-link">
          {{ $t('player.backToDetails') }}
        </router-link>

        <router-link to="/library" class="error-link"> {{ $t('player.backToLibrary') }} </router-link>
      </div>
    </section>

    <section v-else class="player-surface">
      <div v-if="isLoading" class="loading">
        <span class="spinner" aria-hidden="true"></span>
        <span>{{ loadingLabel }}</span>
      </div>

      <video
        v-else-if="showBrowserPlayer && streamUrl"
        ref="videoRef"
        :src="streamUrl"
        controls
        autoplay
        playsinline
        class="video-player"
        @timeupdate="onTimeUpdate"
        @pause="onPause"
        @ended="onEnded"
        @loaded-metadata="onLoadedMetadata"
        @error="onVideoError"
      >
        {{ $t('player.browserNotSupported') }}
      </video>

      <div v-else-if="isNativeReady" class="native-surface">
        <!-- Phase 2.3: the mpv GL layer renders behind this transparent webview root.
             The `.video-mode` class on `.player-view` sets `background: transparent !important`
             so the native NSOpenGL / WGL / GLX layer (created by `attach_render_surface`)
             shows through. HTML controls overlay on top with their own opaque backgrounds. -->
        <span v-if="!isVideoModeActive" class="native-status">{{ $t('player.nativeActive') }}</span>
        <span v-if="!isVideoModeActive" class="native-subtitle"> {{ $t('player.nativeRunning') }} </span>

        <!-- Phase 2.4: HTML controls overlay on top of the transparent webview.
             Only shown when video-mode is active (the GL layer is visible). -->
        <PlayerControls
          v-if="isVideoModeActive"
          :state="controlsState"
          @toggle-pause="onTogglePause"
          @seek="onSeek"
          @seek-relative="onSeekRelative"
          @set-volume="onSetVolume"
          @set-speed="onSetSpeed"
          @select-audio="onSelectAudio"
          @select-sub="onSelectSub"
          @set-color-adjustment="onSetColorAdjustment"
          @set-sub-delay="onSetSubDelay"
          @set-audio-delay="onSetAudioDelay"
          @set-sub-scale="onSetSubScale"
          @set-global-color="onSetGlobalColor"
          @set-chapter="onSetChapter"
          @toggle-fullscreen="onToggleFullscreen"
          @reset-adjustments="onResetAdjustments"
        />
      </div>

      <div v-else class="loading">
        <span>{{ $t('player.loadingPlayer') }}</span>
      </div>
    </section>
  </main>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue';
import { useRoute } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { getApiErrorMessage, getHttpStatus, getMediaDetail, type MediaItem } from '@/api/library';
import {
  attachRenderSurface,
  createInitialNativePlayerState,
  isNativePlaybackRuntimeAvailable,
  resizeRenderSurface,
  setVideoMode,
  subscribeMpvEvents,
  tryInitializeNativePlayer,
  type MpvChapter,
  type NativePlayerState,
} from '@/lib/player/native-player';
import {
  setAudioTrack as bridgeSetAudioTrack,
  setChapter as bridgeSetChapter,
  setSubtitleTrack as bridgeSetSubtitleTrack,
  setVolume as bridgeSetVolume,
  seek as bridgeSeek,
  seekRelative as bridgeSeekRelative,
  togglePause as bridgeTogglePause,
  mpvKeypress as bridgeMpvKeypress,
} from '@/lib/player/native-player';
import { useMediaTracks } from '@/composables/useMediaTracks';
import { usePlaybackAdjustments } from '@/composables/usePlaybackAdjustments';
import { usePlaybackSpeed } from '@/composables/usePlaybackSpeed';
import { usePlaybackHistory } from '@/composables/usePlaybackHistory';
import PlayerControls, { type PlayerControlsState } from '@/components/PlayerControls.vue';
import PlayerFallbackNotice from '@/components/PlayerFallbackNotice.vue';

interface ProgressPayload {
  position: number;
  duration: number;
  finished: boolean;
}

interface TauriInvokeApi {
  invoke?: (command: string, args?: Record<string, unknown>) => Promise<unknown>;
}

interface TauriInternals {
  tauri?: TauriInvokeApi;
}

interface TauriWindow extends Window {
  __TAURI_INTERNALS__?: TauriInternals;
}

const PROGRESS_REPORT_INTERVAL_SECONDS = 10;

const route = useRoute();
const { t } = useI18n();

const videoRef = ref<HTMLVideoElement | null>(null);
const streamUrl = ref('');
const error = ref('');
const loadingMedia = ref(false);

const nativePlayerState = shallowRef<NativePlayerState>(createInitialNativePlayerState());

const nativeInitAttempted = ref(false);
const lastReportedPosition = ref(0);
const progressRequestInFlight = ref(false);
const pendingProgressPayload = ref<ProgressPayload | null>(null);

// Phase 2.5: resume position fetched from the Go backend before playback starts.
// Consumed (reset to 0) once the seek lands — in `onFileLoaded` for native mpv,
// or `onLoadedMetadata` for the HTML5 `<video>` fallback.
const resumePosition = ref(0);

let loadGeneration = 0;
let disposed = false;
let mpvEventsUnlisten: (() => void) | null = null;

const mediaId = computed(() => {
  const id = route.params.id;

  return typeof id === 'string' ? id : '';
});

const isNativeAvailable = computed(() => isNativePlaybackRuntimeAvailable());
const isInitializing = computed(() => nativePlayerState.value.status === 'initializing');
const isNativeReady = computed(() => nativePlayerState.value.status === 'ready');
const isNativeFailed = computed(() => nativePlayerState.value.status === 'failed');
const isNativeUnavailable = computed(() => nativePlayerState.value.status === 'unavailable');
const isNativeIdle = computed(() => nativePlayerState.value.status === 'idle');

const showFallbackBanner = computed(() => {
  return isNativeAvailable.value && isNativeFailed.value && showBrowserPlayer.value;
});

const isLoading = computed(() => {
  return loadingMedia.value || isInitializing.value;
});

const loadingLabel = computed(() => {
  if (loadingMedia.value) return t('player.loadingMedia');
  if (isInitializing.value) return t('player.initializingNative');

  return t('player.loadingPlayer');
});

const showBrowserPlayer = computed(() => {
  if (error.value) return false;
  if (isInitializing.value) return false;
  if (!streamUrl.value) return false;

  if (isNativeFailed.value) return true;
  if (isNativeUnavailable.value) return true;
  if (isNativeIdle.value) return true;

  return false;
});

/**
 * Phase 2.3: when the native player is ready + a stream URL is loaded, activate
 * `.video-mode` (transparent webview root) so the mpv GL layer shows through.
 *
 * Deactivates when:
 * - The native player is not ready (init failed / unavailable / idle).
 * - An error is showing (the error overlay should be opaque for readability).
 * - The browser `<video>` fallback is active (no GL layer to show).
 */
const isVideoModeActive = computed(() => {
  if (error.value) return false;
  if (!isNativeReady.value) return false;
  if (showBrowserPlayer.value) return false;
  if (!streamUrl.value) return false;
  return true;
});

// Phase 2.3: tracks whether `attach_render_surface` has been called (idempotent guard
// so we don't re-attach on every `mediaId` watch).
const renderSurfaceAttached = ref(false);

// ---------------------------------------------------------------------------
// Phase 2.4: mpv event-driven playback state + composable instances.
//
// The mpv event subscription (`subscribeMpvEvents`) drives all native playback state:
// `isPaused`, `currentTime`, `duration`, `volume`, `speed`, `audioTracks`, `subTracks`,
// `currentAudioId`, `currentSubId`, `chapters`, `currentChapter`, `hwdec`, and the
// color-adjustment / A/V-delay values. The HTML controls (PlayerControls.vue) render
// this state + emit user actions, which the handlers below translate back into mpv
// commands via the `native-player.ts` bridge.
// ---------------------------------------------------------------------------

const isPaused = ref(true);
const currentTime = ref(0);
const duration = ref(0);
const volume = ref(80);
const currentAudioId = ref(0);
const currentSubId = ref(0);
const chapters = ref<MpvChapter[]>([]);
const currentChapter = ref(-1);
const hwdec = ref('');
const isBuffering = ref(false);

// Composable instances (track management + adjustments + speed + history).
const tracksComposable = useMediaTracks(() => streamUrl.value);
const adjustmentsComposable = usePlaybackAdjustments();
const speedComposable = usePlaybackSpeed();
const historyComposable = usePlaybackHistory();

/**
 * The full playback state rendered by PlayerControls. Computed from the reactive refs
 * + composable state above. PlayerControls is a pure presentational component.
 */
const controlsState = computed<PlayerControlsState>(() => ({
  isPaused: isPaused.value,
  currentTime: currentTime.value,
  duration: duration.value,
  volume: volume.value,
  speed: speedComposable.currentSpeed.value,
  audioTracks: tracksComposable.audioTracks.value,
  subTracks: tracksComposable.subTracks.value,
  currentAudioId: currentAudioId.value,
  currentSubId: currentSubId.value,
  chapters: chapters.value,
  currentChapter: currentChapter.value,
  hwdec: hwdec.value,
  brightness: adjustmentsComposable.brightness.value,
  contrast: adjustmentsComposable.contrast.value,
  saturation: adjustmentsComposable.saturation.value,
  gamma: adjustmentsComposable.gamma.value,
  hue: adjustmentsComposable.hue.value,
  subDelay: adjustmentsComposable.subDelay.value,
  audioDelay: adjustmentsComposable.audioDelay.value,
  subScale: adjustmentsComposable.subScale.value,
  globalColorAdjustmentsEnabled: adjustmentsComposable.globalColorAdjustmentsEnabled.value,
  isBuffering: isBuffering.value,
}));

/**
 * Subscribe to `fyom://mpv/*` events. Called once on mount when native playback is
 * available. The unlisten function is stored for `onBeforeUnmount` cleanup.
 */
async function setupMpvEventSubscription(): Promise<void> {
  if (mpvEventsUnlisten) return;
  if (!isNativePlaybackRuntimeAvailable()) return;

  mpvEventsUnlisten = await subscribeMpvEvents({
    onTimePos: ({ position, duration: dur }) => {
      currentTime.value = position;
      if (dur > 0) duration.value = dur;
      // Phase 2.5: 10s-throttled progress report for native playback, replacing
      // the HTML5 `onTimeUpdate` path when native is active. The finish
      // threshold (90%) forces an immediate report so the watched status lands.
      void maybeReportNativeProgress();
    },
    onDuration: ({ duration: dur }) => {
      duration.value = dur;
    },
    onPause: ({ paused }) => {
      isPaused.value = paused;
      // Phase 2.5: flush progress on pause so the latest position survives a
      // reload / app quit (the 10s throttle would otherwise drop the tail).
      if (paused) {
        void flushProgressFromMpv(false);
      }
    },
    onVolume: ({ volume: vol }) => {
      volume.value = vol;
    },
    onSpeed: ({ speed }) => {
      speedComposable.reconcileSpeed(speed);
    },
    onTrackList: (e) => {
      // The Phase 2.4 event payload includes `selected`/`external`/`src_id` (the
      // `MpvTrack` type was extended). Feed the composable + reconcile current track ids.
      const payload = {
        audio_tracks: e.audio_tracks,
        sub_tracks: e.sub_tracks,
      };
      tracksComposable.handleTracksUpdate(payload);
      // Reconcile current track ids from the selected flags.
      const selectedAudio = payload.audio_tracks.find((t) => t.selected);
      currentAudioId.value = selectedAudio ? selectedAudio.id : 0;
      const selectedSub = payload.sub_tracks.find((t) => t.selected);
      currentSubId.value = selectedSub ? selectedSub.id : 0;
    },
    onChapterList: ({ chapters: chs }) => {
      chapters.value = chs;
    },
    onChapter: ({ index }) => {
      currentChapter.value = index;
    },
    onHwdecCurrent: ({ hwdec: hw }) => {
      hwdec.value = hw;
    },
    onAid: ({ id }) => {
      currentAudioId.value = id;
    },
    onSid: ({ id }) => {
      currentSubId.value = id;
    },
    onSubDelay: ({ delay }) => {
      adjustmentsComposable.reconcileFromMpv({ subDelay: delay });
    },
    onAudioDelay: ({ delay }) => {
      adjustmentsComposable.reconcileFromMpv({ audioDelay: delay });
    },
    onBrightness: ({ value }) => {
      adjustmentsComposable.reconcileFromMpv({ brightness: value });
    },
    onContrast: ({ value }) => {
      adjustmentsComposable.reconcileFromMpv({ contrast: value });
    },
    onSaturation: ({ value }) => {
      adjustmentsComposable.reconcileFromMpv({ saturation: value });
    },
    onGamma: ({ value }) => {
      adjustmentsComposable.reconcileFromMpv({ gamma: value });
    },
    onHue: ({ value }) => {
      adjustmentsComposable.reconcileFromMpv({ hue: value });
    },
    onPausedForCache: ({ paused }) => {
      isBuffering.value = paused;
    },
    onFileLoaded: ({ path }) => {
      // Phase 2.4: on file-loaded, apply per-media color adjustments + auto-discover
      // external subtitles (LOCAL-only — `findExternalSubtitles` returns [] for remote).
      void adjustmentsComposable.applyColorAdjustmentsForMedia(path || streamUrl.value);
      if (path) {
        void tracksComposable.applyExternalSubtitlesForUrl(path);
      }
      // Phase 2.5: resume from saved position. `resumePosition` is fetched in
      // `loadMedia` before `play_media` is invoked, so it's available here. The
      // seek is absolute (mpv `seek <pos> absolute`); mpv accepts it post-
      // file-loaded. Consumed once so a later re-load of the same file (without
      // a fresh `loadMedia`) starts from 0.
      if (resumePosition.value > 0) {
        const pos = resumePosition.value;
        resumePosition.value = 0;
        void bridgeSeek(pos).then((ok) => {
          if (!ok) return;
          currentTime.value = pos;
          lastReportedPosition.value = pos;
        });
      }
    },
    onEndFile: ({ reason }) => {
      // Phase 2.5: on EOF (reason=0), flush progress with `finished:true`. The
      // Go backend's `UpdateProgress` auto-transitions the user status to
      // `watched` when `Finished && Position > 0`, so no separate status write
      // is needed. Other reasons (stop/quit/error/redirect) flush the current
      // position without marking finished.
      if (reason === 0) {
        void flushProgressFromMpv(true);
      } else {
        void flushProgressFromMpv(false);
      }
      tracksComposable.resetTracks();
      currentTime.value = 0;
      currentChapter.value = -1;
      chapters.value = [];
    },
    onPlaybackRestart: () => {
      // Seek completed — mpv is rendering again.
      isBuffering.value = false;
    },
    onError: ({ message }) => {
      console.warn('[fyom] mpv error:', message);
    },
  });
}

// ---------------------------------------------------------------------------
// Phase 2.4: PlayerControls event handlers (translate UI actions → mpv commands).
// ---------------------------------------------------------------------------

const onTogglePause = (): void => {
  void bridgeTogglePause();
};

const onSeek = (position: number): void => {
  void bridgeSeek(position).then((ok) => {
    if (ok) currentTime.value = position;
  });
};

const onSeekRelative = (delta: number): void => {
  void bridgeSeekRelative(delta);
};

const onSetVolume = (vol: number): void => {
  void bridgeSetVolume(vol);
  volume.value = vol;
};

const onSetSpeed = (speed: number): void => {
  void speedComposable.setSpeed(speed);
};

const onSelectAudio = (trackId: number): void => {
  void bridgeSetAudioTrack(trackId);
  currentAudioId.value = trackId;
};

const onSelectSub = (trackId: number): void => {
  if (trackId === 0) {
    void bridgeSetSubtitleTrack(null);
  } else {
    void bridgeSetSubtitleTrack(trackId);
  }
  currentSubId.value = trackId;
};

const onSetColorAdjustment = (
  name: 'brightness' | 'contrast' | 'saturation' | 'gamma' | 'hue',
  value: number,
): void => {
  // Dispatch to the composable (which calls the bridge + persists if global is on).
  switch (name) {
    case 'brightness':
      void adjustmentsComposable.setBrightness(value);
      break;
    case 'contrast':
      void adjustmentsComposable.setContrast(value);
      break;
    case 'saturation':
      void adjustmentsComposable.setSaturation(value);
      break;
    case 'gamma':
      void adjustmentsComposable.setGamma(value);
      break;
    case 'hue':
      void adjustmentsComposable.setHue(value);
      break;
  }
};

const onSetSubDelay = (seconds: number): void => {
  void adjustmentsComposable.setSubDelay(seconds);
};

const onSetAudioDelay = (seconds: number): void => {
  void adjustmentsComposable.setAudioDelay(seconds);
};

const onSetSubScale = (scale: number): void => {
  void adjustmentsComposable.setSubScale(scale);
};

const onSetGlobalColor = (enabled: boolean): void => {
  void adjustmentsComposable.setGlobalColorAdjustmentsEnabled(enabled);
};

const onSetChapter = (index: number): void => {
  void bridgeSetChapter(index);
};

const onResetAdjustments = (): void => {
  void adjustmentsComposable.setBrightness(0);
  void adjustmentsComposable.setContrast(0);
  void adjustmentsComposable.setSaturation(0);
  void adjustmentsComposable.setGamma(0);
  void adjustmentsComposable.setHue(0);
  void adjustmentsComposable.setSubDelay(0);
  void adjustmentsComposable.setAudioDelay(0);
  void adjustmentsComposable.setSubScale(1.0);
};

const onToggleFullscreen = (): void => {
  // Toggle the browser fullscreen (works in both Tauri + plain browser).
  if (document.fullscreenElement) {
    void document.exitFullscreen();
  } else {
    void document.documentElement.requestFullscreen();
  }
};

// Keyboard shortcuts (ported from soia's `usePlaybackShortcuts`, simplified).
const onKeyDown = (event: KeyboardEvent): void => {
  if (!isNativeReady.value) return;
  // Don't intercept when the user is typing in an input/textarea.
  const target = event.target as HTMLElement | null;
  if (target && (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable)) {
    return;
  }

  switch (event.key) {
    case ' ':
    case 'k':
      event.preventDefault();
      onTogglePause();
      break;
    case 'ArrowLeft':
      event.preventDefault();
      onSeekRelative(-5);
      break;
    case 'ArrowRight':
      event.preventDefault();
      onSeekRelative(5);
      break;
    case 'ArrowUp':
      event.preventDefault();
      onSetVolume(Math.min(100, volume.value + 5));
      break;
    case 'ArrowDown':
      event.preventDefault();
      onSetVolume(Math.max(0, volume.value - 5));
      break;
    case 'f':
      event.preventDefault();
      onToggleFullscreen();
      break;
    case 'm':
      event.preventDefault();
      onSetVolume(volume.value > 0 ? 0 : 80);
      break;
    default:
      // Forward other keys to mpv (mpv keystr format — e.g. "Space", "Ctrl+Right").
      // This lets power-user mpv bindings work (e.g. "j" for subtitle cycle, "#" for
      // audio cycle). The frontend assembles the keystr.
      if (event.key.length === 1 || event.key.startsWith('Arrow') === false) {
        const keystr = event.key === ' ' ? 'Space' : event.key;
        void bridgeMpvKeypress(keystr);
      }
      break;
  }
};

// Phase 2.3: window resize listener (notifies backend to update the GL drawable).
const handleWindowResize = (): void => {
  const w = window.innerWidth;
  const h = window.innerHeight;
  const dpr = window.devicePixelRatio || 1;
  void resizeRenderSurface(w, h, dpr);
};

onMounted(() => {
  window.addEventListener('resize', handleWindowResize, { passive: true });
  window.addEventListener('keydown', onKeyDown);
  // Phase 2.4: subscribe to mpv events early (the subscription is a no-op outside
  // Tauri, so it's safe to call before native init completes — the events will start
  // flowing once `play_media` succeeds).
  void setupMpvEventSubscription();
  void reloadCurrentMedia();
});

watch(
  () => mediaId.value,
  async (nextId, previousId) => {
    if (!nextId || nextId === previousId) return;

    await teardownCurrentPlayback();
    void reloadCurrentMedia();
  }
);

// Phase 2.3: when `.video-mode` toggles, notify the backend (informational — the CSS is
// the actual mechanism; the backend logs the transition).
watch(isVideoModeActive, (active) => {
  void setVideoMode(active);
});

onBeforeUnmount(() => {
  disposed = true;
  window.removeEventListener('resize', handleWindowResize);
  window.removeEventListener('keydown', onKeyDown);
  // Phase 2.3: ensure `.video-mode` is disabled when leaving the player view (so the
  // webview root goes back to opaque for the rest of the app).
  void setVideoMode(false);
  // Phase 2.4: reset track state + tear down mpv event subscription.
  tracksComposable.resetTracks();
  if (mpvEventsUnlisten) {
    mpvEventsUnlisten();
    mpvEventsUnlisten = null;
  }
  // Phase 2.5: flush the latest progress on exit so it survives a reload / app
  // quit. Mode-aware: native mpv reads the reactive refs, HTML5 reads the
  // `<video>` element. Both honor the 90% finish threshold.
  if (isNativeReady.value) {
    void flushProgressFromMpv(false);
  } else {
    void flushProgressFromVideo(false);
  }
  void teardownCurrentPlayback();
});

async function reloadCurrentMedia(): Promise<void> {
  if (!mediaId.value) {
    error.value = t('player.invalidMediaId');
    return;
  }

  await loadMedia(mediaId.value);
}

function resetViewState(): void {
  streamUrl.value = '';
  error.value = '';
  loadingMedia.value = false;
  nativePlayerState.value = createInitialNativePlayerState();
  nativeInitAttempted.value = false;
  lastReportedPosition.value = 0;
  progressRequestInFlight.value = false;
  pendingProgressPayload.value = null;
  // Phase 2.5: reset resume position (a stale value would seek the next media).
  resumePosition.value = 0;
  // Phase 2.4: reset playback state + tracks + chapters.
  isPaused.value = true;
  currentTime.value = 0;
  duration.value = 0;
  volume.value = 80;
  currentAudioId.value = 0;
  currentSubId.value = 0;
  chapters.value = [];
  currentChapter.value = -1;
  hwdec.value = '';
  isBuffering.value = false;
  tracksComposable.resetTracks();
}

async function loadMedia(id: string): Promise<void> {
  const generation = ++loadGeneration;

  resetViewState();
  loadingMedia.value = true;

  try {
    const media = await getMediaDetail(id);

    if (generation !== loadGeneration || disposed) return;

    const resolvedStreamUrl = extractStreamUrl(media);

    if (!resolvedStreamUrl) {
      error.value = t('player.noStream');
      nativePlayerState.value = {
        status: 'unavailable',
        failure: null,
        attempted: false,
      };
      return;
    }

    streamUrl.value = resolvedStreamUrl;

    // Phase 2.5: fetch the saved resume position before native init / `<video>`
    // load. `fetchResumePosition` applies soia's 0.99 skip-resume rule (restart
    // from 0 if the user already finished). Best-effort: a network failure
    // leaves `resumePosition` at 0 (play from start). Awaited so the value is
    // available when `onFileLoaded` (native) / `onLoadedMetadata` (HTML5) fires.
    try {
      const resume = await historyComposable.fetchResumePosition(id);
      if (generation !== loadGeneration || disposed) return;
      resumePosition.value = resume?.position ?? 0;
    } catch {
      // Resume is best-effort — ignore failures + play from the start.
    }

    await nextTick();

    if (generation !== loadGeneration || disposed) return;

    // Media has loaded — clear the "Loading media..." flag BEFORE starting
    // native init so the loading label can switch to the more specific
    // "Initializing native player..." text (see loadingLabel computed).
    // Without this, loadingMedia stays true for the entire duration of
    // attemptNativeInit (which awaits the invoke promise), and the
    // "Initializing native player" branch in loadingLabel is unreachable.
    loadingMedia.value = false;

    await attemptNativeInit(generation);
  } catch (unknownError) {
    if (generation !== loadGeneration || disposed) return;

    const status = getHttpStatus(unknownError);

    if (status === 401 || status === 403) {
      error.value = t('player.notAuthorized');
      return;
    }

    console.error('[fyom] player load media failed:', unknownError);
    error.value = getApiErrorMessage(unknownError, t('player.loadFailed'));
  } finally {
    if (generation === loadGeneration && !disposed) {
      loadingMedia.value = false;
    }
  }
}

function extractStreamUrl(media: MediaItem): string {
  const raw = media.stream_url;

  return typeof raw === 'string' ? raw : '';
}

async function attemptNativeInit(generation: number): Promise<void> {
  if (nativeInitAttempted.value) return;

  nativeInitAttempted.value = true;

  if (!streamUrl.value || !isNativePlaybackRuntimeAvailable()) {
    nativePlayerState.value = {
      status: 'unavailable',
      failure: null,
      attempted: false,
    };
    return;
  }

  nativePlayerState.value = {
    status: 'initializing',
    failure: null,
    attempted: true,
  };

  const result = await tryInitializeNativePlayer({
    mediaUrl: streamUrl.value,
  });

  if (generation !== loadGeneration || disposed) return;

  if (result.ok) {
    nativePlayerState.value = {
      status: 'ready',
      failure: null,
      attempted: true,
    };

    // Phase 2.3: native player init succeeded — attach the GL render surface to the main
    // window + spawn the mpv render thread. On failure, log + fall back to the `<video>`
    // path (the 9.7 guardrail: native playback is an enhancement, never a regression).
    if (!renderSurfaceAttached.value) {
      renderSurfaceAttached.value = true;
      const surfaceResult = await attachRenderSurface();
      if (generation !== loadGeneration || disposed) return;

      if (!surfaceResult.ok) {
        console.warn(
          '[fyom] render surface attach failed — keeping native audio + black video:',
          surfaceResult.failure.stage,
          surfaceResult.failure.message,
        );
        // Note: do NOT mark the native player as failed here — `play_media` succeeded, so
        // mpv is playing audio (with a black video frame). The `<video>` fallback is NOT
        // triggered; the user gets audio + a status overlay. Phase 2.4 may revisit this
        // (e.g. show a "video rendering unavailable, audio playing" notice).
      }
    }
    return;
  }

  nativePlayerState.value = {
    status: 'failed',
    failure: result.failure,
    attempted: true,
  };

  console.warn(
    '[fyom] native player initialization failed:',
    result.failure.stage,
    result.failure.message
  );
}

function buildProgressPayload(
  currentTimeSeconds: number,
  durationSeconds: number,
  finished: boolean
): ProgressPayload | null {
  const duration = Math.floor(durationSeconds || 0);

  if (!Number.isFinite(duration) || duration <= 0) {
    return null;
  }

  const currentTime = Math.floor(currentTimeSeconds || 0);
  const position = finished ? duration : Math.min(duration, Math.max(0, currentTime));

  return {
    position,
    duration,
    finished,
  };
}

async function reportProgress(payload: ProgressPayload): Promise<void> {
  if (!mediaId.value) return;

  if (progressRequestInFlight.value) {
    pendingProgressPayload.value = payload;
    return;
  }

  progressRequestInFlight.value = true;

  try {
    // Phase 2.5: delegate to the history composable (which wraps
    // `setMediaProgress` — 401/403/404 are swallowed there since progress is
    // best-effort). The in-flight queue + pending-payload retry stays here so
    // it remains coupled to this component's `disposed` lifecycle flag.
    await historyComposable.persistProgress(mediaId.value, payload);
  } catch (unknownError) {
    console.warn(
      '[fyom] player progress update failed:',
      getApiErrorMessage(unknownError, t('player.progressUpdateFailed'))
    );
  } finally {
    progressRequestInFlight.value = false;

    const nextPayload = pendingProgressPayload.value;
    pendingProgressPayload.value = null;

    if (nextPayload && !disposed) {
      void reportProgress(nextPayload);
    }
  }
}

function onTimeUpdate(): void {
  const video = videoRef.value;
  if (!video) return;

  const currentTime = Math.floor(video.currentTime || 0);
  const delta = Math.abs(currentTime - lastReportedPosition.value);
  const finished = historyComposable.isFinished(video.currentTime, video.duration || 0);

  // Phase 2.5: force a report when the finish threshold is crossed (so the
  // watched status lands promptly), otherwise throttle to the 10s interval.
  if (delta < PROGRESS_REPORT_INTERVAL_SECONDS && !finished) {
    return;
  }

  const payload = buildProgressPayload(video.currentTime, video.duration || 0, finished);

  if (!payload) return;

  lastReportedPosition.value = payload.position;
  void reportProgress(payload);
}

function onPause(): void {
  void flushProgressFromVideo(false);
}

function onEnded(): void {
  void flushProgressFromVideo(true);
}

/**
 * Phase 2.5: HTML5 `<video>` metadata loaded — seek to the saved resume
 * position (if any) before the user sees playback start from 0. Mirrors the
 * native `onFileLoaded` resume path so the fallback UX matches native.
 */
function onLoadedMetadata(): void {
  if (resumePosition.value <= 0) return;

  const video = videoRef.value;
  if (!video) return;

  const pos = resumePosition.value;
  resumePosition.value = 0;

  try {
    video.currentTime = pos;
    lastReportedPosition.value = Math.floor(pos);
  } catch {
    // Some streams reject seeking before playback starts — ignore + play from 0.
  }
}

async function flushProgressFromVideo(finished: boolean): Promise<void> {
  const video = videoRef.value;
  if (!video) return;

  // Phase 2.5: a `false` flush (pause / unmount) still marks the item finished
  // when the position has crossed the 90% threshold, so pausing near the end
  // credits counts as watched.
  const effectiveFinished =
    finished || historyComposable.isFinished(video.currentTime, video.duration || 0);

  const payload = buildProgressPayload(video.currentTime, video.duration || 0, effectiveFinished);

  if (!payload) return;

  lastReportedPosition.value = payload.position;
  await reportProgress(payload);
}

/**
 * Phase 2.5: native mpv progress flush. Reads the `currentTime` / `duration`
 * reactive refs (driven by `fyom://mpv/time-pos` + `fyom://mpv/duration`) —
 * these are the source of truth in native mode (`videoRef` is null there).
 */
async function flushProgressFromMpv(finished: boolean): Promise<void> {
  if (!isNativeReady.value) return;

  const effectiveFinished =
    finished || historyComposable.isFinished(currentTime.value, duration.value);

  const payload = buildProgressPayload(currentTime.value, duration.value, effectiveFinished);

  if (!payload) return;

  lastReportedPosition.value = payload.position;
  await reportProgress(payload);
}

/**
 * Phase 2.5: 10s-throttled progress report for native playback, invoked from
 * the `onTimePos` mpv event handler. Mirrors the HTML5 `onTimeUpdate` path:
 * throttle by `PROGRESS_REPORT_INTERVAL_SECONDS`, but force a report when the
 * finish threshold is crossed so the watched status lands promptly.
 */
function maybeReportNativeProgress(): void {
  if (!mediaId.value || !isNativeReady.value) return;

  const pos = currentTime.value;
  const dur = duration.value;
  const finished = historyComposable.isFinished(pos, dur);
  const delta = Math.abs(pos - lastReportedPosition.value);

  if (delta < PROGRESS_REPORT_INTERVAL_SECONDS && !finished) {
    return;
  }

  const payload = buildProgressPayload(pos, dur, finished);

  if (!payload) return;

  lastReportedPosition.value = payload.position;
  void reportProgress(payload);
}

function onVideoError(): void {
  if (isNativeFailed.value || isNativeUnavailable.value || isNativeIdle.value) {
    error.value = t('player.browserPlaybackFailed');
    return;
  }

  error.value = t('player.playbackFailed');
}

async function teardownCurrentPlayback(): Promise<void> {
  loadGeneration += 1;

  await stopNativePlayer();

  const video = videoRef.value;

  if (video) {
    try {
      video.pause();
      video.removeAttribute('src');
      video.load();
    } catch {
      // Ignore browser cleanup failures.
    }
  }
}

async function stopNativePlayer(): Promise<void> {
  if (!isNativeReady.value) return;

  try {
    const tauriInvoke = getTauriInvoke();

    if (tauriInvoke) {
      await tauriInvoke('stop_media');
    }
  } catch {
    // Ignore native cleanup failures.
  } finally {
    if (!disposed) {
      nativePlayerState.value = createInitialNativePlayerState();
    }
  }
}

function getTauriInvoke(): TauriInvokeApi['invoke'] | null {
  if (typeof window === 'undefined') return null;

  const tauriWindow = window as TauriWindow;
  const invoke = tauriWindow.__TAURI_INTERNALS__?.tauri?.invoke;

  return typeof invoke === 'function' ? invoke : null;
}
</script>

<style scoped>
.player-view {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  color: #e0e0e0;
  background: #000;
}

/**
 * Phase 2.3: `.video-mode` — when active, the player-view's background goes transparent
 * so the native mpv GL layer (rendered behind the webview by `attach_render_surface`)
 * shows through. This is soia's z-order trick, render-backend-agnostic — it worked for
 * soia's Vulkan layer underneath, and it works identically for fyom's OpenGL layer.
 *
 * PORTED_FROM_SOIA `src/styles/app-shell.css::video-mode` (verbatim):
 *   .video-mode {
 *       background-color: transparent !important;
 *       background-image: none !important;
 *       box-shadow: none !important;
 *   }
 *
 * HTML controls (`.native-surface`, `.fallback-banner`, `.error-state`, etc.) keep their
 * own opaque backgrounds so they remain readable on top of the video.
 */
.player-view.video-mode {
  background-color: transparent !important;
  background-image: none !important;
  box-shadow: none !important;
}

.player-view.video-mode .player-surface {
  background-color: transparent !important;
}

.fallback-banner {
  position: fixed;
  top: 0;
  right: 0;
  left: 0;
  z-index: 100;
}

.player-surface {
  width: 100%;
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
}

.video-player {
  width: 100%;
  max-width: 100vw;
  max-height: 100vh;
  display: block;
  background: #000;
}

.native-surface {
  min-height: 240px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-direction: column;
  color: #aaaacc;
  gap: 8px;
  text-align: center;
}

.native-status {
  color: #f0f0ff;
  font-size: 18px;
  font-weight: 800;
}

.native-subtitle {
  color: #666688;
  font-size: 13px;
}

.loading {
  color: #8888aa;
  font-size: 18px;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 16px;
}

.spinner {
  width: 32px;
  height: 32px;
  box-sizing: border-box;
  border: 3px solid #333344;
  border-top-color: #8888aa;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

.error-state {
  min-height: 100vh;
  box-sizing: border-box;
  display: grid;
  place-items: center;
  padding: 24px;
  text-align: center;
}

.error-state h1 {
  margin: 0 0 8px;
  color: #ffb3b3;
  font-size: 24px;
}

.error-state p {
  max-width: 520px;
  margin: 0;
  color: #aaaacc;
  font-size: 14px;
  line-height: 1.55;
}

.error-actions {
  display: flex;
  justify-content: center;
  flex-wrap: wrap;
  gap: 10px;
  margin-top: 18px;
}

.error-btn,
.error-link {
  min-height: 38px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  box-sizing: border-box;
  padding: 9px 14px;
  border-radius: 8px;
  font-size: 13px;
  font-weight: 800;
}

.error-btn {
  color: #fff;
  background: #6c63ff;
  border: 0;
  cursor: pointer;
}

.error-btn:hover {
  background: #5a52e0;
}

.error-link {
  color: #ccccee;
  background: #1a1a2e;
  border: 1px solid #2a2a3e;
  text-decoration: none;
}

.error-link:hover {
  color: #fff;
  border-color: #3a3a5e;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

@media (max-width: 560px) {
  .error-actions {
    flex-direction: column;
  }

  .error-btn,
  .error-link {
    width: 100%;
  }
}

@media (prefers-reduced-motion: reduce) {
  .spinner {
    animation: none;
  }

  .error-btn,
  .error-link {
    transition: none;
  }
}
</style>
