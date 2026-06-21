import { computed, ref } from 'vue';
import { defineStore } from 'pinia';
import { apiRequest } from '@/api/request';
import { setSupportedLocales } from '@/composables/useLocale';
import type { ApiEnvelope, SystemStatusData } from '@/api/types';

export type SystemStatus = 'unknown' | 'checking' | 'initialized' | 'error';

type MaybeEnvelope<T> = ApiEnvelope<T> | T;

interface NormalizedSystemStatus {
  initialized: boolean;
  defaultLocale?: string;
  supportedLocales?: string[];
}

export const useSystemStore = defineStore('system', () => {
  const status = ref<SystemStatus>('unknown');
  const checkedOnce = ref(false);
  const lastError = ref('');
  const defaultLocale = ref<string>('');
  const supportedLocales = ref<string[]>([]);

  let fetchPromise: Promise<void> | null = null;

  const isChecking = computed(() => status.value === 'checking');
  const isInitialized = computed(() => status.value === 'initialized');
  const hasError = computed(() => status.value === 'error');

  /**
   * The system store is considered ready once the initial system status check
   * has resolved to either initialized or error.
   *
   * The router currently treats error as wait/fail-closed, so this flag is
   * mostly useful for UI diagnostics rather than route allow decisions.
   */
  const isReady = computed(() => {
    return status.value === 'initialized' || status.value === 'error';
  });

  /**
   * Fetch system initialization status.
   *
   * This request is intentionally silent from an auth perspective.
   * System status checks must never invalidate the user session.
   */
  async function fetchSystemStatus(force = false): Promise<void> {
    if (fetchPromise && !force) {
      return fetchPromise;
    }

    fetchPromise = runFetchSystemStatus().finally(() => {
      fetchPromise = null;
    });

    return fetchPromise;
  }

  async function runFetchSystemStatus(): Promise<void> {
    status.value = 'checking';
    lastError.value = '';

    try {
      const response = await apiRequest.get<MaybeEnvelope<SystemStatusData>>('/system/status', {
        authFailureMode: 'silent',
      });

      const normalized = normalizeSystemStatus(response.data);

      // Persist i18n config from the backend response.
      if (normalized.defaultLocale) {
        defaultLocale.value = normalized.defaultLocale;
      }
      if (normalized.supportedLocales && normalized.supportedLocales.length > 0) {
        supportedLocales.value = normalized.supportedLocales;
        // Phase 4: push to the i18n module so LanguageSwitcher and any
        // other reactive consumer re-renders with the backend-advertised
        // locale set. Codes the frontend cannot render (no bundled
        // messages) are silently filtered inside setSupportedLocales().
        setSupportedLocales(normalized.supportedLocales);
      }

      if (normalized.initialized) {
        status.value = 'initialized';
        checkedOnce.value = true;
        lastError.value = '';
        return;
      }

      /**
       * Historical setup-required flow has been deprecated.
       * If the backend reports initialized=false, treat it as a system error.
       */
      status.value = 'error';
      checkedOnce.value = true;
      lastError.value = 'System is not initialized.';
    } catch (unknownError) {
      status.value = 'error';
      checkedOnce.value = true;
      lastError.value = getSystemErrorMessage(unknownError, 'Unable to check system status.');
    }
  }

  function markInitialized(): void {
    status.value = 'initialized';
    checkedOnce.value = true;
    lastError.value = '';
  }

  function markError(message = 'System status is unavailable.'): void {
    status.value = 'error';
    checkedOnce.value = true;
    lastError.value = message;
  }

  function reset(): void {
    status.value = 'unknown';
    checkedOnce.value = false;
    lastError.value = '';
    defaultLocale.value = '';
    supportedLocales.value = [];
    fetchPromise = null;
  }

  return {
    status,
    checkedOnce,
    lastError,
    defaultLocale,
    supportedLocales,
    isChecking,
    isInitialized,
    hasError,
    isReady,
    fetchSystemStatus,
    markInitialized,
    markError,
    reset,
  };
});

function normalizeSystemStatus(value: unknown): NormalizedSystemStatus {
  const data = unwrapUnknownEnvelope(value);

  if (!isRecord(data)) {
    throw new Error('system status response is invalid');
  }

  const result: NormalizedSystemStatus = {
    initialized: data.initialized === true,
  };

  if (typeof data.default_locale === 'string' && data.default_locale) {
    result.defaultLocale = data.default_locale;
  }

  if (Array.isArray(data.supported_locales)) {
    result.supportedLocales = data.supported_locales.filter(
      (loc): loc is string => typeof loc === 'string' && loc.length > 0
    );
  }

  return result;
}

function unwrapUnknownEnvelope(value: unknown): unknown {
  if (isRecord(value) && 'data' in value) {
    return value.data;
  }

  return value;
}

function getSystemErrorMessage(unknownError: unknown, fallback: string): string {
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
