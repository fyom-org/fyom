<template>
  <div class="admin-page">
    <div class="page-header">
      <div>
        <h1>Libraries</h1>
        <p class="page-subtitle">
          Manage library sources, refresh scans, schedules, and missing media checks.
        </p>
      </div>

      <button type="button" class="add-btn" @click="toggleForm">
        {{ showForm ? 'Cancel' : '+ New Library' }}
      </button>
    </div>

    <div v-if="error" class="error-banner" role="alert">
      <span>{{ error }}</span>
      <button type="button" class="error-action" @click="clearError">Dismiss</button>
    </div>

    <div v-if="showForm" class="form-card">
      <div class="form-grid">
        <div class="field">
          <label for="library-name">Name</label>
          <input
            id="library-name"
            v-model.trim="form.name"
            placeholder="e.g. Kids Movies"
            :disabled="saving"
          />
        </div>

        <div class="field">
          <label for="library-type">Type</label>
          <select id="library-type" v-model="form.type" :disabled="saving">
            <option value="mixed">Mixed</option>
            <option value="movie">Movies Only</option>
            <option value="show">TV Shows Only</option>
          </select>
        </div>

        <div class="field">
          <label for="library-provider">Storage Provider</label>
          <select id="library-provider" v-model="form.provider_id" :disabled="saving">
            <option value="local">Local Disk</option>
            <option v-for="provider in providers" :key="provider.id" :value="provider.id">
              {{ provider.display_name }} ({{ provider.type }})
            </option>
          </select>
        </div>

        <div class="field">
          <label for="metadata-source">Metadata Source</label>
          <select id="metadata-source" v-model="form.metadata_source" :disabled="saving">
            <option value="nfo">NFO Files (Kodi/tinyMediaManager)</option>
            <option value="filename">Filename Only</option>
          </select>
        </div>

        <div class="field full-width">
          <label for="source-path">Source Path</label>
          <input
            id="source-path"
            v-model.trim="form.source_path"
            placeholder="/path/to/media or S3 prefix"
            :disabled="saving"
          />
        </div>
      </div>

      <p v-if="formError" class="error-text">
        {{ formError }}
      </p>

      <div class="form-actions">
        <button
          type="button"
          class="submit-btn"
          :disabled="saving || !canCreateLibrary"
          @click="createLibrary"
        >
          {{ saving ? 'Creating...' : 'Create Library' }}
        </button>
      </div>
    </div>

    <div v-if="loading" class="loading-text">Loading libraries...</div>

    <div v-else-if="libraries.length > 0" class="library-list">
      <div v-for="lib in libraries" :key="lib.id" class="library-card">
        <div class="library-header">
          <div class="library-info">
            <h3 class="library-name">
              {{ lib.name }}
            </h3>

            <span class="library-meta">
              {{ typeLabel(lib.type) }} &middot; {{ lib.provider_id }} &middot;
              {{ lib.metadata_source }}
            </span>
          </div>
        </div>

        <div class="library-actions">
          <button
            type="button"
            class="action-btn refresh"
            :disabled="isLibraryBusy(lib.id)"
            @click="refreshLibrary(lib)"
          >
            {{ refreshing === lib.id ? 'Starting...' : 'Refresh' }}
          </button>

          <button
            type="button"
            class="action-btn check"
            :disabled="isLibraryBusy(lib.id)"
            @click="checkMissing(lib)"
          >
            {{ checking === lib.id ? 'Checking...' : 'Check Missing' }}
          </button>

          <select
            class="schedule-select"
            :value="getSchedule(lib.id)"
            :disabled="savingSchedule === lib.id"
            @change="setSchedule(lib.id, ($event.target as HTMLSelectElement).value)"
          >
            <option value="0">Manual</option>
            <option value="3600">Every hour</option>
            <option value="21600">Every 6 hours</option>
            <option value="86400">Daily</option>
            <option value="604800">Weekly</option>
          </select>

          <button
            type="button"
            class="action-btn delete"
            :disabled="isLibraryBusy(lib.id)"
            @click="deleteLibrary(lib)"
          >
            Delete
          </button>
        </div>

        <div v-if="lib.item_count > 0" class="library-stats">
          <span class="stat">{{ lib.item_count }} items</span>
          <span v-if="lib.movie_count" class="stat">{{ lib.movie_count }} movies</span>
          <span v-if="lib.show_count" class="stat">{{ lib.show_count }} shows</span>
          <span v-if="lib.episode_count" class="stat">{{ lib.episode_count }} episodes</span>
          <span v-if="lib.missing_count > 0" class="stat warn">
            {{ lib.missing_count }} missing
          </span>
        </div>

        <div v-else class="library-stats">
          <span class="stat empty">No items yet</span>
        </div>

        <div
          v-if="activeJob && activeJob.libraryId === lib.id"
          class="job-panel"
          :class="[activeJob.status, { unavailable: activeJob.statusUnavailable }]"
        >
          <div class="job-row">
            <div>
              <p class="job-title">
                {{ jobTitle }}
              </p>
              <p v-if="activeJob.id" class="job-meta">Job ID: {{ activeJob.id }}</p>
            </div>

            <span class="job-badge">
              {{ activeJob.statusUnavailable ? 'status unavailable' : activeJob.status }}
            </span>
          </div>

          <div class="job-progress">
            <div class="job-progress-bar" :style="{ width: `${activeJobProgress}%` }"></div>
          </div>

          <p v-if="activeJob.message" class="job-message">
            {{ activeJob.message }}
          </p>

          <p v-if="activeJob.error" class="job-error">
            {{ activeJob.error }}
          </p>

          <div class="job-actions">
            <button type="button" class="job-action-btn" @click="reloadLibraries">
              Reload libraries
            </button>

            <button
              v-if="canCheckActiveJob"
              type="button"
              class="job-action-btn"
              @click="pollActiveJobNow"
            >
              Check now
            </button>

            <button type="button" class="job-action-btn" @click="clearActiveJob">Dismiss</button>
          </div>
        </div>
      </div>
    </div>

    <p v-else class="empty-text">No libraries configured.</p>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue';
import { authRequest } from '@/api/request';
import type { ApiEnvelope } from '@/api/types';
import {
  getApiErrorMessage,
  getJobStatusSilent,
  isFailedJobStatus,
  isTerminalJobStatus,
  tryStartAdminLibraryRefresh,
  type JobStatus,
  type JobStatusValue,
} from '@/api/library';

interface Library {
  id: string;
  name: string;
  type: string;
  provider_id: string;
  metadata_source: string;
  item_count: number;
  movie_count: number;
  show_count: number;
  episode_count: number;
  missing_count: number;
}

interface Provider {
  id: string;
  display_name: string;
  type: string;
}

interface SettingsMap {
  [key: string]: string;
}

interface LibraryForm {
  name: string;
  type: string;
  provider_id: string;
  source_path: string;
  metadata_source: string;
}

interface ActiveJob {
  id: string;
  libraryId: string;
  status: JobStatusValue;
  progress: number;
  message: string;
  error: string;
  statusUnavailable: boolean;
  conflict: boolean;
}

const JOB_POLL_INTERVAL_MS = 2500;

const libraries = ref<Library[]>([]);
const providers = ref<Provider[]>([]);
const schedules = ref<Record<string, string>>({});

const loading = ref(true);
const saving = ref(false);
const error = ref('');
const formError = ref('');

const showForm = ref(false);

const refreshing = ref('');
const checking = ref('');
const savingSchedule = ref('');

const activeJob = ref<ActiveJob | null>(null);

let jobPollTimer: number | undefined;
let jobPollGeneration = 0;

const form = reactive<LibraryForm>({
  name: '',
  type: 'mixed',
  provider_id: 'local',
  source_path: '',
  metadata_source: 'nfo',
});

const canCreateLibrary = computed(() => {
  return Boolean(form.name.trim() && form.source_path.trim() && form.provider_id.trim());
});

const activeJobProgress = computed(() => {
  if (!activeJob.value) return 0;

  const progress = Number(activeJob.value.progress);

  if (!Number.isFinite(progress)) return 0;

  return Math.min(100, Math.max(0, progress));
});

const isTerminalJob = computed(() => {
  if (!activeJob.value) return false;

  return isTerminalJobStatus(activeJob.value.status);
});

const canCheckActiveJob = computed(() => {
  if (!activeJob.value) return false;
  if (activeJob.value.conflict) return false;
  if (activeJob.value.statusUnavailable) return false;

  return !isTerminalJobStatus(activeJob.value.status);
});

const jobTitle = computed(() => {
  if (!activeJob.value) return '';

  if (activeJob.value.conflict) {
    return 'Refresh already in progress';
  }

  if (activeJob.value.statusUnavailable) {
    return 'Refresh started';
  }

  switch (String(activeJob.value.status).toLowerCase()) {
    case 'queued':
    case 'pending':
      return 'Refresh is queued';
    case 'running':
    case 'processing':
      return 'Refresh is running';
    case 'done':
    case 'completed':
    case 'success':
      return 'Refresh completed';
    case 'failed':
    case 'error':
      return 'Refresh failed';
    case 'cancelled':
      return 'Refresh cancelled';
    default:
      return 'Refresh status unknown';
  }
});

onMounted(() => {
  void loadInitialData();
});

onBeforeUnmount(() => {
  stopJobPolling();
});

/* =========================
   Load initial data
   ========================= */

async function loadInitialData(): Promise<void> {
  loading.value = true;
  error.value = '';

  try {
    const [libRes, provRes, settingsRes] = await Promise.all([
      authRequest.get<ApiEnvelope<Library[]>>('/admin/libraries', {
        authFailureMode: 'forbidden',
      }),
      authRequest.get<ApiEnvelope<Provider[]>>('/admin/providers', {
        authFailureMode: 'forbidden',
      }),
      authRequest.get<ApiEnvelope<SettingsMap>>('/admin/settings', {
        authFailureMode: 'forbidden',
      }),
    ]);

    libraries.value = normalizeEnvelopeData(libRes.data, []);
    providers.value = normalizeEnvelopeData(provRes.data, []);

    const settings = normalizeEnvelopeData(settingsRes.data, {});
    schedules.value = extractLibrarySchedules(settings);
  } catch (err: unknown) {
    if (isUnauthorizedOrForbidden(err)) {
      error.value = 'You do not have permission to manage libraries.';
      return;
    }

    console.error('[fyom] load libraries failed:', err);
    error.value = getApiErrorMessage(err, 'Failed to load libraries.');
  } finally {
    loading.value = false;
  }
}

async function reloadLibraries(): Promise<void> {
  try {
    const res = await authRequest.get<ApiEnvelope<Library[]>>('/admin/libraries', {
      authFailureMode: 'forbidden',
    });

    libraries.value = normalizeEnvelopeData(res.data, []);
  } catch (err: unknown) {
    if (isUnauthorizedOrForbidden(err)) {
      error.value = 'Unable to reload libraries because the request was not authorized.';
      return;
    }

    console.error('[fyom] reload libraries failed:', err);
    error.value = getApiErrorMessage(err, 'Failed to reload libraries.');
  }
}

/* =========================
   Schedule
   ========================= */

function getSchedule(libId: string): string {
  return schedules.value[libId] || '0';
}

async function setSchedule(libId: string, interval: string): Promise<void> {
  savingSchedule.value = libId;
  error.value = '';

  try {
    await authRequest.put(
      '/admin/settings',
      {
        [`library_refresh_interval_${libId}`]: interval,
      },
      {
        authFailureMode: 'forbidden',
      }
    );

    schedules.value = {
      ...schedules.value,
      [libId]: interval,
    };
  } catch (err: unknown) {
    if (isUnauthorizedOrForbidden(err)) {
      error.value = 'Unable to save schedule because the request was not authorized.';
      return;
    }

    error.value = getApiErrorMessage(err, 'Failed to save schedule.');
  } finally {
    savingSchedule.value = '';
  }
}

/* =========================
   Create
   ========================= */

async function createLibrary(): Promise<void> {
  error.value = '';
  formError.value = '';

  if (!validateForm()) return;

  saving.value = true;

  try {
    await authRequest.post(
      '/admin/libraries',
      {
        name: form.name.trim(),
        type: form.type,
        provider_id: form.provider_id,
        source_path: form.source_path.trim(),
        metadata_source: form.metadata_source,
      },
      {
        authFailureMode: 'forbidden',
      }
    );

    resetForm();
    showForm.value = false;

    await reloadLibraries();
  } catch (err: unknown) {
    if (isUnauthorizedOrForbidden(err)) {
      error.value = 'Unable to create library because the request was not authorized.';
      return;
    }

    formError.value = getApiErrorMessage(err, 'Failed to create library.');
  } finally {
    saving.value = false;
  }
}

function validateForm(): boolean {
  if (!form.name.trim()) {
    formError.value = 'Library name is required.';
    return false;
  }

  if (!form.source_path.trim()) {
    formError.value = 'Source path is required.';
    return false;
  }

  if (!form.provider_id.trim()) {
    formError.value = 'Storage provider is required.';
    return false;
  }

  return true;
}

function resetForm(): void {
  form.name = '';
  form.type = 'mixed';
  form.provider_id = 'local';
  form.source_path = '';
  form.metadata_source = 'nfo';
}

/* =========================
   Delete
   ========================= */

async function deleteLibrary(lib: Library): Promise<void> {
  error.value = '';

  const confirmed =
    lib.item_count > 0
      ? confirm(`"${lib.name}" has ${lib.item_count} items. Delete everything?`)
      : confirm(`Delete "${lib.name}"?`);

  if (!confirmed) return;

  try {
    if (lib.item_count > 0) {
      await authRequest.delete(
        `/admin/libraries/${encodeURIComponent(lib.id)}/items?mode=cascade`,
        {
          authFailureMode: 'forbidden',
        }
      );
    } else {
      await authRequest.delete(`/admin/libraries/${encodeURIComponent(lib.id)}`, {
        authFailureMode: 'forbidden',
      });
    }

    if (activeJob.value?.libraryId === lib.id) {
      clearActiveJob();
    }

    await reloadLibraries();
  } catch (err: unknown) {
    if (isUnauthorizedOrForbidden(err)) {
      error.value = 'Unable to delete library because the request was not authorized.';
      return;
    }

    error.value = getApiErrorMessage(err, 'Failed to delete library.');
  }
}

/* =========================
   Actions
   ========================= */

async function refreshLibrary(lib: Library): Promise<void> {
  refreshing.value = lib.id;
  error.value = '';
  clearActiveJob();

  try {
    const result = await tryStartAdminLibraryRefresh(lib.id);

    if (!result.ok) {
      activeJob.value = {
        id: '',
        libraryId: lib.id,
        status: 'unknown',
        progress: 0,
        message:
          result.reason === 'already_in_progress'
            ? 'A refresh is already running for this library. New media may appear when the current scan finishes.'
            : result.message,
        error: '',
        statusUnavailable: true,
        conflict: true,
      };

      await reloadLibraries();
      return;
    }

    if (!result.job.job_id) {
      activeJob.value = {
        id: '',
        libraryId: lib.id,
        status: 'unknown',
        progress: 0,
        message:
          'Refresh was started, but the server did not return a job id. New media may still appear after the scan completes.',
        error: '',
        statusUnavailable: true,
        conflict: false,
      };

      await reloadLibraries();
      return;
    }

    activeJob.value = {
      id: result.job.job_id,
      libraryId: lib.id,
      status: normalizeJobStatus(result.job.status || 'queued'),
      progress: 0,
      message: 'Refresh job has been created.',
      error: '',
      statusUnavailable: false,
      conflict: false,
    };

    startJobPolling();
  } catch (err: unknown) {
    if (isUnauthorizedOrForbidden(err)) {
      error.value = 'Unable to refresh library because the request was not authorized.';
      return;
    }

    error.value = getApiErrorMessage(err, 'Refresh failed.');
  } finally {
    refreshing.value = '';
  }
}

async function checkMissing(lib: Library): Promise<void> {
  checking.value = lib.id;
  error.value = '';

  try {
    const res = await authRequest.post<ApiEnvelope<{ missing: number }>>(
      `/admin/libraries/${encodeURIComponent(lib.id)}/check-missing`,
      undefined,
      {
        authFailureMode: 'forbidden',
      }
    );

    const result = normalizeEnvelopeData(res.data, { missing: 0 });

    if (result.missing > 0) {
      alert(
        `Found ${result.missing} missing item${
          result.missing !== 1 ? 's' : ''
        }.\nCheck the Missing page.`
      );
    } else {
      alert('All items available.');
    }

    await reloadLibraries();
  } catch (err: unknown) {
    if (isUnauthorizedOrForbidden(err)) {
      error.value = 'Unable to check missing media because the request was not authorized.';
      return;
    }

    error.value = getApiErrorMessage(err, 'Check failed.');
  } finally {
    checking.value = '';
  }
}

/* =========================
   Job polling
   ========================= */

function startJobPolling(): void {
  stopJobPolling();

  const generation = ++jobPollGeneration;

  void pollJobStatus(generation);

  jobPollTimer = window.setInterval(() => {
    void pollJobStatus(generation);
  }, JOB_POLL_INTERVAL_MS);
}

function stopJobPolling(): void {
  jobPollGeneration += 1;

  if (jobPollTimer) {
    window.clearInterval(jobPollTimer);
    jobPollTimer = undefined;
  }
}

async function pollActiveJobNow(): Promise<void> {
  if (!activeJob.value || !activeJob.value.id) return;

  const generation = jobPollGeneration;

  await pollJobStatus(generation);
}

async function pollJobStatus(generation: number): Promise<void> {
  const job = activeJob.value;

  if (!job || !job.id || generation !== jobPollGeneration) return;

  try {
    const data = await getJobStatusSilent(job.id);

    if (generation !== jobPollGeneration || !activeJob.value) return;

    if (!data) {
      markJobStatusUnavailable();
      stopJobPolling();
      await reloadLibraries();
      return;
    }

    applyJobStatus(data);

    if (isTerminalJobStatus(activeJob.value.status)) {
      stopJobPolling();
      await reloadLibraries();
    }
  } catch (err: unknown) {
    if (generation !== jobPollGeneration || !activeJob.value) return;

    activeJob.value = {
      ...activeJob.value,
      status: 'unknown',
      statusUnavailable: true,
      error: getApiErrorMessage(err, 'Failed to read job status. The scan may still be running.'),
    };

    stopJobPolling();
  }
}

function applyJobStatus(data: JobStatus): void {
  if (!activeJob.value) return;

  const status = normalizeJobStatus(data.status);
  const nextProgress = normalizeJobProgress(
    data.progress,
    data.done_items,
    data.total_items,
    status
  );
  const failed = isFailedJobStatus(status);

  activeJob.value = {
    ...activeJob.value,
    status,
    progress: nextProgress,
    message: data.message || activeJob.value.message || 'Refresh job status updated.',
    error: data.error || data.error_msg || (failed ? 'Refresh job ended with an error.' : ''),
    statusUnavailable: false,
  };
}

function markJobStatusUnavailable(): void {
  if (!activeJob.value) return;

  activeJob.value = {
    ...activeJob.value,
    status: 'unknown',
    progress: activeJob.value.progress || 0,
    message:
      'Refresh started, but this client cannot read job status. New media may still appear after the scan completes.',
    error: '',
    statusUnavailable: true,
  };
}

function clearActiveJob(): void {
  stopJobPolling();
  activeJob.value = null;
}

function normalizeJobStatus(status: unknown): JobStatusValue {
  if (typeof status !== 'string') return 'unknown';

  const normalized = status.toLowerCase();

  switch (normalized) {
    case 'queued':
    case 'pending':
    case 'running':
    case 'processing':
    case 'done':
    case 'completed':
    case 'success':
    case 'failed':
    case 'error':
    case 'cancelled':
      return normalized;
    default:
      return 'unknown';
  }
}

function normalizeJobProgress(
  progress: unknown,
  doneItems: unknown,
  totalItems: unknown,
  status: JobStatusValue
): number {
  if (status === 'done' || status === 'completed' || status === 'success') return 100;
  if (status === 'failed' || status === 'error' || status === 'cancelled') return 100;

  const directProgress = Number(progress);

  if (Number.isFinite(directProgress)) {
    return Math.min(100, Math.max(0, directProgress));
  }

  const done = Number(doneItems);
  const total = Number(totalItems);

  if (Number.isFinite(done) && Number.isFinite(total) && total > 0) {
    return Math.min(100, Math.max(0, (done / total) * 100));
  }

  return activeJob.value?.progress || 0;
}

/* =========================
   Helpers
   ========================= */

function toggleForm(): void {
  showForm.value = !showForm.value;
  formError.value = '';
}

function clearError(): void {
  error.value = '';
}

function isLibraryBusy(libId: string): boolean {
  return (
    refreshing.value === libId ||
    checking.value === libId ||
    savingSchedule.value === libId ||
    Boolean(
      activeJob.value?.libraryId === libId &&
      !activeJob.value.statusUnavailable &&
      !activeJob.value.conflict &&
      !isTerminalJobStatus(activeJob.value.status)
    )
  );
}

function typeLabel(type: string): string {
  switch (type) {
    case 'movie':
      return 'Movies';
    case 'show':
      return 'TV Shows';
    default:
      return 'Mixed';
  }
}

function extractLibrarySchedules(settings: SettingsMap): Record<string, string> {
  const nextSchedules: Record<string, string> = {};

  for (const [key, value] of Object.entries(settings)) {
    if (!key.startsWith('library_refresh_interval_')) continue;

    const libId = key.replace('library_refresh_interval_', '');

    nextSchedules[libId] = String(value);
  }

  return nextSchedules;
}

function normalizeEnvelopeData<T>(envelope: ApiEnvelope<T> | T | undefined, fallback: T): T {
  if (!envelope) return fallback;

  if (isRecord(envelope) && 'data' in envelope) {
    return (envelope.data as T) ?? fallback;
  }

  return envelope as T;
}

function isUnauthorizedOrForbidden(err: unknown): boolean {
  if (!isRecord(err)) return false;

  const response = err.response;

  if (!isRecord(response)) return false;

  return response.status === 401 || response.status === 403;
}

function isRecord(value: unknown): value is Record<string, any> {
  return typeof value === 'object' && value !== null;
}
</script>

<style scoped>
.admin-page {
  width: 100%;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 16px;
  margin-bottom: 20px;
}

h1 {
  margin: 0;
  color: #e0e0e0;
  font-size: 24px;
  font-weight: 800;
}

.page-subtitle {
  margin: 6px 0 0;
  color: #666688;
  font-size: 13px;
  line-height: 1.45;
}

.add-btn,
.submit-btn {
  padding: 9px 16px;
  color: #fff;
  background: #6c63ff;
  border: none;
  border-radius: 8px;
  cursor: pointer;
  font-size: 13px;
  font-weight: 700;
  transition:
    background-color 0.15s ease,
    opacity 0.15s ease;
}

.add-btn:hover,
.submit-btn:hover:not(:disabled) {
  background: #5a52e0;
}

.add-btn:disabled,
.submit-btn:disabled {
  cursor: not-allowed;
  opacity: 0.55;
}

.error-banner {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  margin-bottom: 16px;
  padding: 12px 14px;
  color: #ffb3b3;
  background: #2a1a1a;
  border: 1px solid #5a2a2a;
  border-radius: 10px;
  font-size: 13px;
}

.error-action {
  flex: 0 0 auto;
  padding: 5px 10px;
  color: #fff;
  background: #5a2a2a;
  border: 1px solid #7a3a3a;
  border-radius: 6px;
  cursor: pointer;
  font-size: 12px;
}

.error-action:hover {
  background: #6a3030;
}

.form-card {
  margin-bottom: 20px;
  padding: 20px;
  background: #12121e;
  border: 1px solid #1a1a2e;
  border-radius: 12px;
}

.form-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px;
}

.field.full-width {
  grid-column: 1 / -1;
}

.field label {
  display: block;
  margin-bottom: 6px;
  color: #777799;
  font-size: 12px;
  font-weight: 700;
}

.field input,
.field select {
  width: 100%;
  min-height: 38px;
  box-sizing: border-box;
  padding: 8px 10px;
  color: #ccccee;
  background: #0a0a14;
  border: 1px solid #1a1a2e;
  border-radius: 6px;
  outline: none;
  font-size: 13px;
}

.field input:focus,
.field select:focus {
  border-color: #6c63ff;
}

.field input:disabled,
.field select:disabled {
  cursor: not-allowed;
  opacity: 0.6;
}

.form-actions {
  margin-top: 16px;
}

.error-text {
  margin: 10px 0 0;
  color: #ff6b6b;
  font-size: 12px;
}

.loading-text,
.empty-text {
  color: #555577;
  font-size: 14px;
}

.library-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.library-card {
  padding: 18px 20px;
  background: #12121e;
  border: 1px solid #1a1a2e;
  border-radius: 12px;
}

.library-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.library-info {
  min-width: 0;
}

.library-name {
  margin: 0;
  color: #e0e0e0;
  font-size: 17px;
  font-weight: 800;
}

.library-meta {
  display: inline-block;
  margin-top: 5px;
  color: #555577;
  font-size: 12px;
}

.library-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  margin-top: 14px;
}

.action-btn {
  padding: 7px 12px;
  color: #8888aa;
  background: transparent;
  border: 1px solid #1a1a2e;
  border-radius: 6px;
  cursor: pointer;
  font-size: 12px;
  transition:
    border-color 0.15s ease,
    color 0.15s ease,
    background-color 0.15s ease,
    opacity 0.15s ease;
}

.action-btn:hover:not(:disabled) {
  color: #ccccee;
  border-color: #2a2a3e;
}

.action-btn:disabled {
  cursor: not-allowed;
  opacity: 0.45;
}

.action-btn.refresh:hover:not(:disabled) {
  color: #2196f3;
  border-color: rgb(33 150 243 / 50%);
}

.action-btn.check:hover:not(:disabled) {
  color: #ff9800;
  border-color: rgb(255 152 0 / 50%);
}

.action-btn.delete {
  color: #ff6b6b;
  border-color: rgb(255 107 107 / 30%);
}

.action-btn.delete:hover:not(:disabled) {
  background: #1a0f0f;
}

.schedule-select {
  min-height: 32px;
  padding: 6px 8px;
  color: #aaaacc;
  background: #0a0a14;
  border: 1px solid #1a1a2e;
  border-radius: 6px;
  cursor: pointer;
  font-size: 12px;
}

.schedule-select:focus {
  border-color: #6c63ff;
  outline: none;
}

.schedule-select:disabled {
  cursor: not-allowed;
  opacity: 0.55;
}

.schedule-select option {
  color: #ccccee;
  background: #0a0a14;
}

.library-stats {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  margin-top: 12px;
}

.stat {
  color: #8888aa;
  font-size: 12px;
}

.stat.empty {
  color: #555577;
  font-style: italic;
}

.stat.warn {
  color: #ff9800;
}

.job-panel {
  margin-top: 16px;
  padding: 14px;
  background: #0f0f1a;
  border: 1px solid #23233a;
  border-radius: 10px;
}

.job-panel.completed,
.job-panel.done,
.job-panel.success {
  border-color: rgb(76 175 80 / 34%);
}

.job-panel.failed,
.job-panel.error {
  border-color: rgb(255 107 107 / 34%);
}

.job-panel.unavailable {
  border-color: rgb(251 191 36 / 30%);
}

.job-row {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 12px;
}

.job-title {
  margin: 0;
  color: #e0e0e0;
  font-size: 13px;
  font-weight: 800;
}

.job-meta {
  margin: 5px 0 0;
  color: #555577;
  font-size: 11px;
  word-break: break-all;
}

.job-badge {
  flex: 0 0 auto;
  padding: 4px 8px;
  color: #ccccee;
  background: #1a1a2e;
  border: 1px solid #2a2a3e;
  border-radius: 999px;
  font-size: 11px;
  font-weight: 700;
  text-transform: uppercase;
}

.job-progress {
  height: 6px;
  margin-top: 12px;
  overflow: hidden;
  background: #1a1a2e;
  border-radius: 999px;
}

.job-progress-bar {
  height: 100%;
  background: linear-gradient(90deg, #6c63ff, #2196f3);
  border-radius: inherit;
  transition: width 0.2s ease;
}

.job-message {
  margin: 10px 0 0;
  color: #8888aa;
  font-size: 12px;
  line-height: 1.45;
}

.job-error {
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
}

.job-action-btn:hover {
  color: #fff;
  border-color: #3a3a5e;
}

@media (max-width: 720px) {
  .page-header {
    flex-direction: column;
  }

  .add-btn {
    width: 100%;
  }

  .form-grid {
    grid-template-columns: 1fr;
  }

  .library-actions {
    align-items: stretch;
    flex-direction: column;
  }

  .action-btn,
  .schedule-select {
    width: 100%;
  }

  .error-banner,
  .job-row {
    align-items: flex-start;
    flex-direction: column;
  }

  .error-action,
  .job-action-btn {
    width: 100%;
  }
}
</style>
