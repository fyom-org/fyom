/**
 * UI notification composable — replaces native alert() / confirm().
 *
 * Why not use native alert()/confirm()?
 * - They block the main thread.
 * - They cannot be styled (jarring default browser appearance).
 * - They cannot be translated (the message string is shown verbatim).
 * - They don't work in Tauri desktop shell consistently.
 *
 * This composable provides:
 * - `notify(message, type)` — show a toast notification (non-blocking).
 * - `confirm(message)` — returns a Promise<boolean> for a styled dialog.
 *
 * The actual UI rendering is handled by a global <ToastContainer /> +
 * <ConfirmDialog /> mounted in App.vue. This keeps the composable testable
 * and decoupled from the DOM.
 *
 * Phase 2 milestone: all existing alert()/confirm() calls in admin views
 * are migrated to use this composable.
 */
import { ref, readonly } from 'vue';
import i18n from '@/plugins/i18n';

export type NotificationType = 'success' | 'error' | 'warning' | 'info';

export interface NotificationItem {
  id: number;
  message: string;
  type: NotificationType;
}

export interface ConfirmOptions {
  title?: string;
  confirmLabel?: string;
  cancelLabel?: string;
  variant?: 'danger' | 'default';
}

interface ConfirmRequest {
  message: string;
  options: ConfirmOptions;
  resolve: (value: boolean) => void;
}

// Module-level state (shared across all composable instances, like a Pinia
// store but lighter weight — no need for a full store for 2 pieces of state).
const notifications = ref<NotificationItem[]>([]);
const activeConfirm = ref<ConfirmRequest | null>(null);

let nextNotificationId = 1;

function notify(message: string, type: NotificationType = 'info'): number {
  const id = nextNotificationId++;
  notifications.value.push({ id, message, type });

  // Auto-dismiss success/info after 4s; errors stay until dismissed.
  if (type === 'success' || type === 'info') {
    window.setTimeout(() => dismissNotification(id), 4000);
  }

  return id;
}

function dismissNotification(id: number): void {
  const index = notifications.value.findIndex((n) => n.id === id);
  if (index >= 0) {
    notifications.value.splice(index, 1);
  }
}

function clearNotifications(): void {
  notifications.value = [];
}

/**
 * Show a confirmation dialog. Returns a Promise that resolves to true
 * (confirm) or false (cancel). The dialog is styled and translated.
 *
 * Usage:
 *   const ok = await confirmDialog(t('admin.libraries.confirmDelete', { name: lib.name }));
 *   if (!ok) return;
 */
function confirmDialog(message: string, options: ConfirmOptions = {}): Promise<boolean> {
  return new Promise<boolean>((resolve) => {
    // If a confirm is already active, reject the new one (queueing would
    // confuse users). The caller should handle the false return.
    if (activeConfirm.value !== null) {
      resolve(false);
      return;
    }

    activeConfirm.value = { message, options, resolve };
  });
}

function resolveConfirm(result: boolean): void {
  if (activeConfirm.value) {
    activeConfirm.value.resolve(result);
    activeConfirm.value = null;
  }
}

/**
 * Convenience helpers for common notification patterns.
 */
function notifySuccess(message: string): number {
  return notify(message, 'success');
}

function notifyError(message: string): number {
  return notify(message, 'error');
}

function notifyWarning(message: string): number {
  return notify(message, 'warning');
}

function notifyInfo(message: string): number {
  return notify(message, 'info');
}

/**
 * Notify an error from a caught API error, using the unified error helper.
 * Falls back to the translated generic error if the backend message is unsafe.
 */
function notifyApiError(error: unknown, fallbackKey: string = 'errors.generic'): number {
  return notifyError(i18n.global.t(fallbackKey) === fallbackKey
    ? require_safe_message(error, fallbackKey)
    : i18n.global.t(fallbackKey));
}

// Helper to avoid circular import with errors.ts at module load.
function require_safe_message(error: unknown, fallbackKey: string): string {
  // Inline extraction to avoid circular dependency.
  let message = '';
  if (typeof error === 'object' && error !== null) {
    const e = error as Record<string, unknown>;
    const response = e.response;
    if (typeof response === 'object' && response !== null) {
      const r = response as Record<string, unknown>;
      const data = r.data;
      if (typeof data === 'object' && data !== null) {
        const d = data as Record<string, unknown>;
        const msg = d.message || d.error || d.detail;
        if (typeof msg === 'string' && msg.trim()) {
          message = msg.trim();
        }
      } else if (typeof data === 'string' && data.trim()) {
        message = data.trim();
      }
    }
    if (!message) {
      const msg = e.message;
      if (typeof msg === 'string' && msg.trim()) {
        message = msg.trim();
      }
    }
  }

  const UNSAFE = ['sql', 'stack', 'trace', 'exception', 'internal server', 'jwt', 'token', 'undefined', 'null', 'request failed with status code'];
  const lower = message.toLowerCase();
  if (message && !UNSAFE.some((f) => lower.includes(f))) {
    return message;
  }
  return i18n.global.t(fallbackKey);
}

export function useNotifications() {
  return {
    notifications: readonly(notifications),
    activeConfirm: readonly(activeConfirm),
    notify,
    notifySuccess,
    notifyError,
    notifyWarning,
    notifyInfo,
    notifyApiError,
    dismissNotification,
    clearNotifications,
    confirmDialog,
    resolveConfirm,
  };
}
