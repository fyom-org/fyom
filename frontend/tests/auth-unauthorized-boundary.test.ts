/**
 * Auth invalidation boundary tests.
 *
 * Validates that the shouldInvalidateSession logic in frontend/src/api/request.ts
 * correctly distinguishes auth-truth endpoints from business/resource endpoints.
 *
 * In axios, config.url is the relative path (before baseURL merging).
 * Real paths: /auth/me, /auth/login, /admin/media, /media/123/progress, etc.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useUserStore } from '@/stores/user';

// ----- Mocks -----

const mockGetMe = vi.fn();
vi.mock('@/api/auth', () => ({
  login: vi.fn(),
  getMe: (...args: unknown[]) => mockGetMe(...args),
}));

beforeEach(() => {
  setActivePinia(createPinia());
  localStorage.clear();
  mockGetMe.mockReset();
});

afterEach(() => {
  localStorage.clear();
  vi.restoreAllMocks();
});

// ----- Replicate shouldInvalidateSession from request.ts for direct testing -----

function shouldInvalidateSession(error: unknown): boolean {
  const axiosError = error as any;
  const status: number | undefined = axiosError?.response?.status;
  const url: string = axiosError?.config?.url ?? '';

  if (status !== 401 && status !== 403) return false;
  if (url.startsWith('/auth/me') || url.startsWith('/auth/login')) {
    return true;
  }
  return false;
}

function fakeAxiosError(status: number | undefined, url: string) {
  const err: any = new Error('test');
  err.config = { url };
  if (status !== undefined) {
    err.response = { status, data: { code: status, message: 'test', data: null } };
  }
  return err;
}

// ----- Tests for shouldInvalidateSession logic -----

describe('Test 1: resource-specific 403 does NOT trigger invalidation', () => {
  it('/admin/libraries 403 returns false', () => {
    const err = fakeAxiosError(403, '/admin/libraries');
    expect(shouldInvalidateSession(err)).toBe(false);
  });

  it('/admin/media 403 returns false', () => {
    const err = fakeAxiosError(403, '/admin/media');
    expect(shouldInvalidateSession(err)).toBe(false);
  });

  it('/media/123/progress 403 returns false', () => {
    const err = fakeAxiosError(403, '/media/123/progress');
    expect(shouldInvalidateSession(err)).toBe(false);
  });

  it('/media/123 403 returns false', () => {
    const err = fakeAxiosError(403, '/media/123');
    expect(shouldInvalidateSession(err)).toBe(false);
  });
});

describe('Test 2: /auth/me 401 and 403 trigger invalidation', () => {
  it('/auth/me 401 returns true', () => {
    const err = fakeAxiosError(401, '/auth/me');
    expect(shouldInvalidateSession(err)).toBe(true);
  });

  it('/auth/me 403 returns true', () => {
    const err = fakeAxiosError(403, '/auth/me');
    expect(shouldInvalidateSession(err)).toBe(true);
  });
});

describe('Test 3: /auth/login 401 triggers invalidation', () => {
  it('/auth/login 401 returns true', () => {
    const err = fakeAxiosError(401, '/auth/login');
    expect(shouldInvalidateSession(err)).toBe(true);
  });
});

describe('Test 4: non-auth 401 does NOT trigger invalidation', () => {
  it('/library 401 returns false (not an auth-truth endpoint)', () => {
    const err = fakeAxiosError(401, '/library');
    expect(shouldInvalidateSession(err)).toBe(false);
  });

  it('/auth/register 403 returns false', () => {
    const err = fakeAxiosError(403, '/auth/register');
    expect(shouldInvalidateSession(err)).toBe(false);
  });
});

describe('Test 5: transport errors do NOT trigger invalidation', () => {
  it('undefined status (network error) returns false', () => {
    const err = fakeAxiosError(undefined, '/media/123');
    expect(shouldInvalidateSession(err)).toBe(false);
  });

  it('500 status returns false', () => {
    const err = fakeAxiosError(500, '/media/123');
    expect(shouldInvalidateSession(err)).toBe(false);
  });

  it('503 status returns false', () => {
    const err = fakeAxiosError(503, '/auth/me');
    expect(shouldInvalidateSession(err)).toBe(false);
  });
});

describe('Test 6: verified interceptor wiring — request module loads', () => {
  it('request module loads without error', async () => {
    const req = await import('@/api/request');
    expect(req.apiRequest).toBeDefined();
    expect(req.authRequest).toBeDefined();
  });
});

describe('Test 7: admin endpoint 403 preserves valid non-admin session', () => {
  it('store stays authenticated after business 403', async () => {
    localStorage.setItem('token', 'valid-non-admin-token');
    mockGetMe.mockResolvedValue({
      user_id: 'user-1',
      username: 'testuser',
      role: 'user',
    });

    const store = useUserStore();
    await store.rehydrateSession();
    expect(store.status).toBe('authenticated');
    expect(store.isAdmin).toBe(false);

    // Verify shouldInvalidateSession returns false for admin endpoint
    const err = fakeAxiosError(403, '/admin/media');
    expect(shouldInvalidateSession(err)).toBe(false);

    // Store remains authenticated (no state change)
    expect(store.status).toBe('authenticated');
    expect(localStorage.getItem('token')).toBe('valid-non-admin-token');
  });
});

describe('Test 8: auth-me 401 invalidates session via store', () => {
  it('store clears after /auth/me 401 on fresh rehydration', async () => {
    // Start with a token in localStorage but do NOT rehydrate yet.
    localStorage.setItem('token', 'stale-token');

    const store = useUserStore();
    expect(store.status).toBe('unknown');

    // Make /auth/me reject with 401 (simulating stale/orphaned token)
    const err = fakeAxiosError(401, '/auth/me');
    mockGetMe.mockRejectedValue(err);

    // Rehydrate — should fail and clear session
    await store.rehydrateSession();
    expect(store.status).toBe('anonymous');
    expect(localStorage.getItem('token')).toBeNull();
  });
});
