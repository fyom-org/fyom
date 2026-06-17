<template>
  <div class="app-shell">
    <RouterView />

    <Teleport to="body">
      <ForcePasswordChangeModal v-if="showForcePasswordChangeModal" />
    </Teleport>

    <ToastContainer />
    <ConfirmDialog />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useRoute } from 'vue-router';
import { useUserStore } from '@/stores/user';
import ForcePasswordChangeModal from '@/components/ForcePasswordChangeModal.vue';
import ToastContainer from '@/components/ToastContainer.vue';
import ConfirmDialog from '@/components/ConfirmDialog.vue';

const route = useRoute();
const userStore = useUserStore();

const isGuestRoute = computed(() => {
  return route.path === '/login' || route.path === '/register';
});

const showForcePasswordChangeModal = computed(() => {
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
