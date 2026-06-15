<template>
  <div class="player-fallback-notice" role="status" aria-live="polite">
    <span class="icon">⚠</span>
    <span class="message">{{ message }}</span>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  message: string;
}>();
</script>

<style scoped>
.player-fallback-notice {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 10px 20px;
  background: rgba(255, 193, 7, 0.95);
  color: #1a1a1a;
  font-size: 14px;
  font-weight: 500;
  backdrop-filter: blur(4px);
}

.icon {
  font-size: 16px;
}

.message {
  letter-spacing: 0.2px;
}
</style>
