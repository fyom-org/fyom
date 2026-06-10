<template>
  <div class="login-page">
    <h1>fyom — Login</h1>
    <input v-model="username" placeholder="username" autocomplete="username" />
    <input
      v-model="password"
      type="password"
      placeholder="password"
      autocomplete="current-password"
    />
    <button :disabled="loading" @click="handleLogin">Login</button>
    <p v-if="error" class="error">{{ error }}</p>
    <p class="info"><router-link to="/register">Create an account</router-link></p>
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
  max-width: 400px;
  margin: 120px auto 0;
  padding: 0 24px;
}

h1 {
  font-size: 1.5rem;
  margin-bottom: 24px;
  color: #fafafa;
}

input {
  width: 100%;
  padding: 10px 14px;
  margin-bottom: 12px;
  border: 1px solid #3f3f46;
  border-radius: 6px;
  background: #18181b;
  color: #e4e4e7;
  font-size: 1rem;
  outline: none;
}

input:focus {
  border-color: #6366f1;
}

button {
  width: 100%;
  padding: 10px 14px;
  border: none;
  border-radius: 6px;
  background: #6366f1;
  color: #fff;
  font-size: 1rem;
  cursor: pointer;
  margin-top: 4px;
}

button:hover:not(:disabled) {
  background: #4f46e5;
}

button:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.error {
  color: #f87171;
  font-size: 0.85rem;
  margin-top: 8px;
}

.info {
  color: #a1a1aa;
  font-size: 0.9rem;
  margin-top: 16px;
}

.info a {
  color: #6c63ff;
  text-decoration: none;
}
</style>
