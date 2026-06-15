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
      </div>
      <div class="library-actions">
        <button class="action-btn refresh" @click="refreshLibrary(lib)"
                :disabled="refreshing === lib.id">
          {{ refreshing === lib.id ? 'Scanning...' : '↻ Refresh' }}
        </button>
        <button class="action-btn check" @click="checkMissing(lib)"
                :disabled="checking === lib.id">
          {{ checking === lib.id ? 'Checking...' : '⊕ Check Missing' }}
        </button>
        <!-- Schedule selector -->
        <select class="schedule-select" :value="getSchedule(lib.id)"
                @change="setSchedule(lib.id, ($event.target as HTMLSelectElement).value)"
                :disabled="savingSchedule === lib.id">
          <option value="0">Manual</option>
          <option value="3600">Every hour</option>
          <option value="21600">Every 6 hours</option>
          <option value="86400">Daily</option>
          <option value="604800">Weekly</option>
        </select>
        <button class="action-btn delete" @click="deleteLibrary(lib)">
          Delete
        </button>
      </div>
        <div class="library-stats" v-if="lib.item_count > 0">
          <span class="stat">{{ lib.item_count }} items</span>
          <span class="stat" v-if="lib.movie_count">{{ lib.movie_count }} movies</span>
          <span class="stat" v-if="lib.show_count">{{ lib.show_count }} shows</span>
          <span class="stat" v-if="lib.episode_count">{{ lib.episode_count }} episodes</span>
          <span class="stat warn" v-if="lib.missing_count > 0">
            {{ lib.missing_count }} missing
          </span>
        </div>
        <div class="library-stats" v-else>
          <span class="stat empty">No items yet</span>
        </div>
        <JobStatus v-if="activeJobId" :job-id="activeJobId" />
      </div>
    </div>
    <p class="empty-text" v-else-if="!loading">No libraries configured.</p>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { apiRequest } from '@/api/request';
import JobStatus from '@/components/JobStatus.vue';

const libraries = ref<any[]>([]);
const loading = ref(true);
const error = ref('');
const showForm = ref(false);
const refreshing = ref('');
const checking = ref('');
const savingSchedule = ref('');
const activeJobId = ref('');
const form = ref({
  name: '', type: 'mixed', provider_id: 'local',
  source_path: '/', metadata_source: 'nfo',
});
const providers = ref<any[]>([]);
const schedules = ref<Record<string, string>>({});

onMounted(async () => {
  try {
    const [libRes, provRes, settingsRes] = await Promise.all([
      request.get('/admin/libraries'),
      request.get('/admin/providers'),
      request.get('/admin/settings'),
    ]);
    libraries.value = (libRes as any).data || [];
    providers.value = (provRes as any).data || [];
    const settings = (settingsRes as any).data || {};
    // Extract library refresh schedules from settings
    for (const [key, val] of Object.entries(settings)) {
      if (key.startsWith('library_refresh_interval_')) {
        const libId = key.slice('library_refresh_interval_'.length);
        schedules.value[libId] = val as string;
      }
    }
  } catch {
    error.value = 'Failed to load';
  } finally {
    loading.value = false;
  }
});

function getSchedule(libId: string): string {
  return schedules.value[libId] || '0';
}

async function setSchedule(libId: string, interval: string) {
  savingSchedule.value = libId;
  try {
    await request.put('/admin/settings', {
      [`library_refresh_interval_${libId}`]: interval,
    });
    schedules.value = { ...schedules.value, [libId]: interval };
  } catch (e: any) {
    alert('Failed to save schedule: ' + (e.response?.data?.message || 'Unknown error'));
  } finally {
    savingSchedule.value = '';
  }
}

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
  if (lib.item_count > 0) {
    const confirmed = confirm(
      `"${lib.name}" has ${lib.item_count} items. Delete everything?`
    );
    if (!confirmed) return;
    try {
      await request.delete(`/admin/libraries/${lib.id}/items?mode=cascade`);
      await fetchLibraries();
    } catch (e: any) {
      error.value = e.response?.data?.message || 'Failed to delete';
    }
    return;
  }
  if (!confirm(`Delete "${lib.name}"?`)) return;
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

async function refreshLibrary(lib: any) {
  refreshing.value = lib.id;
  activeJobId.value = '';
  try {
    const res: any = await request.post(`/admin/libraries/${lib.id}/refresh`);
    const config = res.data;
    const jobRes: any = await request.post('/library/import', {
      source_path: config.source_path,
      provider_id: config.provider_id,
      library_id: config.id,
    });
    activeJobId.value = jobRes.data?.job_id || '';
  } catch (e: any) {
    alert('Refresh failed: ' + (e.response?.data?.message || 'Unknown error'));
  } finally {
    refreshing.value = '';
  }
}

async function checkMissing(lib: any) {
  checking.value = lib.id;
  try {
    const res: any = await request.post(`/admin/libraries/${lib.id}/check-missing`);
    const result = res.data;
    if (result.missing > 0) {
      alert(`Found ${result.missing} missing item${result.missing !== 1 ? 's' : ''}.\nView them in the Missing Items page.`);
    } else {
      alert('All items are available ✓');
    }
    await fetchLibraries();
  } catch (e: any) {
    alert('Check failed: ' + (e.response?.data?.message || 'Unknown error'));
  } finally {
    checking.value = '';
  }
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

.library-actions {
  display: flex;
  gap: 8px;
  margin-top: 12px;
  align-items: center;
}

.action-btn {
  padding: 6px 12px;
  border: 1px solid #1a1a2e;
  border-radius: 4px;
  background: transparent;
  color: #8888aa;
  cursor: pointer;
  font-size: 12px;
  transition: all 0.15s;
}

.action-btn:hover {
  border-color: #2a2a3e;
  color: #ccccee;
}

.action-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.action-btn.refresh:hover {
  border-color: rgba(33, 150, 243, 0.5);
  color: #2196f3;
}

.action-btn.check:hover {
  border-color: rgba(255, 152, 0, 0.5);
  color: #ff9800;
}

.action-btn.delete {
  border-color: rgba(255, 107, 107, 0.3);
  color: #ff6b6b;
}

.action-btn.delete:hover {
  background: #1a0f0f;
}

.schedule-select {
  padding: 5px 8px;
  background: #0a0a14;
  border: 1px solid #1a1a2e;
  border-radius: 4px;
  color: #aaaacc;
  font-size: 12px;
  cursor: pointer;
}

.schedule-select:focus {
  border-color: #6c63ff;
  outline: none;
}

.schedule-select option {
  background: #0a0a14;
  color: #ccccee;
}

.stat.warn {
  color: #ff9800;
}

.empty-text {
  color: #555577;
  font-size: 14px;
}
</style>