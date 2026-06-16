import { defineStore } from 'pinia';
import { ref, computed } from 'vue';
import { useRouter } from 'vue-router';
import { login, getMe, type User } from '@/api/auth';
import { authRequest, apiRequest } from '@/api/request';

export type AuthStatus = 'unknown' | 'rehydrating' | 'authenticated' | 'anonymous' | 'error';

export const useUserStore = defineStore('user', () => {
  const router = useRouter();

  const status = ref<AuthStatus>('unknown');
  const token = ref<string | null>(localStorage.getItem('token') ?? null);
  const user = ref<User | null>(null);

  const isAuthenticated = computed(() => status.value === 'authenticated');
  const isAuthReady = computed(
    () => status.value === 'authenticated' || status.value === 'anonymous'
  );
  const isAdmin = computed(
    () => status.value === 'authenticated' && user.value?.role === 'admin'
  );
  const requiresPasswordChange = computed(
    () => status.value === 'authenticated' && user.value?.password_change_required === true
  );

  /**
   * Rehydrate session from a persisted token.
   * Only one caller runs at a time; subsequent calls during rehydration
   * are coalesced into the in-flight promise.
   */
  let rehydrationPromise: Promise<void> | null = null;

  async function rehydrateSession(): Promise<void> {
    // Already authenticated — no-op.
    if (status.value === 'authenticated') return;

    // Coalesce concurrent calls into one in-flight request.
    if (rehydrationPromise) return rehydrationPromise;

    rehydrationPromise = (async () => {
      const persisted = localStorage.getItem('token');
      if (!persisted) {
        setAnonymous();
        return;
      }

      status.value = 'rehydrating';
      try {
        const res = await getMe();
        token.value = persisted;
        user.value = res.data;
        status.value = 'authenticated';
      } catch (err: unknown) {
        // Only clear token on explicit 401/403 HTTP responses.
        // Network errors, timeouts, 5xx etc. mean "try again later" —
        // keep the token so a future rehydration attempt can succeed.
        const httpStatus = (err as any)?.response?.status;
        if (httpStatus === 401 || httpStatus === 403) {
          clearStaleSession();
        } else {
          // Transport failure — leave token in localStorage and status
          // as "rehydrating" so a later rehydration attempt can retry.
          status.value = 'rehydrating';
        }
      } finally {
        rehydrationPromise = null;
      }
    })();

    return rehydrationPromise;
  }

  /**
   * Bootstrap desktop authentication from backend-issued bootstrap token.
   * Called on Tauri startup if no local token exists.
   * Returns true if bootstrap succeeded and user is now authenticated.
   */
  async function bootstrapDesktopAuth(): Promise<boolean> {
    if (status.value === 'authenticated') return true;

    // Only attempt in Tauri environment
    if (typeof window === 'undefined' || !('__TAURI_INTERNALS__' in window)) {
      return false;
    }

    try {
      // Try to get desktop bootstrap session from internal bridge endpoint
      const res = await apiRequest.get<{ token: string; user: User }>('/internal/bootstrap-session');
      const bootstrapToken = res.data.token;
      const bootstrapUser = res.data.user;
      if (!bootstrapToken || !bootstrapUser) return false;

      // Store token and user
      token.value = bootstrapToken;
      localStorage.setItem('token', bootstrapToken);
      user.value = bootstrapUser;
      status.value = 'authenticated';

      return true;
    } catch {
      // No bootstrap session available or failed - fall through to normal flow
      return false;
    }
  }

  /** F2: Explicitly set anonymous state (no token, not "unknown") */
  function setAnonymous(): void {
    token.value = null;
    user.value = null;
    status.value = 'anonymous';
  }

  function clearStaleSession(): void {
    token.value = null;
    user.value = null;
    status.value = 'anonymous';
    localStorage.removeItem('token');
  }

  async function doLogin(username: string, password: string): Promise<void> {
    const res = await login({ username, password });
    const accessToken = res.data.access_token;
    if (!accessToken) throw new Error('login response missing access_token');
    token.value = accessToken;
    localStorage.setItem('token', accessToken);

    // Login response now includes user with password_change_required
    if (res.data.user) {
      user.value = res.data.user;
      status.value = 'authenticated';
    } else {
      // Fallback to rehydrateSession if user not in response
      try {
        const meRes = await authRequest.get<User>('/auth/me');
        user.value = meRes.data;
        status.value = 'authenticated';
      } catch {
        await rehydrateSession();
      }
    }

    // Always navigate to / after login.
    await router.replace('/');
  }

  async function updatePassword(oldPassword: string, newPassword: string): Promise<void> {
    const res = await authRequest.put<{ user: User }>('/auth/me/password', {
      old_password: oldPassword,
      new_password: newPassword,
    });
    if (res.data.user) {
      user.value = res.data.user;
    }
  }

  function logout(): void {
    clearStaleSession();
  }

  if (typeof window !== 'undefined') {
    window.addEventListener('auth:unauthorized', clearStaleSession);
  }

  return {
    status,
    token,
    user,
    isAuthenticated,
    isAuthReady,
    isAdmin,
    requiresPasswordChange,
    rehydrateSession,
    bootstrapDesktopAuth,
    setAnonymous,
    clearStaleSession,
    doLogin,
    updatePassword,
    logout,
  };
});