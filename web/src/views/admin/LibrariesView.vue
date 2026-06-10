<template>
  <div class="admin-page">
    <div class="page-header">
      <h1>Libraries</h1>
      <button class="add-btn" @click="showForm = !showForm">
        {{ showForm ? 'Cancel' : '+ New Library' }}
      </button>
    </div>

    <div class="form-card" v-if="showForm">
      <div class="field">
        <label>Name</label>
        <input v-model="form.name" placeholder="e.g. Kids Movies" />
      </div>
      <div class="field">
        <label>Type</label>
        <select v-model="form.type">
          <option value="mixed">Mixed</option>
          <option value="movie">Movies Only</option>
          <option value="show">TV Shows Only</option>
        </select>
      </div>
      <div class="field">
        <label>Storage Provider</label>
        <select v-model="form.provider_id">
          <option value="local">Local Disk</option>
          <option v-for="p in providers" :key="p.id" :value="p.id">
            {{ p.display_name }} ({{ p.type }})
          </option>
        </select>
      </div>
      <div class="field">
        <label>Source Path</label>
        <input v-model="form.source_path" placeholder="/path/to/media or S3 prefix" />
      </div>
      <div class="field">
        <label>Metadata Source</label>
        <select v-model="form.metadata_source">
          <option value="nfo">NFO Files (Kodi/tinyMediaManager)</option>
          <option value="filename">Filename Only</option>
        </select>
      </div>
      <p class="error-text" v-if="error">{{ error }}</p>
      <button class="submit-btn" @click="createLibrary">Create Library</button>
    </div>

    <div class="library-list" v-if="libraries.length > 0">
      <div class="library-card" v-for="lib in libraries" :key="lib.id">
        <div class="library-header">
          <div class="library-info">
            <h3 class="library-name">{{ lib.name }}</h3>
            <span class="library-meta">
              {{ typeLabel(lib.type) }} &middot; {{ lib.provider_id }} &middot; {{ lib.metadata_source }}
            </span>
          </div>
          <button class="delete-btn" v-if="lib.id !== 'default'"
                  @click="deleteLibrary(lib)">Delete</button>
          <span class="default-badge" v-else>Default</span>
        </div>
        <div class="library-stats" v-if="lib.item_count > 0">
          <span class="stat">{{ lib.item_count }} items</span>
          <span class="stat" v-if="lib.movie_count">{{ lib.movie_count }} movies</span>
          <span class="stat" v-if="lib.show_count">{{ lib.show_count }} shows</span>
          <span class="stat" v-if="lib.episode_count">{{ lib.episode_count }} episodes</span>
        </div>
        <div class="library-stats" v-else>
          <span class="stat empty">No items yet</span>
        </div>
      </div>
    </div>
    <p class="empty-text" v-else-if="!loading">No libraries configured.</p>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import request from '@/api/request';

const libraries = ref<any[]>([]);
const loading = ref(true);
const error = ref('');
const showForm = ref(false);
const form = ref({
  name: '', type: 'mixed', provider_id: 'local',
  source_path: '', metadata_source: 'nfo',
});
const providers = ref<any[]>([]);

onMounted(async () => {
  try {
    const [libRes, provRes] = await Promise.all([
      request.get('/admin/libraries'),
      request.get('/admin/providers'),
    ]);
    libraries.value = (libRes as any).data || [];
    providers.value = (provRes as any).data || [];
  } catch {
    error.value = 'Failed to load';
  } finally {
    loading.value = false;
  }
});

async function createLibrary() {
  error.value = '';
  try {
    await request.post('/admin/libraries', form.value);
    showForm.value = false;
    form.value = { name: '', type: 'mixed', provider_id: 'local', source_path: '', metadata_source: 'nfo' };
    await fetchLibraries();
  } catch (e: any) {
    error.value = e.response?.data?.message || 'Failed';
  }
}

async function deleteLibrary(lib: any) {
  if (!confirm(`Delete "${lib.name}"? This will fail if it has items.`)) return;
  try {
    await request.delete(`/admin/libraries/${lib.id}`);
    await fetchLibraries();
  } catch (e: any) {
    error.value = e.response?.data?.message || 'Failed to delete';
  }
}

async function fetchLibraries() {
  const res: any = await request.get('/admin/libraries');
  libraries.value = res.data || [];
}

function typeLabel(type: string) {
  switch (type) {
    case 'movie': return 'Movies';
    case 'show': return 'TV Shows';
    default: return 'Mixed';
  }
}
</script>

<style scoped>
.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;
}

h1 {
  font-size: 22px;
  color: #e0e0e0;
  margin: 0;
}

.add-btn {
  padding: 8px 16px;
  background: #6c63ff;
  color: #fff;
  border: none;
  border-radius: 6px;
  cursor: pointer;
  font-size: 13px;
}

.add-btn:hover {
  background: #5a52e0;
}

.form-card {
  background: #12121e;
  padding: 20px;
  border-radius: 8px;
  margin-bottom: 20px;
  border: 1px solid #1a1a2e;
}

.field label {
  display: block;
  color: #666688;
  font-size: 12px;
  margin-bottom: 4px;
}

.field input,
.field select {
  width: 100%;
  padding: 8px 10px;
  background: #0a0a14;
  border: 1px solid #1a1a2e;
  border-radius: 4px;
  color: #ccccee;
  font-size: 13px;
  box-sizing: border-box;
}

.field input:focus,
.field select:focus {
  border-color: #6c63ff;
  outline: none;
}

.field + .field {
  margin-top: 12px;
}

.submit-btn {
  margin-top: 16px;
  padding: 8px 20px;
  background: #6c63ff;
  color: #fff;
  border: none;
  border-radius: 6px;
  cursor: pointer;
  font-size: 13px;
}

.submit-btn:hover {
  background: #5a52e0;
}

.error-text {
  color: #ff6b6b;
  font-size: 12px;
  margin-top: 8px;
}

.library-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.library-card {
  background: #12121e;
  border-radius: 8px;
  border: 1px solid #1a1a2e;
  padding: 16px 20px;
}

.library-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.library-name {
  font-size: 16px;
  color: #e0e0e0;
  margin: 0;
}

.library-meta {
  color: #555577;
  font-size: 12px;
  margin-left: 8px;
}

.library-stats {
  margin-top: 10px;
  display: flex;
  gap: 12px;
}

.stat {
  color: #8888aa;
  font-size: 12px;
}

.stat.empty {
  color: #555577;
  font-style: italic;
}

.delete-btn {
  padding: 4px 12px;
  border: 1px solid #3a1a1a;
  border-radius: 4px;
  background: transparent;
  color: #ff6b6b;
  cursor: pointer;
  font-size: 12px;
}

.delete-btn:hover {
  background: #2a1a1a;
}

.default-badge {
  color: #555577;
  font-size: 11px;
  background: #1a1a2e;
  padding: 2px 8px;
  border-radius: 4px;
}

.empty-text {
  color: #555577;
  font-size: 14px;
}
</style>
