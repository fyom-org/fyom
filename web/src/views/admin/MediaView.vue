<template>
  <div class="admin-page">
    <h1>Media Items</h1>

    <div class="toolbar">
      <input
        v-model="searchQuery"
        @input="onSearchInput"
        type="text"
        placeholder="Search items..."
        class="search-input"
      />
      <select
        v-model="typeFilter"
        @change="
          page = 1;
          fetchItems();
        "
        class="filter-select"
      >
        <option value="">All Types</option>
        <option value="movie">Movies</option>
        <option value="show">Shows</option>
        <option value="episode">Episodes</option>
      </select>
      <select
        v-model="libraryFilter"
        @change="
          page = 1;
          fetchItems();
        "
        class="filter-select"
      >
        <option value="">All Libraries</option>
        <option v-for="lib in libraries" :key="lib.id" :value="lib.id">{{ lib.name }}</option>
      </select>
    </div>

    <div class="result-info" v-if="total > 0">{{ total }} items</div>

    <!-- Grouped view (shows with nested episodes; movies standalone) -->
    <div class="media-list" v-if="groupedItems.length > 0">
      <template v-for="g in groupedItems" :key="g.id">
        <!-- Show row -->
        <div v-if="g.type === 'show'" class="media-row show-row" @click="toggleShow(g.id)">
          <span class="expand-icon">{{ expandedShows.has(g.id) ? '▼' : '▶' }}</span>
          <span class="item-type show">Show</span>
          <span class="item-title">{{ g.title }}</span>
          <span class="item-year" v-if="g.year">{{ g.year }}</span>
          <span class="item-library">{{ g.library_id }}</span>
          <span class="item-provider">{{ g.provider_id }}</span>
          <span class="item-date">{{ formatDate(g.created_at) }}</span>
          <span class="ep-count">{{ g.episodeCount }} ep.</span>
          <button class="delete-btn" @click.stop="deleteItem(g)">Delete</button>
        </div>
        <!-- Episode rows (nested under show) -->
        <div v-if="g.type === 'show' && expandedShows.has(g.id)" class="episode-children">
          <div v-for="ep in g.episodes" :key="ep.id" class="media-row ep-row">
            <span class="expand-placeholder"></span>
            <span class="item-type episode">Episode</span>
            <span class="ep-number">{{ ep.season }}x{{ String(ep.episode).padStart(2, '0') }}</span>
            <span class="item-title ep-title">{{ ep.title }}</span>
            <span class="item-year" v-if="ep.year">{{ ep.year }}</span>
            <span class="item-library">{{ ep.library_id }}</span>
            <span class="item-provider">{{ ep.provider_id }}</span>
            <span class="item-date">{{ formatDate(ep.created_at) }}</span>
            <button class="delete-btn" @click.stop="deleteItem(ep)">Delete</button>
          </div>
        </div>
        <!-- Movie row (standalone, no expand) -->
        <div v-else-if="g.type !== 'show'" class="media-row">
          <span class="expand-placeholder"></span>
          <span class="item-type movie">Movie</span>
          <span class="item-title">{{ g.title }}</span>
          <span class="item-year" v-if="g.year">{{ g.year }}</span>
          <span class="item-library">{{ g.library_id }}</span>
          <span class="item-provider">{{ g.provider_id }}</span>
          <span class="item-date">{{ formatDate(g.created_at) }}</span>
          <button class="delete-btn" @click.stop="deleteItem(g)">Delete</button>
        </div>
      </template>
    </div>

    <div class="pagination" v-if="total > limit">
      <button
        :disabled="page <= 1"
        @click="
          page--;
          fetchItems();
        "
      >
        &larr; Previous
      </button>
      <span class="page-info">Page {{ page }}</span>
      <button
        :disabled="page * limit >= total"
        @click="
          page++;
          fetchItems();
        "
      >
        Next &rarr;
      </button>
    </div>

    <p class="empty" v-else-if="!loading && items.length === 0">No items found.</p>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue';
import { authRequest } from '@/api/request';
import type { ApiEnvelope } from '@/api/types';

/* =========================
   Types
   ========================= */

interface AdminItem {
  id: string;
  type: 'movie' | 'show' | 'episode';
  title: string;
  year?: number;
  library_id: string;
  provider_id: string;
  created_at: string;
  parent_id?: string;
  season?: number;
  episode?: number;
}

interface MediaResponse {
  items: AdminItem[];
  total: number;
}

interface Library {
  id: string;
  name: string;
}

interface ShowGroup extends AdminItem {
  episodeCount: number;
  episodes: AdminItem[];
}

/* =========================
   State
   ========================= */

const items = ref<AdminItem[]>([]);
const total = ref(0);
const page = ref(1);
const limit = 20;

const loading = ref(false);

const searchQuery = ref('');
const typeFilter = ref('');
const libraryFilter = ref('');

const libraries = ref<Library[]>([]);
const expandedShows = ref(new Set<string>());

let searchDebounce: number | null = null;

/* =========================
   Derived
   ========================= */

const groupedItems = computed<ShowGroup[]>(() => {
  if (typeFilter.value === 'episode') {
    return items.value.map((i) => ({
      ...i,
      episodeCount: 0,
      episodes: [],
    }));
  }

  const shows = new Map<string, ShowGroup>();
  const standalone: ShowGroup[] = [];

  for (const item of items.value) {
    if (item.type === 'show') {
      shows.set(item.id, {
        ...item,
        episodeCount: 0,
        episodes: [],
      });
    }
  }

  for (const item of items.value) {
    if (item.type === 'episode' && item.parent_id && shows.has(item.parent_id)) {
      const show = shows.get(item.parent_id)!;
      show.episodes.push(item);
      show.episodeCount = show.episodes.length;
    } else if (item.type !== 'show') {
      standalone.push({
        ...item,
        episodeCount: 0,
        episodes: [],
      });
    }
  }

  return [...shows.values(), ...standalone];
});

/* =========================
   Data Loading
   ========================= */

async function loadLibraries(): Promise<void> {
  try {
    const res = await authRequest.get<ApiEnvelope<Library[]>>('/admin/libraries');
    libraries.value = res.data.data || [];
  } catch (err: any) {
    if (err?.response?.status === 401) return;
    console.error('[media] loadLibraries failed', err);
  }
}

async function fetchItems(): Promise<void> {
  loading.value = true;

  try {
    const params: Record<string, any> = {
      page: page.value,
      limit,
      sort: 'created_at_desc',
    };

    if (searchQuery.value) params.q = searchQuery.value;
    if (typeFilter.value) params.type = typeFilter.value;
    if (libraryFilter.value) params.library_id = libraryFilter.value;

    const res = await authRequest.get<ApiEnvelope<MediaResponse>>('/admin/media', { params });

    items.value = res.data.data.items || [];
    total.value = res.data.data.total || 0;
  } catch (err: any) {
    if (err?.response?.status === 401) return;
    console.error('[media] fetchItems failed', err);
  } finally {
    loading.value = false;
  }
}

/* =========================
   Search (debounced)
   ========================= */

function triggerSearch(): void {
  if (searchDebounce) window.clearTimeout(searchDebounce);

  searchDebounce = window.setTimeout(() => {
    page.value = 1;
    fetchItems();
  }, 300);
}

/* =========================
   Watchers (cleaner than inline handlers)
   ========================= */

watch(typeFilter, () => {
  page.value = 1;
  fetchItems();
});

watch(libraryFilter, () => {
  page.value = 1;
  fetchItems();
});

/* =========================
   Actions
   ========================= */

function toggleShow(id: string): void {
  const next = new Set(expandedShows.value);

  if (next.has(id)) next.delete(id);
  else next.add(id);

  expandedShows.value = next;
}

async function deleteItem(item: AdminItem): Promise<void> {
  const label = item.type === 'show' ? `"${item.title}" and all episodes` : `"${item.title}"`;

  if (!confirm(`Delete ${label}?`)) return;

  try {
    await authRequest.delete(`/admin/media/${item.id}`);
    await fetchItems();
  } catch (err: any) {
    if (err?.response?.status === 401) return;

    alert('Delete failed: ' + (err?.response?.data?.message || 'Unknown error'));
  }
}

/* =========================
   Utils
   ========================= */

function formatDate(iso: string): string {
  if (!iso) return '';
  return new Date(iso).toLocaleDateString();
}

/* =========================
   Lifecycle
   ========================= */

onMounted(async () => {
  await loadLibraries();
  await fetchItems();
});
</script>

<style scoped>
h1 {
  font-size: 22px;
  color: #e0e0e0;
  margin: 0 0 20px;
}

.toolbar {
  display: flex;
  gap: 12px;
  margin-bottom: 16px;
}

.search-input {
  flex: 1;
  padding: 8px 12px;
  background: #0a0a14;
  border: 1px solid #1a1a2e;
  border-radius: 4px;
  color: #ccccee;
  font-size: 13px;
  outline: none;
}

.search-input:focus {
  border-color: #6c63ff;
}

.filter-select {
  padding: 8px 10px;
  background: #0a0a14;
  border: 1px solid #1a1a2e;
  border-radius: 4px;
  color: #aaaacc;
  font-size: 13px;
}

.result-info {
  color: #555577;
  font-size: 12px;
  margin-bottom: 12px;
}

.media-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.media-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 12px;
  background: #12121e;
  border-radius: 4px;
  font-size: 13px;
}

.media-row:hover {
  background: #1a1a32;
}

.show-row {
  cursor: pointer;
}

.expand-icon {
  color: #6c63ff;
  font-size: 10px;
  min-width: 16px;
}

.expand-placeholder {
  min-width: 16px;
}

.episode-children {
  display: flex;
  flex-direction: column;
  gap: 1px;
  padding-left: 24px;
}

.ep-row {
  background: #0e0e18;
  font-size: 12px;
}

.ep-row:hover {
  background: #16162e;
}

.item-type {
  font-size: 11px;
  padding: 2px 6px;
  border-radius: 3px;
  min-width: 60px;
  text-align: center;
  font-weight: 600;
  text-transform: uppercase;
}

.item-type.movie {
  background: #1a2a3e;
  color: #4fc3f7;
}

.item-type.show {
  background: #2a1a3e;
  color: #ab47bc;
}

.item-type.episode {
  background: #1a3a2a;
  color: #66bb6a;
}

.item-title {
  flex: 1;
  color: #ccccee;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.ep-title {
  font-size: 12px;
}

.ep-number {
  color: #6c63ff;
  font-weight: 600;
  font-size: 11px;
  min-width: 40px;
}

.item-year,
.item-library,
.item-provider {
  color: #555577;
  font-size: 12px;
  min-width: 60px;
}

.item-date {
  color: #555577;
  font-size: 12px;
  min-width: 80px;
}

.ep-count {
  color: #555577;
  font-size: 11px;
  min-width: 40px;
}

.delete-btn {
  padding: 4px 10px;
  border: 1px solid #3a1a1a;
  border-radius: 4px;
  background: transparent;
  color: #ff6b6b;
  cursor: pointer;
  font-size: 11px;
}

.delete-btn:hover {
  background: #2a1a1a;
}

.pagination {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-top: 16px;
  justify-content: center;
}

.pagination button {
  padding: 6px 14px;
  background: #12121e;
  border: 1px solid #1a1a2e;
  border-radius: 4px;
  color: #aaaacc;
  cursor: pointer;
  font-size: 12px;
}

.pagination button:disabled {
  opacity: 0.3;
  cursor: not-allowed;
}

.pagination button:hover:not(:disabled) {
  background: #1a1a32;
}

.page-info {
  color: #555577;
  font-size: 13px;
}

.empty {
  color: #555577;
  text-align: center;
  padding: 40px 0;
}
</style>
