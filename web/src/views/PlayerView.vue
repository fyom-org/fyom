<template>
  <div class="player-view">
    <!-- Fallback banner: shown only when native was attempted and failed -->
    <PlayerFallbackNotice
      v-if="showFallbackBanner"
      message="Native player unavailable, using browser playback"
    />

    <!-- Error state (fatal: no stream available at all) -->
    <div v-if="error" class="error">{{ error }}</div>

    <!-- Player surface -->
    <div v-else class="player-surface">
      <!-- Loading state while native player initializes -->
      <div v-if="isLoading" class="loading">
        <span class="spinner"></span>
        <span>Initializing native player...</span>
      </div>

      <!-- HTML5 browser player (fallback or default) -->
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

      <!-- Waiting for stream URL -->
      <div v-else-if="!streamUrl" class="loading">
        <span>Loading...</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, shallowRef } from 'vue';
import { useRoute } from 'vue-router';
import { getMediaDetail } from '@/api/library';
import {
  createInitialNativePlayerState,
  tryInitializeNativePlayer,
  isNativePlaybackRuntimeAvailable,
  type NativePlayerState,
} from '@/lib/player/native-player';
import PlayerFallbackNotice from '@/components/PlayerFallbackNotice.vue';

const route = useRoute();
const streamUrl = ref('');
const error = ref('');
const videoRef = ref<HTMLVideoElement | null>(null);
let lastReport = 0;

// Use shallowRef for state object to avoid deep reactivity overhead
const nativePlayerState = shallowRef<NativePlayerState>(
  createInitialNativePlayerState(),
);

// Guard: ensure we only attempt native init once per mount
const nativeInitAttempted = ref(false);

/* ── Computed flags ────────────────────────────────────────────────────── */

const isNativeAvailable = computed(() => isNativePlaybackRuntimeAvailable());
const isInitializing = computed(() => nativePlayerState.value.status === 'initializing');
const isFailed = computed(() => nativePlayerState.value.status === 'failed');
const isReady = computed(() => nativePlayerState.value.status === 'ready');
const isUnavailable = computed(() => nativePlayerState.value.status === 'unavailable');

// Show fallback banner only when native was attempted AND failed
const showFallbackBanner = computed(() => {
  return isNativeAvailable.value && isFailed.value;
});

// Show loading spinner only during active native initialization
const isLoading = computed(() => isInitializing.value);

// Show browser player when:
// - native is unavailable (browser mode, never attempted)
// - native failed (browser fallback)
// - native is idle but stream URL is ready (browser default before native attempt completes)
// - native is ready but we still show browser (shouldn't happen, but safe)
const showBrowserPlayer = computed(() => {
  // Never show browser player while native is initializing
  if (isInitializing.value) return false;
  // Show browser player when native failed
  if (isFailed.value) return true;
  // Show browser player when native is unavailable (browser mode)
  if (isUnavailable.value) return true;
  // Show browser player when native is idle (not yet attempted, stream ready)
  if (nativePlayerState.value.status === 'idle') return true;
  // When native is ready, native surface is used
  return false;
});

/* ── Progress tracking ─────────────────────────────────────────────────── */

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

/* ── Native player lifecycle ───────────────────────────────────────────── */

/**
 * Attempt native player initialization.
 * This function is called exactly once per mount lifecycle.
 * It transitions the state machine: idle → initializing → ready | failed
 */
async function attemptNativeInit(mediaId: string): Promise<void> {
  if (nativeInitAttempted.value) return;
  nativeInitAttempted.value = true;

  // Check runtime availability first
  if (!isNativePlaybackRuntimeAvailable()) {
    nativePlayerState.value = {
      status: 'unavailable',
      failure: null,
      attempted: false,
    };
    return;
  }

  // Transition to initializing
  nativePlayerState.value = {
    status: 'initializing',
    failure: null,
    attempted: true,
  };

  // Attempt native init via bridge function
  const result = await tryInitializeNativePlayer({
    mediaUrl: streamUrl.value,
  });

  if (result.ok) {
    nativePlayerState.value = {
      status: 'ready',
      failure: null,
      attempted: true,
    };
  } else {
    nativePlayerState.value = {
      status: 'failed',
      failure: result.failure,
      attempted: true,
    };
    console.warn('[Player] Native player init failed:', result.failure.stage, result.failure.message);
  }
}

/* ── Lifecycle ─────────────────────────────────────────────────────────── */

onMounted(async () => {
  try {
    const mediaId = route.params.id as string;
    const data = await getMediaDetail(mediaId);

    if (!data.stream_url) {
      error.value = 'No stream available for this media.';
      return;
    }

    // Store stream URL for browser fallback
    streamUrl.value = data.stream_url;

    // Attempt native init (only in Tauri runtime; no-op in browser)
    await attemptNativeInit(mediaId);
  } catch {
    error.value = 'Failed to load media.';
  }
});

onUnmounted(() => {
  // Clean up native player only if it was successfully initialized
  if (isReady.value) {
    try {
      // @ts-expect-error — __TAURI_INTERNALS__ exists in Tauri runtime
      const { invoke } = window.__TAURI_INTERNALS__.tauri;
      invoke('stop_media').catch(() => {});
    } catch {
      // Ignore cleanup errors — native may have already been torn down
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
