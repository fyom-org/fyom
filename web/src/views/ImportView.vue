<template>
  <div class="import-view">
    <h2>Import Media Library</h2>
    <p class="hint">
      Choose a storage provider and specify the source path / S3 prefix to import from.
    </p>

    <div class="form">
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
import { triggerImport } from '@/api/library';
import JobStatus from '@/components/JobStatus.vue';

interface Provider {
  id: string;
  type: string;
  display_name: string;
}

const providers = ref<Provider[]>([]);
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
    const res = await fetch('/api/v1/admin/providers', {
      headers: {
        Authorization: `Bearer ${localStorage.getItem('token') || ''}`,
      },
    });
    if (res.ok) {
      const data = await res.json();
      providers.value = data.data || [];
    }
  } catch {
    // ignore — user may not have admin role
  } finally {
    loadingProviders.value = false;
  }
  // Always include local as first option
  providers.value.unshift({ id: 'local', type: 'local', display_name: 'Local Disk' });
});

async function handleImport() {
  const path = sourcePath.value.trim();
  if (!path) return;

  error.value = '';
  jobId.value = '';
  importing.value = true;

  try {
    const res = await triggerImport(path, selectedProvider.value);
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
.import-view {
  max-width: 600px;
}

h2 {
  margin: 0 0 4px;
  font-size: 20px;
}

.hint {
  color: #a1a1aa;
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
  color: #a1a1aa;
}

.provider-select {
  padding: 8px 12px;
  background: #27272a;
  border: 1px solid #3f3f46;
  color: #e4e4e7;
  border-radius: 6px;
  font-size: 14px;
  outline: none;
  cursor: pointer;
}

.provider-select:focus {
  border-color: #60a5fa;
}

.provider-select option {
  background: #27272a;
  color: #e4e4e7;
}

input {
  flex: 1;
  background: #27272a;
  border: 1px solid #3f3f46;
  color: #e4e4e7;
  padding: 8px 12px;
  border-radius: 6px;
  font-size: 14px;
  outline: none;
}

input:focus {
  border-color: #60a5fa;
}

input:disabled {
  opacity: 0.5;
}

button {
  background: #3b82f6;
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
  background: #2563eb;
}

button:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.error {
  color: #f87171;
  margin-top: 12px;
  font-size: 13px;
}
</style>
