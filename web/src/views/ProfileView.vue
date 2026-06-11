<template>
  <div class="profile-view">
    <h2 class="page-title">Profile</h2>
    <div v-if="user" class="card">
      <div class="info-row">
        <span class="label">Username</span><span>{{ user.username }}</span>
      </div>
      <div class="info-row">
        <span class="label">Role</span><span class="badge">{{ user.role }}</span>
      </div>
    </div>

    <div class="card">
      <h3>Preferences</h3>
      <div class="pref-row">
        <span class="pref-label">Default expand seasons</span>
        <label class="toggle">
          <input type="checkbox" v-model="seasonsExpanded" @change="saveSeasonsPref" />
          <span class="toggle-slider"></span>
        </label>
      </div>
      <p class="hint">When enabled, all season groups in TV show pages are expanded by default.</p>
    </div>

    <div class="card">
      <h3>Change Password</h3>
      <div class="field">
        <label>Current Password</label>
        <input v-model="oldPassword" type="password" />
      </div>
      <div class="field">
        <label>New Password</label>
        <input v-model="newPassword" type="password" />
      </div>
      <p v-if="msg" class="msg">{{ msg }}</p>
      <p v-if="error" class="error">{{ error }}</p>
      <button class="btn" @click="changePassword">Update Password</button>
    </div>
    <div class="card">
      <button class="btn-logout" @click="handleLogout">Logout</button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { useRouter } from 'vue-router';
import request from '@/api/request';
import { useUserStore } from '@/stores/user';

const router = useRouter();
const store = useUserStore();
const user = ref<{ username: string; role: string } | null>(null);
const oldPassword = ref('');
const newPassword = ref('');
const msg = ref('');
const error = ref('');
const seasonsExpanded = ref(true);

onMounted(async () => {
  const res = await request.get('/auth/me');
  user.value = res.data;
  try {
    seasonsExpanded.value = localStorage.getItem('seasons_collapsed_default') !== 'true';
  } catch {
    // ignore
  }
});

function saveSeasonsPref() {
  try {
    localStorage.setItem('seasons_collapsed_default', seasonsExpanded.value ? 'false' : 'true');
  } catch {
    // ignore
  }
}

async function changePassword() {
  error.value = '';
  msg.value = '';
  try {
    await request.put('/auth/me/password', {
      old_password: oldPassword.value,
      new_password: newPassword.value,
    });
    msg.value = 'Password updated';
    oldPassword.value = '';
    newPassword.value = '';
  } catch (e: unknown) {
    const err = e as { response?: { data?: { message?: string } } };
    error.value = err.response?.data?.message || 'Failed';
  }
}

function handleLogout() {
  store.logout();
  router.push({ name: 'login' });
}
</script>

<style scoped>
.page-title {
  font-size: 22px;
  color: #e0e0e0;
  margin-bottom: 20px;
}

.card {
  background: #1a1a2e;
  padding: 24px;
  border-radius: 8px;
  margin-bottom: 20px;
}

.info-row {
  display: flex;
  justify-content: space-between;
  padding: 8px 0;
  color: #aaaacc;
  font-size: 14px;
}

.label {
  color: #8888aa;
}

.badge {
  background: #2a2a3e;
  padding: 2px 8px;
  border-radius: 4px;
  text-transform: capitalize;
  color: #6c63ff;
}

h3 {
  margin: 0 0 16px;
  color: #d0d0d0;
  font-size: 16px;
}

.pref-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 4px 0;
}

.pref-label {
  color: #aaaacc;
  font-size: 14px;
}

.hint {
  color: #555577;
  font-size: 12px;
  margin: 8px 0 0;
}

/* Toggle switch */
.toggle {
  position: relative;
  display: inline-block;
  width: 44px;
  height: 24px;
  cursor: pointer;
}

.toggle input {
  display: none;
}

.toggle-slider {
  position: absolute;
  inset: 0;
  background: #2a2a3e;
  border-radius: 12px;
  transition: background 0.2s;
}

.toggle-slider::before {
  content: '';
  position: absolute;
  width: 18px;
  height: 18px;
  left: 3px;
  bottom: 3px;
  background: #666688;
  border-radius: 50%;
  transition: all 0.2s;
}

.toggle input:checked + .toggle-slider {
  background: #6c63ff;
}

.toggle input:checked + .toggle-slider::before {
  transform: translateX(20px);
  background: #fff;
}

.field label {
  display: block;
  color: #8888aa;
  font-size: 13px;
  margin-bottom: 6px;
}

.field input {
  width: 100%;
  padding: 10px 12px;
  background: #0f0f1a;
  border: 1px solid #2a2a3e;
  border-radius: 6px;
  color: #e0e0e0;
  font-size: 14px;
  box-sizing: border-box;
}

.field + .field {
  margin-top: 12px;
}

.btn {
  margin-top: 16px;
  background: #6c63ff;
  color: #fff;
  border: none;
  padding: 10px 24px;
  border-radius: 6px;
  cursor: pointer;
  font-size: 14px;
  width: 100%;
}

.btn:hover {
  background: #5a52e0;
}

.btn-logout {
  background: none;
  color: #ff6b6b;
  border: none;
  padding: 10px 24px;
  border-radius: 6px;
  cursor: pointer;
  font-size: 14px;
  width: 100%;
  text-align: left;
}

.btn-logout:hover {
  background: rgba(255, 107, 107, 0.1);
}

.msg {
  color: #4caf50;
  font-size: 13px;
  margin-top: 12px;
}

.error {
  color: #ff6b6b;
  font-size: 13px;
  margin-top: 12px;
}
</style>
