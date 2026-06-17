/**
 * Auth lifecycle rehydration tests.
 *
 * Covers session restore, stale-token rejection, network-error tolerance,
 * and route-guard consistency for admin and non-admin paths.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useUserStore, type AuthStatus } from '@/stores/user';

// ----- Mocks -----

const mockGetMe = vi.fn();
vi.mock('@/api/auth', () => ({
  login: vi.fn(),
  getMe: (...args: unknown[]) => mockGetMe(...args),
}));

// Capture dispatched auth:unauthorized events
let unauthorizedHandler: ((event: Event) => void) | null = null;
const originalAddEventListener = window.addEventListener;
const originalRemoveEventListener = window.removeEventListener;

beforeEach(() => {
  // Fresh pinia per test
  setActivePinia(createPinia());

  // Clean localStorage
  localStorage.clear();

  // Reset mock
  mockGetMe.mockReset();

  // Stub event listeners so the store's auth:unauthorized handler is captured
  vi.spyOn(window, 'addEventListener').mockImplementation(
    (event: string, handler: any) => {
      if (event === 'auth:unauthorized') {
        unauthorizedHandler = handler;
      }
      originalAddEventListener.call(window, event, handler);
    }
  );
  vi.spyOn(window, 'removeEventListener').mockImplementation(
    (event: string, handler: any) => {
      if (event === 'auth:unauthorized' && handler === unauthorizedHandler) {
        unauthorizedHandler = null;
      }
      originalRemoveEventListener.call(window, event, handler);
    }
  );
});

afterEach(() => {
  localStorage.clear();
  unauthorizedHandler = null;
  vi.restoreAllMocks();
});

// ----- Helpers -----

function mockMeSuccess(overrides: { role?: string; user_id?: string; username?: string } = {}) {
  // getMe() in @/api/auth unwraps the axios response envelope and returns
  // the User object directly (see normalizeUser()). The mock must match
  // that contract — return the User, NOT { data: User }.
  mockGetMe.mockResolvedValue({
    user_id: overrides.user_id ?? 'user-1',
    username: overrides.username ?? 'testuser',
    role: overrides.role ?? 'user',
  });
}

function mockMeUnauthorized() {
  const err: any = new Error('unauthorized');
  err.response = { status: 401, data: { code: 401, message: 'unauthorized', data: null } };
  mockGetMe.mockRejectedValue(err);
}

function mockMeForbidden() {
  const err: any = new Error('forbidden');
  err.response = { status: 403, data: { code: 403, message: 'forbidden', data: null } };
  mockGetMe.mockRejectedValue(err);
}

function mockMeNetworkError() {
  const err: any = new Error('Network Error');
  err.request = {}; // axios sets request on network error
  err.response = undefined; // no response
  mockGetMe.mockRejectedValue(err);
}

// ----- Tests -----

describe('Test 1: restores valid persisted session on cold start', () => {
  it('store enters rehydrating then authenticated; token is kept', async () => {
    // Simulate token already in localStorage from previous session
    localStorage.setItem('token', 'persisted-jwt');
    mockMeSuccess();

    const store = useUserStore();
    expect(store.status).toBe('unknown');
    expect(store.token).toBe('persisted-jwt');

    await store.rehydrateSession();
    expect('rehydrating' as AuthStatus).toBe('rehydrating');
    expect(store.status).toBe('authenticated');
    expect(store.token).toBe('persisted-jwt');
    expect(store.user).toEqual({ user_id: 'user-1', username: 'testuser', role: 'user' });
    expect(store.isAuthenticated).toBe(true);
    expect(store.isAuthReady).toBe(true);
    expect(localStorage.getItem('token')).toBe('persisted-jwt');
  });
});

describe('Test 2: clears stale token on unauthorized rehydration', () => {
  it('token is removed; store becomes anonymous', async () => {
    localStorage.setItem('token', 'stale-jwt');
    mockMeUnauthorized();

    const store = useUserStore();
    await store.rehydrateSession();

    expect(store.status).toBe('anonymous');
    expect(store.token).toBeNull();
    expect(store.user).toBeNull();
    expect(localStorage.getItem('token')).toBeNull();
    expect(store.isAuthenticated).toBe(false);
    expect(store.isAuthReady).toBe(true);
  });
});

describe('Test 3: does not clear token on network error during bootstrap', () => {
  it('token is not blindly removed on network failure', async () => {
    localStorage.setItem('token', 'valid-but-offline-jwt');
    mockMeNetworkError();

    const store = useUserStore();
    await store.rehydrateSession();

    // Network error = token stays (may still be valid)
    expect(localStorage.getItem('token')).toBe('valid-but-offline-jwt');
    expect(store.token).toBe('valid-but-offline-jwt');
    // Status stays rehydrating (unresolved) — not authenticated, not anonymous
    expect(store.status).toBe('rehydrating');
    expect(store.isAuthenticated).toBe(false);
  });
});

describe('Test 4: non-admin protected route restores valid session without forced relogin', () => {
  it('rehydrateSession coalesces concurrent calls and settles authenticated', async () => {
    localStorage.setItem('token', 'persisted-jwt');
    mockMeSuccess({ role: 'user' });

    const store = useUserStore();

    // Simulate multiple navigations firing rehydration concurrently
    const p1 = store.rehydrateSession();
    const p2 = store.rehydrateSession();
    const p3 = store.rehydrateSession();

    await Promise.all([p1, p2, p3]);

    expect(store.status).toBe('authenticated');
    expect(store.isAdmin).toBe(false);
  });
});

describe('Test 5: admin route requires rehydrated admin user; rejects stale session', () => {
  it('rejects token with 401 (user deleted)', async () => {
    localStorage.setItem('token', 'deleted-user-jwt');
    mockMeUnauthorized();

    const store = useUserStore();
    await store.rehydrateSession();

    expect(store.status).toBe('anonymous');
    expect(store.isAdmin).toBe(false);
  });

  it('rejects token with role=user (not admin)', async () => {
    localStorage.setItem('token', 'user-role-jwt');
    mockMeSuccess({ role: 'user' });

    const store = useUserStore();
    await store.rehydrateSession();

    expect(store.status).toBe('authenticated');
    expect(store.isAdmin).toBe(false);
    expect(store.user?.role).toBe('user');
  });

  it('accepts admin token with role=admin', async () => {
    localStorage.setItem('token', 'admin-jwt');
    mockMeSuccess({ role: 'admin', username: 'admin' });

    const store = useUserStore();
    await store.rehydrateSession();

    expect(store.status).toBe('authenticated');
    expect(store.isAdmin).toBe(true);
    expect(store.user?.role).toBe('admin');
  });
});

describe('Test 6: unauthorized response on later API call clears stale session', () => {
  it('window event clears session from authenticated to anonymous', async () => {
    localStorage.setItem('token', 'session-api-token');
    mockMeSuccess();

    const store = useUserStore();
    await store.rehydrateSession();
    expect(store.status).toBe('authenticated');

    // Simulate request.ts interceptor dispatching auth:unauthorized on 401.
    // Pass a proper CustomEvent so the handler can read .detail.status.
    unauthorizedHandler!(new CustomEvent('auth:unauthorized', { detail: { status: 401 } }));

    expect(store.status).toBe('anonymous');
    expect(store.token).toBeNull();
    expect(store.user).toBeNull();
    expect(localStorage.getItem('token')).toBeNull();
  });
});

describe('Test 7: 403 response also clears stale session', () => {
  it('403 during rehydration clears token and enters anonymous', async () => {
    localStorage.setItem('token', 'forbidden-jwt');
    mockMeForbidden();

    const store = useUserStore();
    await store.rehydrateSession();

    expect(store.status).toBe('anonymous');
    expect(localStorage.getItem('token')).toBeNull();
  });
});

describe('Test 8: rehydration is idempotent after success', () => {
  it('second rehydrate call is a no-op when already authenticated', async () => {
    localStorage.setItem('token', 'good-jwt');
    mockMeSuccess();

    const store = useUserStore();
    await store.rehydrateSession();
    expect(store.status).toBe('authenticated');
    expect(mockGetMe).toHaveBeenCalledTimes(1);

    // Second call should not fire another request
    await store.rehydrateSession();
    expect(mockGetMe).toHaveBeenCalledTimes(1);
    expect(store.status).toBe('authenticated');
  });
});
