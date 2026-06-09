<template>
  <div class="episode-list" v-if="seasons.length">
    <div v-for="s in seasons" :key="s.season" class="season-group">
      <h3 class="season-title">Season {{ s.season }}</h3>
      <div
        v-for="ep in s.episodes"
        :key="ep.id"
        class="episode-row"
      >
        <span class="ep-label">{{ epLabel(ep) }}</span>
        <span class="ep-title">{{ ep.title }}</span>
        <span class="ep-duration" v-if="ep.duration">{{ formatDuration(ep.duration) }}</span>
        <router-link :to="`/play/${ep.id}`" class="ep-play" @click.stop>▶</router-link>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { getEpisodes } from '@/api/library'

const props = defineProps<{ showId: string }>()

interface Episode {
  id: string
  title: string
  season: number
  episode: number
  duration: number
  poster_url?: string
  stream_url?: string
}

const episodes = ref<Episode[]>([])

onMounted(async () => {
  try {
    episodes.value = await getEpisodes(props.showId)
  } catch {
    episodes.value = []
  }
})

const seasons = computed(() => {
  const map: Record<number, Episode[]> = {}
  for (const ep of episodes.value) {
    const s = ep.season || 0
    if (!map[s]) map[s] = []
    map[s].push(ep)
  }
  return Object.entries(map)
    .sort(([a], [b]) => Number(a) - Number(b))
    .map(([s, eps]) => ({ season: Number(s), episodes: eps }))
})

function epLabel(ep: Episode) {
  return `${ep.season}×${String(ep.episode).padStart(2, '0')}`
}

function formatDuration(sec: number) {
  if (!sec) return ''
  const h = Math.floor(sec / 3600)
  const m = Math.floor((sec % 3600) / 60)
  return h > 0 ? `${h}h ${m}m` : `${m}m`
}
</script>

<style scoped>
.episode-list {
  margin-top: 32px;
}

.season-group {
  margin-bottom: 24px;
}

.season-title {
  font-size: 18px;
  color: #c0c0d0;
  margin: 0 0 12px;
  padding-bottom: 8px;
  border-bottom: 1px solid #2a2a3e;
}

.episode-row {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 12px;
  border-radius: 6px;
  text-decoration: none;
  color: #aaaacc;
  font-size: 14px;
  transition: background 0.15s;
}

.episode-row:hover {
  background: #1e1e32;
  color: #e0e0e0;
}

.ep-label {
  color: #6c63ff;
  font-weight: 600;
  min-width: 48px;
  font-size: 13px;
}

.ep-title {
  flex: 1;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.ep-duration {
  color: #666688;
  font-size: 13px;
}

.ep-play {
  color: #6c63ff;
  font-size: 16px;
  text-decoration: none;
  padding: 4px 8px;
  border-radius: 4px;
}

.ep-play:hover {
  color: #8b83ff;
}
</style>
