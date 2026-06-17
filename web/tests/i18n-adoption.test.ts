/**
 * Phase 11 regression tests — locks the locale-aware formatting adoption
 * across the surfaces that were migrated in Phase 11.
 *
 * What these tests lock:
 *  1. The new `library.resultCount` plural key exists in all 3 locales and
 *     respects vue-i18n plural rules (0 → "no items", 1 → "1 item", N → "N items").
 *  2. The new `library.playedCount` key exists in all 3 locales.
 *  3. `formatNumber` produces locale-aware grouping for large counts
 *     (the value embedded into `resultCount`'s {n} placeholder).
 *  4. The `format` namespace (Phase 8) still has full parity across locales.
 *  5. `syncDocumentLang` (extended in Phase 11) sets both `lang` and `dir`.
 *
 * These are pure unit tests — no component mounting required. They guard
 * against accidental key deletion or locale-parity regressions.
 */
import { describe, it, expect, beforeEach } from 'vitest';
import i18n, { DEFAULT_LOCALE, BUNDLED_LOCALES, type LocaleCode } from '@/plugins/i18n';
import { useLocaleFormat } from '@/composables/useLocaleFormat';
import { setGlobalLocale } from '@/composables/useLocale';

/** Restore the default locale after each test so tests are isolated. */
beforeEach(() => {
  i18n.global.locale.value = DEFAULT_LOCALE;
});

// ---------------------------------------------------------------------------
// library.resultCount plural key
// ---------------------------------------------------------------------------

describe('Phase 11 — library.resultCount manual plural keys', () => {
  // Phase 11 uses 3 separate keys (Zero / One / Many) instead of vue-i18n's
  // pipe plural syntax, because the pipe syntax auto-fills {n} with the RAW
  // count, preventing locale-aware number formatting (e.g. "1,234" grouping).
  // The manual approach lets us pass formatNumber() output into {n}.

  function buildResultLabel(locale: LocaleCode, count: number): string {
    i18n.global.locale.value = locale;
    const { formatNumber } = useLocaleFormat();
    const n = formatNumber(count);
    if (count === 0) return i18n.global.t('library.resultCountZero');
    if (count === 1) return i18n.global.t('library.resultCountOne', { n });
    return i18n.global.t('library.resultCountMany', { n });
  }

  const cases: Array<{ locale: LocaleCode; count: number; expectContains: string }> = [
    { locale: 'en', count: 0, expectContains: 'no items' },
    { locale: 'en', count: 1, expectContains: '1 item' },
    { locale: 'en', count: 5, expectContains: '5 items' },
    { locale: 'en', count: 1234, expectContains: '1,234 items' },
    { locale: 'zh', count: 0, expectContains: '无项目' },
    { locale: 'zh', count: 1, expectContains: '1 项' },
    { locale: 'zh', count: 5, expectContains: '5 项' },
    { locale: 'ja', count: 0, expectContains: 'アイテムなし' },
    { locale: 'ja', count: 1, expectContains: '1 件' },
    { locale: 'ja', count: 5, expectContains: '5 件' },
  ];

  for (const { locale, count, expectContains } of cases) {
    it(`locale=${locale} count=${count} → contains "${expectContains}"`, () => {
      const out = buildResultLabel(locale, count);
      expect(out).toContain(expectContains);
    });
  }

  it('English singular vs plural: 1 → "item", 2 → "items"', () => {
    const singular = buildResultLabel('en', 1);
    const plural = buildResultLabel('en', 2);
    expect(singular).toContain('1 item');
    expect(plural).toContain('2 items');
    // The bug we fixed: the old code was `${count} item${count === 1 ? '' : 's'}`
    // which had no "no items" branch for count=0. The new key handles 0 explicitly.
    const zero = buildResultLabel('en', 0);
    expect(zero).toContain('no items');
  });

  it('large count uses locale-aware grouping (en: 1234 → "1,234")', () => {
    const out = buildResultLabel('en', 1234);
    expect(out).toContain('1,234');
    // The old code would have produced "1234 items" (no comma).
    expect(out).not.toContain('1234 items');
  });
});

// ---------------------------------------------------------------------------
// library.playedCount key
// ---------------------------------------------------------------------------

describe('Phase 11 — library.playedCount key', () => {
  const cases: Array<{ locale: LocaleCode; count: number; expectContains: string }> = [
    { locale: 'en', count: 3, expectContains: '3x' },
    { locale: 'zh', count: 3, expectContains: '3 次' },
    { locale: 'ja', count: 3, expectContains: '3 回' },
  ];

  for (const { locale, count, expectContains } of cases) {
    it(`locale=${locale} count=${count} → contains "${expectContains}"`, () => {
      i18n.global.locale.value = locale;
      const { formatNumber } = useLocaleFormat();
      const out = i18n.global.t('library.playedCount', { n: formatNumber(count) });
      expect(out).toContain(expectContains);
    });
  }
});

// ---------------------------------------------------------------------------
// formatNumber grouping (used by resultCount and library stats)
// ---------------------------------------------------------------------------

describe('Phase 11 — formatNumber grouping for large counts', () => {
  it('en: 1234 → contains a thousands separator', () => {
    i18n.global.locale.value = 'en';
    const { formatNumber } = useLocaleFormat();
    const out = formatNumber(1234);
    // en uses comma grouping: "1,234"
    expect(out).toMatch(/1[,.]234/);
  });

  it('ja: 1234 → grouped (locale-aware separator)', () => {
    i18n.global.locale.value = 'ja';
    const { formatNumber } = useLocaleFormat();
    const out = formatNumber(1234);
    // ja also groups: "1,234" (Intl.NumberFormat for ja-JP uses comma)
    expect(out).toMatch(/1[,.\s]234/);
    // Must differ from the raw toString() which has no grouping.
    expect(out).not.toBe('1234');
  });

  it('0 → "0" in all locales (no NaN, no empty)', () => {
    for (const locale of BUNDLED_LOCALES) {
      i18n.global.locale.value = locale;
      const { formatNumber } = useLocaleFormat();
      expect(formatNumber(0)).toBe('0');
    }
  });

  it('NaN → fallback (empty string) in all locales', () => {
    for (const locale of BUNDLED_LOCALES) {
      i18n.global.locale.value = locale;
      const { formatNumber } = useLocaleFormat();
      expect(formatNumber(Number.NaN)).toBe('');
    }
  });
});

// ---------------------------------------------------------------------------
// Locale parity: all 3 locales have the same leaf keys
// ---------------------------------------------------------------------------

describe('Phase 11 — locale JSON parity', () => {
  function loadLocaleMessages(locale: LocaleCode): Record<string, unknown> {
    // vue-i18n v9 composition mode: use the public getLocaleMessage API.
    const messages = i18n.global.getLocaleMessage(locale) as Record<string, unknown>;
    if (!messages || Object.keys(messages).length === 0) {
      throw new Error(`No messages loaded for locale "${locale}"`);
    }
    return messages;
  }

  function collectLeafKeys(obj: unknown, prefix = ''): string[] {
    if (obj === null || typeof obj !== 'object') return [];
    if (Array.isArray(obj)) return [prefix]; // plural arrays count as one key

    const out: string[] = [];
    for (const [k, v] of Object.entries(obj as Record<string, unknown>)) {
      const key = prefix ? `${prefix}.${k}` : k;
      if (v !== null && typeof v === 'object') {
        out.push(...collectLeafKeys(v, key));
      } else {
        out.push(key);
      }
    }
    return out;
  }

  it('all 3 locales have the same leaf key set', () => {
    const enKeys = new Set(collectLeafKeys(loadLocaleMessages('en')));
    const zhKeys = new Set(collectLeafKeys(loadLocaleMessages('zh')));
    const jaKeys = new Set(collectLeafKeys(loadLocaleMessages('ja')));

    const missingInZh = [...enKeys].filter((k) => !zhKeys.has(k));
    const missingInJa = [...enKeys].filter((k) => !jaKeys.has(k));
    const extraInZh = [...zhKeys].filter((k) => !enKeys.has(k));
    const extraInJa = [...jaKeys].filter((k) => !enKeys.has(k));

    expect({ missingInZh, missingInJa, extraInZh, extraInJa }).toEqual({
      missingInZh: [],
      missingInJa: [],
      extraInZh: [],
      extraInJa: [],
    });
  });

  it('library.resultCountZero / One / Many exist in all 3 locales', () => {
    for (const locale of BUNDLED_LOCALES) {
      const messages = loadLocaleMessages(locale);
      const lib = messages.library as Record<string, unknown>;
      expect(lib.resultCountZero, `locale=${locale}`).toBeTruthy();
      expect(lib.resultCountOne, `locale=${locale}`).toBeTruthy();
      expect(lib.resultCountMany, `locale=${locale}`).toBeTruthy();
    }
  });

  it('library.playedCount exists in all 3 locales', () => {
    for (const locale of BUNDLED_LOCALES) {
      const messages = loadLocaleMessages(locale);
      const lib = messages.library as Record<string, unknown>;
      expect(lib.playedCount).toBeTruthy();
    }
  });

  it('format namespace exists with all 10 keys in all 3 locales', () => {
    // Note: durationSeparator is intentionally "" for zh/ja (CJK locales
    // don't use a separator between duration units like "1時間2分"). So we
    // check key existence via `in` rather than truthiness.
    const expectedKeys = [
      'durationHours',
      'durationMinutes',
      'durationSeconds',
      'fileSizeBytes',
      'fileSizeKB',
      'fileSizeMB',
      'fileSizeGB',
      'fileSizeTB',
      'fileSizePB',
      'durationSeparator',
    ];
    for (const locale of BUNDLED_LOCALES) {
      const messages = loadLocaleMessages(locale);
      const format = messages.format as Record<string, unknown>;
      for (const key of expectedKeys) {
        expect(key in format, `locale=${locale} key=format.${key} missing`).toBe(true);
      }
    }
  });
});

// ---------------------------------------------------------------------------
// syncDocumentLang sets both lang and dir (Phase 11 RTL-readiness)
// ---------------------------------------------------------------------------

describe('Phase 11 — syncDocumentLang sets <html lang> and <html dir>', () => {
  it('setGlobalLocale("en") → html lang="en" dir="ltr"', () => {
    setGlobalLocale('en');
    expect(document.documentElement.lang).toBe('en');
    expect(document.documentElement.dir).toBe('ltr');
  });

  it('setGlobalLocale("ja") → html lang="ja" dir="ltr"', () => {
    setGlobalLocale('ja');
    expect(document.documentElement.lang).toBe('ja');
    expect(document.documentElement.dir).toBe('ltr');
  });

  it('setGlobalLocale("zh") → html lang="zh" dir="ltr"', () => {
    setGlobalLocale('zh');
    expect(document.documentElement.lang).toBe('zh');
    expect(document.documentElement.dir).toBe('ltr');
  });
});
