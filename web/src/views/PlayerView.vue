<template>
  <div class="player-view">
    <PlayerFallbackNotice
      v-if="showFallbackBanner"
      message="Native player unavailable, using browser playback"
      class="fallback-banner"
    />

    <div v-if="error" class="error">
      {{ error }}
    </div>

    <div v-else class="player-surface">
      <div v-if="isLoading" class="loading">
        <span class="spinner"></span>
        <span>Initializing native player...</span>
      </div>

      <video
        v-else-if="showBrowserPlayer && streamUrl"
        ref="videoRef"
        :src="streamUrl"
        controls
        autoplay
        class="video-player"
        @timeupdate="onTimeUpdate"
        @ended="onEnded"
      >
        Your browser does not support the video tag.
      </video>

      <div v-else-if="isNativeReady" class="native-surface">
        <span class="native-status">Native playback active</span>
      </div>

      <div v-else class="loading">
        <span>Loading...</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, shallowRef, watch } from 'vue';
import { useRoute } from 'vue-router';
import { getMediaDetail, type MediaItem } from '@/api/library';
import { authRequest } from '@/api/request';
import {
  createInitialNativePlayerState,
  tryInitializeNativePlayer,
  isNativePlaybackRuntimeAvailable,
  type NativePlayerState,
} from '@/lib/player/native-player';
import PlayerFallbackNotice from '@/components/PlayerFallbackNotice.vue';

interface ProgressPayload {
  position: number;
  duration: number;
  finished: boolean;
}

const route = useRoute();

const videoRef = ref<HTMLVideoElement | null>(null);
const streamUrl = ref<string>('');
const error = ref('');

const nativePlayerState = shallowRef<NativePlayerState>(createInitialNativePlayerState());

const nativeInitAttempted = ref(false);
const lastReportedPosition = ref(0);
const progressRequestInFlight = ref(false);

const mediaId = computed(() => String(route.params.id ?? ''));

const isNativeAvailable = computed(() => isNativePlaybackRuntimeAvailable());
const isInitializing = computed(() => nativePlayerState.value.status === 'initializing');
const isNativeReady = computed(() => nativePlayerState.value.status === 'ready');
const isNativeFailed = computed(() => nativePlayerState.value.status === 'failed');
const isNativeUnavailable = computed(() => nativePlayerState.value.status === 'unavailable');
const isNativeIdle = computed(() => nativePlayerState.value.status === 'idle');

const showFallbackBanner = computed(() => isNativeAvailable.value && isNativeFailed.value);
const isLoading = computed(() => isInitializing.value);

const showBrowserPlayer = computed(() => {
  if (error.value) return false;
  if (isInitializing.value) return false;
  if (!streamUrl.value) return false;

  if (isNativeFailed.value) return true;
  if (isNativeUnavailable.value) return true;
  if (isNativeIdle.value) return true;

  return false;
});

function resetViewState(): void {
  streamUrl.value = '';
  error.value = '';
  nativePlayerState.value = createInitialNativePlayerState();
  nativeInitAttempted.value = false;
  lastReportedPosition.value = 0;
  progressRequestInFlight.value = false;
}

function buildProgressPayload(video: HTMLVideoElement, finished: boolean): ProgressPayload {
  const duration = Math.floor(video.duration || 0);
  const position = finished ? duration : Math.floor(video.currentTime || 0);

  return {
    position,
    duration,
    finished,
  };
}

async function reportProgress(payload: ProgressPayload): Promise<void> {
  if (!mediaId.value) return;
  if (progressRequestInFlight.value) return;

  progressRequestInFlight.value = true;

  try {
    await authRequest.put(`/media/${mediaId.value}/progress`, payload);
  } catch (err: any) {
    const status = err?.response?.status;
    if (status === 401 || status === 403) {
      return;
    }

    console.error('[player] reportProgress failed:', err);
  } finally {
    progressRequestInFlight.value = false;
  }
}

function onTimeUpdate(): void {
  const video = videoRef.value;
  if (!video) return;

  const currentTime = Math.floor(video.currentTime || 0);

  if (currentTime - lastReportedPosition.value < 10) {
    return;
  }

  lastReportedPosition.value = currentTime;
  void reportProgress(buildProgressPayload(video, false));
}

function onEnded(): void {
  const video = videoRef.value;
  if (!video) return;

  lastReportedPosition.value = Math.floor(video.duration || 0);
  void reportProgress(buildProgressPayload(video, true));
}

async function stopNativePlayer(): Promise<void> {
  if (!isNativeReady.value) return;

  try {
    const tauriInvoke = (window as any)?.__TAURI_INTERNALS__?.tauri?.invoke;
    if (typeof tauriInvoke === 'function') {
      await tauriInvoke('stop_media');
    }
  } catch {
    // Ignore native cleanup failures
  }
}

async function attemptNativeInit(): Promise<void> {
  if (nativeInitAttempted.value) return;
  nativeInitAttempted.value = true;

  if (!streamUrl.value) {
    nativePlayerState.value = {
      status: 'unavailable',
      failure: null,
      attempted: false,
    };
    return;
  }

  if (!isNativePlaybackRuntimeAvailable()) {
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

  if (result.ok) {
    nativePlayerState.value = {
      status: 'ready',
      failure: null,
      attempted: true,
    };
    return;
  }

  nativePlayerState.value = {
    status: 'failed',
    failure: result.failure,
    attempted: true,
  };

  console.warn(
    '[player] native initialization failed:',
    result.failure.stage,
    result.failure.message
  );
}

function extractStreamUrl(media: MediaItem): string {
  const raw = media.stream_url;
  return typeof raw === 'string' ? raw : '';
}

async function loadMedia(): Promise<void> {
  if (!mediaId.value) {
    error.value = 'Invalid media id.';
    return;
  }

  resetViewState();

  try {
    const media = await getMediaDetail(mediaId.value);
    const resolvedStreamUrl = extractStreamUrl(media);

    if (!resolvedStreamUrl) {
      error.value = 'No stream available for this media.';
      nativePlayerState.value = {
        status: 'unavailable',
        failure: null,
        attempted: false,
      };
      return;
    }

    streamUrl.value = resolvedStreamUrl;
    await attemptNativeInit();
  } catch (err: any) {
    const status = err?.response?.status;

    if (status === 401 || status === 403) {
      return;
    }

    console.error('[player] loadMedia failed:', err);
    error.value = 'Failed to load media.';
  }
}

onMounted(() => {
  void loadMedia();
});

watch(
  () => mediaId.value,
  async (nextId, previousId) => {
    if (!nextId || nextId === previousId) return;

    await stopNativePlayer();
    void loadMedia();
  }
);

onUnmounted(() => {
  void stopNativePlayer();
});
</script>

<style scoped>
.player-view {
  background: #000;
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
}

.fallback-banner {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  z-index: 100;
}

.player-surface {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100%;
}

.video-player {
  width: 100%;
  max-width: 100vw;
  max-height: 100vh;
  display: block;
}

.loading {
  color: #888;
  font-size: 18px;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 16px;
}

.spinner {
  width: 32px;
  height: 32px;
  border: 3px solid #333;
  border-top-color: #888;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

.error {
  color: #ff4444;
  font-size: 18px;
  text-align: center;
  padding: 40px;
}
</style>
