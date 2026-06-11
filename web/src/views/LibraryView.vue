<template>
  <div class="library-view">
    <router-link to="/library" class="back-link" v-if="currentLibraryId && showBackLink">
      ← All Libraries
    </router-link>
    <h2 class="page-title" v-if="!searchQuery">{{ currentLibraryName }}</h2>

    <!-- Toolbar -->
    <div class="toolbar">
      <div class="search-wrap">
        <input
          v-model="searchQuery"
          type="text"
          placeholder="Search library..."
          class="search-input"
          autocomplete="off"
          @input="onSearchInput"
        />
      </div>
      <select v-model="activeType" class="mobile-filter-select" v-if="!activeStatus">
        <option value="movie,show">All</option>
        <option value="movie">Movies</option>
        <option value="show">Shows</option>
      </select>
      <div class="filter-group">
        <button
          :class="['filter-btn', { active: activeType === TYPE_ALL }]"
          @click="setType(TYPE_ALL)"
          v-if="!activeStatus"
        >
          All
        </button>
        <button
          :class="['filter-btn', { active: activeType === TYPE_MOVIE }]"
          @click="setType(TYPE_MOVIE)"
          v-if="!activeStatus"
        >
          Movies
        </button>
        <button
          :class="['filter-btn', { active: activeType === TYPE_SHOW }]"
          @click="setType(TYPE_SHOW)"
          v-if="!activeStatus"
        >
          Shows
        </button>
      </div>
      <div class="filter-group">
        <button :class="['filter-btn', { active: activeStatus === '' }]" @click="setStatus('')">
          All
        </button>
        <button
          :class="['filter-btn status-watching', { active: activeStatus === 'watching' }]"
          @click="setStatus('watching')"
        >
          ▶ Watching
        </button>
        <button
          :class="['filter-btn status-want', { active: activeStatus === 'want_to_watch' }]"
          @click="setStatus('want_to_watch')"
        >
          🔖 Want
        </button>
        <button
          :class="['filter-btn status-watched', { active: activeStatus === 'watched' }]"
          @click="setStatus('watched')"
        >
          ✓ Watched
        </button>
        <button
          :class="['filter-btn status-dropped', { active: activeStatus === 'dropped' }]"
          @click="setStatus('dropped')"
        >
          ✕ Dropped
        </button>
      </div>
      <select v-model="activeSort" class="sort-select" @change="resetAndFetch" v-if="!activeStatus">
        <option value="title_asc">Title A–Z</option>
        <option value="title_desc">Title Z–A</option>
        <option value="year_desc">Newest First</option>
        <option value="year_asc">Oldest First</option>
        <option value="rating_desc">Top Rated</option>
        <option value="created_desc">Recently Added</option>
      </select>
    </div>

    <!-- Genre filter -->
    <div class="genre-filter" v-if="availableGenres.length > 1 && !activeStatus">
      <button :class="['genre-tag-btn', { active: !selectedGenre }]" @click="filterByGenre('')">
        All
      </button>
      <button
        :class="['genre-tag-btn', { active: selectedGenre === g }]"
        v-for="g in availableGenres"
        :key="g"
        @click="filterByGenre(g)"
      >
        {{ g }}
      </button>
    </div>

    <!-- Result count -->
    <div v-if="!loading && total > 0" class="result-info">
      {{ total }} item{{ total !== 1 ? 's' : '' }}
    </div>

    <!-- Inline error -->
    <div v-if="error" class="error-banner">{{ error }}</div>

    <!-- Grid -->
    <div v-if="items.length > 0" class="grid">
      <MediaCard v-for="m in items" :key="m.id" :item="m" @status-changed="onStatusChanged" />
    </div>

    <!-- Empty state -->
    <div v-if="items.length === 0 && !loading && hasFetched && !error" class="empty-state">
      <p v-if="searchQuery">No results for "{{ searchQuery }}"</p>
      <p v-else>Your library is empty. Import some media to get started.</p>
    </div>

    <!-- Load more -->
    <div v-if="!allLoaded && items.length > 0" class="load-more-wrap">
      <button class="load-more-btn" :disabled="loading" @click="fetchPage">
        {{ loading ? 'Loading...' : 'Load More' }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, computed, watch } from 'vue';
import { useRoute } from 'vue-router';
import { getMediaList, getMediaByStatus } from '@/api/library';
import request from '@/api/request';
import MediaCard from '@/components/MediaCard.vue';

interface MediaItem {
  id: string;
  title: string;
  year?: number;
  poster_url?: string;
  user_status?: string;
  [key: string]: unknown;
}

// ── Type constants (do not use raw strings elsewhere in this component) ───
const TYPE_ALL = 'movie,show';
const TYPE_MOVIE = 'movie';
const TYPE_SHOW = 'show';

const route = useRoute();

// Read library_id from route param (library/:libraryId) or query param (backward compat)
const currentLibraryId = computed(
  () => (route.params.libraryId as string) || (route.query.library_id as string) || ''
);

// ── Libraries map for name resolution ─────────────────────────────────────
const libraryNameMap = ref<Record<string, string>>({});
const librariesCount = ref(0);

const currentLibraryName = computed(() => {
  if (!currentLibraryId.value) return 'Library';
  return libraryNameMap.value[currentLibraryId.value] || 'Library';
});

const showBackLink = computed(() => librariesCount.value >= 2);

// ── State ──────────────────────────────────────────────────────────────────
const items = ref<MediaItem[]>([]);
const page = ref(1);
const total = ref(0);
const loading = ref(true); // Start true to prevent empty-state flash
const hasFetched = ref(false);
const allLoaded = ref(false);
const error = ref('');
const searchQuery = ref<string>('');
const activeType = ref<string>(TYPE_ALL);
const activeSort = ref<string>('title_asc');
const activeStatus = ref<string>('');
const selectedGenre = ref<string>('');
let searchTimer = 0;
let abortCtrl = new AbortController();
let fetchGen = 0;

const availableGenres = computed(() => {
  const set = new Set<string>();
  for (const item of items.value) {
    if (item.genres) item.genres.forEach((g: string) => set.add(g));
  }
  return Array.from(set).sort();
});

onMounted(async () => {
  // Fetch libraries for name resolution
  try {
    const libRes: any = await request.get('/libraries');
    const libs = libRes.data || [];
    const map: Record<string, string> = {};
    for (const lib of libs) {
      map[lib.id] = lib.name;
    }
    libraryNameMap.value = map;
    librariesCount.value = libs.length;
  } catch {
    // ignore
  }
  fetchPage();
});

// Watch for route changes (navigating between libraries)
watch(currentLibraryId, () => {
  resetAndFetch();
});

// ── Debounced search ───────────────────────────────────────────────────────
function onSearchInput() {
  clearTimeout(searchTimer);
  searchTimer = window.setTimeout(() => resetAndFetch(), 300);
}

function setType(type: string) {
  activeType.value = type;
  resetAndFetch();
}

function setStatus(status: string) {
  activeStatus.value = status;
  resetAndFetch();
}

function filterByGenre(genre: string) {
  selectedGenre.value = genre;
  resetAndFetch();
}

// ── Reset pagination and re-fetch from page 1 ─────────────────────────────
function resetAndFetch() {
  abortCtrl.abort();
  abortCtrl = new AbortController();
  items.value = [];
  page.value = 1;
  total.value = 0;
  allLoaded.value = false;
  fetchPage();
}

// ── Fetch one page ─────────────────────────────────────────────────────────
async function fetchPage() {
  if (allLoaded.value) return;
  const gen = ++fetchGen;
  loading.value = true;
  error.value = '';
  try {
    let data: any;
    if (activeStatus.value) {
      data = await getMediaByStatus(activeStatus.value, 20);
      allLoaded.value = true;
    } else {
      const params: any = {
        type: activeType.value,
        q: searchQuery.value || undefined,
        sort: activeSort.value,
      };
      if (currentLibraryId.value) {
        params.library_id = currentLibraryId.value;
      }
      data = await getMediaList(page.value, 20, params);
    }
    if (!data.items?.length) {
      allLoaded.value = true;
      return;
    }
    items.value.push(...data.items);
    total.value = data.total || items.value.length;

    // Client-side genre filtering
    if (selectedGenre.value) {
      items.value = items.value.filter((m: MediaItem) => m.genres?.includes(selectedGenre.value));
    }

    if (!activeStatus.value) {
      if (items.value.length >= total.value) {
        allLoaded.value = true;
      } else {
        page.value++;
      }
    }
  } catch (e: unknown) {
    if (e instanceof Error && e.name !== 'AbortError') {
      error.value = 'Failed to load library. Please try again.';
    }
  } finally {
    if (gen === fetchGen) {
      loading.value = false;
      hasFetched.value = true;
    }
  }
}

function onStatusChanged(id: string, newStatus: string) {
  const item = items.value.find((m) => m.id === id);
  if (item) {
    item.user_status = newStatus;
  }
}
</script>

<style scoped>
.page-title {
  font-size: clamp(1.25rem, 3vw, 1.5rem);
  color: var(--color-text);
  margin-bottom: var(--spacing-md);
}

.back-link {
  color: #555577;
  font-size: var(--font-size-sm);
  text-decoration: none;
  display: inline-block;
  margin-bottom: var(--spacing-sm);
}

.back-link:hover {
  color: #8888aa;
}

.toolbar {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  margin-bottom: var(--spacing-md);
  flex-wrap: wrap;
}

.search-wrap {
  flex: 1;
  min-width: 150px;
}

.search-input {
  width: 100%;
  padding: var(--spacing-sm) var(--spacing-md);
  background: #0f0f1a;
  border: 1px solid var(--color-border);
  border-radius: 8px;
  color: var(--color-text);
  font-size: var(--font-size-md);
  outline: none;
  box-sizing: border-box;
  transition: border-color 0.15s;
}

.search-input:focus {
  border-color: var(--color-primary);
}

.search-input::placeholder {
  color: #555577;
}

.filter-group {
  display: flex;
  gap: 4px;
}

.filter-btn {
  padding: var(--spacing-sm) var(--spacing-md);
  background: #1a1a2e;
  border: 1px solid var(--color-border);
  color: #8888aa;
  border-radius: 6px;
  cursor: pointer;
  font-size: var(--font-size-sm);
  transition: all 0.15s;
  min-width: var(--touch-target);
}

.filter-btn:hover {
  color: #ccccee;
  border-color: #3a3a5e;
}

.filter-btn.active {
  color: #fff;
}

.filter-btn.active.status-watching {
  background: #6c63ff;
  border-color: #6c63ff;
}

.filter-btn.active.status-want {
  background: #2196f3;
  border-color: #2196f3;
}

.filter-btn.active.status-watched {
  background: #4caf50;
  border-color: #4caf50;
}

.filter-btn.active.status-dropped {
  background: #ff6b6b;
  border-color: #ff6b6b;
}

.sort-select {
  padding: var(--spacing-sm) var(--spacing-md);
  background: #1a1a2e;
  border: 1px solid var(--color-border);
  color: #aaaacc;
  border-radius: 6px;
  font-size: var(--font-size-sm);
  outline: none;
  cursor: pointer;
  min-width: 120px;
}

.sort-select option {
  background: #1a1a2e;
  color: var(--color-text);
}

.genre-filter {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-bottom: var(--spacing-sm);
}

.genre-tag-btn {
  background: #1a1a2e;
  color: #8888aa;
  padding: 3px 10px;
  border-radius: 4px;
  font-size: var(--font-size-sm);
  border: 1px solid var(--color-border);
  cursor: pointer;
  transition: all 0.15s;
}

.genre-tag-btn:hover {
  border-color: #3a3a5e;
  color: #ccccee;
}

.genre-tag-btn.active {
  background: var(--color-primary);
  color: #fff;
  border-color: var(--color-primary);
}

.result-info {
  color: #555577;
  font-size: var(--font-size-sm);
  margin-bottom: var(--spacing-sm);
}

.error-banner {
  color: #ff6b6b;
  font-size: var(--font-size-sm);
  margin-bottom: var(--spacing-sm);
  padding: var(--spacing-sm) var(--spacing-md);
  background: #2a1a1a;
  border: 1px solid #5a2a2a;
  border-radius: 6px;
}

.empty-state {
  text-align: center;
  padding: 5rem 1rem;
  color: #555577;
  font-size: var(--font-size-md);
}

.grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(min(120px, 100%), 1fr));
  gap: var(--spacing-md);
}

@media (max-width: 600px) {
  .grid {
    grid-template-columns: repeat(auto-fill, minmax(100px, 1fr));
    gap: var(--spacing-sm);
  }
}

.load-more-wrap {
  text-align: center;
  padding: 2.5rem 0;
}

.load-more-btn {
  background: #2a2a3e;
  color: #aaaacc;
  border: 1px solid var(--color-border);
  border-radius: 6px;
  padding: var(--spacing-sm) var(--spacing-lg);
  cursor: pointer;
  font-size: var(--font-size-md);
  min-width: var(--touch-target);
}

.load-more-btn:hover:not(:disabled) {
  background: #3a3a5e;
}

.load-more-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.mobile-filter-select {
  display: none;
  width: 100%;
  margin-bottom: var(--spacing-sm);
  padding: var(--spacing-sm) var(--spacing-md);
  background: #1a1a2e;
  border: 1px solid var(--color-border);
  color: var(--color-text);
  border-radius: 6px;
  font-size: var(--font-size-md);
}

@media (max-width: 600px) {
  .mobile-filter-select {
    display: block;
  }
  .filter-group {
    display: none;
  }
}
</style>
