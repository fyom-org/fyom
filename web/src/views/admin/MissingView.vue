<template>
  <div class="admin-page">
    <div class="page-header">
      <h1>Missing Items</h1>
      <button class="danger-btn" @click="deleteAllMissing"
              :disabled="deleting || items.length === 0">
        {{ deleting ? 'Deleting...' : 'Delete All Missing' }}
      </button>
    </div>

    <p class="hint">Items whose files no longer exist on disk. These are hidden from users automatically.</p>

    <div class="toolbar" v-if="libraries.length > 1">
      <select v-model="libraryFilter" @change="fetchMissing()" class="filter-select">
        <option value="">All Libraries</option>
        <option v-for="lib in libraries" :key="lib.id" :value="lib.id">{{ lib.name }}</option>
      </select>
    </div>

    <div class="result-info" v-if="total > 0">
      {{ total }} missing item{{ total !== 1 ? 's' : '' }}
    </div>

    <div class="missing-list" v-if="items.length > 0">
      <div class="missing-row" v-for="item in items" :key="item.id">
        <span class="item-type" :class="item.type">{{ item.type }}</span>
        <span class="item-title">{{ item.title }}</span>
        <span class="item-path">{{ item.file_path }}</span>
        <span class="item-library">{{ item.library_id }}</span>
        <button class="delete-btn" @click="deleteSingle(item.id)">Remove</button>
      </div>
    </div>

    <div class="all-clear" v-else-if="!loading">
      <span class="check-icon">&#10003;</span>
      <p>All items are available on disk.</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import request from '@/api/request';

const items = ref<any[]>([]);
const total = ref(0);
const loading = ref(true);
const deleting = ref(false);
const libraryFilter = ref('');
const libraries = ref<any[]>([]);

onMounted(async () => {
  try {
    const res: any = await request.get('/admin/libraries');
    libraries.value = res.data || [];
  } catch {
    // ignore
  }
  await fetchMissing();
});

async function fetchMissing() {
  loading.value = true;
  try {
    const params: any = {};
    if (libraryFilter.value) params.library_id = libraryFilter.value;
    const res: any = await request.get('/admin/media/missing', { params });
    items.value = res.data?.items || [];
    total.value = res.data?.total || 0;
  } catch {
    // ignore
  } finally {
    loading.value = false;
  }
}

async function deleteSingle(id: string) {
  try {
    await request.delete(`/admin/media/${id}`);
    await fetchMissing();
  } catch {
    // ignore
  }
}

async function deleteAllMissing() {
  if (!confirm(`Delete all ${total.value} missing item${total.value !== 1 ? 's' : ''}? This cannot be undone.`)) return;
  deleting.value = true;
  try {
    const body: any = {};
    if (libraryFilter.value) body.library_id = libraryFilter.value;
    const res: any = await request.delete('/admin/media/missing', { data: body });
    alert(`Deleted ${res.data?.deleted_count || 0} items`);
    await fetchMissing();
  } catch {
    // ignore
  } finally {
    deleting.value = false;
  }
}
</script>

<style scoped>
.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8px;
}

h1 {
  font-size: 22px;
  color: #e0e0e0;
  margin: 0;
}

.hint {
  color: #555577;
  font-size: 13px;
  margin-bottom: 20px;
}

.danger-btn {
  padding: 8px 16px;
  background: #1a0f0f;
  color: #ff6b6b;
  border: 1px solid rgba(255, 107, 107, 0.3);
  border-radius: 6px;
  cursor: pointer;
  font-size: 13px;
}

.danger-btn:hover {
  background: #2a1515;
}

.danger-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.toolbar {
  margin-bottom: 16px;
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
  color: #ff9800;
  font-size: 13px;
  margin-bottom: 12px;
}

.missing-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.missing-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 12px;
  background: #12121e;
  border-radius: 4px;
  border-left: 2px solid rgba(255, 152, 0, 0.5);
  font-size: 13px;
}

.item-type {
  font-size: 11px;
  padding: 2px 6px;
  border-radius: 3px;
  min-width: 50px;
  text-align: center;
  text-transform: uppercase;
  background: #1a1510;
  color: #ff9800;
}

.item-title {
  color: #ccccee;
  min-width: 120px;
}

.item-path {
  flex: 1;
  color: #555577;
  font-size: 12px;
  font-family: monospace;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.item-library {
  color: #555577;
  font-size: 12px;
  min-width: 80px;
}

.delete-btn {
  padding: 4px 10px;
  border: 1px solid rgba(255, 107, 107, 0.3);
  border-radius: 4px;
  background: transparent;
  color: #ff6b6b;
  cursor: pointer;
  font-size: 11px;
}

.delete-btn:hover {
  background: #1a0f0f;
}

.all-clear {
  text-align: center;
  padding: 60px 20px;
}

.check-icon {
  font-size: 48px;
  color: #4caf50;
  display: block;
  margin-bottom: 16px;
}

.all-clear p {
  color: #555577;
  font-size: 15px;
  margin: 0;
}
</style>
