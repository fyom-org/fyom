<template>
  <div class="admin-page">
    <div class="page-header">
      <h1>Storage Providers</h1>
      <button class="add-btn" @click="showForm = !showForm">
        {{ showForm ? 'Cancel' : '+ Add Provider' }}
      </button>
    </div>
    <div v-if="showForm" class="form-card">
      <div class="field">
        <label>Provider ID</label><input v-model="form.id" placeholder="wasabi-main" />
      </div>
      <div class="field">
        <label>Type</label
        ><select v-model="form.type">
          <option value="s3">S3 Compatible</option>
          <option value="remote_fyom">Remote fyom</option>
        </select>
      </div>
      <div class="field">
        <label>Display Name</label
        ><input v-model="form.display_name" placeholder="Wasabi US-East" />
      </div>
      <div class="field">
        <label>Config (JSON)</label
        ><textarea
          v-model="form.config"
          rows="6"
          placeholder='{"bucket":"media","region":"us-east-1","access_key_id":"...","secret_access_key":"..."}'
        />
      </div>
      <p v-if="error" class="error">{{ error }}</p>
      <button class="submit-btn" @click="createProvider">Create Provider</button>
    </div>
    <div v-if="providers.length > 0" class="provider-list">
      <div v-for="p in providers" :key="p.id" class="provider-row">
        <div class="provider-info">
          <span class="provider-name">{{ p.display_name }}</span>
          <span class="provider-meta">{{ p.id }} · {{ p.type }}</span>
        </div>
        <div class="provider-actions">
          <button :class="['toggle-btn', { active: p.enabled }]" @click="toggleEnabled(p)">
            {{ p.enabled ? 'Enabled' : 'Disabled' }}
          </button>
          <button class="delete-btn" @click="deleteProvider(p.id)">Delete</button>
        </div>
      </div>
    </div>
    <p v-else-if="!loading" class="empty-text">No providers configured.</p>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';

interface Provider {
  id: string;
  type: string;
  display_name: string;
  config: string;
  enabled: boolean;
}

const providers = ref<Provider[]>([]);
const loading = ref(true);
const error = ref('');
const showForm = ref(false);
const form = ref({ id: '', type: 's3', display_name: '', config: '', enabled: true });

onMounted(async () => {
  await fetchProviders();
});

async function fetchProviders() {
  loading.value = true;
  try {
    const res = await fetch('/api/v1/admin/providers', {
      headers: { Authorization: `Bearer ${localStorage.getItem('token') || ''}` },
    });
    if (res.ok) {
      const data = await res.json();
      providers.value = data.data || [];
    }
  } catch {
    error.value = 'Failed to load providers';
  } finally {
    loading.value = false;
  }
}

async function createProvider() {
  error.value = '';
  try {
    await fetch('/api/v1/admin/providers', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${localStorage.getItem('token') || ''}`,
      },
      body: JSON.stringify(form.value),
    });
    showForm.value = false;
    form.value = { id: '', type: 's3', display_name: '', config: '', enabled: true };
    await fetchProviders();
  } catch {
    error.value = 'Failed to create provider';
  }
}

async function deleteProvider(id: string) {
  try {
    await fetch(`/api/v1/admin/providers/${id}`, {
      method: 'DELETE',
      headers: { Authorization: `Bearer ${localStorage.getItem('token') || ''}` },
    });
    await fetchProviders();
  } catch (e: unknown) {
    error.value = e instanceof Error ? e.message : 'Failed to delete provider';
  }
}

async function toggleEnabled(p: Provider) {
  try {
    await fetch(`/api/v1/admin/providers/${p.id}`, {
      method: 'PUT',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${localStorage.getItem('token') || ''}`,
      },
      body: JSON.stringify({ display_name: p.display_name, config: p.config, enabled: !p.enabled }),
    });
    await fetchProviders();
  } catch {
    error.value = 'Failed to update provider';
  }
}
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
.add-btn:hover {
  background: #5a52e0;
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
.delete-btn {
  padding: 4px 12px;
  border: 1px solid #3a1a1a;
  border-radius: 4px;
  background: transparent;
  color: #ff6b6b;
  cursor: pointer;
  font-size: 12px;
}
.delete-btn:hover {
  background: #2a1a1a;
}
.empty-text {
  color: #555577;
  font-size: 14px;
}
</style>
