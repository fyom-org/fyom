<template>
  <div class="login-page">
    <div class="setup-card">
      <div class="logo">fyom</div>
      <h1 class="title">Welcome back</h1>
      <p class="subtitle">Sign in to your account</p>

      <form @submit.prevent="handleLogin">
        <div class="field">
          <label>Username</label>
          <input v-model="username" type="text" required autocomplete="username" />
        </div>
        <div class="field">
          <label>Password</label>
          <input v-model="password" type="password" required autocomplete="current-password" />
        </div>

        <p v-if="error" class="error">{{ error }}</p>
        <button type="submit" class="submit-btn" :disabled="loading">
          {{ loading ? 'Signing in...' : 'Sign In' }}
        </button>
      </form>

      <p class="bottom-link">
        No account? <router-link to="/register">Create one</router-link>
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { useRouter } from 'vue-router';
import { useUserStore } from '@/stores/user';
import request from '@/api/request';

const router = useRouter();
const store = useUserStore();

const username = ref('');
const password = ref('');
const loading = ref(false);
const error = ref('');

onMounted(async () => {
  try {
    const res = await request.get('/system/status');
    if (!res.data.initialized) {
      router.push('/setup');
    }
  } catch {
    // system not initialized — router.push handled above
  }
});

async function handleLogin() {
  error.value = '';
  loading.value = true;
  try {
    await store.doLogin(username.value, password.value);
    // Role is validated server-side; no localStorage storage needed
  } catch (err) {
    console.error('[fyom] login failed:', err);
    error.value = err instanceof Error ? err.message : 'Login failed';
  } finally {
    loading.value = false;
  }
}
</script>

<style scoped>
.login-page {
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

.bottom-link {
  text-align: center;
  margin-top: 20px;
  color: #666688;
  font-size: 14px;
}

.bottom-link a {
  color: #6c63ff;
  text-decoration: none;
}
</style>