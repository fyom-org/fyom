<template>
  <div class="register-page">
    <div class="setup-card">
      <div class="logo">fyom</div>
      <h1 class="title">Create Account</h1>
      <p class="subtitle">Register for a new account</p>

      <form @submit.prevent="submit">
        <div class="field">
          <label>Username</label>
          <input v-model="username" type="text" required autocomplete="username" />
        </div>
        <div class="field">
          <label>Password</label>
          <input v-model="password" type="password" required autocomplete="new-password" />
        </div>

        <p class="error" v-if="error">{{ error }}</p>
        <button type="submit" class="submit-btn" :disabled="loading">
          {{ loading ? 'Registering...' : 'Register' }}
        </button>
      </form>

      <p class="bottom-link">
        Already have an account? <router-link to="/login">Login</router-link>
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import request from '@/api/request'

const router = useRouter()
const username = ref('')
const password = ref('')
const error = ref('')
const loading = ref(false)

async function submit() {
  error.value = ''
  loading.value = true
  try {
    await request.post('/auth/register', { username: username.value, password: password.value })
    router.push('/login')
  } catch (e: unknown) {
    const err = e as { response?: { data?: { message?: string } } }
    error.value = err.response?.data?.message || 'Registration failed'
  } finally { loading.value = false }
}
</script>

<style scoped>
.register-page {
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
