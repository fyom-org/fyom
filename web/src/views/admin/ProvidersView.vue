<template>
  <div class="admin-page">
    <div class="page-header">
      <h1>Storage Providers</h1>
      <button class="add-btn" @click="toggleForm" :disabled="loading || saving">
        {{ showForm ? 'Cancel' : '+ Add Provider' }}
      </button>
    </div>

    <div v-if="showForm" class="form-card">
      <div class="field">
        <label>Provider ID</label>
        <input v-model.trim="form.id" placeholder="wasabi-main" :disabled="saving" />
      </div>

      <div class="field">
        <label>Type</label>
        <select v-model="form.type" :disabled="saving">
          <option value="s3">S3 Compatible</option>
          <option value="remote_fyom">Remote fyom</option>
        </select>
      </div>

      <div class="field">
        <label>Display Name</label>
        <input v-model.trim="form.display_name" placeholder="Wasabi US-East" :disabled="saving" />
      </div>

      <div class="field">
        <label>Config (JSON)</label>
        <textarea
          v-model="form.config"
          rows="6"
          :disabled="saving"
          placeholder='{"bucket":"media","region":"us-east-1","access_key_id":"...","secret_access_key":"..."}'
        />
      </div>

      <p v-if="error" class="error">{{ error }}</p>

      <button class="submit-btn" @click="createProvider" :disabled="saving">
        {{ saving ? 'Creating...' : 'Create Provider' }}
      </button>
    </div>

    <div v-if="loading" class="loading">Loading...</div>

    <template v-else>
      <p v-if="!showForm && error" class="error">{{ error }}</p>

      <div v-if="providers.length > 0" class="provider-list">
        <div v-for="provider in providers" :key="provider.id" class="provider-row">
          <div class="provider-info">
            <span class="provider-name">{{ provider.display_name }}</span>
            <span class="provider-meta"> {{ provider.id }} · {{ provider.type }} </span>
          </div>

          <div class="provider-actions">
            <button
              :class="['toggle-btn', { active: provider.enabled }]"
              :disabled="togglePendingId === provider.id || deletePendingId === provider.id"
              @click="toggleEnabled(provider)"
            >
              {{
                togglePendingId === provider.id
                  ? 'Saving...'
                  : provider.enabled
                    ? 'Enabled'
                    : 'Disabled'
              }}
            </button>

            <button
              class="delete-btn"
              :disabled="deletePendingId === provider.id || togglePendingId === provider.id"
              @click="deleteProvider(provider)"
            >
              {{ deletePendingId === provider.id ? 'Deleting...' : 'Delete' }}
            </button>
          </div>
        </div>
      </div>

      <p v-else class="empty-text">No providers configured.</p>
    </template>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { authRequest } from '@/api/request';
import type { ApiEnvelope } from '@/api/types';

type ProviderType = 's3' | 'remote_fyom';

interface Provider {
  id: string;
  type: ProviderType | string;
  display_name: string;
  config: unknown;
  enabled: boolean;
}

interface ProviderForm {
  id: string;
  type: ProviderType;
  display_name: string;
  config: string;
  enabled: boolean;
}

const providers = ref<Provider[]>([]);
const loading = ref(true);
const saving = ref(false);
const error = ref('');
const showForm = ref(false);

const togglePendingId = ref('');
const deletePendingId = ref('');

const form = ref<ProviderForm>(createEmptyForm());

function createEmptyForm(): ProviderForm {
  return {
    id: '',
    type: 's3',
    display_name: '',
    config: '',
    enabled: true,
  };
}

function toggleForm(): void {
  error.value = '';
  showForm.value = !showForm.value;

  if (!showForm.value) {
    form.value = createEmptyForm();
  }
}

function validateForm(): string {
  if (!form.value.id) {
    return 'Provider ID is required';
  }

  if (!form.value.display_name) {
    return 'Display name is required';
  }

  if (!form.value.config.trim()) {
    return 'Config JSON is required';
  }

  try {
    JSON.parse(form.value.config);
  } catch {
    return 'Config must be valid JSON';
  }

  return '';
}

async function fetchProviders(): Promise<void> {
  loading.value = true;
  error.value = '';

  try {
    const res = await authRequest.get<ApiEnvelope<Provider[]>>('/admin/providers');
    providers.value = res.data.data || [];
  } catch (err: any) {
    const status = err?.response?.status;

    if (status === 401 || status === 403) {
      return;
    }

    console.error('[providers] fetchProviders failed:', err);
    error.value = 'Failed to load providers';
  } finally {
    loading.value = false;
  }
}

async function createProvider(): Promise<void> {
  error.value = '';

  const validationError = validateForm();
  if (validationError) {
    error.value = validationError;
    return;
  }

  saving.value = true;

  try {
    await authRequest.post<ApiEnvelope<Provider>>('/admin/providers', {
      id: form.value.id,
      type: form.value.type,
      display_name: form.value.display_name,
      config: form.value.config,
      enabled: form.value.enabled,
    });

    showForm.value = false;
    form.value = createEmptyForm();

    await fetchProviders();
  } catch (err: any) {
    const status = err?.response?.status;

    if (status === 401 || status === 403) {
      return;
    }

    console.error('[providers] createProvider failed:', err);
    error.value =
      err?.response?.data?.message || err?.response?.data?.error || 'Failed to create provider';
  } finally {
    saving.value = false;
  }
}

async function deleteProvider(provider: Provider): Promise<void> {
  if (!confirm(`Delete provider "${provider.display_name}"?`)) {
    return;
  }

  deletePendingId.value = provider.id;
  error.value = '';

  try {
    await authRequest.delete<ApiEnvelope<null>>(`/admin/providers/${provider.id}`);
    await fetchProviders();
  } catch (err: any) {
    const status = err?.response?.status;

    if (status === 401 || status === 403) {
      return;
    }

    console.error('[providers] deleteProvider failed:', err);
    error.value =
      err?.response?.data?.message || err?.response?.data?.error || 'Failed to delete provider';
  } finally {
    deletePendingId.value = '';
  }
}

function normalizeProviderConfig(config: unknown): string | unknown {
  if (typeof config === 'string') {
    return config;
  }

  try {
    return JSON.stringify(config);
  } catch {
    return config;
  }
}

async function toggleEnabled(provider: Provider): Promise<void> {
  togglePendingId.value = provider.id;
  error.value = '';

  try {
    await authRequest.put<ApiEnvelope<Provider>>(`/admin/providers/${provider.id}`, {
      type: provider.type,
      display_name: provider.display_name,
      config: normalizeProviderConfig(provider.config),
      enabled: !provider.enabled,
    });

    await fetchProviders();
  } catch (err: any) {
    const status = err?.response?.status;

    if (status === 401 || status === 403) {
      return;
    }

    console.error('[providers] toggleEnabled failed:', err);
    error.value =
      err?.response?.data?.message || err?.response?.data?.error || 'Failed to update provider';
  } finally {
    togglePendingId.value = '';
  }
}

onMounted(() => {
  void fetchProviders();
});
</script>

<style scoped>
.admin-page h1 {
  font-size: 22px;
  color: #e0e0e0;
  margin: 0;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;
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

.add-btn:hover:not(:disabled) {
  background: #5a52e0;
}

.add-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.loading {
  color: #555577;
  font-size: 14px;
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
.field select,
.field textarea {
  width: 100%;
  padding: 8px 10px;
  background: #0a0a14;
  border: 1px solid #1a1a2e;
  border-radius: 4px;
  color: #ccccee;
  font-size: 13px;
  box-sizing: border-box;
  font-family: monospace;
}

.field input:disabled,
.field select:disabled,
.field textarea:disabled {
  opacity: 0.7;
  cursor: not-allowed;
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

.submit-btn:hover:not(:disabled) {
  background: #5a52e0;
}

.submit-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.error {
  color: #ff6b6b;
  font-size: 12px;
  margin-top: 8px;
}

.provider-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.provider-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 16px;
  background: #12121e;
  border-radius: 6px;
  border: 1px solid #1a1a2e;
}

.provider-name {
  color: #e0e0e0;
  font-size: 14px;
}

.provider-meta {
  color: #555577;
  font-size: 12px;
  margin-left: 8px;
}

.provider-actions {
  display: flex;
  gap: 8px;
}

.toggle-btn {
  padding: 4px 12px;
  border: 1px solid #2a2a3e;
  border-radius: 4px;
  background: transparent;
  color: #555577;
  cursor: pointer;
  font-size: 12px;
}

.toggle-btn.active {
  background: #1a3a1a;
  color: #4caf50;
  border-color: #2a4a2a;
}

.toggle-btn:hover:not(:disabled) {
  border-color: #3a3a4f;
}

.toggle-btn:disabled {
  opacity: 0.6;
  cursor: wait;
}

.delete-btn {
  padding: 4px 12px;
  border: 1px solid #3a1a1a;
  border-radius: 4px;
  background: transparent;
  color: #ff6b6b;
  cursor: pointer;
  font-size: 12px;
}

.delete-btn:hover:not(:disabled) {
  background: #2a1a1a;
}

.delete-btn:disabled {
  opacity: 0.6;
  cursor: wait;
}

.empty-text {
  color: #555577;
  font-size: 14px;
}
</style>
