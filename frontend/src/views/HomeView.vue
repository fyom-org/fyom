<template>
  <div>
    <h1>{{ $t('dashboard.welcome') }}{{ displayName }}</h1>
    <p class="info">{{ $t('dashboard.role') }}{{ store.user?.role ?? '—' }}</p>
    <p class="info">{{ $t('dashboard.userId') }}{{ store.user?.user_id ?? '—' }}</p>
    <button style="margin-top: 24px; background: #3f3f46" @click="handleLogout">
      {{ $t('profile.logout') }}
    </button>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { useUserStore } from '@/stores/user';

const router = useRouter();
const store = useUserStore();
const { t } = useI18n();
const error = ref('');

const displayName = computed(() => store.user?.username ?? t('common.loading'));

onMounted(async () => {
  try {
    await store.fetchMe();
  } catch {
    error.value = t('dashboard.sessionExpired');
    store.logout();
    router.push({ name: 'login' });
  }
});

function handleLogout() {
  store.logout();
  router.push({ name: 'login' });
}
</script>
