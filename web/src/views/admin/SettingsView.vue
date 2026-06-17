<template>
  <div class="admin-page">
    <h1>Settings</h1>

    <div v-if="loading" class="loading">Loading...</div>

    <div v-else-if="error" class="error">{{ error }}</div>

    <template v-else>
      <div class="settings-section">
        <h2>Registration</h2>

        <label class="toggle-row">
          <input type="checkbox" v-model="allowRegistration" :disabled="saving" />
          <span>Allow public registration</span>
        </label>

        <p class="hint">When disabled, only admins can create new accounts.</p>
      </div>

      <button class="save-btn" :disabled="saving" @click="saveSettings">
        {{ saving ? 'Saving...' : 'Save Settings' }}
      </button>

      <p v-if="message" class="msg">{{ message }}</p>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { authRequest } from '@/api/request';
import type { ApiEnvelope } from '@/api/types';

/**
 * Backend shape assumption:
 * GET /admin/settings -> { allow_registration: "true" | "false" }
 */
interface SettingsData {
  allow_registration: string;
}

const allowRegistration = ref(false);

const loading = ref(true);
const saving = ref(false);

const message = ref('');
const error = ref('');

async function loadSettings(): Promise<void> {
  loading.value = true;
  error.value = '';

  try {
    const res = await authRequest.get<ApiEnvelope<SettingsData>>('/admin/settings');

    const data = res.data.data;

    allowRegistration.value = data.allow_registration === 'true';
  } catch (err: any) {
    const status = err?.response?.status;

    if (status === 401 || status === 403) {
      // 由 router / store 统一处理，不污染控制台
      return;
    }

    console.error('[settings] loadSettings failed:', err);
    error.value = 'Failed to load settings';
  } finally {
    loading.value = false;
  }
}

async function saveSettings(): Promise<void> {
  saving.value = true;
  message.value = '';
  error.value = '';

  try {
    await authRequest.put<ApiEnvelope<null>>('/admin/settings', {
      allow_registration: allowRegistration.value ? 'true' : 'false',
    });

    message.value = 'Settings saved';
  } catch (err: any) {
    const status = err?.response?.status;

    if (status === 401 || status === 403) {
      return;
    }

    console.error('[settings] saveSettings failed:', err);

    message.value =
      err?.response?.data?.message || err?.response?.data?.error || 'Failed to save settings';
  } finally {
    saving.value = false;
  }
}

onMounted(() => {
  void loadSettings();
});
</script>

<style scoped>
.admin-page h1 {
  font-size: 22px;
  color: #e0e0e0;
  margin: 0 0 24px;
}

h2 {
  font-size: 16px;
  color: #c0c0d0;
  margin: 0 0 12px;
}

.loading {
  color: #555577;
}

.error {
  color: #ff6b6b;
  font-size: 14px;
}

.settings-section {
  margin-bottom: 24px;
  padding: 20px;
  background: #12121e;
  border-radius: 8px;
  border: 1px solid #1a1a2e;
}

.toggle-row {
  display: flex;
  align-items: center;
  gap: 10px;
  color: #ccccee;
  font-size: 14px;
  cursor: pointer;
}

.toggle-row input[type='checkbox'] {
  accent-color: #6c63ff;
  width: 18px;
  height: 18px;
}

.hint {
  color: #555577;
  font-size: 12px;
  margin-top: 8px;
}

.save-btn {
  padding: 10px 24px;
  background: #6c63ff;
  color: #fff;
  border: none;
  border-radius: 6px;
  cursor: pointer;
  font-size: 14px;
}

.save-btn:hover:not(:disabled) {
  background: #5a52e0;
}

.save-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.msg {
  margin-top: 12px;
  font-size: 13px;
  color: #4caf50;
}
</style>
