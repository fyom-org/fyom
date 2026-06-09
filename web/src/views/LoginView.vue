<template>
  <div>
    <h1>fyom — Login</h1>
    <input v-model="username" placeholder="username" autocomplete="username" />
    <input v-model="password" type="password" placeholder="password" autocomplete="current-password" />
    <button @click="handleLogin" :disabled="loading">Login</button>
    <p v-if="error" class="error">{{ error }}</p>
    <p class="info">Default: admin / admin123</p>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useUserStore } from '@/stores/user'

const store = useUserStore()

const username = ref('')
const password = ref('')
const loading = ref(false)
const error = ref('')

async function handleLogin() {
  error.value = ''
  loading.value = true
  try {
    await store.doLogin(username.value, password.value)
  } catch (err) {
    console.error('[fyom] login failed:', err)
    error.value = err instanceof Error ? err.message : 'Login failed'
  } finally {
    loading.value = false
  }
}
</script>
