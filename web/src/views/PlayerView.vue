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

        <router-link to="/library" class="error-link">
          {{ $t('player.backToLibrary') }}
        </router-link>
      </div>
    </section>

    <!-- Player surface area. Native mode and browser fallback mode are mutually exclusive -->
    <section v-else class="player-surface">
      <!-- 1. Native playback mount point (Tauri environment) -->
      <div v-if="isNativeReady" class="native-surface-wrapper">
        <div ref="nativeSurfaceRef" class="native-surface" />
      </div>

      <!-- 2. Browser fallback player (non-Tauri environment, or when native initialization fails) -->
      <div v-else-if="useBrowserFallback && streamUrl" class="browser-player-shell">
        <video
          :key="videoElementKey"
          ref="videoRef"
          :src="streamUrl"
          controls
          autoplay
          playsinline
          preload="metadata"
          class="video-player"
          @loadstart="onVideoLoadStart"
          @loaded-metadata="onLoadedMetadata"
          @canplay="onCanPlay"
          @playing="onPlaying"
          @waiting="onWaiting"
          @stalled="onStalled"
          @timeupdate="onTimeUpdate"
          @pause="onPause"
          @ended="onEnded"
          @error="onVideoError"
        >
          {{ $t('player.browserNotSupported') }}
        </video>

        <div v-if="browserLoading" class="player-overlay">
          <span class="spinner" aria-hidden="true"></span>
          <span>{{ loadingLabel }}</span>
        </div>

        <div v-if="browserPlaybackBlocked" class="player-overlay player-overlay-action">
          <p class="overlay-title">{{ $t('player.playbackBlocked') }}</p>
          <p class="overlay-subtitle">
            {{ $t('player.browserBlockedAutoplay') }}
          </p>
          <button type="button" class="overlay-btn" @click="onManualBrowserPlay">
            {{ $t('player.startPlayback') }}
          </button>
        </div>
      </div>

      <!-- 3. Global loading state (native is initializing, or media info is being fetched) -->
      <div v-if="isLoading && !isNativeReady && !useBrowserFallback" class="loading player-overlay">
        <span class="spinner" aria-hidden="true"></span>
        <span>{{ loadingLabel }}</span>
      </div>
    </section>

    <!-- Controls layer: floats as an overlay on the player surface -->
    <PlayerControls
      v-if="!error && (isNativeReady || useBrowserFallback)"
      :state="controlsState"
      class="player-controls-overlay"
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
  stopMedia,
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

const PROGRESS_REPORT_INTERVAL_SECONDS = 10;

const route = useRoute();
const { t } = useI18n();

const videoRef = ref<HTMLVideoElement | null>(null);
const nativeSurfaceRef = ref<HTMLDivElement | null>(null);
const videoElementKey = ref(0);

const streamUrl = ref('');
const error = ref('');
const loadingMedia = ref(false);
const browserLoading = ref(false);
const browserPlaybackBlocked = ref(false);

const nativePlayerState = shallowRef<NativePlayerState>(createInitialNativePlayerState());

const nativeInitAttempted = ref(false);
const lastReportedPosition = ref(0);
const progressRequestInFlight = ref(false);
const pendingProgressPayload = ref<ProgressPayload | null>(null);
const resumePosition = ref(0);

let loadGeneration = 0;
let disposed = false;
let mpvEventsUnlisten: (() => void) | null = null;

const mediaId = computed(() => {
  const id = route.params.id;
  return typeof id === 'string' ? id : '';
});

// Use official Tauri runtime detection
const isNativeAvailable = computed(() => isNativePlaybackRuntimeAvailable());
const isInitializing = computed(() => nativePlayerState.value.status === 'initializing');
const isNativeReady = computed(() => nativePlayerState.value.status === 'ready');
const isNativeFailed = computed(() => nativePlayerState.value.status === 'failed');

// Determine whether to fall back to <video> tag
const useBrowserFallback = computed(() => {
  if (error.value) return false;
  if (!isNativeAvailable.value) return true; // Non-Tauri environment
  if (isNativeFailed.value) return true; // Tauri environment but native initialization failed
  return false;
});

const showFallbackBanner = computed(() => {
  return (
    isNativeAvailable.value && nativeInitAttempted.value && isNativeFailed.value && streamUrl.value
  );
});

const isLoading = computed(() => {
  return (
    loadingMedia.value || (isNativeAvailable.value && isInitializing.value) || browserLoading.value
  );
});

const loadingLabel = computed(() => {
  if (loadingMedia.value) return t('player.loadingMedia');
  if (isInitializing.value) return t('player.initializingNative');
  return t('player.loadingPlayer');
});

const isVideoModeActive = computed(() => {
  if (error.value) return false;
  if (!isNativeReady.value) return false;
  if (!streamUrl.value) return false;
  return true;
});

const renderSurfaceAttached = ref(false);

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

const tracksComposable = useMediaTracks(() => streamUrl.value);
const adjustmentsComposable = usePlaybackAdjustments();
const speedComposable = usePlaybackSpeed();
const historyComposable = usePlaybackHistory();

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

async function setupMpvEventSubscription(): Promise<void> {
  if (mpvEventsUnlisten) return;
  if (!isNativeAvailable.value) return;

  mpvEventsUnlisten = await subscribeMpvEvents({
    onTimePos: ({ position, duration: dur }) => {
      currentTime.value = position;
      if (dur > 0) duration.value = dur;
      void maybeReportNativeProgress();
    },
    onDuration: ({ duration: dur }) => {
      duration.value = dur;
    },
    onPause: ({ paused }) => {
      isPaused.value = paused;
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
      const payload = {
        audio_tracks: e.audio_tracks,
        sub_tracks: e.sub_tracks,
      };

      tracksComposable.handleTracksUpdate(payload);

      const selectedAudio = payload.audio_tracks.find((track) => track.selected);
      currentAudioId.value = selectedAudio ? selectedAudio.id : 0;

      const selectedSub = payload.sub_tracks.find((track) => track.selected);
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
      void adjustmentsComposable.applyColorAdjustmentsForMedia(path || streamUrl.value);

      if (path) {
        void tracksComposable.applyExternalSubtitlesForUrl(path);
      }

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
      isBuffering.value = false;
    },
    onError: ({ message }) => {
      console.warn('[fyom] mpv error:', message);
    },
  });
}

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
  value: number
): void => {
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
  if (document.fullscreenElement) {
    void document.exitFullscreen();
  } else {
    void document.documentElement.requestFullscreen();
  }
};

const onKeyDown = (event: KeyboardEvent): void => {
  if (!isNativeReady.value) return;

  const target = event.target as HTMLElement | null;

  if (
    target &&
    (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable)
  ) {
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
      if (event.key.length === 1 || event.key.startsWith('Arrow') === false) {
        const keystr = event.key === ' ' ? 'Space' : event.key;
        void bridgeMpvKeypress(keystr);
      }
      break;
  }
};

const handleWindowResize = (): void => {
  const w = window.innerWidth;
  const h = window.innerHeight;
  const dpr = window.devicePixelRatio || 1;
  void resizeRenderSurface(w, h, dpr);
};

onMounted(() => {
  window.addEventListener('resize', handleWindowResize, { passive: true });
  window.addEventListener('keydown', onKeyDown);

  if (isNativeAvailable.value) {
    void setupMpvEventSubscription();
  }

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

watch(isVideoModeActive, (active) => {
  void setVideoMode(active);
});

onBeforeUnmount(() => {
  disposed = true;

  window.removeEventListener('resize', handleWindowResize);
  window.removeEventListener('keydown', onKeyDown);

  void setVideoMode(false);

  tracksComposable.resetTracks();

  if (mpvEventsUnlisten) {
    mpvEventsUnlisten();
    mpvEventsUnlisten = null;
  }

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
  browserLoading.value = false;
  browserPlaybackBlocked.value = false;

  nativePlayerState.value = createInitialNativePlayerState();
  nativeInitAttempted.value = false;

  lastReportedPosition.value = 0;
  progressRequestInFlight.value = false;
  pendingProgressPayload.value = null;
  resumePosition.value = 0;

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
      return;
    }

    streamUrl.value = resolvedStreamUrl;
    videoElementKey.value += 1;

    try {
      const resume = await historyComposable.fetchResumePosition(id);

      if (generation !== loadGeneration || disposed) return;

      resumePosition.value = resume?.position ?? 0;
    } catch {
      // Resume is best-effort.
    }

    await nextTick();

    if (generation !== loadGeneration || disposed) return;

    loadingMedia.value = false;

    // Web always uses HTML5 video. Tauri attempts native only when runtime exists.
    if (isNativeAvailable.value) {
      await attemptNativeInit(generation);

      // If native initialization failed, automatically fall back to browser playback
      if (
        nativePlayerState.value.status === 'failed' &&
        !disposed &&
        generation === loadGeneration
      ) {
        await prepareBrowserPlayback(generation);
      }
      return;
    }

    await prepareBrowserPlayback(generation);
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

  if (typeof raw !== 'string') return '';

  return normalizeStreamUrl(raw);
}

function normalizeStreamUrl(raw: string): string {
  const trimmed = raw.trim();

  if (!trimmed) return '';

  // Some backends accidentally return HTML-escaped signed URLs.
  const unescaped = trimmed.replaceAll('&amp;', '&');

  try {
    return new URL(unescaped, window.location.origin).toString();
  } catch {
    return unescaped;
  }
}

async function prepareBrowserPlayback(generation: number): Promise<void> {
  await nextTick();

  if (generation !== loadGeneration || disposed) return;
  if (!useBrowserFallback.value || !streamUrl.value) return;

  const video = videoRef.value;

  if (!video) return;

  browserLoading.value = true;
  browserPlaybackBlocked.value = false;

  try {
    video.load();
  } catch {
    // Browser load() can throw if the element is already being detached.
  }

  await safePlayBrowserVideo('prepare');
}

async function attemptNativeInit(generation: number): Promise<void> {
  if (nativeInitAttempted.value) return;

  nativeInitAttempted.value = true;

  if (!streamUrl.value || !isNativeAvailable.value) {
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

    if (!renderSurfaceAttached.value) {
      renderSurfaceAttached.value = true;

      const surfaceResult = await attachRenderSurface();

      if (generation !== loadGeneration || disposed) return;

      if (!surfaceResult.ok) {
        console.warn(
          '[fyom] render surface attach failed:',
          surfaceResult.failure.stage,
          surfaceResult.failure.message
        );
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
  const durationValue = Math.floor(durationSeconds || 0);

  if (!Number.isFinite(durationValue) || durationValue <= 0) {
    return null;
  }

  const currentTimeValue = Math.floor(currentTimeSeconds || 0);
  const position = finished
    ? durationValue
    : Math.min(durationValue, Math.max(0, currentTimeValue));

  return {
    position,
    duration: durationValue,
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

function onVideoLoadStart(): void {
  if (!useBrowserFallback.value) return;

  browserLoading.value = true;
  browserPlaybackBlocked.value = false;
}

function onLoadedMetadata(): void {
  const video = videoRef.value;

  if (!video) return;

  duration.value = Number.isFinite(video.duration) ? video.duration : 0;

  if (resumePosition.value > 0) {
    const pos = resumePosition.value;
    resumePosition.value = 0;

    try {
      video.currentTime = pos;
      lastReportedPosition.value = Math.floor(pos);
    } catch {
      // Some streams reject early seeking before enough data is available.
    }
  }

  void safePlayBrowserVideo('loaded-metadata');
}

function onCanPlay(): void {
  browserLoading.value = false;
  void safePlayBrowserVideo('canplay');
}

function onPlaying(): void {
  browserLoading.value = false;
  browserPlaybackBlocked.value = false;
  isPaused.value = false;
}

function onWaiting(): void {
  if (!useBrowserFallback.value) return;
  browserLoading.value = true;
}

function onStalled(): void {
  if (!useBrowserFallback.value) return;
  browserLoading.value = true;
}

async function safePlayBrowserVideo(reason: string): Promise<void> {
  const video = videoRef.value;

  if (!video) return;
  if (!useBrowserFallback.value) return;
  if (disposed) return;

  try {
    const playResult = video.play();

    if (playResult && typeof playResult.then === 'function') {
      await playResult;
    }

    browserPlaybackBlocked.value = false;
    browserLoading.value = false;
    isPaused.value = false;
  } catch (unknownError) {
    const err = unknownError instanceof DOMException ? unknownError : null;

    if (err?.name === 'NotAllowedError') {
      browserPlaybackBlocked.value = true;
      browserLoading.value = false;
      isPaused.value = true;
      console.info('[fyom] browser autoplay blocked:', reason);
      return;
    }

    if (err?.name === 'AbortError') {
      // AbortError commonly happens when src changes during navigation/reload.
      return;
    }

    console.warn('[fyom] browser video play() failed:', reason, unknownError);
  }
}

function onManualBrowserPlay(): void {
  browserPlaybackBlocked.value = false;
  void safePlayBrowserVideo('manual');
}

function onTimeUpdate(): void {
  const video = videoRef.value;

  if (!video) return;

  currentTime.value = video.currentTime || 0;
  duration.value = Number.isFinite(video.duration) ? video.duration : 0;

  const currentTimeValue = Math.floor(video.currentTime || 0);
  const delta = Math.abs(currentTimeValue - lastReportedPosition.value);
  const finished = historyComposable.isFinished(video.currentTime, video.duration || 0);

  if (delta < PROGRESS_REPORT_INTERVAL_SECONDS && !finished) {
    return;
  }

  const payload = buildProgressPayload(video.currentTime, video.duration || 0, finished);

  if (!payload) return;

  lastReportedPosition.value = payload.position;
  void reportProgress(payload);
}

function onPause(): void {
  isPaused.value = true;
  void flushProgressFromVideo(false);
}

function onEnded(): void {
  isPaused.value = true;
  void flushProgressFromVideo(true);
}

async function flushProgressFromVideo(finished: boolean): Promise<void> {
  const video = videoRef.value;

  if (!video) return;

  const effectiveFinished =
    finished || historyComposable.isFinished(video.currentTime, video.duration || 0);

  const payload = buildProgressPayload(video.currentTime, video.duration || 0, effectiveFinished);

  if (!payload) return;

  lastReportedPosition.value = payload.position;
  await reportProgress(payload);
}

async function flushProgressFromMpv(finished: boolean): Promise<void> {
  if (!isNativeReady.value) return;

  const effectiveFinished =
    finished || historyComposable.isFinished(currentTime.value, duration.value);

  const payload = buildProgressPayload(currentTime.value, duration.value, effectiveFinished);

  if (!payload) return;

  lastReportedPosition.value = payload.position;
  await reportProgress(payload);
}

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
  const video = videoRef.value;

  if (!video) {
    return;
  }

  browserLoading.value = false;
  browserPlaybackBlocked.value = false;

  const details = getVideoErrorDetails(video);

  // Don't show error for autoplay block - that's handled by browserPlaybackBlocked
  if (video.error?.code === MediaError.MEDIA_ERR_ABORTED) {
    return;
  }

  // More specific error messages for debugging
  const prefix =
    isNativeAvailable.value && isNativeReady.value
      ? t('player.nativePlaybackFailed')
      : t('player.browserPlaybackFailed');

  error.value = details ? `${prefix}: ${details}` : prefix;
}

function getVideoErrorDetails(video: HTMLVideoElement): string {
  const mediaError = video.error;

  if (!mediaError) {
    return '';
  }

  const codeLabel = (() => {
    switch (mediaError.code) {
      case MediaError.MEDIA_ERR_ABORTED:
        return 'MEDIA_ERR_ABORTED';
      case MediaError.MEDIA_ERR_NETWORK:
        return 'MEDIA_ERR_NETWORK (check CORS, URL, or network connectivity)';
      case MediaError.MEDIA_ERR_DECODE:
        return 'MEDIA_ERR_DECODE (check codec support or file corruption)';
      case MediaError.MEDIA_ERR_SRC_NOT_SUPPORTED:
        return 'MEDIA_ERR_SRC_NOT_SUPPORTED (check MIME type or URL format)';
      default:
        return `UNKNOWN_${mediaError.code}`;
    }
  })();

  const message = mediaError.message ? `, message="${mediaError.message}"` : '';
  const currentSrc = video.currentSrc ? `, src="${video.currentSrc}"` : '';
  const readyState = `, readyState=${video.readyState}`;
  const networkState = `, networkState=${video.networkState}`;

  return `${codeLabel}${message}${currentSrc}${readyState}${networkState}`;
}

async function teardownCurrentPlayback(): Promise<void> {
  loadGeneration += 1;

  await stopNativePlayer();

  const video = videoRef.value;

  if (video) {
    try {
      video.pause();
      // Keep video element mounted, just clear the source
      video.removeAttribute('src');
      video.load();
    } catch {
      // Ignore browser cleanup failures.
    }
  }

  browserLoading.value = false;
  browserPlaybackBlocked.value = false;
}

async function stopNativePlayer(): Promise<void> {
  if (!isNativeReady.value) return;

  try {
    // Use the officially imported stopMedia method
    await stopMedia();
  } catch {
    // Ignore native cleanup failures.
  } finally {
    if (!disposed) {
      nativePlayerState.value = createInitialNativePlayerState();
    }
  }
}
</script>

<style scoped>
.player-view {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  color: #e0e0e0;
  background: #000;
  position: relative;
}

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
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  background: #000;
}

.browser-player-shell {
  position: relative;
  width: 100%;
  height: 100vh;
  min-height: 240px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: #000;
}

.video-player {
  width: 100%;
  height: 100%;
  max-width: 100vw;
  max-height: 100vh;
  display: block;
  object-fit: contain;
  background: #000;
}

.player-overlay {
  position: absolute;
  inset: 0;
  z-index: 20;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-direction: column;
  color: #cfcfff;
  background: rgba(0, 0, 0, 0.68);
  gap: 14px;
  text-align: center;
  padding: 24px;
  box-sizing: border-box;
}

.player-overlay-action {
  background: rgba(0, 0, 0, 0.82);
}

.overlay-title {
  margin: 0;
  color: #f0f0ff;
  font-size: 18px;
  font-weight: 800;
}

.overlay-subtitle {
  max-width: 520px;
  margin: 0;
  color: #aaaacc;
  font-size: 14px;
  line-height: 1.5;
}

.overlay-btn {
  min-height: 40px;
  padding: 9px 16px;
  border: 0;
  border-radius: 8px;
  color: #fff;
  background: #6c63ff;
  font-size: 13px;
  font-weight: 800;
  cursor: pointer;
}

.overlay-btn:hover {
  background: #5a52e0;
}

.native-surface-wrapper {
  width: 100%;
  height: 100vh;
  position: relative;
}

.native-surface {
  width: 100%;
  height: 100%;
  min-height: 240px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-direction: column;
  color: #aaaacc;
  gap: 8px;
  text-align: center;
}

.player-controls-overlay {
  position: absolute;
  bottom: 0;
  left: 0;
  right: 0;
  z-index: 30;
  pointer-events: auto;
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
  max-width: 760px;
  margin: 0;
  color: #aaaacc;
  font-size: 14px;
  line-height: 1.55;
  overflow-wrap: anywhere;
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

  .overlay-title {
    font-size: 16px;
  }

  .overlay-subtitle {
    font-size: 13px;
  }
}

@media (prefers-reduced-motion: reduce) {
  .spinner {
    animation: none;
  }

  .error-btn,
  .error-link,
  .overlay-btn {
    transition: none;
  }
}
</style>
