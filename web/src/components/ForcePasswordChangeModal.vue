<template>
  <div class="modal-overlay" @click.self="preventClose" @keydown.esc.prevent="preventClose">
    <section
      class="modal"
      role="dialog"
      aria-modal="true"
      aria-labelledby="modal-title"
      aria-describedby="modal-description"
    >
      <header class="modal-header">
        <h2 id="modal-title">Password Change Required</h2>
      </header>

      <div class="modal-body">
        <p id="modal-description" class="modal-message">
          You must change your password before continuing. This is required because your account was
          created with a temporary password.
        </p>

        <form class="password-form" novalidate @submit.prevent="handleSubmit">
          <div class="field">
            <label for="new-password">New Password</label>

            <div class="password-wrap">
              <input
                id="new-password"
                ref="newPasswordInput"
                v-model="newPassword"
                :type="showNewPassword ? 'text' : 'password'"
                required
                autocomplete="new-password"
                class="input"
                :class="{ invalid: Boolean(fieldErrors.newPassword) }"
                :aria-invalid="Boolean(fieldErrors.newPassword)"
                :aria-describedby="fieldErrors.newPassword ? 'new-password-error' : undefined"
                :disabled="loading"
                @input="clearFieldError('newPassword')"
              />

              <button
                type="button"
                class="password-toggle"
                :disabled="loading || newPassword.length === 0"
                :aria-label="showNewPassword ? 'Hide new password' : 'Show new password'"
                @click="showNewPassword = !showNewPassword"
              >
                {{ showNewPassword ? 'Hide' : 'Show' }}
              </button>
            </div>

            <p v-if="fieldErrors.newPassword" id="new-password-error" class="field-error">
              {{ fieldErrors.newPassword }}
            </p>
          </div>

          <div class="field">
            <label for="confirm-password">Confirm New Password</label>

            <div class="password-wrap">
              <input
                id="confirm-password"
                v-model="confirmPassword"
                :type="showConfirmPassword ? 'text' : 'password'"
                required
                autocomplete="new-password"
                class="input"
                :class="{ invalid: Boolean(fieldErrors.confirmPassword) }"
                :aria-invalid="Boolean(fieldErrors.confirmPassword)"
                :aria-describedby="
                  fieldErrors.confirmPassword ? 'confirm-password-error' : undefined
                "
                :disabled="loading"
                @input="clearFieldError('confirmPassword')"
              />

              <button
                type="button"
                class="password-toggle"
                :disabled="loading || confirmPassword.length === 0"
                :aria-label="
                  showConfirmPassword ? 'Hide password confirmation' : 'Show password confirmation'
                "
                @click="showConfirmPassword = !showConfirmPassword"
              >
                {{ showConfirmPassword ? 'Hide' : 'Show' }}
              </button>
            </div>

            <p v-if="fieldErrors.confirmPassword" id="confirm-password-error" class="field-error">
              {{ fieldErrors.confirmPassword }}
            </p>
          </div>

          <p v-if="error" class="error" role="alert">
            {{ error }}
          </p>

          <button type="submit" class="submit-btn" :disabled="loading || !canSubmit">
            <span v-if="loading" class="spinner" aria-hidden="true"></span>
            <span>{{ loading ? 'Changing...' : 'Change Password' }}</span>
          </button>
        </form>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, reactive, ref } from 'vue';
import { useUserStore } from '@/stores/user';

type PasswordField = 'newPassword' | 'confirmPassword';

const MIN_PASSWORD_LENGTH = 8;

const store = useUserStore();

const newPasswordInput = ref<HTMLInputElement | null>(null);

const newPassword = ref('');
const confirmPassword = ref('');
const loading = ref(false);
const error = ref('');
const showNewPassword = ref(false);
const showConfirmPassword = ref(false);

const fieldErrors = reactive<Record<PasswordField, string>>({
  newPassword: '',
  confirmPassword: '',
});

const canSubmit = computed(() => {
  return newPassword.value.length > 0 && confirmPassword.value.length > 0;
});

onMounted(async () => {
  await nextTick();
  newPasswordInput.value?.focus();
});

function preventClose(): void {
  /**
   * This modal is intentionally non-dismissible.
   * The backend requires password change before the user can continue.
   */
}

async function handleSubmit(): Promise<void> {
  if (loading.value) return;

  error.value = '';
  clearAllFieldErrors();

  if (!validateForm()) {
    focusFirstInvalidField();
    return;
  }

  loading.value = true;

  try {
    await store.updatePassword('', newPassword.value);

    resetForm();
    /**
     * App.vue controls this modal via userStore.requiresPasswordChange.
     * When updatePassword refreshes the user and clears password_change_required,
     * the modal will unmount automatically.
     */
  } catch (unknownError) {
    error.value = getPasswordChangeErrorMessage(unknownError);

    await nextTick();
    newPasswordInput.value?.focus();
  } finally {
    loading.value = false;
  }
}

function validateForm(): boolean {
  let valid = true;

  if (!newPassword.value) {
    fieldErrors.newPassword = 'New password is required.';
    valid = false;
  } else if (newPassword.value.length < MIN_PASSWORD_LENGTH) {
    fieldErrors.newPassword = `Password must be at least ${MIN_PASSWORD_LENGTH} characters.`;
    valid = false;
  }

  if (!confirmPassword.value) {
    fieldErrors.confirmPassword = 'Please confirm your new password.';
    valid = false;
  } else if (newPassword.value !== confirmPassword.value) {
    fieldErrors.confirmPassword = 'Passwords do not match.';
    valid = false;
  }

  return valid;
}

function clearFieldError(field: PasswordField): void {
  fieldErrors[field] = '';

  if (error.value) {
    error.value = '';
  }
}

function clearAllFieldErrors(): void {
  fieldErrors.newPassword = '';
  fieldErrors.confirmPassword = '';
}

function focusFirstInvalidField(): void {
  if (fieldErrors.newPassword) {
    newPasswordInput.value?.focus();
  }
}

function resetForm(): void {
  newPassword.value = '';
  confirmPassword.value = '';
  showNewPassword.value = false;
  showConfirmPassword.value = false;
  error.value = '';
  clearAllFieldErrors();
}

function getPasswordChangeErrorMessage(unknownError: unknown): string {
  const status = getHttpStatus(unknownError);

  if (status === 401 || status === 403) {
    void store.verifySession();

    return 'Unable to verify your session. Please sign in again if the problem continues.';
  }

  const message = extractErrorMessage(unknownError);

  if (message && isSafeUserFacingMessage(message)) {
    return message;
  }

  return 'Password change failed. Please try again.';
}

function extractErrorMessage(unknownError: unknown): string {
  if (isRecord(unknownError)) {
    const response = unknownError.response;

    if (isRecord(response)) {
      const data = response.data;

      if (isRecord(data)) {
        const message = data.message || data.error || data.detail;

        if (typeof message === 'string' && message.trim()) {
          return message.trim();
        }
      }

      if (typeof data === 'string' && data.trim()) {
        return data.trim();
      }
    }

    const message = unknownError.message;

    if (typeof message === 'string' && message.trim()) {
      return message.trim();
    }
  }

  return '';
}

function getHttpStatus(unknownError: unknown): number | undefined {
  if (!isRecord(unknownError)) return undefined;

  const response = unknownError.response;

  if (!isRecord(response)) return undefined;

  const status = response.status;

  return typeof status === 'number' ? status : undefined;
}

function isSafeUserFacingMessage(message: string): boolean {
  const normalized = message.toLowerCase();

  const unsafeFragments = [
    'sql',
    'stack',
    'trace',
    'exception',
    'internal server',
    'jwt',
    'token',
    'undefined',
    'null',
    'request failed with status code',
  ];

  return !unsafeFragments.some((fragment) => normalized.includes(fragment));
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null;
}
</script>

<style scoped>
.modal-overlay {
  position: fixed;
  inset: 0;
  z-index: 1000;
  box-sizing: border-box;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 20px;
  background: rgb(0 0 0 / 72%);
  animation: fade-in 0.18s ease-out;
}

.modal {
  width: 100%;
  max-width: 430px;
  box-sizing: border-box;
  overflow: hidden;
  background: #1a1a2e;
  border: 1px solid rgb(255 255 255 / 6%);
  border-radius: 14px;
  box-shadow: 0 18px 52px rgb(0 0 0 / 52%);
  animation: slide-up 0.22s ease-out;
}

.modal-header {
  padding: 24px 24px 0;
}

.modal-header h2 {
  margin: 0;
  color: #f0f0ff;
  font-size: 21px;
  font-weight: 800;
  line-height: 1.25;
}

.modal-body {
  padding: 18px 24px 24px;
}

.modal-message {
  margin: 0 0 22px;
  color: #aaaacc;
  font-size: 14px;
  line-height: 1.55;
}

.password-form {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.field label {
  display: block;
  margin-bottom: 7px;
  color: #aaaacc;
  font-size: 13px;
  font-weight: 600;
}

.password-wrap {
  position: relative;
}

.input {
  width: 100%;
  min-height: 44px;
  box-sizing: border-box;
  padding: 10px 72px 10px 12px;
  color: #f0f0ff;
  background: #0f0f1a;
  border: 1px solid #2a2a3e;
  border-radius: 8px;
  outline: none;
  font-size: 14px;
  transition:
    border-color 0.15s ease,
    box-shadow 0.15s ease,
    opacity 0.15s ease;
}

.input:hover:not(:disabled) {
  border-color: #3a3a5e;
}

.input:focus {
  border-color: #6c63ff;
  box-shadow: 0 0 0 3px rgb(108 99 255 / 16%);
}

.input.invalid {
  border-color: #ff6b6b;
}

.input.invalid:focus {
  box-shadow: 0 0 0 3px rgb(255 107 107 / 14%);
}

.input:disabled {
  cursor: not-allowed;
  opacity: 0.7;
}

.password-toggle {
  position: absolute;
  top: 50%;
  right: 8px;
  min-width: 52px;
  padding: 5px 8px;
  color: #aaaacc;
  background: transparent;
  border: 0;
  border-radius: 6px;
  cursor: pointer;
  font-size: 12px;
  font-weight: 700;
  transform: translateY(-50%);
  transition:
    color 0.15s ease,
    background-color 0.15s ease,
    opacity 0.15s ease;
}

.password-toggle:hover:not(:disabled) {
  color: #fff;
  background: rgb(255 255 255 / 6%);
}

.password-toggle:disabled {
  cursor: not-allowed;
  opacity: 0.45;
}

.field-error {
  margin: 6px 0 0;
  color: #ff8f8f;
  font-size: 12px;
  line-height: 1.4;
}

.error {
  margin: 0;
  color: #ff8f8f;
  font-size: 13px;
  line-height: 1.45;
}

.submit-btn {
  width: 100%;
  min-height: 46px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  margin-top: 4px;
  padding: 12px 16px;
  color: #fff;
  background: #6c63ff;
  border: 0;
  border-radius: 10px;
  cursor: pointer;
  font-size: 15px;
  font-weight: 800;
  transition:
    background-color 0.15s ease,
    transform 0.15s ease,
    opacity 0.15s ease;
}

.submit-btn:hover:not(:disabled) {
  background: #5a52e0;
}

.submit-btn:active:not(:disabled) {
  transform: translateY(1px);
}

.submit-btn:disabled {
  cursor: not-allowed;
  opacity: 0.62;
}

.spinner {
  width: 15px;
  height: 15px;
  box-sizing: border-box;
  border: 2px solid rgb(255 255 255 / 35%);
  border-top-color: #fff;
  border-radius: 999px;
  animation: spin 0.75s linear infinite;
}

@keyframes fade-in {
  from {
    opacity: 0;
  }

  to {
    opacity: 1;
  }
}

@keyframes slide-up {
  from {
    opacity: 0;
    transform: translateY(18px);
  }

  to {
    opacity: 1;
    transform: translateY(0);
  }
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

@media (max-width: 520px) {
  .modal-overlay {
    align-items: stretch;
    padding: 16px;
  }

  .modal {
    margin: auto 0;
  }

  .modal-header {
    padding: 22px 20px 0;
  }

  .modal-body {
    padding: 16px 20px 22px;
  }
}

@media (prefers-reduced-motion: reduce) {
  .modal-overlay,
  .modal,
  .input,
  .password-toggle,
  .submit-btn,
  .spinner {
    animation: none;
    transition: none;
  }
}
</style>
