<template>
  <main class="system-view">
    <header class="page-header">
      <div>
        <h1>System Health</h1>
        <p class="page-subtitle">
          Review library totals, provider status, user counts, storage distribution, and recent
          import activity.
        </p>
      </div>

      <button type="button" class="refresh-btn" :disabled="loading" @click="loadSystemData">
        {{ loading ? 'Loading...' : 'Reload' }}
      </button>
    </header>

    <div v-if="loading && !stats" class="loading">Loading system health...</div>

    <div v-else-if="error" class="error-card" role="alert">
      <div>
        <strong>Unable to load system health</strong>
        <p>{{ error }}</p>
      </div>

      <button type="button" class="error-action" @click="loadSystemData">Retry</button>
    </div>

    <template v-else-if="stats">
      <section class="stats-grid" aria-label="System statistics">
        <article class="stat-card">
          <div class="stat-value">
            {{ stats.library.total_items }}
          </div>
          <div class="stat-label">Library Items</div>
          <div class="stat-detail">
            {{ stats.library.movies }} movies · {{ stats.library.shows }} shows ·
            {{ stats.library.episodes }} episodes
          </div>
        </article>

        <article class="stat-card">
          <div class="stat-value" :class="{ warn: stats.imports.error > 0 }">
            {{ stats.imports.done }}/{{ stats.imports.total }}
          </div>
          <div class="stat-label">Imports Completed</div>
          <div v-if="stats.imports.running > 0" class="stat-detail">
            {{ stats.imports.running }} running
          </div>
          <div v-if="stats.imports.error > 0" class="stat-detail error-text">
            {{ stats.imports.error }} failed
          </div>
          <div v-if="stats.imports.running === 0 && stats.imports.error === 0" class="stat-detail">
            No active import issues
          </div>
        </article>

        <article class="stat-card">
          <div class="stat-value">{{ stats.providers.enabled }}/{{ stats.providers.total }}</div>
          <div class="stat-label">Providers Active</div>
          <div class="stat-detail">
            {{ providerTypesLabel }}
          </div>
        </article>

        <article class="stat-card">
          <div class="stat-value">
            {{ stats.users.total }}
          </div>
          <div class="stat-label">Users</div>
          <div class="stat-detail">
            {{ stats.users.admins }} admin{{ stats.users.admins !== 1 ? 's' : '' }}
          </div>
        </article>
      </section>

      <section class="section">
        <div class="section-header">
          <div>
            <h2>Recent Imports</h2>
            <p class="section-subtitle">
              Latest import jobs reported by the admin job history endpoint.
            </p>
          </div>

          <span v-if="jobsTotal > jobs.length" class="count"> {{ jobsTotal }} total </span>
        </div>

        <div v-if="jobsError" class="inline-warning" role="status">
          {{ jobsError }}
        </div>

        <div v-if="jobs.length > 0" class="jobs-list">
          <article v-for="job in jobs" :key="job.id" class="job-row">
            <span class="job-status" :class="statusClass(job.status)">
              {{ statusLabel(job.status) }}
            </span>

            <span class="job-path" :title="job.source_path || 'Unknown source path'">
              {{ job.source_path || 'Unknown source path' }}
            </span>

            <span v-if="job.total_items > 0" class="job-progress">
              {{ job.done_items }}/{{ job.total_items }}
            </span>

            <span v-else class="job-progress muted"> — </span>

            <span class="job-date">
              {{ formatDate(job.created_at) }}
            </span>

            <span
              v-if="job.error_msg"
              class="job-error"
              :title="job.error_msg"
              aria-label="Import job has an error"
            >
              Warning
            </span>
          </article>
        </div>

        <p v-else-if="!jobsError" class="empty">No import jobs yet.</p>
      </section>

      <section class="section">
        <div class="section-header">
          <div>
            <h2>Storage Distribution</h2>
            <p class="section-subtitle">Media items grouped by storage provider.</p>
          </div>
        </div>

        <div v-if="storageRows.length > 0" class="storage-bars">
          <div v-for="row in storageRows" :key="row.provider" class="storage-row">
            <span class="storage-provider">
              {{ row.provider }}
            </span>

            <div class="storage-bar-track" aria-hidden="true">
              <div class="storage-bar-fill" :style="{ transform: `scaleX(${row.ratio})` }"></div>
            </div>

            <span class="storage-count">
              {{ row.count }} item{{ row.count === 1 ? '' : 's' }}
            </span>
          </div>
        </div>

        <p v-else class="empty">No storage distribution data available.</p>
      </section>
    </template>
  </main>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { authRequest } from '@/api/request';
import type { ApiEnvelope } from '@/api/types';

interface Stats {
  library: {
    total_items: number;
    movies: number;
    shows: number;
    episodes: number;
  };
  imports: {
    total: number;
    done: number;
    running: number;
    error: number;
  };
  providers: {
    total: number;
    enabled: number;
    types: string[];
  };
  users: {
    total: number;
    admins: number;
  };
  storage: Record<string, number>;
}

interface Job {
  id: string;
  status: string;
  source_path: string;
  total_items: number;
  done_items: number;
  created_at: string;
  error_msg?: string;
}

interface JobsResponse {
  items: Job[];
  total: number;
}

interface StorageRow {
  provider: string;
  count: number;
  ratio: number;
}

const stats = ref<Stats | null>(null);
const jobs = ref<Job[]>([]);
const jobsTotal = ref(0);
const loading = ref(true);
const error = ref('');
const jobsError = ref('');

const providerTypesLabel = computed(() => {
  const types = stats.value?.providers.types || [];

  if (types.length === 0) {
    return 'No provider types reported';
  }

  return types.join(', ');
});

const storageRows = computed<StorageRow[]>(() => {
  const currentStats = stats.value;

  if (!currentStats) return [];

  return Object.entries(currentStats.storage)
    .map(([provider, count]) => ({
      provider,
      count,
      ratio: safeRatio(count, currentStats.library.total_items),
    }))
    .sort((a, b) => b.count - a.count);
});

onMounted(() => {
  void loadSystemData();
});

async function loadSystemData(): Promise<void> {
  loading.value = true;
  error.value = '';
  jobsError.value = '';

  try {
    stats.value = await fetchStats();

    const recentJobs = await fetchRecentJobs();
    jobs.value = recentJobs.items;
    jobsTotal.value = recentJobs.total;
  } catch (unknownError) {
    if (isUnauthorizedOrForbidden(unknownError)) {
      error.value = 'You do not have permission to view system health.';
      return;
    }

    console.error('[fyom] load system health failed:', unknownError);
    error.value = getErrorMessage(unknownError, 'Failed to load system stats.');
  } finally {
    loading.value = false;
  }
}

async function fetchStats(): Promise<Stats> {
  const response = await authRequest.get<ApiEnvelope<Stats>>('/admin/stats', {
    authFailureMode: 'forbidden',
  });

  return normalizeStats(response.data);
}

async function fetchRecentJobs(): Promise<JobsResponse> {
  try {
    const response = await authRequest.get<ApiEnvelope<JobsResponse>>('/admin/import-jobs', {
      params: {
        limit: 5,
      },
      authFailureMode: 'forbidden',
    });

    return normalizeJobsResponse(response.data);
  } catch (unknownError) {
    if (isUnauthorizedOrForbidden(unknownError)) {
      jobsError.value = 'Recent import jobs are not available for this account.';
      return {
        items: [],
        total: 0,
      };
    }

    console.error('[fyom] load recent import jobs failed:', unknownError);
    jobsError.value = getErrorMessage(unknownError, 'Failed to load recent import jobs.');

    return {
      items: [],
      total: 0,
    };
  }
}

function normalizeStats(value: ApiEnvelope<Stats> | Stats | unknown): Stats {
  const data = unwrapUnknownEnvelope(value);
  const record = isRecord(data) ? data : {};

  const library = isRecord(record.library) ? record.library : {};
  const imports = isRecord(record.imports) ? record.imports : {};
  const providers = isRecord(record.providers) ? record.providers : {};
  const users = isRecord(record.users) ? record.users : {};

  return {
    library: {
      total_items: toNumber(library.total_items),
      movies: toNumber(library.movies),
      shows: toNumber(library.shows),
      episodes: toNumber(library.episodes),
    },
    imports: {
      total: toNumber(imports.total),
      done: toNumber(imports.done),
      running: toNumber(imports.running),
      error: toNumber(imports.error),
    },
    providers: {
      total: toNumber(providers.total),
      enabled: toNumber(providers.enabled),
      types: toStringArray(providers.types),
    },
    users: {
      total: toNumber(users.total),
      admins: toNumber(users.admins),
    },
    storage: toNumberRecord(record.storage),
  };
}

function normalizeJobsResponse(
  value: ApiEnvelope<JobsResponse> | JobsResponse | unknown
): JobsResponse {
  const data = unwrapUnknownEnvelope(value);

  if (!isRecord(data)) {
    return {
      items: [],
      total: 0,
    };
  }

  const rawItems = Array.isArray(data.items) ? data.items : [];

  const items = rawItems
    .filter(isRecord)
    .map(
      (job): Job => ({
        id: toStringValue(job.id),
        status: toStringValue(job.status || 'pending'),
        source_path: toStringValue(job.source_path),
        total_items: toNumber(job.total_items),
        done_items: toNumber(job.done_items),
        created_at: toStringValue(job.created_at),
        error_msg: typeof job.error_msg === 'string' ? job.error_msg : undefined,
      })
    )
    .filter((job) => job.id);

  const total = toNumber(data.total);

  return {
    items,
    total: total > 0 ? total : items.length,
  };
}

function unwrapUnknownEnvelope(value: unknown): unknown {
  if (isRecord(value) && 'data' in value) {
    return value.data;
  }

  return value;
}

function safeRatio(a: number, b: number): number {
  if (!Number.isFinite(a) || !Number.isFinite(b) || b <= 0) return 0;

  return Math.min(1, Math.max(0, a / b));
}

function statusClass(status: string): string {
  switch (status.toLowerCase()) {
    case 'done':
    case 'completed':
    case 'success':
      return 'status-ok';
    case 'running':
    case 'processing':
      return 'status-running';
    case 'error':
    case 'failed':
      return 'status-error';
    case 'cancelled':
      return 'status-cancelled';
    default:
      return 'status-pending';
  }
}

function statusLabel(status: string): string {
  switch (status.toLowerCase()) {
    case 'done':
    case 'completed':
    case 'success':
      return 'Done';
    case 'running':
    case 'processing':
      return 'Running';
    case 'error':
    case 'failed':
      return 'Error';
    case 'cancelled':
      return 'Cancelled';
    case 'queued':
      return 'Queued';
    default:
      return 'Pending';
  }
}

function formatDate(iso: string): string {
  if (!iso) return '—';

  const date = new Date(iso);

  if (Number.isNaN(date.getTime())) {
    return '—';
  }

  return date.toLocaleDateString(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
}

function getErrorMessage(errorValue: unknown, fallback: string): string {
  if (isRecord(errorValue)) {
    const response = errorValue.response;

    if (isRecord(response)) {
      const data = response.data;

      if (isRecord(data)) {
        const message = data.message || data.error || data.detail;

        if (typeof message === 'string' && message.trim()) {
          return message;
        }
      }

      if (typeof data === 'string' && data.trim()) {
        return data;
      }
    }

    const message = errorValue.message;

    if (typeof message === 'string' && message.trim()) {
      return message;
    }
  }

  return fallback;
}

function getHttpStatus(errorValue: unknown): number | undefined {
  if (!isRecord(errorValue)) return undefined;

  const response = errorValue.response;

  if (!isRecord(response)) return undefined;

  const status = response.status;

  return typeof status === 'number' ? status : undefined;
}

function isUnauthorizedOrForbidden(errorValue: unknown): boolean {
  const status = getHttpStatus(errorValue);

  return status === 401 || status === 403;
}

function toNumber(value: unknown): number {
  const numberValue = Number(value);

  return Number.isFinite(numberValue) ? numberValue : 0;
}

function toStringValue(value: unknown): string {
  return typeof value === 'string' ? value : '';
}

function toStringArray(value: unknown): string[] {
  if (!Array.isArray(value)) return [];

  return value.filter((item): item is string => typeof item === 'string' && item.trim().length > 0);
}

function toNumberRecord(value: unknown): Record<string, number> {
  if (!isRecord(value)) return {};

  const result: Record<string, number> = {};

  for (const [key, rawValue] of Object.entries(value)) {
    result[key] = toNumber(rawValue);
  }

  return result;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null;
}
</script>

<style scoped>
.system-view {
  width: 100%;
  padding: 24px;
  box-sizing: border-box;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 16px;
  margin-bottom: 24px;
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

.refresh-btn,
.error-action {
  padding: 8px 14px;
  color: #fff;
  background: #6c63ff;
  border: 0;
  border-radius: 8px;
  cursor: pointer;
  font-size: 13px;
  font-weight: 700;
  transition:
    background-color 0.15s ease,
    opacity 0.15s ease;
}

.refresh-btn:hover:not(:disabled),
.error-action:hover {
  background: #5a52e0;
}

.refresh-btn:disabled {
  cursor: not-allowed;
  opacity: 0.55;
}

.loading {
  padding: 48px 0;
  color: #555577;
  text-align: center;
  font-size: 14px;
}

.error-card {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 16px;
  color: #ffb3b3;
  background: #2a1a1a;
  border: 1px solid #5a2a2a;
  border-radius: 12px;
  font-size: 14px;
}

.error-card strong {
  display: block;
  color: #ffd0d0;
  margin-bottom: 4px;
}

.error-card p {
  margin: 0;
  line-height: 1.45;
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
  gap: 16px;
  margin-bottom: 32px;
}

.stat-card {
  padding: 20px;
  background: #12121e;
  border: 1px solid #1a1a2e;
  border-radius: 12px;
}

.stat-value {
  color: #e0e0e0;
  font-size: 34px;
  font-weight: 800;
  line-height: 1;
}

.stat-value.warn {
  color: #ff9800;
}

.stat-label {
  margin-top: 8px;
  color: #8888aa;
  font-size: 13px;
  font-weight: 700;
}

.stat-detail {
  margin-top: 8px;
  color: #555577;
  font-size: 12px;
  line-height: 1.45;
}

.error-text {
  color: #ff6b6b;
}

.section {
  margin-bottom: 32px;
}

.section-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 12px;
}

h2 {
  margin: 0;
  color: #c0c0d0;
  font-size: 17px;
  font-weight: 800;
}

.section-subtitle {
  margin: 4px 0 0;
  color: #555577;
  font-size: 12px;
  line-height: 1.45;
}

.count {
  flex: 0 0 auto;
  color: #555577;
  font-size: 12px;
}

.inline-warning {
  margin-bottom: 12px;
  padding: 10px 12px;
  color: #ffcc80;
  background: #2a2115;
  border: 1px solid #5a4320;
  border-radius: 8px;
  font-size: 12px;
  line-height: 1.45;
}

.jobs-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.job-row {
  display: grid;
  grid-template-columns: 96px minmax(0, 1fr) 90px 120px 72px;
  align-items: center;
  gap: 12px;
  padding: 11px 14px;
  background: #12121e;
  border: 1px solid #1a1a2e;
  border-radius: 8px;
  font-size: 13px;
}

.job-status {
  font-size: 12px;
  font-weight: 800;
}

.job-status.status-ok {
  color: #4caf50;
}

.job-status.status-running {
  color: #2196f3;
}

.job-status.status-error {
  color: #ff6b6b;
}

.job-status.status-cancelled {
  color: #ff9800;
}

.job-status.status-pending {
  color: #666688;
}

.job-path {
  min-width: 0;
  color: #aaaacc;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.job-progress {
  color: #666688;
  font-size: 12px;
  text-align: right;
}

.job-progress.muted {
  color: #44445e;
}

.job-date {
  color: #555577;
  font-size: 12px;
  text-align: right;
}

.job-error {
  color: #ff9800;
  cursor: help;
  font-size: 12px;
  text-align: right;
}

.empty {
  padding: 40px 0;
  color: #555577;
  text-align: center;
  font-size: 14px;
}

.storage-bars {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.storage-row {
  display: grid;
  grid-template-columns: 100px minmax(0, 1fr) 90px;
  align-items: center;
  gap: 12px;
}

.storage-provider {
  color: #aaaacc;
  font-size: 13px;
  text-transform: capitalize;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.storage-bar-track {
  height: 8px;
  overflow: hidden;
  background: #1a1a2e;
  border-radius: 999px;
}

.storage-bar-fill {
  width: 100%;
  height: 100%;
  background: linear-gradient(90deg, #6c63ff, #2196f3);
  border-radius: inherit;
  transform-origin: left;
  transition: transform 0.3s ease;
}

.storage-count {
  color: #555577;
  font-size: 12px;
  text-align: right;
}

@media (max-width: 820px) {
  .system-view {
    padding: 18px;
  }

  .page-header,
  .section-header,
  .error-card {
    flex-direction: column;
  }

  .refresh-btn,
  .error-action {
    width: 100%;
  }

  .job-row {
    grid-template-columns: 1fr;
    gap: 6px;
  }

  .job-progress,
  .job-date,
  .job-error {
    text-align: left;
  }

  .storage-row {
    grid-template-columns: 1fr;
    gap: 6px;
  }

  .storage-count {
    text-align: left;
  }
}

@media (prefers-reduced-motion: reduce) {
  .refresh-btn,
  .error-action,
  .storage-bar-fill {
    transition: none;
  }
}
</style>
