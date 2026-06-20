<template>
  <div class="admin-page">
    <div class="page-header">
      <h1>{{ $t('admin.missing.title') }}</h1>
      <button
        class="danger-btn"
        :disabled="deleting || loading || items.length === 0"
        @click="deleteAllMissing"
      >
        {{ deleting ? $t('admin.missing.deleting') : $t('admin.missing.deleteAll') }}
      </button>
    </div>

    <p class="hint">
      {{ $t('admin.missing.subtitle') }}
    </p>

    <div v-if="libraries.length > 1" class="toolbar">
      <select
        v-model="libraryFilter"
        class="filter-select"
        :disabled="loading || deleting"
        @change="fetchMissing"
      >
        <option value="">{{ $t('admin.missing.allLibraries') }}</option>
        <option v-for="lib in libraries" :key="lib.id" :value="lib.id">
          {{ lib.name }}
        </option>
      </select>
    </div>

    <div v-if="loading" class="loading">{{ $t('common.loading') }}</div>
    <p v-else-if="error" class="error-text">{{ error }}</p>

    <template v-else>
      <div v-if="total > 0" class="result-info">
        {{ total }} {{ $t('admin.missing.itemCount', total) }}
      </div>

      <div v-if="items.length > 0" class="missing-list">
        <div v-for="item in items" :key="item.id" class="missing-row">
          <span class="item-type" :class="item.type">{{ item.type }}</span>
          <span class="item-title">{{ item.title }}</span>
          <span class="item-path">{{ item.file_path }}</span>
          <span class="item-library">{{ resolveLibraryName(item.library_id) }}</span>
          <button
            class="delete-btn"
            :disabled="deletingId === item.id || deleting"
            @click="deleteSingle(item)"
          >
            {{ deletingId === item.id ? $t('admin.missing.removing') : $t('admin.missing.removeButton') }}
          </button>
        </div>
      </div>

      <div v-else class="all-clear">
        <span class="check-icon">&#10003;</span>
        <p>{{ $t('admin.missing.allAvailable') }}</p>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { useI18n } from 'vue-i18n';
import { authRequest } from '@/api/request';
import { getSafeApiErrorMessage, isUnauthorizedOrForbidden } from '@/lib/api/errors';
import { useNotifications } from '@/composables/useNotifications';
import type { ApiEnvelope } from '@/api/types';

const { t } = useI18n();
const { notifySuccess, confirmDialog } = useNotifications();

interface Library {
  id: string;
  name: string;
}

interface MissingItem {
  id: string;
  type: string;
  title: string;
  file_path: string;
  library_id: string;
}

interface MissingItemsResponse {
  items: MissingItem[];
  total: number;
}

interface DeleteAllMissingResponse {
  deleted_count: number;
}

const items = ref<MissingItem[]>([]);
const total = ref(0);
const loading = ref(true);
const deleting = ref(false);
const deletingId = ref('');
const libraryFilter = ref('');
const libraries = ref<Library[]>([]);
const error = ref('');

async function loadLibraries(): Promise<void> {
  try {
    const res = await authRequest.get<ApiEnvelope<Library[]>>('/admin/libraries');
    libraries.value = res.data.data || [];
  } catch (err: any) {
    if (isUnauthorizedOrForbidden(err)) {
      return;
    }

    console.error('[missing] loadLibraries failed:', err);
  }
}

async function fetchMissing(): Promise<void> {
  loading.value = true;
  error.value = '';

  try {
    const params: Record<string, string> = {};
    if (libraryFilter.value) {
      params.library_id = libraryFilter.value;
    }

    const res = await authRequest.get<ApiEnvelope<MissingItemsResponse>>('/admin/media/missing', {
      params,
    });

    items.value = res.data.data?.items || [];
    total.value = res.data.data?.total || 0;
  } catch (err: any) {
    if (isUnauthorizedOrForbidden(err)) {
      return;
    }

    console.error('[missing] fetchMissing failed:', err);
    error.value = t('admin.missing.loadFailed');
  } finally {
    loading.value = false;
  }
}

async function deleteSingle(item: MissingItem): Promise<void> {
  deletingId.value = item.id;
  error.value = '';

  try {
    await authRequest.delete(`/admin/media/${item.id}`);
    await fetchMissing();
  } catch (err: any) {
    if (isUnauthorizedOrForbidden(err)) {
      return;
    }

    console.error('[missing] deleteSingle failed:', err);
    error.value =
      err?.response?.data?.message ||
      err?.response?.data?.error ||
      t('admin.missing.removeFailed', { title: item.title });
  } finally {
    deletingId.value = '';
  }
}

async function deleteAllMissing(): Promise<void> {
  if (!(await confirmDialog(t('admin.missing.confirmDeleteAll', { n: total.value })))) {
    return;
  }

  deleting.value = true;
  error.value = '';

  try {
    const body: Record<string, string> = {};
    if (libraryFilter.value) {
      body.library_id = libraryFilter.value;
    }

    const res = await authRequest.delete<ApiEnvelope<DeleteAllMissingResponse>>(
      '/admin/media/missing',
      { data: body }
    );

    const deletedCount = res.data.data?.deleted_count || 0;
    notifySuccess(t('admin.missing.deletedCount', { n: deletedCount }));

    await fetchMissing();
  } catch (err: any) {
    if (isUnauthorizedOrForbidden(err)) {
      return;
    }

    console.error('[missing] deleteAllMissing failed:', err);
    error.value = getSafeApiErrorMessage(err, 'admin.missing.loadFailed');
  } finally {
    deleting.value = false;
  }
}

function resolveLibraryName(libraryId: string): string {
  const lib = libraries.value.find((entry) => entry.id === libraryId);
  return lib?.name || libraryId;
}

onMounted(async () => {
  await loadLibraries();
  await fetchMissing();
});
</script>

<style scoped>
.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8px;
}

h1 {
  font-size: 22px;
  color: #e0e0e0;
  margin: 0;
}

.hint {
  color: #555577;
  font-size: 13px;
  margin-bottom: 20px;
}

.loading {
  color: #555577;
  font-size: 14px;
  padding: 20px 0;
}

.error-text {
  color: #ff6b6b;
  font-size: 13px;
  margin-bottom: 16px;
}

.danger-btn {
  padding: 8px 16px;
  background: #1a0f0f;
  color: #ff6b6b;
  border: 1px solid rgba(255, 107, 107, 0.3);
  border-radius: 6px;
  cursor: pointer;
  font-size: 13px;
}

.danger-btn:hover:not(:disabled) {
  background: #2a1515;
}

.danger-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.toolbar {
  margin-bottom: 16px;
}

.filter-select {
  padding: 8px 10px;
  background: #0a0a14;
  border: 1px solid #1a1a2e;
  border-radius: 4px;
  color: #aaaacc;
  font-size: 13px;
}

.filter-select:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.result-info {
  color: #ff9800;
  font-size: 13px;
  margin-bottom: 12px;
}

.missing-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.missing-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 12px;
  background: #12121e;
  border-radius: 4px;
  border-left: 2px solid rgba(255, 152, 0, 0.5);
  font-size: 13px;
}

.item-type {
  font-size: 11px;
  padding: 2px 6px;
  border-radius: 3px;
  min-width: 50px;
  text-align: center;
  text-transform: uppercase;
  background: #1a1510;
  color: #ff9800;
}

.item-title {
  color: #ccccee;
  min-width: 120px;
}

.item-path {
  flex: 1;
  color: #555577;
  font-size: 12px;
  font-family: monospace;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.item-library {
  color: #555577;
  font-size: 12px;
  min-width: 80px;
}

.delete-btn {
  padding: 4px 10px;
  border: 1px solid rgba(255, 107, 107, 0.3);
  border-radius: 4px;
  background: transparent;
  color: #ff6b6b;
  cursor: pointer;
  font-size: 11px;
}

.delete-btn:hover:not(:disabled) {
  background: #1a0f0f;
}

.delete-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.all-clear {
  text-align: center;
  padding: 60px 20px;
}

.check-icon {
  font-size: 48px;
  color: #4caf50;
  display: block;
  margin-bottom: 16px;
}

.all-clear p {
  color: #555577;
  font-size: 15px;
  margin: 0;
}
</style>
