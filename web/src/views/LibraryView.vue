<template>
  <div class="library-view">
    <h2 class="page-title">Library</h2>
    <div class="grid">
      <MediaCard v-for="m in items" :key="m.id" :item="m" />
    </div>
    <div class="load-more-wrap" v-if="!allLoaded">
      <button class="load-more-btn" @click="fetchPage" :disabled="loading">
        {{ loading ? 'Loading...' : 'Load More' }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { getMediaList } from '@/api/library'
import MediaCard from '@/components/MediaCard.vue'

interface MediaItem {
  id: string
  title: string
  year?: number
}

const items = ref<MediaItem[]>([])
const page = ref(1)
const total = ref(0)
const loading = ref(false)
const allLoaded = ref(false)

onMounted(() => fetchPage())

async function fetchPage() {
  if (loading.value || allLoaded.value) return
  loading.value = true
  try {
    const data = await getMediaList(page.value, 20)
    if (!data.items?.length) {
      allLoaded.value = true
      return
    }
    items.value.push(...data.items)
    total.value = data.total
    if (items.value.length >= total.value) {
      allLoaded.value = true
    } else {
      page.value++
    }
  } catch {} finally {
    loading.value = false
  }
}
</script>

<style scoped>
.page-title {
  font-size: 22px;
  color: #e0e0e0;
  margin-bottom: 20px;
}

.grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
  gap: 20px;
}

.load-more-wrap {
  text-align: center;
  padding: 40px 0;
}

.load-more-btn {
  background: #2a2a3e;
  color: #aaaacc;
  border: 1px solid #3a3a5e;
  border-radius: 6px;
  padding: 10px 28px;
  cursor: pointer;
  font-size: 14px;
}

.load-more-btn:hover:not(:disabled) {
  background: #3a3a5e;
}

.load-more-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
</style>
