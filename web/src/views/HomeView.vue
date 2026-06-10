<template>
  <div>
    <h1>Welcome, {{ displayName }}</h1>
    <p class="info">Role: {{ store.user?.role ?? '—' }}</p>
    <p class="info">User ID: {{ store.user?.user_id ?? '—' }}</p>
    <button style="margin-top: 24px; background: #3f3f46" @click="handleLogout">Logout</button>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { useRouter } from 'vue-router';
import { useUserStore } from '@/stores/user';

const router = useRouter();
const store = useUserStore();
const error = ref('');

const displayName = computed(() => store.user?.username ?? 'Loading...');

onMounted(async () => {
  try {
    await store.fetchMe();
  } catch {
    error.value = 'Session expired';
    store.logout();
    router.push({ name: 'login' });
  }
});

function handleLogout() {
  store.logout();
  router.push({ name: 'login' });
}
</script>
