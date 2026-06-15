<template>
  <div class="player-view">
    <!-- Fallback banner: shown when native player was attempted but failed -->
    <PlayerFallbackNotice
      v-if="showFallbackBanner"
      message="Native player unavailable, using browser playback"
    />

    <!-- Error state -->
    <div v-if="error" class="error">{{ error }}</div>

    <!-- Player surface -->
    <div v-else class="player-surface">
      <!-- Loading state while initializing -->
      <div v-if="isLoading" class="loading">
        <span class="spinner"></span>
        <span>{{ loadingText }}</span>
      </div>

      <!-- HTML5 browser player (fallback or default) -->
      <video
        v-if="showBrowserPlayer && streamUrl"
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

      <!-- No stream available -->
      <div v-if="!streamUrl && !isLoading && !error" class="loading">
        {{ nativePlayerState.status === 'initializing' ? 'Initializing native player...' : 'Loading...' }}
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue';
import { useRoute } from 'vue-router';
import { getMediaDetail } from '@/api/library';
import {
  createInitialNativePlayerState,
  mapNativePlayerInitError,
  isNativePlayerAvailable,
  type NativePlayerState,
} from '@/lib/player/native-player';
import PlayerFallbackNotice from '@/components/PlayerFallbackNotice.vue';

const route = useRoute();
const streamUrl = ref('');
const error = ref('');
const videoRef = ref<HTMLVideoElement | null>(null);
let lastReport = 0;

// Native player state
const nativePlayerState = ref<NativePlayerState>(createInitialNativePlayerState());

// Computed flags for UI decisions
const isNativeAvailable = computed(() => isNativePlayerAvailable());
const isInitializing = computed(() => nativePlayerState.value.status === 'initializing');
const isFailed = computed(() => nativePlayerState.value.status === 'failed');
const isReady = computed(() => nativePlayerState.value.status === 'ready');

// Show fallback banner only when native was attempted and failed
const showFallbackBanner = computed(() => {
  return isNativeAvailable.value && isFailed.value;
});

// Show loading state while native player is initializing
const isLoading = computed(() => {
  return isInitializing.value;
});

// Show browser player when:
// - native player failed (fallback)
// - native player is not available (browser mode)
// - native player is ready but we're using browser fallback
const showBrowserPlayer = computed(() => {
  // Always show browser player in non-Tauri (browser) mode
  if (!isNativeAvailable.value) return true;
  // Show browser player when native failed
  if (isFailed.value) return true;
  // Show browser player when native is idle (not yet attempted)
  if (nativePlayerState.value.status === 'idle') return true;
  // When native is ready, native player surface is used (not browser)
  if (isReady.value) return false;
  // During initialization, don't show browser player yet
  return false;
});

const loadingText = computed(() => {
  if (isInitializing.value) return 'Initializing native player...';
  return 'Loading...';
});

function onTimeUpdate() {
  const video = videoRef.value;
  if (!video) return;
  if (video.currentTime - lastReport > 10) {
    lastReport = video.currentTime;
    fetch(`/api/v1/media/${route.params.id}/progress`, {
      method: 'PUT',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${localStorage.getItem('token') || ''}`,
      },
      body: JSON.stringify({
        position: Math.floor(video.currentTime),
        duration: Math.floor(video.duration || 0),
        finished: false,
      }),
    }).catch(() => {});
  }
}

function onEnded() {
  const video = videoRef.value;
  if (!video) return;
  fetch(`/api/v1/media/${route.params.id}/progress`, {
    method: 'PUT',
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${localStorage.getItem('token') || ''}`,
    },
    body: JSON.stringify({
      position: Math.floor(video.duration || 0),
      duration: Math.floor(video.duration || 0),
      finished: true,
    }),
  }).catch(() => {});
}

/**
 * Attempt to initialize the native player.
 * In Tauri desktop mode, this would invoke the native libmpv backend.
 * On failure, the state transitions to 'failed' and the UI falls back to HTML5.
 */
async function tryInitNativePlayer(mediaId: string): Promise<boolean> {
  if (!isNativeAvailable.value) {
    nativePlayerState.value = { status: 'unavailable', failure: null };
    return false;
  }

  nativePlayerState.value = { status: 'initializing', failure: null };

  try {
    // Attempt native player initialization via Tauri invoke
    // @ts-expect-error - tauri is available in Tauri environment
    const { invoke } = window.__TAURI_INTERNALS__.tauri;
    const result = await invoke('play_media', { mediaId });

    if (result && result.success) {
      nativePlayerState.value = { status: 'ready', failure: null };
      return true;
    }

    // Native player reported failure
    const failure = mapNativePlayerInitError(new Error(result?.error || 'Unknown native player error'));
    nativePlayerState.value = { status: 'failed', failure };
    console.warn('[Player] Native player init failed:', failure.stage, failure.message);
    return false;
  } catch (err) {
    // Native player initialization threw
    const failure = mapNativePlayerInitError(err);
    nativePlayerState.value = { status: 'failed', failure };
    console.warn('[Player] Native player init failed:', failure.stage, failure.message);
    return false;
  }
}

onMounted(async () => {
  try {
    const mediaId = route.params.id as string;
    const data = await getMediaDetail(mediaId);

    if (!data.stream_url) {
      error.value = 'No stream available for this media.';
      return;
    }

    // Store stream URL for potential browser fallback
    streamUrl.value = data.stream_url;

    // Attempt native player initialization (Tauri desktop only)
    const nativeSuccess = await tryInitNativePlayer(mediaId);

    if (!nativeSuccess) {
      // Native player failed or unavailable — browser player will be shown
      // streamUrl is already set, so HTML5 player will render
      console.log('[Player] Using browser playback' + (isFailed.value ? ' (native player failed)' : ''));
    }
  } catch {
    error.value = 'Failed to load media.';
  }
});

onUnmounted(() => {
  // Clean up native player if it was initialized
  if (isReady.value) {
    try {
      // @ts-expect-error - tauri is available in Tauri environment
      const { invoke } = window.__TAURI_INTERNALS__.tauri;
      invoke('stop_media').catch(() => {});
    } catch {
      // Ignore cleanup errors
    }
  }
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
  to { transform: rotate(360deg); }
}

.error {
  color: #ff4444;
  font-size: 18px;
  text-align: center;
  padding: 40px;
}
</style>
