<template>
  <div class="system-view">
    <h1>System Health</h1>

    <div v-if="loading" class="loading">Loading...</div>
    <div v-else-if="error" class="error">{{ error }}</div>

    <template v-else-if="stats">
      <div class="stats-grid">
        <div class="stat-card">
          <div class="stat-value">{{ stats.library.total_items }}</div>
          <div class="stat-label">Library Items</div>
          <div class="stat-detail">
            {{ stats.library.movies }} movies · {{ stats.library.shows }} shows ·
            {{ stats.library.episodes }} episodes
          </div>
        </div>

        <div class="stat-card">
          <div class="stat-value" :class="stats.imports.error > 0 ? 'warn' : ''">
            {{ stats.imports.done }}/{{ stats.imports.total }}
          </div>
          <div class="stat-label">Imports Completed</div>
          <div class="stat-detail" v-if="stats.imports.running > 0">
            {{ stats.imports.running }} running
          </div>
          <div class="stat-detail error-text" v-if="stats.imports.error > 0">
            {{ stats.imports.error }} failed
          </div>
        </div>

        <div class="stat-card">
          <div class="stat-value">{{ stats.providers.enabled }}/{{ stats.providers.total }}</div>
          <div class="stat-label">Providers Active</div>
          <div class="stat-detail">
            {{ stats.providers.types.join(', ') }}
          </div>
        </div>

        <div class="stat-card">
          <div class="stat-value">{{ stats.users.total }}</div>
          <div class="stat-label">Users</div>
          <div class="stat-detail">
            {{ stats.users.admins }} admin{{ stats.users.admins !== 1 ? 's' : '' }}
          </div>
        </div>
      </div>

      <div class="section">
        <div class="section-header">
          <h2>Recent Imports</h2>
          <span class="count" v-if="jobsTotal > 5">{{ jobsTotal }} total</span>
        </div>

        <div class="jobs-list" v-if="jobs.length > 0">
          <div class="job-row" v-for="job in jobs" :key="job.id">
            <span class="job-status" :class="statusClass(job.status)">
              {{ statusLabel(job.status) }}
            </span>
            <span class="job-path">{{ job.source_path }}</span>
            <span class="job-progress" v-if="job.total_items > 0">
              {{ job.done_items }}/{{ job.total_items }}
            </span>
            <span class="job-date">{{ formatDate(job.created_at) }}</span>
            <span class="job-error" v-if="job.error_msg" :title="job.error_msg">⚠</span>
          </div>
        </div>

        <p class="empty" v-else>No import jobs yet.</p>
      </div>

      <div class="section">
        <h2>Storage Distribution</h2>

        <div class="storage-bars">
          <div v-for="(count, provider) in stats.storage" :key="provider" class="storage-row">
            <span class="storage-provider">{{ provider }}</span>

            <div class="storage-bar-track">
              <div
                class="storage-bar-fill"
                :style="{
                  transform: `scaleX(${safeRatio(count, stats.library.total_items)})`,
                }"
              />
            </div>

            <span class="storage-count">{{ count }} items</span>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
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

const stats = ref<Stats | null>(null);
const jobs = ref<Job[]>([]);
const jobsTotal = ref(0);
const loading = ref(true);
const error = ref('');

async function loadSystemData(): Promise<void> {
  loading.value = true;
  error.value = '';

  try {
    const [statsRes, jobsRes] = await Promise.all([
      authRequest.get<ApiEnvelope<Stats>>('/admin/stats'),
      authRequest.get<ApiEnvelope<JobsResponse>>('/admin/import-jobs?limit=5'),
    ]);

    stats.value = statsRes.data.data;
    jobs.value = jobsRes.data.data.items || [];
    jobsTotal.value = jobsRes.data.data.total || 0;
  } catch (err: any) {
    const status = err?.response?.status;

    if (status === 401 || status === 403) {
      // 被 router / store 接管，不打扰控制台
      return;
    }

    console.error('[system] loadSystemData failed:', err);
    error.value = 'Failed to load system stats';
  } finally {
    loading.value = false;
  }
}

function safeRatio(a: number, b: number): number {
  if (!b || b === 0) return 0;
  return a / b;
}

function statusClass(status: string): string {
  switch (status) {
    case 'done':
      return 'status-ok';
    case 'running':
      return 'status-running';
    case 'error':
      return 'status-error';
    default:
      return 'status-pending';
  }
}

function statusLabel(status: string): string {
  switch (status) {
    case 'done':
      return '✓ Done';
    case 'running':
      return '↻ Running';
    case 'error':
      return '✕ Error';
    default:
      return '○ Pending';
  }
}

function formatDate(iso: string): string {
  if (!iso) return '';

  return new Date(iso).toLocaleDateString(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
}

onMounted(() => {
  void loadSystemData();
});
</script>

<style scoped>
.system-view {
  padding: 24px;
}

h1 {
  font-size: 22px;
  color: #e0e0e0;
  margin: 0 0 24px;
}

.loading {
  color: #555577;
  font-size: 14px;
  padding: 40px 0;
  text-align: center;
}

.error {
  color: #ff6b6b;
  font-size: 14px;
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 16px;
  margin-bottom: 32px;
}

.stat-card {
  background: #12121e;
  border: 1px solid #1a1a2e;
  border-radius: 8px;
  padding: 20px;
}

.stat-value {
  font-size: 32px;
  font-weight: 700;
  color: #e0e0e0;
}

.stat-value.warn {
  color: #ff9800;
}

.stat-label {
  font-size: 13px;
  color: #8888aa;
  margin-top: 4px;
}

.stat-detail {
  font-size: 12px;
  color: #555577;
  margin-top: 8px;
}

.error-text {
  color: #ff6b6b;
}

.section {
  margin-bottom: 32px;
}

.section-header {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
}

h2 {
  font-size: 16px;
  color: #c0c0d0;
  margin: 0;
}

.count {
  font-size: 12px;
  color: #555577;
}

.jobs-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.job-row {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 16px;
  background: #12121e;
  border-radius: 6px;
  border: 1px solid #1a1a2e;
  font-size: 13px;
}

.job-status {
  font-size: 12px;
  font-weight: 600;
  min-width: 80px;
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

.job-status.status-pending {
  color: #555577;
}

.job-path {
  flex: 1;
  color: #aaaacc;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.job-progress {
  color: #555577;
  font-size: 12px;
}

.job-date {
  color: #555577;
  font-size: 12px;
  min-width: 100px;
  text-align: right;
}

.job-error {
  color: #ff9800;
  cursor: help;
}

.empty {
  color: #555577;
  font-size: 14px;
  padding: 40px 0;
  text-align: center;
}

.storage-bars {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.storage-row {
  display: flex;
  align-items: center;
  gap: 12px;
}

.storage-provider {
  color: #aaaacc;
  font-size: 13px;
  min-width: 80px;
  text-transform: capitalize;
}

.storage-bar-track {
  flex: 1;
  height: 8px;
  background: #1a1a2e;
  border-radius: 4px;
  overflow: hidden;
}

.storage-bar-fill {
  height: 100%;
  background: #6c63ff;
  border-radius: 4px;
  transition: transform 0.3s;
  transform-origin: left;
  width: 100%;
}

.storage-count {
  color: #555577;
  font-size: 12px;
  min-width: 60px;
  text-align: right;
}
</style>
