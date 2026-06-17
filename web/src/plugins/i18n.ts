/**
 * vue-i18n plugin initialization.
 *
 * Architecture:
 * - Composition API mode (`legacy: false`). Components use `useI18n()` or
 *   the globally-injected `$t()` in templates.
 * - Messages are statically imported JSON files; Vite bundles them into the
 *   main chunk. Locale switching is in-memory (no network fetch) and triggers
 *   reactive re-render without page refresh.
 * - `fallbackLocale: 'en'` guarantees any missing key in a non-English locale
 *   falls back to the source English string rather than rendering a key path.
 * - `missingWarn` / `fallbackWarn` are disabled to keep the console quiet
 *   during Phase 2 string extraction (the codebase will temporarily have many
 *   keys present in only one locale).
 *
 * Locale resolution is performed by `useLocale.detectAndApplyLocale()` at
 * bootstrap, NOT here. The plugin only establishes the default locale ('en')
 * so SSR / pre-bootstrap rendering is deterministic.
 *
 * Phase 4: The set of supported locales is now backend-driven. The static
 * `BUNDLED_LOCALES` constant is the immutable list of locales whose message
 * files are imported at build time. The runtime effective list
 * (`runtimeSupportedLocales`) starts as `BUNDLED_LOCALES` and is overridden
 * by `setSupportedLocales()` once `/system/status` returns. Unknown backend
 * codes (locales without a bundled message file) are filtered out so the UI
 * never offers a locale it cannot render.
 */
import { computed, ref } from 'vue';
import { createI18n } from 'vue-i18n';
import en from '@/locales/en.json';
import zh from '@/locales/zh.json';
import ja from '@/locales/ja.json';

export type LocaleCode = 'en' | 'zh' | 'ja';

/**
 * Locales whose message JSON is statically imported above.
 *
 * This is the immutable lower bound — the runtime supported list is always
 * a subset of this. Adding a new locale requires:
 *  1. Drop a new `web/src/locales/<code>.json` file (mirror en.json keys).
 *  2. Import it above and add it to the `messages` map.
 *  3. Add the code to this array.
 *  4. Add a label entry to `LOCALE_DISPLAY_LABELS` below.
 *  5. Add the code to `pkg/locale/locale.go` SupportedLocales.
 */
export const BUNDLED_LOCALES: readonly LocaleCode[] = ['en', 'zh', 'ja'] as const;

/**
 * The locale used when no preference can be determined. Always the source
 * locale (English) so a fresh install with no `navigator.language` hint
 * (e.g. headless integration tests) renders deterministically.
 */
export const DEFAULT_LOCALE: LocaleCode = 'en';

/**
 * Display labels for each bundled locale, rendered IN THE LOCALE'S OWN
 * language so users can recognize their language regardless of the current
 * UI locale. Centralized here so LanguageSwitcher and admin SettingsView
 * share a single source of truth.
 */
export const LOCALE_DISPLAY_LABELS: Readonly<Record<LocaleCode, string>> = Object.freeze({
  en: 'English',
  zh: '简体中文',
  ja: '日本語',
});

/**
 * Flag emoji for each bundled locale, shown in the LanguageSwitcher dropdown
 * so users can visually scan for their language. Country flags are a common
 * convention for language selectors; the chosen flags represent the primary
 * country for each language (US for English, CN for Chinese, JP for Japanese).
 *
 * Centralized here so the switcher and any future locale-picking UI share a
 * single source of truth. Rendered as a string emoji for maximum portability
 * (no asset dependencies, renders on all platforms with emoji font support).
 */
export const LOCALE_FLAGS: Readonly<Record<LocaleCode, string>> = Object.freeze({
  en: '🇺🇸',
  zh: '🇨🇳',
  ja: '🇯🇵',
});

/**
 * Runtime-overridable list of supported locales.
 *
 * Initialized to BUNDLED_LOCALES so the UI is functional before
 * /system/status responds. Once the backend reports its supported_locales,
 * `setSupportedLocales()` filters that list to bundled-only codes and
 * updates this ref — reactive consumers (LanguageSwitcher, admin SettingsView)
 * re-render automatically.
 */
const runtimeSupportedLocales = ref<readonly LocaleCode[]>([...BUNDLED_LOCALES]);

/**
 * Override the runtime supported-locales list.
 *
 * Called by `systemStore.runFetchSystemStatus()` after extracting
 * `supported_locales` from the backend response. Codes that are not in
 * BUNDLED_LOCALES (i.e. the frontend has no messages for them) are silently
 * filtered out — the backend may advertise locales the UI cannot render yet.
 *
 * If the filtered list is empty (backend misconfig or empty response), the
 * runtime list is left unchanged so the UI does not collapse to a state
 * where no locale is selectable.
 */
export function setSupportedLocales(codes: readonly string[]): void {
  if (!Array.isArray(codes) || codes.length === 0) return;

  const filtered: LocaleCode[] = [];
  for (const code of codes) {
    if (typeof code === 'string' && (BUNDLED_LOCALES as readonly string[]).includes(code)) {
      filtered.push(code as LocaleCode);
    }
  }

  if (filtered.length === 0) return;

  // Preserve backend ordering (the first entry is typically the default).
  runtimeSupportedLocales.value = filtered;
}

/**
 * Read-only reactive accessor for the current runtime supported-locales list.
 *
 * Use this (not BUNDLED_LOCALES) in any component that renders a locale
 * selector so the selector reflects backend configuration changes without
 * a page refresh.
 */
export const supportedLocales = computed<readonly LocaleCode[]>(() => runtimeSupportedLocales.value);

/**
 * Type guard: returns true if `value` is one of the currently-supported
 * locale codes (consults the runtime list, not just the bundled list).
 */
export function isSupportedLocale(value: unknown): value is LocaleCode {
  if (typeof value !== 'string') return false;
  return (runtimeSupportedLocales.value as readonly string[]).includes(value);
}

/**
 * Human-readable display name for a locale code, rendered IN THE LOCALE'S
 * OWN language. Falls back to the raw code if no label is registered.
 *
 * Used by LanguageSwitcher and admin SettingsView so both surfaces show
 * identical labels.
 */
export function localeDisplayName(code: LocaleCode | string): string {
  const label = LOCALE_DISPLAY_LABELS[code as LocaleCode];
  return label ?? code;
}

/**
 * Flag emoji for a locale code. Falls back to the globe emoji 🌐 if no flag
 * is registered, so the UI never renders a blank where a flag is expected.
 */
export function localeFlag(code: LocaleCode | string): string {
  const flag = LOCALE_FLAGS[code as LocaleCode];
  return flag ?? '🌐';
}

const i18n = createI18n({
  legacy: false,
  globalInjection: true,
  locale: DEFAULT_LOCALE,
  fallbackLocale: DEFAULT_LOCALE,
  missingWarn: false,
  fallbackWarn: false,
  messages: {
    en,
    zh,
    ja,
  },
});

export default i18n;
