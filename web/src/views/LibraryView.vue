<template>
  <div class="library-view">
    <h2 class="page-title">Library</h2>

    <!-- Toolbar -->
    <div class="toolbar">
      <div class="search-wrap">
        <input
          v-model="searchQuery"
          @input="onSearchInput"
          type="text"
          placeholder="Search library..."
          class="search-input"
          autocomplete="off"
        />
      </div>
      <div class="filter-group">
        <button :class="['filter-btn', { active: activeType === TYPE_ALL }]"
                @click="setType(TYPE_ALL)">All</button>
        <button :class="['filter-btn', { active: activeType === TYPE_MOVIE }]"
                @click="setType(TYPE_MOVIE)">Movies</button>
        <button :class="['filter-btn', { active: activeType === TYPE_SHOW }]"
                @click="setType(TYPE_SHOW)">Shows</button>
      </div>
      <select v-model="activeSort" @change="resetAndFetch" class="sort-select">
        <option value="title_asc">Title A–Z</option>
        <option value="title_desc">Title Z–A</option>
        <option value="year_desc">Newest First</option>
        <option value="year_asc">Oldest First</option>
        <option value="rating_desc">Top Rated</option>
        <option value="created_desc">Recently Added</option>
      </select>
    </div>

    <!-- Result count -->
    <div class="result-info" v-if="!loading && total > 0">
      {{ total }} item{{ total !== 1 ? 's' : '' }}
    </div>

    <!-- Inline error -->
    <div class="error-banner" v-if="error">{{ error }}</div>

    <!-- Grid -->
    <div class="grid" v-if="items.length > 0">
      <MediaCard v-for="m in items" :key="m.id" :item="m" />
    </div>

    <!-- Empty state -->
    <div class="empty-state" v-if="items.length === 0 && !loading && !error">
      <p v-if="searchQuery">No results for "{{ searchQuery }}"</p>
      <p v-else>Your library is empty. Import some media to get started.</p>
    </div>

    <!-- Load more -->
    <div class="load-more-wrap" v-if="!allLoaded && items.length > 0">
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
  poster_url?: string
}

// ── Type constants (do not use raw strings elsewhere in this component) ───
// TODO(phase3): These type values will need to match the Provider registry
// once non-filesystem providers are introduced.
const TYPE_ALL   = 'movie,show'
const TYPE_MOVIE = 'movie'
const TYPE_SHOW  = 'show'

// ── State ──────────────────────────────────────────────────────────────────
const items = ref<MediaItem[]>([])
const page = ref(1)
const total = ref(0)
const loading = ref(false)
const allLoaded = ref(false)
const error = ref('')
const searchQuery = ref<string>('')
const activeType = ref<string>(TYPE_ALL)
const activeSort = ref<string>('title_asc')
let searchTimer = 0
let abortCtrl = new AbortController()

onMounted(() => fetchPage())

// ── Debounced search ───────────────────────────────────────────────────────
function onSearchInput() {
  clearTimeout(searchTimer)
  searchTimer = window.setTimeout(() => resetAndFetch(), 300)
}

function setType(type: string) {
  activeType.value = type
  resetAndFetch()
}

function setSort(sort: string) {
  activeSort.value = sort
  resetAndFetch()
}

// ── Reset pagination and re-fetch from page 1 ─────────────────────────────
function resetAndFetch() {
  abortCtrl.abort()
  abortCtrl = new AbortController()
  items.value = []
  page.value = 1
  total.value = 0
  allLoaded.value = false
  fetchPage()
}

// ── Fetch one page ─────────────────────────────────────────────────────────
async function fetchPage() {
  if (loading.value || allLoaded.value) return
  loading.value = true
  error.value = ''
  try {
    const data = await getMediaList(page.value, 20, {
      type: activeType.value,
      q:    searchQuery.value || undefined,
      sort: activeSort.value,
    })
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
  } catch (e: unknown) {
    if (e instanceof Error && e.name !== 'AbortError') {
      error.value = 'Failed to load library. Please try again.'
    }
  } finally {
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

.toolbar {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-bottom: 20px;
  flex-wrap: wrap;
}

.search-wrap {
  flex: 1;
  min-width: 200px;
}

.search-input {
  width: 100%;
  padding: 10px 14px;
  background: #0f0f1a;
  border: 1px solid #2a2a3e;
  border-radius: 8px;
  color: #e0e0e0;
  font-size: 14px;
  outline: none;
  box-sizing: border-box;
  transition: border-color 0.15s;
}

.search-input:focus {
  border-color: #6c63ff;
}

.search-input::placeholder {
  color: #555577;
}

.filter-group {
  display: flex;
  gap: 4px;
}

.filter-btn {
  padding: 8px 16px;
  background: #1a1a2e;
  border: 1px solid #2a2a3e;
  color: #8888aa;
  border-radius: 6px;
  cursor: pointer;
  font-size: 13px;
  transition: all 0.15s;
}

.filter-btn:hover {
  color: #ccccee;
  border-color: #3a3a5e;
}

.filter-btn.active {
  background: #6c63ff;
  color: #fff;
  border-color: #6c63ff;
}

.sort-select {
  padding: 8px 12px;
  background: #1a1a2e;
  border: 1px solid #2a2a3e;
  color: #aaaacc;
  border-radius: 6px;
  font-size: 13px;
  outline: none;
  cursor: pointer;
}

.sort-select option {
  background: #1a1a2e;
  color: #e0e0e0;
}

.result-info {
  color: #555577;
  font-size: 13px;
  margin-bottom: 16px;
}

.error-banner {
  color: #ff6b6b;
  font-size: 13px;
  margin-bottom: 16px;
  padding: 10px 14px;
  background: #2a1a1a;
  border: 1px solid #5a2a2a;
  border-radius: 6px;
}

.empty-state {
  text-align: center;
  padding: 80px 20px;
  color: #555577;
  font-size: 15px;
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
