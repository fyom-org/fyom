import { defineStore } from 'pinia';
import { ref, computed } from 'vue';
import { login, getMe, type MeData } from '@/api/auth';

export type AuthStatus = 'unknown' | 'rehydrating' | 'authenticated' | 'anonymous' | 'error';

export const useUserStore = defineStore('user', () => {
  const status = ref<AuthStatus>('unknown');
  const token = ref<string | null>(localStorage.getItem('token') ?? null);
  const user = ref<MeData | null>(null);

  const isAuthenticated = computed(() => status.value === 'authenticated');
  const isAuthReady = computed(
    () => status.value === 'authenticated' || status.value === 'anonymous'
  );
  const isAdmin = computed(
    () => status.value === 'authenticated' && user.value?.role === 'admin'
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
        // keep the token so a future rehydration can succeed.
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

    // F3: Centralized revalidation handles post-login redirect
    await rehydrateSession();
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
    rehydrateSession,
    setAnonymous,
    clearStaleSession,
    doLogin,
    logout,
  };
});
