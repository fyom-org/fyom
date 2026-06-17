<template>
  <main class="profile-view">
    <header class="page-header">
      <div>
        <h2 class="page-title">Profile</h2>
        <p class="page-subtitle">Manage your account, preferences, and password.</p>
      </div>
    </header>

    <section v-if="loading" class="card">
      <p class="hint">Loading profile...</p>
    </section>

    <section v-else-if="loadError" class="card error-card" role="alert">
      <div>
        <p class="error-title">Unable to load profile</p>
        <p class="error">{{ loadError }}</p>
      </div>

      <button type="button" class="secondary-btn" @click="loadProfile">Retry</button>
    </section>

    <section v-else-if="currentUser" class="card">
      <h3>Account</h3>

      <div class="info-row">
        <span class="label">Username</span>
        <span class="value">{{ currentUser.username }}</span>
      </div>

      <div class="info-row">
        <span class="label">Role</span>

        <button
          v-if="canAccessAdmin"
          type="button"
          class="badge admin-badge"
          @click="navigateToAdmin"
        >
          {{ currentUser.role }}
        </button>

        <span v-else class="badge">
          {{ currentUser.role }}
        </span>
      </div>

      <div v-if="currentUser.password_change_required" class="info-row">
        <span class="label">Password Status</span>
        <span class="warning-text">Password change required</span>
      </div>
    </section>

    <section class="card">
      <h3>Preferences</h3>

      <div class="pref-row">
        <div>
          <span class="pref-label">Default expand seasons</span>
          <p class="hint">
            When enabled, all season groups in TV show pages are expanded by default.
          </p>
        </div>

        <label class="toggle">
          <input
            v-model="seasonsExpanded"
            type="checkbox"
            :disabled="prefSaving"
            @change="saveSeasonsPref"
          />
          <span class="toggle-slider"></span>
        </label>
      </div>

      <p v-if="prefMessage" class="msg">
        {{ prefMessage }}
      </p>
    </section>

    <section class="card">
      <h3>Change Password</h3>

      <p v-if="currentUser?.password_change_required" class="warning-box">
        Your account requires a password change before continuing.
      </p>

      <form class="password-form" novalidate @submit.prevent="submitPasswordChange">
        <div class="field">
          <label for="old-password">Current Password</label>

          <div class="password-wrap">
            <input
              id="old-password"
              v-model="oldPassword"
              :type="showOldPassword ? 'text' : 'password'"
              autocomplete="current-password"
              class="input"
              :class="{ invalid: Boolean(fieldErrors.oldPassword) }"
              :aria-invalid="Boolean(fieldErrors.oldPassword)"
              :aria-describedby="fieldErrors.oldPassword ? 'old-password-error' : undefined"
              :disabled="passwordSaving"
              @input="clearPasswordFieldError('oldPassword')"
            />

            <button
              type="button"
              class="password-toggle"
              :disabled="passwordSaving || oldPassword.length === 0"
              :aria-label="showOldPassword ? 'Hide current password' : 'Show current password'"
              @click="showOldPassword = !showOldPassword"
            >
              {{ showOldPassword ? 'Hide' : 'Show' }}
            </button>
          </div>

          <p v-if="fieldErrors.oldPassword" id="old-password-error" class="field-error">
            {{ fieldErrors.oldPassword }}
          </p>
        </div>

        <div class="field">
          <label for="new-password">New Password</label>

          <div class="password-wrap">
            <input
              id="new-password"
              v-model="newPassword"
              :type="showNewPassword ? 'text' : 'password'"
              autocomplete="new-password"
              class="input"
              :class="{ invalid: Boolean(fieldErrors.newPassword) }"
              :aria-invalid="Boolean(fieldErrors.newPassword)"
              :aria-describedby="fieldErrors.newPassword ? 'new-password-error' : undefined"
              :disabled="passwordSaving"
              @input="clearPasswordFieldError('newPassword')"
            />

            <button
              type="button"
              class="password-toggle"
              :disabled="passwordSaving || newPassword.length === 0"
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
              autocomplete="new-password"
              class="input"
              :class="{ invalid: Boolean(fieldErrors.confirmPassword) }"
              :aria-invalid="Boolean(fieldErrors.confirmPassword)"
              :aria-describedby="fieldErrors.confirmPassword ? 'confirm-password-error' : undefined"
              :disabled="passwordSaving"
              @input="clearPasswordFieldError('confirmPassword')"
            />

            <button
              type="button"
              class="password-toggle"
              :disabled="passwordSaving || confirmPassword.length === 0"
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

        <p v-if="passwordMessage" class="msg" role="status">
          {{ passwordMessage }}
        </p>

        <p v-if="passwordError" class="error" role="alert">
          {{ passwordError }}
        </p>

        <button type="submit" class="primary-btn" :disabled="passwordSaving || !canSubmitPassword">
          {{ passwordSaving ? 'Updating...' : 'Update Password' }}
        </button>
      </form>
    </section>

    <section class="card danger-card">
      <div>
        <h3>Session</h3>
        <p class="hint">Sign out of this device.</p>
      </div>

      <button type="button" class="btn-logout" :disabled="loggingOut" @click="handleLogout">
        {{ loggingOut ? 'Signing out...' : 'Logout' }}
      </button>
    </section>
  </main>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { useRouter } from 'vue-router';
import type { User } from '@/api/auth';
import { useUserStore } from '@/stores/user';

type PasswordField = 'oldPassword' | 'newPassword' | 'confirmPassword';

const SEASONS_COLLAPSED_KEY = 'seasons_collapsed_default';
const MIN_PASSWORD_LENGTH = 6;

const router = useRouter();
const userStore = useUserStore();

const loading = ref(false);
const loadError = ref('');

const profile = ref<User | null>(userStore.user);

const oldPassword = ref('');
const newPassword = ref('');
const confirmPassword = ref('');

const passwordSaving = ref(false);
const passwordMessage = ref('');
const passwordError = ref('');

const showOldPassword = ref(false);
const showNewPassword = ref(false);
const showConfirmPassword = ref(false);

const prefSaving = ref(false);
const prefMessage = ref('');
const seasonsExpanded = ref(true);

const loggingOut = ref(false);

const fieldErrors = reactive<Record<PasswordField, string>>({
  oldPassword: '',
  newPassword: '',
  confirmPassword: '',
});

const currentUser = computed<User | null>(() => {
  return profile.value ?? userStore.user;
});

const canAccessAdmin = computed(() => {
  const role = currentUser.value?.role?.toLowerCase();

  return role === 'admin' || role === 'owner';
});

const canSubmitPassword = computed(() => {
  return newPassword.value.length > 0 && confirmPassword.value.length > 0;
});

onMounted(() => {
  readSeasonsPreference();
  void loadProfile();
});

function readSeasonsPreference(): void {
  try {
    seasonsExpanded.value = window.localStorage.getItem(SEASONS_COLLAPSED_KEY) !== 'true';
  } catch {
    seasonsExpanded.value = true;
  }
}

function saveSeasonsPref(): void {
  prefSaving.value = true;
  prefMessage.value = '';

  try {
    window.localStorage.setItem(SEASONS_COLLAPSED_KEY, seasonsExpanded.value ? 'false' : 'true');

    prefMessage.value = 'Preference saved.';
  } catch {
    prefMessage.value = 'Unable to save preference in this browser.';
  } finally {
    prefSaving.value = false;
  }
}

async function loadProfile(): Promise<void> {
  loading.value = true;
  loadError.value = '';

  try {
    if (userStore.user) {
      profile.value = userStore.user;
      return;
    }

    const verified = await userStore.verifySession();

    if (!verified) {
      loadError.value = 'Unable to verify your session.';
      return;
    }

    profile.value = userStore.user;
  } catch (unknownError) {
    loadError.value = getUserFacingErrorMessage(unknownError, 'Failed to load profile.');
  } finally {
    loading.value = false;
  }
}

async function submitPasswordChange(): Promise<void> {
  passwordError.value = '';
  passwordMessage.value = '';
  clearAllPasswordFieldErrors();

  if (!validatePasswordForm()) {
    return;
  }

  passwordSaving.value = true;

  try {
    await userStore.updatePassword(oldPassword.value, newPassword.value);

    profile.value = userStore.user;
    passwordMessage.value = 'Password updated.';

    resetPasswordForm();
  } catch (unknownError) {
    const status = getHttpStatus(unknownError);

    if (status === 401 || status === 403) {
      void userStore.verifySession();
      passwordError.value = 'Unable to update password because the session could not be verified.';
      return;
    }

    passwordError.value = getUserFacingErrorMessage(unknownError, 'Failed to update password.');
  } finally {
    passwordSaving.value = false;
  }
}

function validatePasswordForm(): boolean {
  let valid = true;

  const passwordChangeRequired = currentUser.value?.password_change_required === true;

  if (!passwordChangeRequired && !oldPassword.value) {
    fieldErrors.oldPassword = 'Current password is required.';
    valid = false;
  }

  if (!newPassword.value) {
    fieldErrors.newPassword = 'New password is required.';
    valid = false;
  } else if (newPassword.value.length < MIN_PASSWORD_LENGTH) {
    fieldErrors.newPassword = `New password must be at least ${MIN_PASSWORD_LENGTH} characters.`;
    valid = false;
  }

  if (!confirmPassword.value) {
    fieldErrors.confirmPassword = 'Please confirm your new password.';
    valid = false;
  } else if (newPassword.value !== confirmPassword.value) {
    fieldErrors.confirmPassword = 'Passwords do not match.';
    valid = false;
  }

  if (oldPassword.value && oldPassword.value === newPassword.value) {
    fieldErrors.newPassword = 'New password must be different from the current password.';
    valid = false;
  }

  return valid;
}

function clearPasswordFieldError(field: PasswordField): void {
  fieldErrors[field] = '';

  if (passwordError.value) {
    passwordError.value = '';
  }

  if (passwordMessage.value) {
    passwordMessage.value = '';
  }
}

function clearAllPasswordFieldErrors(): void {
  fieldErrors.oldPassword = '';
  fieldErrors.newPassword = '';
  fieldErrors.confirmPassword = '';
}

function resetPasswordForm(): void {
  oldPassword.value = '';
  newPassword.value = '';
  confirmPassword.value = '';
  showOldPassword.value = false;
  showNewPassword.value = false;
  showConfirmPassword.value = false;
}

async function handleLogout(): Promise<void> {
  if (loggingOut.value) return;

  loggingOut.value = true;

  try {
    userStore.logout();

    await router.replace({
      path: '/login',
    });
  } finally {
    loggingOut.value = false;
  }
}

function navigateToAdmin(): void {
  if (!canAccessAdmin.value) return;

  void router.push('/admin/libraries');
}

function getUserFacingErrorMessage(unknownError: unknown, fallback: string): string {
  const message = extractErrorMessage(unknownError);

  if (message && isSafeUserFacingMessage(message)) {
    return message;
  }

  return fallback;
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
.profile-view {
  width: 100%;
  max-width: 760px;
  margin: 0 auto;
}

.page-header {
  margin-bottom: 20px;
}

.page-title {
  margin: 0;
  color: #e0e0e0;
  font-size: 24px;
  font-weight: 800;
}

.page-subtitle {
  margin: 6px 0 0;
  color: #666688;
  font-size: 13px;
  line-height: 1.45;
}

.card {
  margin-bottom: 20px;
  padding: 24px;
  background: #1a1a2e;
  border: 1px solid rgb(255 255 255 / 5%);
  border-radius: 12px;
}

.error-card {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.error-title {
  margin: 0 0 4px;
  color: #ffb3b3;
  font-size: 14px;
  font-weight: 800;
}

.info-row {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  padding: 9px 0;
  color: #aaaacc;
  font-size: 14px;
}

.label {
  color: #8888aa;
}

.value {
  color: #dadaf0;
  font-weight: 700;
}

.badge {
  display: inline-flex;
  align-items: center;
  padding: 3px 9px;
  color: #8f89ff;
  background: #2a2a3e;
  border: 1px solid transparent;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 800;
  text-transform: capitalize;
  user-select: none;
}

button.badge {
  cursor: pointer;
}

button.badge:hover {
  background: rgb(108 99 255 / 18%);
  border-color: rgb(108 99 255 / 32%);
}

.warning-text {
  color: #ffb86b;
  font-weight: 700;
}

.warning-box {
  margin: 0 0 16px;
  padding: 10px 12px;
  color: #ffcc80;
  background: #2a2115;
  border: 1px solid #5a4320;
  border-radius: 8px;
  font-size: 13px;
  line-height: 1.45;
}

h3 {
  margin: 0 0 16px;
  color: #d0d0d0;
  font-size: 16px;
  font-weight: 800;
}

.pref-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.pref-label {
  color: #aaaacc;
  font-size: 14px;
  font-weight: 700;
}

.hint {
  margin: 8px 0 0;
  color: #555577;
  font-size: 12px;
  line-height: 1.45;
}

.toggle {
  position: relative;
  width: 44px;
  height: 24px;
  flex: 0 0 auto;
  display: inline-block;
  cursor: pointer;
}

.toggle input {
  display: none;
}

.toggle-slider {
  position: absolute;
  inset: 0;
  background: #2a2a3e;
  border-radius: 999px;
  transition: background-color 0.2s ease;
}

.toggle-slider::before {
  position: absolute;
  left: 3px;
  bottom: 3px;
  width: 18px;
  height: 18px;
  content: '';
  background: #666688;
  border-radius: 50%;
  transition:
    transform 0.2s ease,
    background-color 0.2s ease;
}

.toggle input:checked + .toggle-slider {
  background: #6c63ff;
}

.toggle input:checked + .toggle-slider::before {
  background: #fff;
  transform: translateX(20px);
}

.password-form {
  width: 100%;
}

.field + .field {
  margin-top: 14px;
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
}

.password-toggle:hover:not(:disabled) {
  color: #fff;
  background: rgb(255 255 255 / 6%);
}

.password-toggle:disabled {
  cursor: not-allowed;
  opacity: 0.45;
}

.primary-btn,
.secondary-btn,
.btn-logout {
  min-height: 42px;
  box-sizing: border-box;
  border-radius: 8px;
  cursor: pointer;
  font-size: 14px;
  font-weight: 800;
}

.primary-btn {
  width: 100%;
  margin-top: 18px;
  padding: 10px 24px;
  color: #fff;
  background: #6c63ff;
  border: 0;
}

.primary-btn:hover:not(:disabled) {
  background: #5a52e0;
}

.primary-btn:disabled {
  cursor: not-allowed;
  opacity: 0.62;
}

.secondary-btn {
  flex: 0 0 auto;
  padding: 8px 14px;
  color: #ccccee;
  background: #2a2a3e;
  border: 1px solid #3a3a5e;
}

.secondary-btn:hover {
  color: #fff;
}

.danger-card {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.btn-logout {
  flex: 0 0 auto;
  padding: 10px 18px;
  color: #ff8f8f;
  background: rgb(255 107 107 / 8%);
  border: 1px solid rgb(255 107 107 / 22%);
}

.btn-logout:hover:not(:disabled) {
  background: rgb(255 107 107 / 13%);
  border-color: rgb(255 107 107 / 34%);
}

.btn-logout:disabled {
  cursor: not-allowed;
  opacity: 0.62;
}

.msg {
  margin: 12px 0 0;
  color: #7bd88f;
  font-size: 13px;
}

.error {
  margin: 12px 0 0;
  color: #ff8f8f;
  font-size: 13px;
  line-height: 1.45;
}

.field-error {
  margin: 6px 0 0;
  color: #ff8f8f;
  font-size: 12px;
  line-height: 1.4;
}

@media (max-width: 640px) {
  .profile-view {
    max-width: none;
  }

  .card {
    padding: 18px;
  }

  .info-row,
  .pref-row,
  .danger-card,
  .error-card {
    align-items: flex-start;
    flex-direction: column;
  }

  .secondary-btn,
  .btn-logout {
    width: 100%;
  }
}

@media (prefers-reduced-motion: reduce) {
  .toggle-slider,
  .toggle-slider::before,
  .input {
    transition: none;
  }
}
</style>
