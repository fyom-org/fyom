<template>
  <div class="admin-page">
    <h1>Import Media</h1>
    <p class="hint">
      Choose a target library, storage provider, and specify the source path to import from.
    </p>

    <div class="form">
      <div class="field">
        <label>Target Library</label>
        <select v-model="selectedLibrary" class="library-select">
          <option v-for="lib in libraries" :key="lib.id" :value="lib.id">
            {{ lib.name }}
          </option>
        </select>
      </div>

      <div class="field">
        <label>Storage Provider</label>
        <select v-model="selectedProvider" class="provider-select" :disabled="loadingProviders">
          <option v-for="p in providers" :key="p.id" :value="p.id">
            {{ p.display_name }} ({{ p.type }})
          </option>
        </select>
      </div>

      <div class="field">
        <label>{{ isLocal ? 'Directory Path' : 'S3 Prefix' }}</label>
        <input
          v-model="sourcePath"
          type="text"
          :placeholder="pathPlaceholder"
          :disabled="importing"
          @keyup.enter="handleImport"
        />
      </div>

      <button :disabled="importing || !sourcePath.trim()" @click="handleImport">
        {{ importing ? 'Starting...' : 'Start Import' }}
      </button>
    </div>

    <p v-if="error" class="error">{{ error }}</p>

    <JobStatus v-if="jobId" :job-id="jobId" />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import request from '@/api/request';
import JobStatus from '@/components/JobStatus.vue';

interface Provider {
  id: string;
  type: string;
  display_name: string;
}

interface Library {
  id: string;
  name: string;
}

const libraries = ref<Library[]>([]);
const providers = ref<Provider[]>([]);
const selectedLibrary = ref('default');
const selectedProvider = ref('local');
const sourcePath = ref('');
const importing = ref(false);
const loadingProviders = ref(true);
const jobId = ref('');
const error = ref('');

const isLocal = computed(() => selectedProvider.value === 'local');
const pathPlaceholder = computed(() =>
  isLocal.value ? '/path/to/media/library' : 'S3 prefix (e.g. Shows/ or Movies/)'
);

onMounted(async () => {
  try {
    const [libRes, provRes] = await Promise.all([
      request.get('/admin/libraries'),
      request.get('/admin/providers'),
    ]);
    libraries.value = (libRes as any).data || [];
    providers.value = (provRes as any).data || [];
  } catch {
    // ignore
  } finally {
    loadingProviders.value = false;
  }
  providers.value.unshift({ id: 'local', type: 'local', display_name: 'Local Disk' });
});

async function handleImport() {
  const path = sourcePath.value.trim();
  if (!path) return;

  error.value = '';
  jobId.value = '';
  importing.value = true;

  try {
    const res: any = await request.post('/library/import', {
      source_path: path,
      provider_id: selectedProvider.value,
      library_id: selectedLibrary.value,
    });
    jobId.value = res.data.job_id;
  } catch (err: unknown) {
    console.error('[fyom] import trigger failed:', err);
    if (err instanceof Error) {
      error.value = err.message;
    } else {
      error.value = 'Failed to start import';
    }
  } finally {
    importing.value = false;
  }
}
</script>

<style scoped>
.admin-page h1 {
  font-size: 22px;
  color: #e0e0e0;
  margin-bottom: 16px;
}
.hint {
  color: #555577;
  font-size: 13px;
  margin: 0 0 20px;
}

.form {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.field {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

label {
  font-size: 13px;
  color: #666688;
}

.library-select,
.provider-select {
  padding: 8px 12px;
  background: #0e0e1a;
  border: 1px solid #1a1a2e;
  color: #ccccee;
  border-radius: 6px;
  font-size: 14px;
  outline: none;
  cursor: pointer;
}

.library-select:focus,
.provider-select:focus {
  border-color: #6c63ff;
}

.provider-select option {
  background: #0e0e1a;
  color: #ccccee;
}

input {
  flex: 1;
  background: #0e0e1a;
  border: 1px solid #1a1a2e;
  color: #ccccee;
  padding: 8px 12px;
  border-radius: 6px;
  font-size: 14px;
  outline: none;
}

input:focus {
  border-color: #6c63ff;
}

input:disabled {
  opacity: 0.5;
}

button {
  background: #6c63ff;
  color: #fff;
  border: none;
  padding: 8px 20px;
  border-radius: 6px;
  font-size: 14px;
  font-weight: 500;
  cursor: pointer;
  white-space: nowrap;
}

button:hover:not(:disabled) {
  background: #5a52e0;
}

button:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.error {
  color: #ff6b6b;
  margin-top: 12px;
  font-size: 13px;
}
</style>
