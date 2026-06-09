<template>
  <div class="detail-view" v-if="item">
    <div class="backdrop">
      <img v-if="!backdropFailed"
           :src="`/api/v1/media/${item.id}/backdrop`"
           @error="backdropFailed = true" />
      <div class="backdrop-overlay"></div>
    </div>

    <div class="content">
      <router-link to="/library" class="back-link">← Back to Library</router-link>

      <div class="main-row">
        <img class="poster" :src="`/api/v1/media/${item.id}/poster`" />

        <div class="meta">
          <h1 class="title">{{ item.title }}</h1>
          <div class="facts">
            <span v-if="item.year">{{ item.year }}</span>
            <span v-if="item.rating">★ {{ item.rating.toFixed(1) }}</span>
            <span v-if="item.duration">{{ formatDuration(item.duration) }}</span>
            <span class="type-badge">{{ item.type }}</span>
          </div>
          <p class="overview" v-if="item.overview">{{ item.overview }}</p>

          <a v-if="item.type !== 'show'"
             :href="`/api/v1/media/${item.id}/stream`"
             class="play-btn">▶ Play</a>
        </div>
      </div>

      <EpisodeList v-if="item.type === 'show'" :show-id="item.id" />
    </div>
  </div>

  <div v-else-if="loading" class="loading">Loading...</div>
  <div v-else class="loading">Not found</div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { getMediaDetail } from '@/api/library'
import EpisodeList from '@/components/EpisodeList.vue'

const route = useRoute()
const item = ref(null)
const loading = ref(true)
const backdropFailed = ref(false)

onMounted(async () => {
  try { item.value = await getMediaDetail(route.params.id as string) }
  finally { loading.value = false }
})

function formatDuration(sec: number) {
  if (!sec) return ''
  const h = Math.floor(sec / 3600)
  const m = Math.floor((sec % 3600) / 60)
  return h > 0 ? `${h}h ${m}m` : `${m}m`
}
</script>

<style scoped>
.detail-view {
  min-height: 100vh;
  background: #0f0f1a;
}

.backdrop {
  position: relative;
  height: 300px;
  overflow: hidden;
  background: #1a1a2e;
}

.backdrop img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  filter: blur(2px) brightness(0.6);
}

.backdrop-overlay {
  position: absolute;
  inset: 0;
  background: linear-gradient(to bottom, rgba(15, 15, 26, 0.3), #0f0f1a);
}

.content {
  max-width: 960px;
  margin: 0 auto;
  padding: 0 24px 40px;
  margin-top: -80px;
}

.back-link {
  color: #8888aa;
  font-size: 14px;
  text-decoration: none;
  display: inline-block;
  margin-bottom: 16px;
}

.back-link:hover {
  color: #e0e0e0;
}

.main-row {
  display: flex;
  gap: 24px;
}

.poster {
  width: 180px;
  min-width: 180px;
  aspect-ratio: 2 / 3;
  border-radius: 8px;
  object-fit: cover;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.6);
}

.meta {
  flex: 1;
}

.title {
  font-size: 28px;
  color: #e0e0e0;
  margin: 0 0 12px;
}

.facts {
  display: flex;
  gap: 16px;
  color: #8888aa;
  font-size: 14px;
  margin-bottom: 16px;
  align-items: center;
}

.type-badge {
  background: #2a2a3e;
  padding: 2px 10px;
  border-radius: 4px;
  font-size: 12px;
  text-transform: capitalize;
}

.overview {
  color: #aaaacc;
  font-size: 14px;
  line-height: 1.7;
  margin-bottom: 24px;
  max-width: 600px;
}

.play-btn {
  display: inline-block;
  background: #6c63ff;
  color: #fff;
  padding: 12px 32px;
  border-radius: 8px;
  text-decoration: none;
  font-size: 15px;
  font-weight: 600;
}

.play-btn:hover {
  background: #5a52e0;
}

.loading {
  text-align: center;
  padding: 80px;
  color: #666;
}
</style>
