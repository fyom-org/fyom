<template>
  <div class="job-status">
    <p class="job-id">Job: {{ jobId }}</p>
    <p class="status-line">
      Status: <span :class="['status', jobStatus.status]">{{ jobStatus.status }}</span>
      <span v-if="jobStatus.total_items > 0">
        &nbsp;|&nbsp; Items: {{ jobStatus.done_items }}/{{ jobStatus.total_items }}
      </span>
    </p>
    <p v-if="jobStatus.error_msg" class="error">{{ jobStatus.error_msg }}</p>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { getJobStatus, type JobStatus as JobStatusType } from '@/api/library'

const props = defineProps<{ jobId: string }>()

const jobStatus = ref<JobStatusType>({
  id: props.jobId,
  source_path: '',
  status: 'pending',
  total_items: 0,
  done_items: 0,
  created_at: '',
  updated_at: '',
})

let timer: ReturnType<typeof setInterval> | null = null

onMounted(async () => {
  await fetchStatus()
  timer = setInterval(fetchStatus, 2000)
})

onUnmounted(() => {
  if (timer) clearInterval(timer)
})

async function fetchStatus() {
  try {
    const res = await getJobStatus(props.jobId)
    jobStatus.value = res.data
    if (res.data.status === 'done' || res.data.status === 'error') {
      if (timer) {
        clearInterval(timer)
        timer = null
      }
    }
  } catch (err) {
    console.error('[fyom] job status poll failed:', err)
  }
}
</script>

<style scoped>
.job-status {
  margin-top: 16px;
  padding: 12px 16px;
  background: #27272a;
  border-radius: 6px;
  font-size: 13px;
}

.job-id {
  color: #71717a;
  font-family: monospace;
  margin: 0 0 4px;
}

.status-line {
  margin: 0;
}

.status {
  font-weight: 600;
  text-transform: capitalize;
}

.status.pending { color: #fbbf24; }
.status.running { color: #60a5fa; }
.status.done { color: #34d399; }
.status.error { color: #f87171; }

.error {
  color: #f87171;
  margin: 8px 0 0;
}
</style>
