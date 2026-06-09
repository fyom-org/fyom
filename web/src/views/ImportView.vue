<template>
  <div class="import-view">
    <h2>Import Media Library</h2>
    <p class="hint">Enter the absolute server-side path to your media directory.</p>

    <div class="form">
      <input
        v-model="dirPath"
        type="text"
        placeholder="/media/movies"
        :disabled="importing"
        @keyup.enter="handleImport"
      />
      <button @click="handleImport" :disabled="importing || !dirPath.trim()">
        {{ importIng ? 'Starting...' : 'Start Import' }}
      </button>
    </div>

    <p v-if="error" class="error">{{ error }}</p>

    <JobStatus v-if="jobId" :job-id="jobId" />
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { triggerImport } from '@/api/library'
import JobStatus from '@/components/JobStatus.vue'

const dirPath = ref('')
const importing = ref(false)
const jobId = ref('')
const error = ref('')

async function handleImport() {
  const path = dirPath.value.trim()
  if (!path) return

  error.value = ''
  jobId.value = ''
  importing.value = true

  try {
    const res = await triggerImport(path)
    jobId.value = res.data.job_id
  } catch (err) {
    console.error('[fyom] import trigger failed:', err)
    error.value = err instanceof Error ? err.message : 'Failed to start import'
  } finally {
    importing.value = false
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
  gap: 8px;
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
