<template>
  <main class="login-page">
    <section class="login-card" aria-labelledby="login-title">
      <router-link to="/" class="brand-link" aria-label="Go to home">
        <div class="logo">fyom</div>
      </router-link>

      <header class="login-header">
        <h1 id="login-title" class="title">{{ $t('auth.welcomeBack') }}</h1>
        <p class="subtitle">{{ $t('auth.signInSubtitle') }}</p>
      </header>

      <div v-if="registeredMessage" class="success-banner" role="status">
        {{ registeredMessage }}
      </div>

      <form class="login-form" novalidate @submit.prevent="handleLogin">
        <div class="field">
          <label for="username">{{ $t('auth.username') }}</label>
          <input
            id="username"
            ref="usernameInput"
            v-model.trim="username"
            type="text"
            required
            autocomplete="username"
            inputmode="text"
            class="input"
            :class="{ invalid: Boolean(fieldErrors.username) }"
            :aria-invalid="Boolean(fieldErrors.username)"
            :aria-describedby="fieldErrors.username ? 'username-error' : undefined"
            :disabled="loading"
            @input="clearFieldError('username')"
          />
          <p v-if="fieldErrors.username" id="username-error" class="field-error">
            {{ fieldErrors.username }}
          </p>
        </div>

        <div class="field">
          <label for="password">{{ $t('auth.password') }}</label>

          <div class="password-wrap">
            <input
              id="password"
              ref="passwordInput"
              v-model="password"
              :type="showPassword ? 'text' : 'password'"
              required
              autocomplete="current-password"
              class="input password-input"
              :class="{ invalid: Boolean(fieldErrors.password) }"
              :aria-invalid="Boolean(fieldErrors.password)"
              :aria-describedby="fieldErrors.password ? 'password-error' : undefined"
              :disabled="loading"
              @input="clearFieldError('password')"
            />

            <button
              type="button"
              class="password-toggle"
              :disabled="loading || password.length === 0"
              :aria-label="showPassword ? $t('auth.hidePassword') : $t('auth.showPassword')"
              @click="showPassword = !showPassword"
            >
              {{ showPassword ? $t('auth.hide') : $t('auth.show') }}
            </button>
          </div>

          <p v-if="fieldErrors.password" id="password-error" class="field-error">
            {{ fieldErrors.password }}
          </p>
        </div>

        <div v-if="error" class="error-banner" role="alert">
          {{ error }}
        </div>

        <button type="submit" class="submit-btn" :disabled="loading || !canSubmit">
          <span v-if="loading" class="spinner" aria-hidden="true"></span>
          <span>{{ loading ? $t('auth.signingIn') : $t('auth.signIn') }}</span>
        </button>
      </form>

      <p class="bottom-link">
        {{ $t('auth.noAccount') }}
        <router-link to="/register"> {{ $t('auth.createOne') }} </router-link>
      </p>

      <div class="locale-row">
        <LanguageSwitcher variant="compact" />
      </div>
    </section>
  </main>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, reactive, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { useUserStore } from '@/stores/user';
import LanguageSwitcher from '@/components/LanguageSwitcher.vue';

const { t } = useI18n();

type LoginField = 'username' | 'password';

const router = useRouter();
const route = useRoute();
const store = useUserStore();

const usernameInput = ref<HTMLInputElement | null>(null);
const passwordInput = ref<HTMLInputElement | null>(null);

const username = ref('');
const password = ref('');
const loading = ref(false);
const error = ref('');
const showPassword = ref(false);

const fieldErrors = reactive<Record<LoginField, string>>({
  username: '',
  password: '',
});

const canSubmit = computed(() => {
  return username.value.trim().length > 0 && password.value.length > 0;
});

const registeredMessage = computed(() => {
  return route.query.registered === '1' ? t('auth.accountCreated') : '';
});

onMounted(async () => {
  await nextTick();
  usernameInput.value?.focus();
});

async function handleLogin(): Promise<void> {
  if (loading.value) return;

  error.value = '';
  clearAllFieldErrors();

  if (!validateForm()) {
    focusFirstInvalidField();
    return;
  }

  loading.value = true;

  try {
    await store.doLogin(username.value.trim(), password.value);

    await router.replace(getRedirectPath());
  } catch (unknownError) {
    error.value = getLoginErrorMessage(unknownError);
    password.value = '';
    showPassword.value = false;

    await nextTick();

    if (username.value.trim()) {
      passwordInput.value?.focus();
    } else {
      usernameInput.value?.focus();
    }
  } finally {
    loading.value = false;
  }
}

function validateForm(): boolean {
  let valid = true;

  if (!username.value.trim()) {
    fieldErrors.username = t('auth.usernameRequired');
    valid = false;
  }

  if (!password.value) {
    fieldErrors.password = t('auth.passwordRequired');
    valid = false;
  }

  return valid;
}

function clearFieldError(field: LoginField): void {
  fieldErrors[field] = '';

  if (error.value) {
    error.value = '';
  }
}

function clearAllFieldErrors(): void {
  fieldErrors.username = '';
  fieldErrors.password = '';
}

function focusFirstInvalidField(): void {
  if (fieldErrors.username) {
    usernameInput.value?.focus();
    return;
  }

  if (fieldErrors.password) {
    passwordInput.value?.focus();
  }
}

function getRedirectPath(): string {
  const redirect = route.query.redirect;

  if (typeof redirect !== 'string') {
    return '/';
  }

  if (!isSafeRedirectPath(redirect)) {
    return '/';
  }

  return redirect;
}

function isSafeRedirectPath(value: string): boolean {
  if (!value.startsWith('/')) return false;
  if (value.startsWith('//')) return false;
  if (value.startsWith('/login')) return false;
  if (value.startsWith('/register')) return false;

  return true;
}

function getLoginErrorMessage(unknownError: unknown): string {
  const status = getHttpStatus(unknownError);

  if (status === 401 || status === 403) {
    return t('auth.signInFailed');
  }

  const message = extractErrorMessage(unknownError);

  if (message && isSafeLoginMessage(message)) {
    return message;
  }

  return t('auth.signInFailed');
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

function isSafeLoginMessage(message: string): boolean {
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
.login-page {
  min-height: 100vh;
  box-sizing: border-box;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  color: #e0e0e0;
  background:
    radial-gradient(circle at top left, rgb(108 99 255 / 18%), transparent 34rem),
    radial-gradient(circle at bottom right, rgb(33 150 243 / 10%), transparent 28rem), #0f0f1a;
}

.login-card {
  width: 100%;
  max-width: 420px;
  box-sizing: border-box;
  padding: 40px;
  background: rgb(26 26 46 / 94%);
  border: 1px solid rgb(255 255 255 / 6%);
  border-radius: 16px;
  box-shadow:
    0 24px 70px rgb(0 0 0 / 42%),
    inset 0 1px 0 rgb(255 255 255 / 4%);
  backdrop-filter: blur(14px);
}

.brand-link {
  display: block;
  width: fit-content;
  margin: 0 auto 10px;
  text-decoration: none;
}

.logo {
  color: #6c63ff;
  font-size: 30px;
  font-weight: 900;
  letter-spacing: -0.04em;
  line-height: 1;
  text-align: center;
}

.login-header {
  margin-bottom: 32px;
  text-align: center;
}

.title {
  margin: 0 0 6px;
  color: #f3f3ff;
  font-size: 24px;
  font-weight: 800;
  line-height: 1.2;
}

.subtitle {
  margin: 0;
  color: #777799;
  font-size: 14px;
  line-height: 1.5;
}

.login-form {
  width: 100%;
}

.field {
  margin-bottom: 16px;
}

.field label {
  display: block;
  margin-bottom: 7px;
  color: #aaaacc;
  font-size: 13px;
  font-weight: 600;
}

.input {
  width: 100%;
  min-height: 44px;
  box-sizing: border-box;
  padding: 10px 12px;
  color: #f0f0ff;
  background: #0f0f1a;
  border: 1px solid #2a2a3e;
  border-radius: 8px;
  outline: none;
  font-size: 14px;
  transition:
    border-color 0.15s ease,
    box-shadow 0.15s ease,
    background-color 0.15s ease;
}

.input:hover:not(:disabled) {
  border-color: #3a3a5e;
}

.input:focus {
  border-color: #6c63ff;
  box-shadow: 0 0 0 3px rgb(108 99 255 / 16%);
}

.input:disabled {
  cursor: not-allowed;
  opacity: 0.72;
}

.input.invalid {
  border-color: #ff6b6b;
}

.input.invalid:focus {
  box-shadow: 0 0 0 3px rgb(255 107 107 / 14%);
}

.password-wrap {
  position: relative;
}

.password-input {
  padding-right: 72px;
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
  font-weight: 600;
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

.success-banner {
  margin-bottom: 16px;
  padding: 10px 12px;
  color: #c9f7d1;
  background: #14251a;
  border: 1px solid #2e6b3c;
  border-radius: 8px;
  font-size: 13px;
  line-height: 1.45;
}

.error-banner {
  margin: 4px 0 0;
  padding: 10px 12px;
  color: #ffb3b3;
  background: #2a1a1a;
  border: 1px solid #5a2a2a;
  border-radius: 8px;
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
  margin-top: 20px;
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

.bottom-link {
  margin: 22px 0 0;
  color: #777799;
  font-size: 14px;
  line-height: 1.5;
  text-align: center;
}

.bottom-link a {
  color: #8f89ff;
  font-weight: 700;
  text-decoration: none;
}

.bottom-link a:hover {
  color: #b4b0ff;
  text-decoration: underline;
  text-underline-offset: 3px;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

.locale-row {
  display: flex;
  justify-content: center;
  margin-top: 18px;
  padding-top: 16px;
  border-top: 1px solid rgb(255 255 255 / 5%);
}

.locale-row :deep(.ls-label) {
  color: #777799;
}

.locale-row :deep(.ls-select) {
  color: #c8c8e0;
  background: rgb(255 255 255 / 4%);
  border-color: rgb(255 255 255 / 10%);
}

.locale-row :deep(.ls-select:hover:not(:disabled)) {
  border-color: rgb(255 255 255 / 20%);
  background: rgb(255 255 255 / 7%);
}

@media (max-width: 520px) {
  .login-page {
    align-items: stretch;
    padding: 16px;
  }

  .login-card {
    max-width: none;
    margin: auto 0;
    padding: 28px 22px;
    border-radius: 14px;
  }

  .title {
    font-size: 22px;
  }
}

@media (prefers-reduced-motion: reduce) {
  .input,
  .password-toggle,
  .submit-btn,
  .bottom-link a {
    transition: none;
  }

  .spinner {
    animation: none;
  }
}
</style>
