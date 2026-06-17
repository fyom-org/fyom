<template>
  <div class="job-status" :class="statusClass">
    <div class="job-header">
      <div>
        <p class="job-label">Refresh job</p>
        <p class="job-id">
          {{ jobId }}
        </p>
      </div>

      <span class="status-badge">
        {{ displayStatus }}
      </span>
    </div>

    <div v-if="hasProgress" class="progress-track" aria-hidden="true">
      <div class="progress-fill" :style="{ width: `${progressPercent}%` }"></div>
    </div>

    <p class="status-line">
      {{ statusMessage }}
    </p>

    <p v-if="itemProgressLabel" class="item-progress">Items: {{ itemProgressLabel }}</p>

    <p v-if="jobError" class="error" role="alert">
      {{ jobError }}
    </p>

    <div class="job-actions">
      <button
        v-if="statusUnavailable"
        type="button"
        class="job-action-btn"
        @click="emitUnavailableReload"
      >
        {{ $t('admin.libraries.reloadLibraries') }}
      </button>

      <button
        v-if="canCheckNow"
        type="button"
        class="job-action-btn"
        :disabled="checking"
        @click="fetchStatusNow"
      >
        {{ checking ? $t('admin.libraries.checking') : $t('admin.libraries.checkNow') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import {
  getApiErrorMessage,
  getJobStatusSilent,
  isFailedJobStatus,
  isTerminalJobStatus,
  type JobStatus as JobStatusType,
} from '@/api/library';

const props = withDefaults(
  defineProps<{
    jobId: string;
    pollIntervalMs?: number;
    autoStart?: boolean;
  }>(),
  {
    pollIntervalMs: 2000,
    autoStart: true,
  }
);

const emit = defineEmits<{
  (event: 'updated', job: JobStatusType): void;
  (event: 'completed', job: JobStatusType): void;
  (event: 'failed', job: JobStatusType): void;
  (event: 'unavailable', jobId: string): void;
  (event: 'reload-requested'): void;
}>();

const { t } = useI18n();

const jobStatus = ref<JobStatusType>(createInitialJob(props.jobId));
const jobError = ref('');
const statusUnavailable = ref(false);
const checking = ref(false);

let timer: ReturnType<typeof setInterval> | null = null;
let generation = 0;

const displayStatus = computed(() => {
  return normalizeStatusLabel(jobStatus.value.status);
});

const statusClass = computed(() => {
  if (statusUnavailable.value) return 'unavailable';

  const status = jobStatus.value.status.toLowerCase();

  if (['done', 'completed', 'success'].includes(status)) return 'success';
  if (['failed', 'error', 'cancelled'].includes(status)) return 'failed';
  if (['running', 'processing'].includes(status)) return 'running';

  return 'pending';
});

const statusMessage = computed(() => {
  if (statusUnavailable.value) {
    return t('admin.libraries.refreshNoJobStatus');
  }

  if (jobStatus.value.message) {
    return jobStatus.value.message;
  }

  const status = jobStatus.value.status.toLowerCase();

  if (status === 'queued' || status === 'pending') {
    return t('admin.libraries.refreshQueued');
  }

  if (status === 'running' || status === 'processing') {
    return t('admin.libraries.refreshRunning');
  }

  if (status === 'done' || status === 'completed' || status === 'success') {
    return t('admin.libraries.refreshCompleted');
  }

  if (status === 'failed' || status === 'error') {
    return t('admin.libraries.refreshFailed');
  }

  if (status === 'cancelled') {
    return t('admin.libraries.refreshCancelled');
  }

  return t('admin.libraries.refreshUnknown');
});

const hasProgress = computed(() => {
  return progressPercent.value > 0 || Number(jobStatus.value.total_items) > 0;
});

const progressPercent = computed(() => {
  const directProgress = Number(jobStatus.value.progress);

  if (Number.isFinite(directProgress) && directProgress >= 0) {
    return clampProgress(directProgress);
  }

  const done = Number(jobStatus.value.done_items);
  const total = Number(jobStatus.value.total_items);

  if (!Number.isFinite(done) || !Number.isFinite(total) || total <= 0) {
    return 0;
  }

  return clampProgress((done / total) * 100);
});

const itemProgressLabel = computed(() => {
  const done = Number(jobStatus.value.done_items);
  const total = Number(jobStatus.value.total_items);

  if (!Number.isFinite(done) || !Number.isFinite(total) || total <= 0) {
    return '';
  }

  return `${done}/${total}`;
});

const canCheckNow = computed(() => {
  return !checking.value && !isTerminalJobStatus(jobStatus.value.status);
});

watch(
  () => props.jobId,
  (nextJobId) => {
    resetJob(nextJobId);

    if (props.autoStart && nextJobId) {
      startPolling();
    }
  }
);

onMounted(() => {
  if (props.autoStart && props.jobId) {
    startPolling();
  }
});

onUnmounted(() => {
  stopPolling();
});

function createInitialJob(jobId: string): JobStatusType {
  return {
    id: jobId,
    status: 'pending',
    total_items: 0,
    done_items: 0,
    message: t('admin.libraries.refreshJobCreated'),
  };
}

function resetJob(jobId: string): void {
  generation += 1;
  stopPolling();

  jobStatus.value = createInitialJob(jobId);
  jobError.value = '';
  statusUnavailable.value = false;
  checking.value = false;
}

function startPolling(): void {
  stopPolling();

  const currentGeneration = ++generation;

  void fetchStatus(currentGeneration);

  timer = setInterval(() => {
    void fetchStatus(currentGeneration);
  }, props.pollIntervalMs);
}

function stopPolling(): void {
  if (timer) {
    clearInterval(timer);
    timer = null;
  }
}

async function fetchStatusNow(): Promise<void> {
  const currentGeneration = ++generation;

  await fetchStatus(currentGeneration);
}

async function fetchStatus(currentGeneration: number): Promise<void> {
  if (!props.jobId || currentGeneration !== generation) return;

  checking.value = true;
  jobError.value = '';

  try {
    const nextStatus = await getJobStatusSilent(props.jobId);

    if (currentGeneration !== generation) return;

    if (!nextStatus) {
      statusUnavailable.value = true;
      jobStatus.value = {
        ...jobStatus.value,
        status: 'unknown',
        message: t('admin.libraries.refreshNoJobStatus'),
      };

      stopPolling();
      emit('unavailable', props.jobId);
      return;
    }

    statusUnavailable.value = false;
    jobStatus.value = nextStatus;
    emit('updated', nextStatus);

    if (isTerminalJobStatus(nextStatus.status)) {
      stopPolling();

      if (isFailedJobStatus(nextStatus.status)) {
        jobError.value =
          nextStatus.error_msg || nextStatus.error || t('admin.libraries.jobEndedError');
        emit('failed', nextStatus);
      } else {
        emit('completed', nextStatus);
      }
    }
  } catch (error: unknown) {
    if (currentGeneration !== generation) return;

    statusUnavailable.value = true;
    jobStatus.value = {
      ...jobStatus.value,
      status: 'unknown',
    };

    jobError.value = getApiErrorMessage(
      error,
      t('admin.libraries.jobStatusReadFailed')
    );

    stopPolling();
    emit('unavailable', props.jobId);
  } finally {
    if (currentGeneration === generation) {
      checking.value = false;
    }
  }
}

function emitUnavailableReload(): void {
  emit('reload-requested');
}

function normalizeStatusLabel(status: string): string {
  if (!status) return 'unknown';

  return status.replace(/_/g, ' ');
}

function clampProgress(value: number): number {
  if (!Number.isFinite(value)) return 0;

  return Math.min(100, Math.max(0, value));
}
</script>

<style scoped>
.job-status {
  margin-top: 16px;
  padding: 14px 16px;
  background: #11111d;
  border: 1px solid #23233a;
  border-radius: 10px;
  font-size: 13px;
}

.job-status.pending {
  border-color: rgb(251 191 36 / 24%);
}

.job-status.running {
  border-color: rgb(96 165 250 / 28%);
}

.job-status.success {
  border-color: rgb(52 211 153 / 28%);
}

.job-status.failed {
  border-color: rgb(248 113 113 / 30%);
}

.job-status.unavailable {
  border-color: rgb(251 191 36 / 28%);
}

.job-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.job-label {
  margin: 0 0 4px;
  color: #8888aa;
  font-size: 12px;
  font-weight: 700;
}

.job-id {
  margin: 0;
  color: #666688;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', monospace;
  font-size: 11px;
  word-break: break-all;
}

.status-badge {
  flex: 0 0 auto;
  padding: 4px 8px;
  color: #ccccee;
  background: #1a1a2e;
  border: 1px solid #2a2a3e;
  border-radius: 999px;
  font-size: 11px;
  font-weight: 800;
  text-transform: capitalize;
}

.progress-track {
  height: 6px;
  margin-top: 12px;
  overflow: hidden;
  background: #1a1a2e;
  border-radius: 999px;
}

.progress-fill {
  height: 100%;
  background: linear-gradient(90deg, #6c63ff, #2196f3);
  border-radius: inherit;
  transition: width 0.2s ease;
}

.status-line {
  margin: 10px 0 0;
  color: #aaaacc;
  font-size: 12px;
  line-height: 1.5;
}

.item-progress {
  margin: 6px 0 0;
  color: #777799;
  font-size: 12px;
}

.error {
  margin: 10px 0 0;
  color: #ff8f8f;
  font-size: 12px;
  line-height: 1.45;
}

.job-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 12px;
}

.job-action-btn {
  padding: 6px 10px;
  color: #aaaacc;
  background: #1a1a2e;
  border: 1px solid #2a2a3e;
  border-radius: 6px;
  cursor: pointer;
  font-size: 12px;
  font-weight: 700;
}

.job-action-btn:hover:not(:disabled) {
  color: #fff;
  border-color: #3a3a5e;
}

.job-action-btn:disabled {
  cursor: not-allowed;
  opacity: 0.55;
}

@media (max-width: 560px) {
  .job-header {
    flex-direction: column;
  }

  .status-badge,
  .job-action-btn {
    width: 100%;
    box-sizing: border-box;
    text-align: center;
  }
}

@media (prefers-reduced-motion: reduce) {
  .progress-fill {
    transition: none;
  }
}
</style>
