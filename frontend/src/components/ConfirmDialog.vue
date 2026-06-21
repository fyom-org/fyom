<!--
  ConfirmDialog — renders the active confirmation request from useNotifications().

  Mounted once in App.vue. Watches `activeConfirm` and shows a modal dialog
  when a request is pending. The dialog is styled, accessible, and translated.

  Behavior:
  - Blocks interaction with the rest of the page (overlay).
  - Focuses the confirm button on mount for keyboard users.
  - Esc key cancels; Enter confirms.
  - Clicking the overlay cancels.
-->
<template>
  <Teleport to="body">
    <Transition name="confirm">
      <div
        v-if="activeConfirm"
        class="confirm-overlay"
        role="presentation"
        @click.self="handleCancel"
        @keydown.esc="handleCancel"
      >
        <div
          ref="dialogRef"
          class="confirm-dialog"
          role="alertdialog"
          aria-modal="true"
          :aria-label="activeConfirm.options.title || $t('common.confirm')"
        >
          <h3 v-if="activeConfirm.options.title" class="confirm-title">
            {{ activeConfirm.options.title }}
          </h3>

          <p class="confirm-message">{{ activeConfirm.message }}</p>

          <div class="confirm-actions">
            <button
              type="button"
              class="confirm-btn cancel-btn"
              @click="handleCancel"
            >
              {{ activeConfirm.options.cancelLabel || $t('common.cancel') }}
            </button>
            <button
              ref="confirmBtnRef"
              type="button"
              class="confirm-btn ok-btn"
              :class="{ 'danger': activeConfirm.options.variant === 'danger' }"
              @click="handleConfirm"
            >
              {{ activeConfirm.options.confirmLabel || $t('common.confirm') }}
            </button>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { nextTick, ref, watch } from 'vue';
import { useNotifications } from '@/composables/useNotifications';

const { activeConfirm, resolveConfirm } = useNotifications();

const dialogRef = ref<HTMLDivElement | null>(null);
const confirmBtnRef = ref<HTMLButtonElement | null>(null);

watch(activeConfirm, async (value) => {
  if (value) {
    await nextTick();
    // Focus the confirm button for keyboard users.
    // Using confirm (not cancel) because Enter should trigger the primary action.
    confirmBtnRef.value?.focus();
  }
});

function handleConfirm(): void {
  resolveConfirm(true);
}

function handleCancel(): void {
  resolveConfirm(false);
}
</script>

<style scoped>
.confirm-overlay {
  position: fixed;
  inset: 0;
  z-index: 10001;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 16px;
  background: rgb(0 0 0 / 55%);
  backdrop-filter: blur(4px);
}

.confirm-dialog {
  width: 100%;
  max-width: 420px;
  padding: 24px;
  background: rgb(26 26 46 / 98%);
  border: 1px solid rgb(255 255 255 / 10%);
  border-radius: 14px;
  box-shadow:
    0 24px 70px rgb(0 0 0 / 50%),
    inset 0 1px 0 rgb(255 255 255 / 4%);
  color: #e0e0e0;
}

.confirm-title {
  margin: 0 0 10px;
  font-size: 16px;
  font-weight: 700;
  color: #f3f3ff;
}

.confirm-message {
  margin: 0 0 20px;
  font-size: 14px;
  line-height: 1.5;
  color: #ccccee;
  word-break: break-word;
}

.confirm-actions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
}

.confirm-btn {
  min-height: 36px;
  padding: 8px 18px;
  border: 0;
  border-radius: 8px;
  font-size: 13px;
  font-weight: 600;
  font-family: inherit;
  cursor: pointer;
  transition: background-color 0.15s ease, opacity 0.15s ease;
}

.confirm-btn:focus-visible {
  outline: 2px solid #6c63ff;
  outline-offset: 2px;
}

.cancel-btn {
  background: rgb(255 255 255 / 6%);
  color: #ccccee;
}

.cancel-btn:hover {
  background: rgb(255 255 255 / 10%);
}

.ok-btn {
  background: #6c63ff;
  color: #fff;
}

.ok-btn:hover {
  background: #5a52e0;
}

.ok-btn.danger {
  background: #d33;
}

.ok-btn.danger:hover {
  background: #b22;
}

.confirm-enter-active,
.confirm-leave-active {
  transition: opacity 0.2s ease;
}

.confirm-enter-active .confirm-dialog,
.confirm-leave-active .confirm-dialog {
  transition: transform 0.2s ease, opacity 0.2s ease;
}

.confirm-enter-from,
.confirm-leave-to {
  opacity: 0;
}

.confirm-enter-from .confirm-dialog,
.confirm-leave-to .confirm-dialog {
  transform: scale(0.95);
  opacity: 0;
}

@media (prefers-reduced-motion: reduce) {
  .confirm-enter-active,
  .confirm-leave-active,
  .confirm-enter-active .confirm-dialog,
  .confirm-leave-active .confirm-dialog {
    transition: none;
  }
}
</style>
