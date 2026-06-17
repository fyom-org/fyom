/**
 * Locale detection, switching, and persistence composable.
 *
 * Resolution chain (anonymous users, Phase 0):
 *   navigator.language → DEFAULT_LOCALE ('en')
 *
 * Resolution chain (authenticated users, Phase 1):
 *   users.preferred_language → system_settings.default_locale
 *   → navigator.language → 'en'
 *
 * Persistence:
 * - Anonymous locale is NOT persisted. The spec explicitly forbids writing
 *   to localStorage without a user context, because doing so would shadow
 *   admin-managed `system_settings.default_locale` after a future login.
 * - Authenticated locale is persisted server-side via
 *   `PUT /api/v1/auth/me/preferences` (Phase 1).
 *
 * Hot-swap contract:
 * - Locale changes are reactive. Setting `i18n.global.locale.value` triggers
 *   Vue re-render of all `{{ $t(...) }}` / `t(...)` consumers WITHOUT a page
 *   refresh. This is a hard requirement (locked scope decision).
 *
 * Phase 4: `supportedLocales` is now a reactive computed that reflects the
 * runtime list populated from `/system/status`. `setSupportedLocales()`
 * is called by systemStore after the backend responds.
 */
import { computed } from 'vue';
import i18n, {
  DEFAULT_LOCALE,
  BUNDLED_LOCALES,
  supportedLocales as runtimeSupportedLocales,
  isSupportedLocale as isSupportedLocaleImpl,
  localeDisplayName,
  setSupportedLocales,
  type LocaleCode,
} from '@/plugins/i18n';

// Re-export so consumers can import from a single module.
// `isSupportedLocaleImpl` is re-exported under its public name `isSupportedLocale`
// so existing imports in stores/user.ts and elsewhere keep working.
export {
  DEFAULT_LOCALE,
  BUNDLED_LOCALES,
  isSupportedLocaleImpl as isSupportedLocale,
  localeDisplayName,
  setSupportedLocales,
  type LocaleCode,
};

/**
 * Map a raw `navigator.language` value (e.g. "zh-CN", "en-US", "zh-Hant")
 * to one of the supported locale codes.
 *
 * Strategy: longest-prefix match on the lowercased language tag.
 * - "zh-CN" → "zh"
 * - "zh-Hant" → "zh"
 * - "en-US" → "en"
 * - "fr-FR" → no match → returns null (caller falls back to DEFAULT_LOCALE)
 *
 * Consults the runtime supported-locales list (Phase 4) so newly-added
 * locales become browser-detectable without code changes here.
 */
export function matchBrowserLanguageTag(tag: string | undefined): LocaleCode | null {
  if (!tag) return null;

  const lower = tag.toLowerCase();

  for (const supported of runtimeSupportedLocales.value) {
    // Exact match ("en", "zh") OR prefix match ("en-us", "zh-cn", "zh-hant").
    if (lower === supported || lower.startsWith(`${supported}-`)) {
      return supported;
    }
  }

  return null;
}

/**
 * Detect the user's preferred locale from `navigator.languages`.
 *
 * Returns DEFAULT_LOCALE when:
 * - Running outside a browser (SSR / Node test env)
 * - `navigator.languages` is empty or contains no supported prefix
 */
export function detectBrowserLocale(): LocaleCode {
  if (typeof navigator === 'undefined') return DEFAULT_LOCALE;

  const candidates: ReadonlyArray<string | undefined> =
    navigator.languages ?? (navigator.language ? [navigator.language] : []);

  for (const candidate of candidates) {
    const matched = matchBrowserLanguageTag(candidate);
    if (matched) return matched;
  }

  return DEFAULT_LOCALE;
}

/**
 * Sync the `<html lang="...">` attribute so screen readers, browser
 * spell-check, and search engines see the active locale. Also improves
 * CSS logical-property rendering (`:lang()` selectors, hyphenation).
 *
 * Phase 11: also syncs `<html dir="...">` so the document directionality
 * matches the locale. All v1 locales (en, zh, ja) are LTR, so `dir` is
 * always set to `"ltr"`. The architecture is RTL-ready: if a future RTL
 * locale (ar, he) is added, extend `RTL_LOCALES` below and the existing
 * CSS logical properties (`padding-inline-start`, `margin-inline-end`,
 * `text-align: start`) will flip automatically.
 */
const RTL_LOCALES: ReadonlySet<string> = new Set(['ar', 'he', 'fa', 'ur']);

function syncDocumentLang(locale: LocaleCode): void {
  if (typeof document === 'undefined') return;

  document.documentElement.lang = locale;
  document.documentElement.dir = RTL_LOCALES.has(locale) ? 'rtl' : 'ltr';
}

/**
 * Public composable. Safe to call from any component setup function.
 *
 * For non-component callers (e.g. axios interceptors, store actions), use
 * the lower-level `setGlobalLocale()` / `getCurrentLocale()` exports below.
 */
export function useLocale() {
  const currentLocale = computed<LocaleCode>(() => {
    const value = i18n.global.locale.value;
    return isSupportedLocaleImpl(value) ? value : DEFAULT_LOCALE;
  });

  function setLocale(locale: LocaleCode): void {
    if (!isSupportedLocaleImpl(locale)) {
      console.warn(`[i18n] Ignoring unsupported locale: ${locale}`);
      return;
    }

    if (i18n.global.locale.value === locale) return;

    i18n.global.locale.value = locale;
    syncDocumentLang(locale);
  }

  return {
    currentLocale,
    /**
     * Reactive list of supported locales. Reflects backend /system/status
     * updates pushed via setSupportedLocales().
     */
    supportedLocales: runtimeSupportedLocales,
    defaultLocale: DEFAULT_LOCALE,
    setLocale,
    /**
     * Display name for a locale code, rendered in the locale's own language.
     * Shared by all locale selectors so labels stay consistent.
     */
    localeDisplayName,
  };
}

/**
 * Imperative setter for use outside Vue component setup context
 * (e.g. from a Pinia action or an axios interceptor).
 */
export function setGlobalLocale(locale: LocaleCode): void {
  if (!isSupportedLocaleImpl(locale)) return;
  i18n.global.locale.value = locale;
  syncDocumentLang(locale);
}

/**
 * Imperative getter for non-component callers.
 */
export function getCurrentLocale(): LocaleCode {
  const value = i18n.global.locale.value;
  return isSupportedLocaleImpl(value) ? value : DEFAULT_LOCALE;
}

/**
 * Bootstrap-time locale resolution for anonymous users.
 *
 * Called once from `main.ts` after the i18n plugin is installed.
 * Reads `navigator.language` and applies the best supported match.
 *
 * Phase 1 extends this to consult `systemStore.defaultLocale` and
 * `userStore.user.preferred_language` before falling back to the browser.
 */
export function applyInitialLocale(): LocaleCode {
  const detected = detectBrowserLocale();
  setGlobalLocale(detected);
  return detected;
}
