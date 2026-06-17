<template>
  <RouterView />
  <Teleport to="body">
    <ForcePasswordChangeModal
      v-if="userStore.isAuthenticated && userStore.user?.password_change_required"
    />
  </Teleport>
</template>

<script setup lang="ts">
import { useUserStore } from '@/stores/user';
import ForcePasswordChangeModal from '@/components/ForcePasswordChangeModal.vue';

const userStore = useUserStore();
</script>
