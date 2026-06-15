<template>
  <div class="admin-page">
    <h1>Settings</h1>
    <div v-if="loading" class="loading">Loading...</div>
    <template v-else>
      <div class="settings-section">
        <h2>Registration</h2>
        <label class="toggle-row">
          <input type="checkbox" v-model="allowRegistration" />
          <span>Allow public registration</span>
        </label>
        <p class="hint">When disabled, only admins can create new accounts.</p>
      </div>

      <button class="save-btn" @click="save" :disabled="saving">
        {{ saving ? 'Saving...' : 'Save Settings' }}
      </button>
      <p class="msg" v-if="msg">{{ msg }}</p>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { apiRequest } from '@/api/request';

const allowRegistration = ref(false);
const loading = ref(true);
const saving = ref(false);
const msg = ref('');

onMounted(async () => {
  try {
    const res: any = await request.get('/admin/settings');
    allowRegistration.value = res.data?.allow_registration === 'true';
  } catch {
    // ignore
  } finally {
    loading.value = false;
  }
});

async function save() {
  saving.value = true;
  msg.value = '';
  try {
    await request.put('/admin/settings', {
      allow_registration: allowRegistration.value ? 'true' : 'false',
    });
    msg.value = 'Settings saved';
  } catch {
    msg.value = 'Failed to save';
  } finally {
    saving.value = false;
  }
}
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
}

.msg {
  margin-top: 12px;
  font-size: 13px;
  color: #4caf50;
}
</style>
