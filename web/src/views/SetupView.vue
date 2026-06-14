<template>
  <div class="setup-view">
    <div class="setup-card">
      <div class="logo">fyom</div>
      <h1 class="title">Welcome to fyom</h1>
      <p class="subtitle">Create your admin account to get started</p>

      <form @submit.prevent="submit">
        <div class="field">
          <label>Username</label>
          <input v-model="username" type="text" required autocomplete="username" />
        </div>
        <div class="field">
          <label>Password</label>
          <input v-model="password" type="password" required autocomplete="new-password" />
        </div>
        <div class="field">
          <label>Confirm Password</label>
          <input v-model="confirmPassword" type="password" required autocomplete="new-password" />
        </div>

        <div class="toggle-row">
          <label class="toggle-label">
            <input v-model="allowRegistration" type="checkbox" />
            <span>Allow public registration</span>
          </label>
        </div>

        <p v-if="error" class="error">{{ error }}</p>
        <button type="submit" class="submit-btn" :disabled="loading">
          {{ loading ? 'Setting up...' : 'Complete Setup' }}
        </button>
      </form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import { useRouter } from 'vue-router';
import request from '@/api/request';

const router = useRouter();
const username = ref('');
const password = ref('');
const confirmPassword = ref('');
const allowRegistration = ref(false);
const error = ref('');
const loading = ref(false);

async function submit() {
  error.value = '';
  if (!username.value || !password.value) {
    error.value = 'Fill all fields';
    return;
  }
  if (password.value !== confirmPassword.value) {
    error.value = 'Passwords do not match';
    return;
  }
  loading.value = true;
  try {
    // Step 1: Initialize system (create admin account)
    await request.post('/system/initialize', {
      username: username.value,
      password: password.value,
      allow_registration: allowRegistration.value,
    });

    // Step 2: Login to get token
    const loginRes: any = await request.post('/auth/login', {
      username: username.value,
      password: password.value,
    });
    localStorage.setItem('token', loginRes.data.access_token);

    // Step 3: Navigate to dashboard
    router.push('/');
  } catch (e: unknown) {
    const err = e as { response?: { data?: { message?: string } } };
    error.value = err.response?.data?.message || 'Setup failed';
  } finally {
    loading.value = false;
  }
}
</script>

<style scoped>
.setup-view {
  min-height: 100vh;
  background: #0f0f1a;
  display: flex;
  align-items: center;
  justify-content: center;
}

.setup-card {
  background: #1a1a2e;
  padding: 40px;
  border-radius: 12px;
  width: 100%;
  max-width: 420px;
  box-shadow: 0 16px 48px rgba(0, 0, 0, 0.4);
}

.logo {
  font-size: 28px;
  font-weight: 800;
  color: #6c63ff;
  text-align: center;
  margin-bottom: 8px;
}

.title {
  font-size: 22px;
  color: #e0e0e0;
  text-align: center;
  margin: 0 0 4px;
}

.subtitle {
  font-size: 14px;
  color: #666688;
  text-align: center;
  margin: 0 0 32px;
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
  outline: none;
  box-sizing: border-box;
}

.field input:focus {
  border-color: #6c63ff;
}

.field + .field {
  margin-top: 16px;
}

.toggle-row {
  margin-top: 20px;
}

.toggle-label {
  display: flex;
  align-items: center;
  gap: 8px;
  color: #aaaacc;
  font-size: 14px;
  cursor: pointer;
}

.toggle-label input[type='checkbox'] {
  accent-color: #6c63ff;
}

.library-fields {
  margin-top: 12px;
  padding-left: 28px;
}

.hint {
  color: #444466;
  font-size: 12px;
  margin-top: 6px;
  line-height: 1.5;
}

.error {
  color: #ff6b6b;
  font-size: 13px;
  margin-top: 12px;
}

.submit-btn {
  width: 100%;
  padding: 12px;
  background: #6c63ff;
  color: #fff;
  border: none;
  border-radius: 8px;
  font-size: 15px;
  font-weight: 600;
  cursor: pointer;
  margin-top: 20px;
}

.submit-btn:hover:not(:disabled) {
  background: #5a52e0;
}

.submit-btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}
</style>
