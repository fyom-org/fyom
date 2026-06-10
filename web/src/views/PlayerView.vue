<template>
  <div v-if="!error" class="player-view">
    <video
      v-if="streamUrl"
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
    <div v-else class="loading">Loading...</div>
  </div>
  <div v-else class="error">{{ error }}</div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { useRoute } from 'vue-router';
import { getMediaDetail } from '@/api/library';

const route = useRoute();
const streamUrl = ref('');
const error = ref('');
const videoRef = ref<HTMLVideoElement | null>(null);
let lastReport = 0;

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

onMounted(async () => {
  try {
    const data = await getMediaDetail(route.params.id as string);
    if (data.stream_url) {
      streamUrl.value = data.stream_url;
    } else {
      error.value = 'No stream available for this media.';
    }
  } catch {
    error.value = 'Failed to load media.';
  }
});
</script>

<style scoped>
.player-view {
  background: #000;
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
}

.video-player {
  width: 100%;
  max-width: 100vw;
  max-height: 100vh;
  display: block;
}

.loading {
  color: #666;
  font-size: 18px;
}

.error {
  color: #ff4444;
  font-size: 18px;
  text-align: center;
  padding: 40px;
}
</style>
