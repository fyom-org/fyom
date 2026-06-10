<template>
  <div class="admin-page">
    <h1>Media Items</h1>

    <div class="toolbar">
      <input v-model="searchQuery" @input="onSearchInput" type="text"
             placeholder="Search items..." class="search-input" />
      <select v-model="typeFilter" @change="page = 1; fetchItems()" class="filter-select">
        <option value="">All Types</option>
        <option value="movie">Movies</option>
        <option value="show">Shows</option>
        <option value="episode">Episodes</option>
      </select>
      <select v-model="libraryFilter" @change="page = 1; fetchItems()" class="filter-select">
        <option value="">All Libraries</option>
        <option v-for="lib in libraries" :key="lib.id" :value="lib.id">{{ lib.name }}</option>
      </select>
    </div>

    <div class="result-info" v-if="total > 0">{{ total }} items</div>

    <div class="media-list" v-if="items.length > 0">
      <div class="media-row" v-for="item in items" :key="item.id">
        <span class="item-type" :class="item.type">{{ typeBadge(item.type) }}</span>
        <span class="item-title">{{ item.title }}</span>
        <span class="item-year" v-if="item.year">{{ item.year }}</span>
        <span class="item-library">{{ item.library_id }}</span>
        <span class="item-provider">{{ item.provider_id }}</span>
        <span class="item-date">{{ formatDate(item.created_at) }}</span>
        <button class="delete-btn" @click="deleteItem(item)">Delete</button>
      </div>
    </div>

    <div class="pagination" v-if="total > limit">
      <button :disabled="page <= 1" @click="page--; fetchItems()">&larr; Previous</button>
      <span class="page-info">Page {{ page }}</span>
      <button :disabled="page * limit >= total" @click="page++; fetchItems()">Next &rarr;</button>
    </div>

    <p class="empty" v-else-if="!loading && items.length === 0">No items found.</p>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import request from '@/api/request';

const items = ref<any[]>([]);
const total = ref(0);
const page = ref(1);
const limit = 20;
const loading = ref(true);
const searchQuery = ref('');
const typeFilter = ref('');
const libraryFilter = ref('');
const libraries = ref<any[]>([]);
let searchTimer: any = 0;

onMounted(async () => {
  try {
    const res: any = await request.get('/admin/libraries');
    libraries.value = res.data || [];
  } catch {
    // ignore
  }
  await fetchItems();
});

async function fetchItems() {
  loading.value = true;
  try {
    const params: any = { page: page.value, limit, sort: 'created_at_desc' };
    if (searchQuery.value) params.q = searchQuery.value;
    if (typeFilter.value) params.type = typeFilter.value;
    if (libraryFilter.value) params.library_id = libraryFilter.value;
    const res: any = await request.get('/admin/media', { params });
    items.value = res.data?.items || [];
    total.value = res.data?.total || 0;
  } catch {
    // ignore
  } finally {
    loading.value = false;
  }
}

function onSearchInput() {
  clearTimeout(searchTimer);
  searchTimer = setTimeout(() => {
    page.value = 1;
    fetchItems();
  }, 300);
}

async function deleteItem(item: any) {
  const label = item.type === 'show'
    ? `"${item.title}" and all its episodes`
    : `"${item.title}"`;
  if (!confirm(`Delete ${label}?`)) return;
  try {
    await request.delete(`/admin/media/${item.id}`);
    await fetchItems();
  } catch {
    // ignore
  }
}

function formatDate(iso: string) {
  if (!iso) return '';
  return new Date(iso).toLocaleDateString();
}

function typeBadge(type: string) {
  switch (type) {
    case 'movie': return 'Movie';
    case 'show': return 'Show';
    case 'episode': return 'Episode';
    default: return type;
  }
}
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

.item-year {
  color: #555577;
  min-width: 40px;
}

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
