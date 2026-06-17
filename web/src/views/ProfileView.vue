<template>
  <main class="profile-view">
    <header class="page-header">
      <div>
        <h2 class="page-title">{{ $t('profile.title') }}</h2>
        <p class="page-subtitle">{{ $t('profile.subtitle') }}</p>
      </div>
    </header>

    <section v-if="loading" class="card">
      <p class="hint">{{ $t('profile.loadingProfile') }}</p>
    </section>

    <section v-else-if="loadError" class="card error-card" role="alert">
      <div>
        <p class="error-title">{{ $t('profile.unableToLoad') }}</p>
        <p class="error">{{ loadError }}</p>
      </div>

      <button type="button" class="secondary-btn" @click="loadProfile">{{ $t('common.retry') }}</button>
    </section>

    <section v-else-if="currentUser" class="card">
      <h3>{{ $t('profile.account') }}</h3>

      <div class="info-row">
        <span class="label">{{ $t('auth.username') }}</span>
        <span class="value">{{ currentUser.username }}</span>
      </div>

      <div class="info-row">
        <span class="label">{{ $t('profile.role') }}</span>

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
        <span class="label">{{ $t('profile.passwordStatus') }}</span>
        <span class="warning-text">{{ $t('profile.passwordChangeRequired') }}</span>
      </div>
    </section>

    <section class="card">
      <h3>{{ $t('profile.preferences') }}</h3>

      <div class="pref-row">
        <div>
          <span class="pref-label">{{ $t('profile.defaultExpandSeasons') }}</span>
          <p class="hint">
            {{ $t('profile.defaultExpandSeasonsHint') }}
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

      <div class="pref-row language-pref">
        <div>
          <span class="pref-label">{{ $t('profile.interfaceLanguage') }}</span>
          <p class="hint">
            {{ $t('profile.interfaceLanguageHint') }}
          </p>
        </div>

        <LanguageSwitcher variant="full" />
      </div>

      <p v-if="prefMessage" class="msg">
        {{ prefMessage }}
      </p>
    </section>

    <section class="card">
      <h3>{{ $t('profile.changePassword') }}</h3>

      <p v-if="currentUser?.password_change_required" class="warning-box">
        {{ $t('profile.passwordChangeWarning') }}
      </p>

      <form class="password-form" novalidate @submit.prevent="submitPasswordChange">
        <div class="field">
          <label for="old-password">{{ $t('profile.currentPassword') }}</label>

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
              :aria-label="showOldPassword ? $t('profile.hideCurrentPassword') : $t('profile.showCurrentPassword')"
              @click="showOldPassword = !showOldPassword"
            >
              {{ showOldPassword ? $t('auth.hide') : $t('auth.show') }}
            </button>
          </div>

          <p v-if="fieldErrors.oldPassword" id="old-password-error" class="field-error">
            {{ fieldErrors.oldPassword }}
          </p>
        </div>

        <div class="field">
          <label for="new-password">{{ $t('auth.newPassword') }}</label>

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
              :aria-label="showNewPassword ? $t('auth.forceChangeHideNewPassword') : $t('auth.forceChangeShowNewPassword')"
              @click="showNewPassword = !showNewPassword"
            >
              {{ showNewPassword ? $t('auth.hide') : $t('auth.show') }}
            </button>
          </div>

          <p v-if="fieldErrors.newPassword" id="new-password-error" class="field-error">
            {{ fieldErrors.newPassword }}
          </p>
        </div>

        <div class="field">
          <label for="confirm-password">{{ $t('auth.confirmNewPassword') }}</label>

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
                showConfirmPassword ? $t('auth.forceChangeHideConfirm') : $t('auth.forceChangeShowConfirm')
              "
              @click="showConfirmPassword = !showConfirmPassword"
            >
              {{ showConfirmPassword ? $t('auth.hide') : $t('auth.show') }}
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
          {{ passwordSaving ? $t('profile.updating') : $t('profile.updatePassword') }}
        </button>
      </form>
    </section>

    <section class="card danger-card">
      <div>
        <h3>{{ $t('profile.session') }}</h3>
        <p class="hint">{{ $t('profile.signOutDevice') }}</p>
      </div>

      <button type="button" class="btn-logout" :disabled="loggingOut" @click="handleLogout">
        {{ loggingOut ? $t('profile.signingOut') : $t('profile.logout') }}
      </button>
    </section>
  </main>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import type { User } from '@/api/auth';
import { useUserStore } from '@/stores/user';
import { getSafeApiErrorMessage, isUnauthorizedOrForbidden } from '@/lib/api/errors';
import LanguageSwitcher from '@/components/LanguageSwitcher.vue';

type PasswordField = 'oldPassword' | 'newPassword' | 'confirmPassword';

const SEASONS_COLLAPSED_KEY = 'seasons_collapsed_default';
const MIN_PASSWORD_LENGTH = 6;

const router = useRouter();
const userStore = useUserStore();
const { t } = useI18n();

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

    prefMessage.value = t('profile.preferenceSaved');
  } catch {
    prefMessage.value = t('profile.preferenceSaveFailed');
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
      loadError.value = t('profile.sessionVerifyFailed');
      return;
    }

    profile.value = userStore.user;
  } catch (unknownError) {
    loadError.value = getSafeApiErrorMessage(unknownError, 'profile.loadFailed');
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
    passwordMessage.value = t('profile.passwordUpdated');

    resetPasswordForm();
  } catch (unknownError) {
    if (isUnauthorizedOrForbidden(unknownError)) {
      void userStore.verifySession();
      passwordError.value = t('profile.passwordUpdateSessionError');
      return;
    }

    passwordError.value = getSafeApiErrorMessage(unknownError, 'errors.generic');
  } finally {
    passwordSaving.value = false;
  }
}

function validatePasswordForm(): boolean {
  let valid = true;

  const passwordChangeRequired = currentUser.value?.password_change_required === true;

  if (!passwordChangeRequired && !oldPassword.value) {
    fieldErrors.oldPassword = t('profile.currentPasswordRequired');
    valid = false;
  }

  if (!newPassword.value) {
    fieldErrors.newPassword = t('auth.newPasswordRequired');
    valid = false;
  } else if (newPassword.value.length < MIN_PASSWORD_LENGTH) {
    fieldErrors.newPassword = t('profile.newPasswordMinLength', { n: MIN_PASSWORD_LENGTH });
    valid = false;
  }

  if (!confirmPassword.value) {
    fieldErrors.confirmPassword = t('auth.forceChangeConfirmRequired');
    valid = false;
  } else if (newPassword.value !== confirmPassword.value) {
    fieldErrors.confirmPassword = t('auth.passwordMismatch');
    valid = false;
  }

  if (oldPassword.value && oldPassword.value === newPassword.value) {
    fieldErrors.newPassword = t('profile.newPasswordMustDiffer');
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

.language-pref {
  padding-top: 14px;
  margin-top: 14px;
  border-top: 1px solid rgb(255 255 255 / 5%);
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
