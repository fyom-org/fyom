/**
 * Unified API error extraction utilities.
 *
 * Historically, each view had its own copy of `extractErrorMessage` +
 * `isSafeUserFacingMessage` + `getHttpStatus`. This module consolidates
 * them into a single source of truth.
 *
 * Phase 3 extension:
 * - The backend now emits a stable `error_code` field on every error
 *   response (see pkg/errors/codes.go for the taxonomy).
 * - `getSafeApiErrorMessage(error, fallbackKey)` first looks up
 *   `api_error.<error_code>` in the active locale. If a translation
 *   exists, it is returned — the backend's English `message` is ignored.
 * - If error_code is absent OR no translation key exists, the previous
 *   behavior applies: the backend message is used if safe; otherwise the
 *   caller's fallbackKey is used.
 *
 * Safety filter:
 * - Some backend error messages contain sensitive fragments (SQL errors,
 *   stack traces, JWT internals). These must NEVER be shown to end users.
 * - `isSafeUserFacingMessage` returns false for messages containing such
 *   fragments; the caller then falls back to a translated default.
 *
 * i18n integration:
 * - `getSafeApiErrorMessage(error, fallbackKey)` accepts an i18n key for the
 *   fallback message (e.g. 'errors.generic') and returns the translated
 *   string. This is the preferred entry point for views.
 */
import i18n from '@/plugins/i18n';

/**
 * Fragments that indicate a message is NOT safe to show to end users.
 * If any of these appear (case-insensitive), the message is discarded and
 * the caller falls back to a translated generic error.
 *
 * Keep this list in sync across all callers — previously each view had its
 * own copy, and RegisterView was missing the 'request failed with status code'
 * entry, causing axios boilerplate to leak to users.
 */
const UNSAFE_FRAGMENTS: readonly string[] = [
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

export function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null;
}

export function getHttpStatus(error: unknown): number | undefined {
  if (!isRecord(error)) return undefined;

  const response = error.response;
  if (!isRecord(response)) return undefined;

  const status = response.status;
  return typeof status === 'number' ? status : undefined;
}

/**
 * Extract the raw error_code string from an axios-style error object.
 *
 * Returns '' if no code is present (legacy server, success response, or
 * network error). The string is the canonical snake_case identifier from
 * pkg/errors/codes.go (e.g. 'invalid_credentials').
 */
export function getErrorCode(error: unknown): string {
  if (!isRecord(error)) return '';

  const response = error.response;
  if (!isRecord(response)) return '';

  const data = response.data;
  if (!isRecord(data)) return '';

  const code = data.error_code;
  return typeof code === 'string' ? code.trim() : '';
}

/**
 * Look up a translation for the given error_code in the active locale.
 *
 * Returns '' if the code is empty, OR if no `api_error.<code>` key exists
 * in the current locale (vue-i18n returns the key path itself when a
 * translation is missing; we detect that case by checking for a dot in the
 * result and the absence of a registered key).
 */
export function translateErrorCode(code: string): string {
  if (!code) return '';

  const key = `api_error.${code}`;

  // vue-i18n's `te` (translation exists) is the canonical way to check
  // whether a key is present in the active locale's message table.
  if (typeof i18n.global.te === 'function' && i18n.global.te(key)) {
    return i18n.global.t(key);
  }

  // Legacy fallback for vue-i18n configurations without `globalInjection`
  // of the `te` helper: probe `t()` and reject the literal key path.
  const translated = i18n.global.t(key);
  if (translated === key) return '';
  return translated;
}

/**
 * Extract the raw error message from an axios-style error object.
 * Returns '' if no message can be found.
 *
 * Does NOT apply the safety filter — callers should use
 * `getSafeApiErrorMessage` for user-facing contexts.
 */
export function extractErrorMessage(error: unknown): string {
  if (!isRecord(error)) return '';

  const response = error.response;
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

  const message = (error as { message?: unknown }).message;
  if (typeof message === 'string' && message.trim()) {
    return message.trim();
  }

  return '';
}

/**
 * Returns true if the message is safe to show to end users.
 * False if it contains sensitive fragments (SQL, stack traces, JWT, etc.).
 */
export function isSafeUserFacingMessage(message: string): boolean {
  if (!message) return false;

  const normalized = message.toLowerCase();
  return !UNSAFE_FRAGMENTS.some((fragment) => normalized.includes(fragment));
}

/**
 * Extract a safe, user-facing error message from an axios error.
 *
 * Resolution order (first non-empty result wins):
 *  1. `api_error.<error_code>` translation — if the backend emitted a
 *     stable error_code AND a translation exists in the active locale.
 *     This is the preferred path: the message is always translated and
 *     never leaks server internals.
 *  2. The backend's English `message` field — only if it passes the
 *     safety filter. Useful when error_code is absent (legacy) or when
 *     no translation key has been added yet for a new code.
 *  3. The translated fallbackKey (default: 'errors.generic').
 *
 * @param error - The caught error (typically an API error)
 * @param fallbackKey - i18n key for the fallback message (default: 'errors.generic')
 * @returns A user-facing string (translated when possible)
 */
export function getSafeApiErrorMessage(
  error: unknown,
  fallbackKey: string = 'errors.generic'
): string {
  // 1. Prefer translated error_code.
  const code = getErrorCode(error);
  if (code) {
    const translated = translateErrorCode(code);
    if (translated) return translated;
  }

  // 2. Fall back to the backend's English message if it is safe.
  const message = extractErrorMessage(error);
  if (message && isSafeUserFacingMessage(message)) {
    return message;
  }

  // 3. Last resort: translated fallback key.
  return i18n.global.t(fallbackKey);
}

/**
 * Check if an axios error represents a 401 or 403 (auth failure).
 */
export function isUnauthorizedOrForbidden(error: unknown): boolean {
  const status = getHttpStatus(error);
  return status === 401 || status === 403;
}

/**
 * Type guard for API errors (formerly axios errors).
 * Works with both ofetch FetchError and plain error objects that have
 * a `.response` property (duck-type compatible).
 */
export function isApiError(error: unknown): boolean {
  return (
    typeof error === 'object' &&
    error !== null &&
    'response' in error &&
    typeof (error as { response?: unknown }).response === 'object'
  );
}
