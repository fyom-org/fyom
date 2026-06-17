/**
 * Locale-aware formatting composable (Phase 8).
 *
 * Problem this solves:
 * Before Phase 8, the codebase scattered `formatDate` / `formatDuration`
 * helpers across SystemView, MediaView, MediaDetailView, and EpisodeList.
 * Each one called `Date.toLocaleDateString(undefined, …)` or hand-rolled
 * `"1h 23m"` strings — `undefined` resolves to the BROWSER locale, NOT the
 * app locale the user picked in the LanguageSwitcher. A user who selected
 * Japanese still saw English-formatted dates and Latin-only duration strings.
 *
 * Solution:
 * This composable exposes a single set of formatters that:
 *   1. Read the reactive app locale from `useLocale().currentLocale`.
 *   2. Re-create the underlying `Intl.*` formatters via `computed` whenever
 *      the locale changes, so every consumer re-renders with the new locale
 *      immediately (no page refresh — same hot-swap contract as `t()`).
 *   3. Share one source of truth, eliminating 4 duplicate implementations.
 *
 * Formatters provided:
 *   - formatDate(iso, opts?)       — date only
 *   - formatDateTime(iso, opts?)   — date + time
 *   - formatTime(iso, opts?)       — time only
 *   - formatRelativeTime(iso)      — "2 hours ago" via Intl.RelativeTimeFormat
 *   - formatDuration(seconds)      — "1h 23m" / "1時間23分" / "1小时23分"
 *   - formatNumber(n, opts?)       — grouped digits per locale
 *   - formatFileSize(bytes)        — "1.5 MB" / "1.5 メガバイト" / "1.5 兆字节"
 *   - formatList(items)            — "a, b and c" via Intl.ListFormat
 *
 * Edge cases mirror the previous per-view helpers so this is a drop-in:
 *   - Invalid / empty ISO strings → fallback string (default '—', '' for
 *     duration to preserve the "hide empty durations" UI contract).
 *   - Non-finite / non-positive durations → '' (empty).
 *   - Negative file sizes → '' (empty).
 *
 * Non-component callers (stores, interceptors) should use the standalone
 * `formatDateForLocale()` / `formatDurationForLocale()` helpers at the bottom
 * of this module, which accept an explicit locale argument and do not require
 * a Vue reactivity context.
 */
import { computed } from 'vue';
import i18n, { DEFAULT_LOCALE, type LocaleCode } from '@/plugins/i18n';
import { isSupportedLocale } from '@/plugins/i18n';

/** BCP-47 tag mapping for Intl APIs. Our LocaleCode is already valid BCP-47. */
function intlLocale(code: LocaleCode): string {
  return code;
}

/** Safe locale getter for use inside computed — always returns a valid LocaleCode. */
function currentAppLocale(): LocaleCode {
  const value = i18n.global.locale.value;
  return isSupportedLocale(value) ? value : DEFAULT_LOCALE;
}

/**
 * Public composable. Returns reactive formatters bound to the current app
 * locale. Call this once per component setup; the returned functions are
 * stable references but read the live locale on each invocation.
 */
export function useLocaleFormat() {
  // The locale is read lazily inside each formatter so that a locale switch
  // is reflected on the next render without needing to re-create the
  // formatter closures themselves. Intl instances are memoized per-locale
  // via the `computed` blocks below.

  const activeLocale = computed<LocaleCode>(() => currentAppLocale());

  // --- DateTime formatters (memoized per locale) ---------------------------
  const dateFormatter = computed(
    () =>
      new Intl.DateTimeFormat(intlLocale(activeLocale.value), {
        month: 'short',
        day: 'numeric',
        year: 'numeric',
      }),
  );

  const dateTimeFormatter = computed(
    () =>
      new Intl.DateTimeFormat(intlLocale(activeLocale.value), {
        month: 'short',
        day: 'numeric',
        year: 'numeric',
        hour: '2-digit',
        minute: '2-digit',
      }),
  );

  const timeFormatter = computed(
    () =>
      new Intl.DateTimeFormat(intlLocale(activeLocale.value), {
        hour: '2-digit',
        minute: '2-digit',
      }),
  );

  const relativeFormatter = computed(
    () =>
      new Intl.RelativeTimeFormat(intlLocale(activeLocale.value), {
        numeric: 'auto',
        style: 'long',
      }),
  );

  const numberFormatter = computed(() => new Intl.NumberFormat(intlLocale(activeLocale.value)));

  const listFormatter = computed(
    () =>
      new Intl.ListFormat(intlLocale(activeLocale.value), {
        style: 'long',
        type: 'conjunction',
      }),
  );

  // --- Public formatter functions -----------------------------------------

  /** Format an ISO date string as a localized date (e.g. "Jan 5, 2026"). */
  function formatDate(iso: string | number | Date, fallback = '—'): string {
    const date = toDate(iso);
    if (!date) return fallback;
    return dateFormatter.value.format(date);
  }

  /** Format an ISO date string as a localized date + time (e.g. "Jan 5, 2026, 14:30"). */
  function formatDateTime(iso: string | number | Date, fallback = '—'): string {
    const date = toDate(iso);
    if (!date) return fallback;
    return dateTimeFormatter.value.format(date);
  }

  /** Format an ISO date string as a localized time only (e.g. "14:30"). */
  function formatTime(iso: string | number | Date, fallback = '—'): string {
    const date = toDate(iso);
    if (!date) return fallback;
    return timeFormatter.value.format(date);
  }

  /**
   * Format an ISO date string as a relative time (e.g. "2 hours ago",
   * "yesterday", "in 3 days"). Uses Intl.RelativeTimeFormat with the
   * appropriate unit automatically selected from the delta magnitude.
   *
   * Returns the fallback string when the input is invalid.
   */
  function formatRelativeTime(iso: string | number | Date, fallback = '—'): string {
    const date = toDate(iso);
    if (!date) return fallback;

    const now = Date.now();
    const deltaMs = date.getTime() - now;
    const absMs = Math.abs(deltaMs);

    const seconds = Math.round(deltaMs / 1000);
    const minutes = Math.round(seconds / 60);
    const hours = Math.round(minutes / 60);
    const days = Math.round(hours / 24);

    const rtf = relativeFormatter.value;

    // Choose the most natural unit. Thresholds follow common practice
    // (relative-time libraries like dayjs use the same ladder).
    if (absMs < 60_000) return rtf.format(seconds, 'second');
    if (absMs < 3_600_000) return rtf.format(minutes, 'minute');
    if (absMs < 86_400_000) return rtf.format(hours, 'hour');
    if (absMs < 2_592_000_000) return rtf.format(days, 'day');

    // Beyond ~30 days, fall back to an absolute date — relative strings
    // like "5 months ago" lose precision and an absolute date is clearer.
    return formatDate(iso, fallback);
  }

  /**
   * Format a duration in seconds as a compact localized string.
   *
   * Examples:
   *   - 0 or negative → '' (empty, preserves the "hide empty durations" UI contract)
   *   - 65 → "1m 5s" / "1分5秒" / "1分5秒"
   *   - 3725 → "1h 2m 5s" / "1時間2分5秒" / "1小时2分5秒"
   *
   * Uses the `format.duration{Hours,Minutes,Seconds}` i18n keys for the unit
   * abbreviations so each locale renders its native units.
   */
  function formatDuration(seconds: number): string {
    if (!Number.isFinite(seconds) || seconds <= 0) return '';

    const totalSeconds = Math.floor(seconds);
    const hours = Math.floor(totalSeconds / 3600);
    const minutes = Math.floor((totalSeconds % 3600) / 60);
    const secs = totalSeconds % 60;

    const parts: string[] = [];

    if (hours > 0) {
      parts.push(i18n.global.t('format.durationHours', { n: hours }));
    }
    if (minutes > 0) {
      parts.push(i18n.global.t('format.durationMinutes', { n: minutes }));
    }
    if (secs > 0 && hours === 0) {
      // Only show seconds when there are no hours — avoids "1h 0m 5s".
      parts.push(i18n.global.t('format.durationSeconds', { n: secs }));
    }

    // If everything was zero (e.g. input 0.4 rounded down), show "0m".
    if (parts.length === 0) {
      parts.push(i18n.global.t('format.durationMinutes', { n: 0 }));
    }

    // Locale-aware join: English uses a space ("1h 2m"), Japanese/Chinese
    // use no separator ("1時間2分"). The separator is an i18n key so each
    // locale controls its own typography.
    const separator = i18n.global.t('format.durationSeparator');
    return parts.join(separator);
  }

  /** Format a number with locale-aware grouping (e.g. "1,234" / "1.234"). */
  function formatNumber(n: number, fallback = ''): string {
    if (!Number.isFinite(n)) return fallback;
    return numberFormatter.value.format(n);
  }

  /**
   * Format a byte count as a localized file size with the appropriate unit.
   *
   * Examples: 1536 → "1.5 KB" / "1.5 キロバイト" / "1.5 千字节"
   *
   * Uses binary (1024-based) units, matching the prior convention. The unit
   * labels come from the `format.fileSize{Unit}` i18n keys so each locale
   * shows its native abbreviation.
   */
  function formatFileSize(bytes: number, fallback = ''): string {
    if (!Number.isFinite(bytes) || bytes < 0) return fallback;
    if (bytes === 0) return i18n.global.t('format.fileSizeBytes', { n: 0 });

    const units: Array<{ threshold: number; key: string }> = [
      { threshold: 1024 ** 5, key: 'format.fileSizePB' },
      { threshold: 1024 ** 4, key: 'format.fileSizeTB' },
      { threshold: 1024 ** 3, key: 'format.fileSizeGB' },
      { threshold: 1024 ** 2, key: 'format.fileSizeMB' },
      { threshold: 1024, key: 'format.fileSizeKB' },
      { threshold: 1, key: 'format.fileSizeBytes' },
    ];

    let value = bytes;
    let unitKey = 'format.fileSizeBytes';

    for (const u of units) {
      if (bytes >= u.threshold) {
        value = bytes / u.threshold;
        unitKey = u.key;
        break;
      }
    }

    // Two significant decimals, but drop trailing zeros (1.50 → 1.5, 2.00 → 2).
    const rounded = Math.round(value * 100) / 100;
    const display = Number.isInteger(rounded) ? rounded.toString() : rounded.toFixed(2).replace(/0+$/, '').replace(/\.$/, '');

    return i18n.global.t(unitKey, { n: display });
  }

  /** Format an array of strings as a localized conjunction list ("a, b and c"). */
  function formatList(items: ReadonlyArray<string>): string {
    return listFormatter.value.format([...items]);
  }

  return {
    /** Reactive ref to the active locale (re-evaluates on locale switch). */
    activeLocale,
    formatDate,
    formatDateTime,
    formatTime,
    formatRelativeTime,
    formatDuration,
    formatNumber,
    formatFileSize,
    formatList,
  };
}

// --- Standalone helpers (non-reactive, for stores / interceptors) --------

/**
 * Format a duration for a specific locale, WITHOUT Vue reactivity.
 *
 * Use this from Pinia actions, axios interceptors, or anywhere outside a
 * component setup where the `useLocaleFormat()` composable is unavailable.
 */
export function formatDurationForLocale(locale: LocaleCode, seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return '';

  // Temporarily read the unit keys via i18n.global.t with the requested locale.
  // We do NOT mutate the global locale — instead we compose the string using
  // the messages for the requested locale directly.
  const messages = i18n.global.messages.value[locale] ?? i18n.global.messages.value[DEFAULT_LOCALE];
  // vue-i18n message values may be plain strings (raw JSON imports) or
  // compiled message functions (composition mode). Cast through `unknown`
  // because the union is intentionally permissive — the `resolve()` helper
  // below handles both shapes at runtime.
  const fmt = messages?.format as unknown as
    | { durationHours?: string | ((n: number) => string); durationMinutes?: string | ((n: number) => string); durationSeconds?: string | ((n: number) => string); durationSeparator?: string | (() => string) }
    | undefined;

  // The JSON message values are template strings like "{n}h". We resolve them
  // by replacing the {n} placeholder manually (vue-i18n's compiled message
  // functions live on the locale messages when using composition mode, but
  // raw JSON imports keep them as strings — so we handle both shapes).
  const totalSeconds = Math.floor(seconds);
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const secs = totalSeconds % 60;

  const parts: string[] = [];

  const resolve = (raw: unknown, n: number): string => {
    if (typeof raw === 'function') return raw(n);
    if (typeof raw === 'string') return raw.replace(/\{n\}/g, String(n));
    return String(n);
  };

  if (hours > 0 && fmt?.durationHours) parts.push(resolve(fmt.durationHours, hours));
  if (minutes > 0 && fmt?.durationMinutes) parts.push(resolve(fmt.durationMinutes, minutes));
  if (secs > 0 && hours === 0 && fmt?.durationSeconds) parts.push(resolve(fmt.durationSeconds, secs));
  if (parts.length === 0 && fmt?.durationMinutes) parts.push(resolve(fmt.durationMinutes, 0));

  // Locale-aware separator (en: " ", ja/zh: "").
  const rawSep = fmt?.durationSeparator;
  const separator = typeof rawSep === 'function' ? rawSep() : typeof rawSep === 'string' ? rawSep : ' ';
  return parts.join(separator);
}

// --- Internal helpers ----------------------------------------------------

/** Coerce an ISO string / timestamp / Date into a Date, or null if invalid. */
function toDate(input: string | number | Date): Date | null {
  if (!input && input !== 0) return null;

  const date = input instanceof Date ? input : new Date(input);
  if (Number.isNaN(date.getTime())) return null;

  return date;
}
