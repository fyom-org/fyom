<template>
  <div class="app-shell">
    <RouterView />

    <Teleport to="body">
      <ForcePasswordChangeModal v-if="showForcePasswordChangeModal" />
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useRoute } from 'vue-router';
import { useUserStore } from '@/stores/user';
import ForcePasswordChangeModal from '@/components/ForcePasswordChangeModal.vue';

const route = useRoute();
const userStore = useUserStore();

const isGuestRoute = computed(() => {
  return route.path === '/login' || route.path === '/register';
});

const showForcePasswordChangeModal = computed(() => {
  /**
   * App.vue must not decide authentication truth.
   * It only renders the modal when the user store already has a verified
   * authenticated session and the current route is not a guest route.
   */
  return (
    userStore.isAuthenticated &&
    !isGuestRoute.value &&
    userStore.user?.password_change_required === true
  );
});
</script>

<style scoped>
.app-shell {
  min-height: 100vh;
}
</style>
