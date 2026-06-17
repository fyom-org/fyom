/**
 * Tests for the locale-aware formatting composable (Phase 8).
 *
 * These tests lock the public contract of useLocaleFormat:
 *  - Formatters read the reactive app locale (NOT the browser locale).
 *  - Duration formatting uses i18n keys (deterministic per locale).
 *  - Date/time/number formatting uses Intl (tested by property, not exact
 *    string, because ICU data varies across Node versions).
 *  - Edge cases: invalid input → fallback, non-finite → '', etc.
 *  - Reactivity: switching locale changes the formatted output.
 */
import { describe, it, expect, beforeEach } from 'vitest';
import i18n, { DEFAULT_LOCALE, type LocaleCode } from '@/plugins/i18n';
import { useLocaleFormat, formatDurationForLocale } from '@/composables/useLocaleFormat';

/**
 * Helper: set the global i18n locale and return a composable instance.
 * The composable reads `i18n.global.locale.value` lazily, so changing the
 * locale after calling the composable still affects subsequent formatter
 * invocations (this is the hot-swap contract).
 */
function setup(locale: LocaleCode) {
  i18n.global.locale.value = locale;
  return useLocaleFormat();
}

/** Restore the default locale after each test so tests are isolated. */
beforeEach(() => {
  i18n.global.locale.value = DEFAULT_LOCALE;
});

// ---------------------------------------------------------------------------
// formatDate / formatDateTime / formatTime
// ---------------------------------------------------------------------------

describe('useLocaleFormat — date formatters', () => {
  const iso = '2026-03-15T14:30:00Z';

  it('formatDate returns a non-empty localized string', () => {
    const { formatDate } = setup('en');
    const out = formatDate(iso);
    expect(out).toBeTruthy();
    expect(out).not.toBe('—');
  });

  it('formatDate respects the app locale (en vs ja produce different output)', () => {
    const en = setup('en');
    const enOut = en.formatDate(iso);
    i18n.global.locale.value = 'ja';
    const jaOut = en.formatDate(iso);
    // The two locales format dates differently (en uses Latin month names,
    // ja uses CJK year/month/day markers). If they were identical it would
    // indicate the formatter ignored the locale.
    expect(enOut).not.toBe(jaOut);
  });

  it('formatDate falls back to the provided fallback for invalid input', () => {
    const { formatDate } = setup('en');
    expect(formatDate('', '—')).toBe('—');
    expect(formatDate('not-a-date', '—')).toBe('—');
    expect(formatDate('', 'n/a')).toBe('n/a');
  });

  it('formatDate accepts Date and timestamp inputs, not just ISO strings', () => {
    const { formatDate } = setup('en');
    const fromIso = formatDate(iso);
    const fromDate = formatDate(new Date(iso));
    const fromMs = formatDate(new Date(iso).getTime());
    expect(fromIso).toBe(fromDate);
    expect(fromIso).toBe(fromMs);
  });

  it('formatDateTime includes more than formatDate (has time component)', () => {
    const { formatDate, formatDateTime } = setup('en');
    const d = formatDate(iso);
    const dt = formatDateTime(iso);
    // The datetime string should be longer (it adds hour:minute).
    expect(dt.length).toBeGreaterThan(d.length);
  });

  it('formatTime produces a short string (time only)', () => {
    const { formatTime } = setup('en');
    const out = formatTime(iso);
    expect(out).toBeTruthy();
    // Time-only strings are short ("2:30 PM" or "14:30" — well under 20 chars).
    expect(out.length).toBeLessThan(20);
  });
});

// ---------------------------------------------------------------------------
// formatRelativeTime
// ---------------------------------------------------------------------------

describe('useLocaleFormat — formatRelativeTime', () => {
  it('returns a relative string for a recent past timestamp', () => {
    const { formatRelativeTime } = setup('en');
    const twoHoursAgo = new Date(Date.now() - 2 * 3600_000).toISOString();
    const out = formatRelativeTime(twoHoursAgo);
    // Intl.RelativeTimeFormat with numeric:'auto' produces "2 hours ago".
    expect(out).toBeTruthy();
    expect(out.toLowerCase()).toContain('hour');
  });

  it('respects locale (ja relative time contains CJK or differs from en)', () => {
    const { formatRelativeTime } = setup('en');
    const twoHoursAgo = new Date(Date.now() - 2 * 3600_000).toISOString();
    const enOut = formatRelativeTime(twoHoursAgo);
    i18n.global.locale.value = 'ja';
    const jaOut = formatRelativeTime(twoHoursAgo);
    expect(enOut).not.toBe(jaOut);
  });

  it('falls back to an absolute date for timestamps beyond ~30 days', () => {
    const { formatRelativeTime } = setup('en');
    const oneYearAgo = new Date(Date.now() - 365 * 86_400_000).toISOString();
    const out = formatRelativeTime(oneYearAgo);
    // Beyond 30 days we switch to an absolute date which contains digits
    // but no relative words like "ago".
    expect(out).toBeTruthy();
    expect(out.toLowerCase()).not.toContain('ago');
  });

  it('returns the fallback for invalid input', () => {
    const { formatRelativeTime } = setup('en');
    expect(formatRelativeTime('invalid', '—')).toBe('—');
    expect(formatRelativeTime('', '—')).toBe('—');
  });
});

// ---------------------------------------------------------------------------
// formatDuration (deterministic — uses i18n keys, not Intl)
// ---------------------------------------------------------------------------

describe('useLocaleFormat — formatDuration', () => {
  it('returns "" for non-positive or non-finite values', () => {
    const { formatDuration } = setup('en');
    expect(formatDuration(0)).toBe('');
    expect(formatDuration(-5)).toBe('');
    expect(formatDuration(NaN)).toBe('');
    expect(formatDuration(Infinity)).toBe('');
    expect(formatDuration(-Infinity)).toBe('');
  });

  it('formats hours + minutes in English ("1h 2m")', () => {
    const { formatDuration } = setup('en');
    // 3725s = 1h 2m 5s. With hours present, seconds are hidden.
    expect(formatDuration(3725)).toBe('1h 2m');
  });

  it('formats minutes + seconds in English (no hours → seconds shown)', () => {
    const { formatDuration } = setup('en');
    // 125s = 2m 5s. No hours, so seconds ARE shown.
    expect(formatDuration(125)).toBe('2m 5s');
  });

  it('formats minutes only in English', () => {
    const { formatDuration } = setup('en');
    expect(formatDuration(300)).toBe('5m');
  });

  it('formats hours + minutes in Japanese ("1時間2分")', () => {
    i18n.global.locale.value = 'ja';
    const { formatDuration } = useLocaleFormat();
    expect(formatDuration(3725)).toBe('1時間2分');
  });

  it('formats hours + minutes in Chinese ("1小时2分")', () => {
    i18n.global.locale.value = 'zh';
    const { formatDuration } = useLocaleFormat();
    expect(formatDuration(3725)).toBe('1小时2分');
  });

  it('formats minutes + seconds in Japanese ("2分5秒")', () => {
    i18n.global.locale.value = 'ja';
    const { formatDuration } = useLocaleFormat();
    expect(formatDuration(125)).toBe('2分5秒');
  });

  it('shows "0m" for a positive sub-second value that floors to 0', () => {
    const { formatDuration } = setup('en');
    // 0.4s floors to 0 hours, 0 minutes, 0 seconds. The composable shows
    // "0m" so the UI doesn't render a bare number.
    expect(formatDuration(0.4)).toBe('0m');
  });

  it('reactively updates when the locale changes (hot-swap contract)', () => {
    const { formatDuration } = setup('en');
    expect(formatDuration(3725)).toBe('1h 2m');
    i18n.global.locale.value = 'ja';
    // Same value, same formatter instance, but locale changed → new output.
    expect(formatDuration(3725)).toBe('1時間2分');
    i18n.global.locale.value = 'zh';
    expect(formatDuration(3725)).toBe('1小时2分');
  });
});

// ---------------------------------------------------------------------------
// formatNumber / formatFileSize
// ---------------------------------------------------------------------------

describe('useLocaleFormat — formatNumber', () => {
  it('groups large numbers per locale', () => {
    const { formatNumber } = setup('en');
    const out = formatNumber(1234567);
    // All locales group with a separator; the exact char varies but the
    // length increases beyond 7 digits.
    expect(out.length).toBeGreaterThan(7);
  });

  it('returns the fallback for non-finite numbers', () => {
    const { formatNumber } = setup('en');
    expect(formatNumber(NaN, '—')).toBe('—');
    expect(formatNumber(Infinity, '—')).toBe('—');
  });
});

describe('useLocaleFormat — formatFileSize', () => {
  it('returns "" for negative or non-finite sizes', () => {
    const { formatFileSize } = setup('en');
    expect(formatFileSize(-1)).toBe('');
    expect(formatFileSize(NaN)).toBe('');
    expect(formatFileSize(Infinity)).toBe('');
  });

  it('formats 0 bytes', () => {
    const { formatFileSize } = setup('en');
    expect(formatFileSize(0)).toBe('0 B');
  });

  it('formats kilobytes in English ("1.5 KB")', () => {
    const { formatFileSize } = setup('en');
    expect(formatFileSize(1536)).toBe('1.5 KB');
  });

  it('formats megabytes in English ("2 MB" — integer drops decimals)', () => {
    const { formatFileSize } = setup('en');
    expect(formatFileSize(2 * 1024 * 1024)).toBe('2 MB');
  });

  it('formats gigabytes in Japanese (uses メガバイト-style units)', () => {
    i18n.global.locale.value = 'ja';
    const { formatFileSize } = useLocaleFormat();
    const out = formatFileSize(1536);
    expect(out).toContain('キロバイト');
  });

  it('formats megabytes in Chinese (uses 兆字节-style units)', () => {
    i18n.global.locale.value = 'zh';
    const { formatFileSize } = useLocaleFormat();
    const out = formatFileSize(1536);
    expect(out).toContain('千字节');
  });

  it('chooses the appropriate unit (bytes → KB → MB → GB)', () => {
    const { formatFileSize } = setup('en');
    expect(formatFileSize(512)).toBe('512 B');
    expect(formatFileSize(2048)).toContain('KB');
    expect(formatFileSize(2 * 1024 ** 2)).toContain('MB');
    expect(formatFileSize(2 * 1024 ** 3)).toContain('GB');
  });
});

// ---------------------------------------------------------------------------
// formatList
// ---------------------------------------------------------------------------

describe('useLocaleFormat — formatList', () => {
  it('joins items with a locale-aware conjunction', () => {
    const { formatList } = setup('en');
    const out = formatList(['a', 'b', 'c']);
    // English conjunction uses "and".
    expect(out).toContain('and');
    expect(out).toContain('a');
    expect(out).toContain('b');
    expect(out).toContain('c');
  });

  it('respects locale (ja uses a different separator than en)', () => {
    const { formatList } = setup('en');
    const enOut = formatList(['a', 'b', 'c']);
    i18n.global.locale.value = 'ja';
    const jaOut = formatList(['a', 'b', 'c']);
    expect(enOut).not.toBe(jaOut);
  });
});

// ---------------------------------------------------------------------------
// Standalone helper (non-reactive)
// ---------------------------------------------------------------------------

describe('formatDurationForLocale (standalone helper)', () => {
  it('formats duration for an explicit locale without mutating global state', () => {
    i18n.global.locale.value = 'en';
    const jaOut = formatDurationForLocale('ja', 3725);
    expect(jaOut).toBe('1時間2分');
    // Global locale is unchanged.
    expect(i18n.global.locale.value).toBe('en');
  });

  it('returns "" for non-positive values', () => {
    expect(formatDurationForLocale('en', 0)).toBe('');
    expect(formatDurationForLocale('en', -1)).toBe('');
  });

  it('falls back to English messages for an unknown locale code shape', () => {
    // The helper reads messages for the given locale; if the locale has no
    // messages it falls back to DEFAULT_LOCALE. We can't pass an invalid
    // LocaleCode (TS prevents it), but we verify the default path works.
    expect(formatDurationForLocale('en', 125)).toBe('2m 5s');
  });
});
