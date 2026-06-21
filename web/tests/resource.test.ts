/**
 * Unit tests for resolveResourceUrl
 *
 * Run with: npx vitest run web/tests/resource.test.ts
 * (requires vitest to be installed: pnpm add -D vitest)
 */

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

// Mock the env module before importing the module under test
vi.mock('@/lib/runtime/env', () => ({
  isDesktopMode: vi.fn().mockReturnValue(false),
}));

import { isDesktopMode } from '@/lib/runtime/env';
import { resolveResourceUrl } from '@/lib/runtime/resource';

const mockIsDesktop = isDesktopMode as ReturnType<typeof vi.fn>;

describe('resolveResourceUrl', () => {
  afterEach(() => {
    mockIsDesktop.mockReturnValue(false);
  });

  it('returns empty string for empty input', () => {
    expect(resolveResourceUrl('')).toBe('');
    expect(resolveResourceUrl(undefined)).toBe('');
  });

  it('returns absolute http:// URL unchanged', () => {
    const url = 'http://example.com/image.jpg';
    expect(resolveResourceUrl(url)).toBe(url);
  });

  it('returns absolute https:// URL unchanged', () => {
    const url = 'https://example.com/image.jpg';
    expect(resolveResourceUrl(url)).toBe(url);
  });

  describe('browser mode', () => {
    beforeEach(() => {
      mockIsDesktop.mockReturnValue(false);
    });

    it('keeps relative /api/v1/... paths unchanged', () => {
      const url = '/api/v1/media/123/poster?exp=123456&sig=abc';
      expect(resolveResourceUrl(url)).toBe(url);
    });

    it('keeps other root-relative paths unchanged', () => {
      const url = '/static/image.png';
      expect(resolveResourceUrl(url)).toBe(url);
    });
  });

  describe('Tauri mode', () => {
    beforeEach(() => {
      mockIsDesktop.mockReturnValue(true);
    });

    it('converts /api/v1/... to absolute sidecar URL', () => {
      const input = '/api/v1/media/123/poster?exp=123456&sig=abc';
      const expected = 'http://127.0.0.1:27403/api/v1/media/123/poster?exp=123456&sig=abc';
      expect(resolveResourceUrl(input)).toBe(expected);
    });

    it('preserves query string exactly', () => {
      const input = '/api/v1/media/456/backdrop?exp=999999&sig=xyz123';
      const result = resolveResourceUrl(input);
      expect(result).toContain('?exp=999999&sig=xyz123');
      expect(result.startsWith('http://127.0.0.1:27403')).toBe(true);
    });

    it('converts other root-relative paths to sidecar origin', () => {
      const input = '/static/image.png';
      const expected = 'http://127.0.0.1:27403/static/image.png';
      expect(resolveResourceUrl(input)).toBe(expected);
    });

    it('preserves signature-bearing URLs byte-for-byte (except base)', () => {
      const input = '/api/v1/media/789/poster?exp=1700000000&sig=a1b2c3d4e5f6';
      const result = resolveResourceUrl(input);
      expect(result).toBe(`http://127.0.0.1:27403${input}`);
    });
  });
});
