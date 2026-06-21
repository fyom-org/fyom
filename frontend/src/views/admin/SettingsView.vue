<template>
  <div class="admin-page">
    <h1>{{ $t('admin.settings.title') }}</h1>

    <div v-if="loading" class="loading">{{ $t('common.loading') }}</div>

    <div v-else-if="error" class="error">{{ error }}</div>

    <template v-else>
      <div class="settings-section">
        <h2>{{ $t('admin.settings.registration') }}</h2>

        <label class="toggle-row">
          <input v-model="allowRegistration" type="checkbox" :disabled="saving" />
          <span>{{ $t('admin.settings.allowRegistration') }}</span>
        </label>

        <p class="hint">{{ $t('admin.settings.allowRegistrationHint') }}</p>
      </div>

      <div class="settings-section">
        <h2>{{ $t('admin.settings.systemDefaultLanguage') }}</h2>

        <div class="locale-row">
          <label for="default-locale" class="locale-label">
            {{ $t('admin.settings.defaultLocale') }}
          </label>

          <select
            id="default-locale"
            v-model="defaultLocale"
            :disabled="saving"
            class="locale-select"
          >
            <option v-for="loc in supportedLocales" :key="loc" :value="loc">
              {{ localeFlag(loc) }} {{ localeDisplayName(loc) }}
            </option>
          </select>
        </div>

        <p class="hint">
          {{ $t('admin.settings.defaultLocaleHint') }}
        </p>
      </div>

      <button class="save-btn" :disabled="saving" @click="saveSettings">
        {{ saving ? $t('admin.settings.saving') : $t('admin.settings.saveButton') }}
      </button>

      <p v-if="message" class="msg">{{ message }}</p>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { useI18n } from 'vue-i18n';
import { authRequest } from '@/api/request';
import { useSystemStore } from '@/stores/system';
import { getSafeApiErrorMessage, isUnauthorizedOrForbidden } from '@/lib/api/errors';
import { localeDisplayName, localeFlag, supportedLocales as i18nSupportedLocales } from '@/plugins/i18n';
import type { ApiEnvelope } from '@/api/types';

const { t } = useI18n();

/**
 * Backend shape assumption:
 * GET /admin/settings -> { allow_registration: "true" | "false", default_locale: "en" }
 * PUT /admin/settings -> { allow_registration: "...", default_locale: "..." }
 */
interface SettingsData {
  allow_registration: string;
  default_locale: string;
}

const systemStore = useSystemStore();

const allowRegistration = ref(false);
const defaultLocale = ref<string>('en');

const loading = ref(true);
const saving = ref(false);

const message = ref('');
const error = ref('');

/**
 * The list of locale codes the admin can choose from.
 *
 * Phase 4: this is now a reactive computed that reads from the i18n
 * module's runtime supported-locales list. That list is initialized to
 * the static BUNDLED_LOCALES (['en','zh']) and updated automatically when
 * `systemStore.runFetchSystemStatus()` calls `setSupportedLocales()` with
 * the backend-advertised list. So this dropdown stays in sync with both
 * the backend configuration and the LanguageSwitcher shown to end users.
 */
const supportedLocales = computed<readonly string[]>(() => i18nSupportedLocales.value);

async function loadSettings(): Promise<void> {
  loading.value = true;
  error.value = '';

  try {
    // Ensure the i18n module's runtime supported-locales list is populated
    // from the backend. The reactive `supportedLocales` computed above will
    // pick up the result automatically; we don't need to assign it here.
    if (systemStore.supportedLocales.length === 0) {
      await systemStore.fetchSystemStatus();
    }

    const res = await authRequest.get<ApiEnvelope<SettingsData>>('/admin/settings');

    const data = res.data.data;

    allowRegistration.value = data.allow_registration === 'true';

    // Use the backend's default_locale if present and valid, otherwise fall
    // back to the systemStore's value (which defaults to 'en').
    const backendLocale = data.default_locale;
    if (backendLocale && supportedLocales.value.includes(backendLocale)) {
      defaultLocale.value = backendLocale;
    } else if (systemStore.defaultLocale) {
      defaultLocale.value = systemStore.defaultLocale;
    }
  } catch (err: unknown) {
    if (isUnauthorizedOrForbidden(err)) {
      // Handled centrally by router / store; do not pollute the console
      return;
    }

    console.error('[settings] loadSettings failed:', err);
    error.value = t('admin.settings.loadFailed');
  } finally {
    loading.value = false;
  }
}

async function saveSettings(): Promise<void> {
  saving.value = true;
  message.value = '';
  error.value = '';

  try {
    await authRequest.put<ApiEnvelope<null>>('/admin/settings', {
      allow_registration: allowRegistration.value ? 'true' : 'false',
      default_locale: defaultLocale.value,
    });

    message.value = t('admin.settings.saved');
  } catch (err: unknown) {
    if (isUnauthorizedOrForbidden(err)) {
      return;
    }

    console.error('[settings] saveSettings failed:', err);

    message.value = getSafeApiErrorMessage(err, 'admin.settings.saveFailed');
  } finally {
    saving.value = false;
  }
}

onMounted(() => {
  void loadSettings();
});
</script>

<style scoped>
.admin-page h1 {
  font-size: 22px;
  color: #e0e0e0;
  margin: 0 0 24px;
}

h2 {
  font-size: 16px;
  color: #c0c0d0;
  margin: 0 0 12px;
}

.loading {
  color: #555577;
}

.error {
  color: #ff6b6b;
  font-size: 14px;
}

.settings-section {
  margin-bottom: 24px;
  padding: 20px;
  background: #12121e;
  border-radius: 8px;
  border: 1px solid #1a1a2e;
}

.toggle-row {
  display: flex;
  align-items: center;
  gap: 10px;
  color: #ccccee;
  font-size: 14px;
  cursor: pointer;
}

.toggle-row input[type='checkbox'] {
  accent-color: #6c63ff;
  width: 18px;
  height: 18px;
}

.locale-row {
  display: flex;
  align-items: center;
  gap: 12px;
}

.locale-label {
  color: #ccccee;
  font-size: 14px;
  font-weight: 600;
  white-space: nowrap;
}

.locale-select {
  min-width: 180px;
  min-height: 36px;
  padding: 6px 12px;
  color: #e0e0e0;
  background: #0f0f1a;
  border: 1px solid #2a2a3e;
  border-radius: 6px;
  font-size: 14px;
  font-family: inherit;
  cursor: pointer;
  transition: border-color 0.15s ease;
}

.locale-select:hover:not(:disabled) {
  border-color: #3a3a5e;
}

.locale-select:focus {
  outline: 2px solid #6c63ff;
  outline-offset: 1px;
  border-color: #6c63ff;
}

.locale-select option {
  color: #1a1a2e;
  background: #fff;
}

.hint {
  color: #555577;
  font-size: 12px;
  margin-top: 8px;
}

.save-btn {
  padding: 10px 24px;
  background: #6c63ff;
  color: #fff;
  border: none;
  border-radius: 6px;
  cursor: pointer;
  font-size: 14px;
}

.save-btn:hover:not(:disabled) {
  background: #5a52e0;
}

.save-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.msg {
  margin-top: 12px;
  font-size: 13px;
  color: #4caf50;
}
</style>
