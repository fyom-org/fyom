<template>
  <div class="profile-view">
    <h2 class="page-title">Profile</h2>

    <div v-if="loading" class="card">
      <p class="hint">Loading profile...</p>
    </div>

    <div v-else-if="loadError" class="card">
      <p class="error">{{ loadError }}</p>
    </div>

    <div v-else-if="currentUser" class="card">
      <div class="info-row">
        <span class="label">Username</span>
        <span>{{ currentUser.username }}</span>
      </div>
      <div class="info-row">
        <span class="label">Role</span>
        <span
          class="badge"
          :class="{ 'admin-badge': currentUser.role === 'admin' }"
          @click="navigateToAdmin"
        >
          {{ currentUser.role }}
        </span>
      </div>
      <div class="info-row" v-if="currentUser.password_change_required">
        <span class="label">Password Status</span>
        <span class="warning-text">Password change required</span>
      </div>
    </div>

    <div class="card">
      <h3>Preferences</h3>
      <div class="pref-row">
        <span class="pref-label">Default expand seasons</span>
        <label class="toggle">
          <input v-model="seasonsExpanded" type="checkbox" @change="saveSeasonsPref" />
          <span class="toggle-slider"></span>
        </label>
      </div>
      <p class="hint">When enabled, all season groups in TV show pages are expanded by default.</p>
    </div>

    <div class="card">
      <h3>Change Password</h3>

      <div class="field">
        <label>Current Password</label>
        <input
          v-model="oldPassword"
          type="password"
          autocomplete="current-password"
          :disabled="passwordSaving"
        />
      </div>

      <div class="field">
        <label>New Password</label>
        <input
          v-model="newPassword"
          type="password"
          autocomplete="new-password"
          :disabled="passwordSaving"
        />
      </div>

      <div class="field">
        <label>Confirm New Password</label>
        <input
          v-model="confirmPassword"
          type="password"
          autocomplete="new-password"
          :disabled="passwordSaving"
          @keyup.enter="submitPasswordChange"
        />
      </div>

      <p v-if="passwordMessage" class="msg">{{ passwordMessage }}</p>
      <p v-if="passwordError" class="error">{{ passwordError }}</p>

      <button class="btn" :disabled="passwordSaving" @click="submitPasswordChange">
        {{ passwordSaving ? 'Updating...' : 'Update Password' }}
      </button>
    </div>

    <div class="card">
      <button class="btn-logout" @click="handleLogout">Logout</button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { getMe, type User } from '@/api/auth';
import { useUserStore } from '@/stores/user';

const router = useRouter();
const userStore = useUserStore();

const loading = ref(false);
const loadError = ref('');

const profile = ref<User | null>(userStore.user);

const oldPassword = ref('');
const newPassword = ref('');
const confirmPassword = ref('');

const passwordSaving = ref(false);
const passwordMessage = ref('');
const passwordError = ref('');

const seasonsExpanded = ref(true);

const currentUser = computed<User | null>(() => profile.value ?? userStore.user);

function readSeasonsPreference(): void {
  try {
    seasonsExpanded.value = localStorage.getItem('seasons_collapsed_default') !== 'true';
  } catch {
    // ignore localStorage failures
  }
}

function saveSeasonsPref(): void {
  try {
    localStorage.setItem('seasons_collapsed_default', seasonsExpanded.value ? 'false' : 'true');
  } catch {
    // ignore localStorage failures
  }
}

async function loadProfile(): Promise<void> {
  loading.value = true;
  loadError.value = '';

  try {
    if (userStore.user) {
      profile.value = userStore.user;
      return;
    }

    const me = await getMe();
    profile.value = me;
  } catch (err: any) {
    const httpStatus = err?.response?.status;

    if (httpStatus === 401 || httpStatus === 403) {
      // Auth invalidation is handled centrally by store/router.
      return;
    }

    console.error('[profile] loadProfile failed:', err);
    loadError.value = 'Failed to load profile';
  } finally {
    loading.value = false;
  }
}

async function submitPasswordChange(): Promise<void> {
  passwordError.value = '';
  passwordMessage.value = '';

  if (!newPassword.value) {
    passwordError.value = 'New password is required';
    return;
  }

  if (newPassword.value !== confirmPassword.value) {
    passwordError.value = 'Passwords do not match';
    return;
  }

  passwordSaving.value = true;

  try {
    await userStore.updatePassword(oldPassword.value, newPassword.value);

    profile.value = userStore.user;
    passwordMessage.value = 'Password updated';

    oldPassword.value = '';
    newPassword.value = '';
    confirmPassword.value = '';
  } catch (err: any) {
    const httpStatus = err?.response?.status;

    if (httpStatus === 401 || httpStatus === 403) {
      // Auth invalidation is handled centrally by store/router.
      return;
    }

    passwordError.value =
      err?.response?.data?.message || err?.response?.data?.error || 'Failed to update password';

    console.error('[profile] submitPasswordChange failed:', err);
  } finally {
    passwordSaving.value = false;
  }
}

function handleLogout(): void {
  // Centralized route revalidation will redirect after auth state changes.
  userStore.logout();
}

function navigateToAdmin(): void {
  if (currentUser.value?.role === 'admin') {
    void router.push('/admin/libraries');
  }
}

onMounted(() => {
  readSeasonsPreference();
  void loadProfile();
});
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
  gap: 16px;
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
  cursor: default;
  user-select: none;
}

.admin-badge {
  cursor: pointer;
}

.admin-badge:hover {
  background: rgba(108, 99, 255, 0.2);
}

.warning-text {
  color: #ffb86b;
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

.field input:disabled {
  opacity: 0.7;
  cursor: not-allowed;
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

.btn:hover:not(:disabled) {
  background: #5a52e0;
}

.btn:disabled {
  opacity: 0.7;
  cursor: not-allowed;
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
