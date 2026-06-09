<template>
  <div class="player-view" v-if="!error">
    <video v-if="streamUrl" :src="streamUrl" controls autoplay class="video-player">
      Your browser does not support the video tag.
    </video>
    <div v-else class="loading">Loading...</div>
  </div>
  <div v-else class="error">{{ error }}</div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { getMediaDetail } from '@/api/library'

const route = useRoute()
const streamUrl = ref('')
const error = ref('')

onMounted(async () => {
  try {
    const data = await getMediaDetail(route.params.id as string)
    if (data.stream_url) {
      streamUrl.value = data.stream_url
    } else {
      error.value = 'No stream available for this media.'
    }
  } catch (e) {
    error.value = 'Failed to load media.'
  }
})
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
