<!--
  ToastContainer — renders active notifications from useNotifications().

  Mounted once in App.vue. Listens to the notifications array and renders
  stacked toast cards in the top-right corner. Auto-dismisses success/info
  toasts after 4s (handled in the composable); error/warning toasts stay
  until dismissed by the user.

  Accessibility:
  - role="region" aria-label for the container.
  - role="alert" for error/warning toasts (assertive).
  - role="status" for success/info toasts (polite).
  - Each toast has a close button with aria-label.
-->
<template>
  <Teleport to="body">
    <div class="toast-container" role="region" aria-label="Notifications">
      <TransitionGroup name="toast">
        <div
          v-for="toast in notifications"
          :key="toast.id"
          class="toast"
          :class="`toast-${toast.type}`"
          :role="toast.type === 'error' || toast.type === 'warning' ? 'alert' : 'status'"
        >
          <span class="toast-icon" aria-hidden="true">{{ iconFor(toast.type) }}</span>
          <span class="toast-message">{{ toast.message }}</span>
          <button
            type="button"
            class="toast-close"
            :aria-label="$t('common.close')"
            @click="dismissNotification(toast.id)"
          >
            ×
          </button>
        </div>
      </TransitionGroup>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { useNotifications, type NotificationType } from '@/composables/useNotifications';

const { notifications, dismissNotification } = useNotifications();

function iconFor(type: NotificationType): string {
  switch (type) {
    case 'success':
      return '✓';
    case 'error':
      return '✕';
    case 'warning':
      return '⚠';
    case 'info':
    default:
      return 'ℹ';
  }
}
</script>

<style scoped>
.toast-container {
  position: fixed;
  top: 16px;
  right: 16px;
  z-index: 10000;
  display: flex;
  flex-direction: column;
  gap: 10px;
  max-width: 380px;
  pointer-events: none;
}

.toast {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 12px 14px;
  border-radius: 10px;
  font-size: 13px;
  line-height: 1.45;
  box-shadow:
    0 8px 24px rgb(0 0 0 / 32%),
    0 2px 8px rgb(0 0 0 / 18%);
  backdrop-filter: blur(10px);
  pointer-events: auto;
  color: #f0f0ff;
}

.toast-icon {
  flex-shrink: 0;
  width: 18px;
  height: 18px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 13px;
  font-weight: 700;
  border-radius: 50%;
}

.toast-message {
  flex: 1;
  min-width: 0;
  word-break: break-word;
}

.toast-close {
  flex-shrink: 0;
  width: 22px;
  height: 22px;
  padding: 0;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: inherit;
  opacity: 0.6;
  font-size: 18px;
  line-height: 1;
  cursor: pointer;
  transition: opacity 0.15s ease, background-color 0.15s ease;
}

.toast-close:hover {
  opacity: 1;
  background: rgb(255 255 255 / 10%);
}

.toast-close:focus-visible {
  outline: 2px solid #6c63ff;
  outline-offset: 1px;
  opacity: 1;
}

.toast-success {
  background: rgb(20 36 26 / 95%);
  border: 1px solid #2e6b3c;
}

.toast-success .toast-icon {
  background: #2e6b3c;
  color: #c9f7d1;
}

.toast-error {
  background: rgb(42 26 26 / 95%);
  border: 1px solid #5a2a2a;
}

.toast-error .toast-icon {
  background: #5a2a2a;
  color: #ffb3b3;
}

.toast-warning {
  background: rgb(42 36 20 / 95%);
  border: 1px solid #5a4a2a;
}

.toast-warning .toast-icon {
  background: #5a4a2a;
  color: #ffd9b3;
}

.toast-info {
  background: rgb(26 26 46 / 95%);
  border: 1px solid #2a2a3e;
}

.toast-info .toast-icon {
  background: #2a2a3e;
  color: #b3b3ff;
}

.toast-enter-active,
.toast-leave-active {
  transition:
    opacity 0.25s ease,
    transform 0.25s ease;
}

.toast-enter-from {
  opacity: 0;
  transform: translateX(40px);
}

.toast-leave-to {
  opacity: 0;
  transform: translateX(40px);
}

@media (max-width: 520px) {
  .toast-container {
    top: 8px;
    right: 8px;
    left: 8px;
    max-width: none;
  }

  .toast {
    padding: 10px 12px;
    font-size: 12px;
  }
}

@media (prefers-reduced-motion: reduce) {
  .toast-enter-active,
  .toast-leave-active {
    transition: none;
  }
}
</style>
