<template>
  <main class="player-view">
    <PlayerFallbackNotice
      v-if="showFallbackBanner"
      message="Native player unavailable, using browser playback"
      class="fallback-banner"
    />

    <section v-if="error" class="error-state" role="alert">
      <h1>Unable to play media</h1>
      <p>{{ error }}</p>

      <div class="error-actions">
        <button type="button" class="error-btn" @click="reloadCurrentMedia">Retry</button>

        <router-link v-if="mediaId" :to="`/media/${mediaId}`" class="error-link">
          Back to details
        </router-link>

        <router-link to="/library" class="error-link"> Back to library </router-link>
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
        @error="onVideoError"
      >
        Your browser does not support the video tag.
      </video>

      <div v-else-if="isNativeReady" class="native-surface">
        <span class="native-status">Native playback active</span>
        <span class="native-subtitle"> Playback is running in the native player. </span>
      </div>

      <div v-else class="loading">
        <span>Loading player...</span>
      </div>
    </section>
  </main>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue';
import { useRoute } from 'vue-router';
import { getApiErrorMessage, getHttpStatus, getMediaDetail, type MediaItem } from '@/api/library';
import { authRequest } from '@/api/request';
import {
  createInitialNativePlayerState,
  isNativePlaybackRuntimeAvailable,
  tryInitializeNativePlayer,
  type NativePlayerState,
} from '@/lib/player/native-player';
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

const videoRef = ref<HTMLVideoElement | null>(null);
const streamUrl = ref('');
const error = ref('');
const loadingMedia = ref(false);

const nativePlayerState = shallowRef<NativePlayerState>(createInitialNativePlayerState());

const nativeInitAttempted = ref(false);
const lastReportedPosition = ref(0);
const progressRequestInFlight = ref(false);
const pendingProgressPayload = ref<ProgressPayload | null>(null);

let loadGeneration = 0;
let disposed = false;

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
  if (loadingMedia.value) return 'Loading media...';
  if (isInitializing.value) return 'Initializing native player...';

  return 'Loading player...';
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

onMounted(() => {
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

onBeforeUnmount(() => {
  disposed = true;
  void flushProgressFromVideo(false);
  void teardownCurrentPlayback();
});

async function reloadCurrentMedia(): Promise<void> {
  if (!mediaId.value) {
    error.value = 'Invalid media id.';
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
      error.value = 'No stream is available for this media.';
      nativePlayerState.value = {
        status: 'unavailable',
        failure: null,
        attempted: false,
      };
      return;
    }

    streamUrl.value = resolvedStreamUrl;

    await nextTick();

    if (generation !== loadGeneration || disposed) return;

    await attemptNativeInit(generation);
  } catch (unknownError) {
    if (generation !== loadGeneration || disposed) return;

    const status = getHttpStatus(unknownError);

    if (status === 401 || status === 403) {
      error.value = 'You are not authorized to play this media.';
      return;
    }

    console.error('[fyom] player load media failed:', unknownError);
    error.value = getApiErrorMessage(unknownError, 'Failed to load media.');
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

function buildProgressPayload(video: HTMLVideoElement, finished: boolean): ProgressPayload | null {
  const duration = Math.floor(video.duration || 0);

  if (!Number.isFinite(duration) || duration <= 0) {
    return null;
  }

  const currentTime = Math.floor(video.currentTime || 0);
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
    await authRequest.put(`/media/${encodeURIComponent(mediaId.value)}/progress`, payload, {
      authFailureMode: 'silent',
    });
  } catch (unknownError) {
    const status = getHttpStatus(unknownError);

    if (status === 401 || status === 403 || status === 404) {
      return;
    }

    console.warn(
      '[fyom] player progress update failed:',
      getApiErrorMessage(unknownError, 'Progress update failed.')
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

  if (delta < PROGRESS_REPORT_INTERVAL_SECONDS) {
    return;
  }

  const payload = buildProgressPayload(video, false);

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

async function flushProgressFromVideo(finished: boolean): Promise<void> {
  const video = videoRef.value;
  if (!video) return;

  const payload = buildProgressPayload(video, finished);

  if (!payload) return;

  lastReportedPosition.value = payload.position;
  await reportProgress(payload);
}

function onVideoError(): void {
  if (isNativeFailed.value || isNativeUnavailable.value || isNativeIdle.value) {
    error.value = 'Browser playback failed for this media.';
    return;
  }

  error.value = 'Playback failed for this media.';
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
