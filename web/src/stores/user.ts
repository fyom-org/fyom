import { defineStore } from 'pinia';
import { computed, ref } from 'vue';
import router from '@/router';
import { login, getMe, changePassword, type User } from '@/api/auth';
import { apiRequest } from '@/api/request';
import type { ApiEnvelope } from '@/api/types';

export type AuthStatus = 'unknown' | 'rehydrating' | 'authenticated' | 'anonymous' | 'error';

interface BootstrapSessionData {
  token: string;
  user: User;
}

function readPersistedToken(): string | null {
  if (typeof window === 'undefined') return null;
  return localStorage.getItem('token');
}

function persistToken(value: string): void {
  if (typeof window === 'undefined') return;
  localStorage.setItem('token', value);
}

function clearPersistedToken(): void {
  if (typeof window === 'undefined') return;
  localStorage.removeItem('token');
}

export const useUserStore = defineStore('user', () => {
  const status = ref<AuthStatus>('unknown');
  const token = ref<string | null>(readPersistedToken());
  const user = ref<User | null>(null);

  const isAuthenticated = computed(() => status.value === 'authenticated');
  const isAuthReady = computed(
    () => status.value === 'authenticated' || status.value === 'anonymous'
  );
  const isAdmin = computed(() => status.value === 'authenticated' && user.value?.role === 'admin');
  const requiresPasswordChange = computed(
    () => status.value === 'authenticated' && user.value?.password_change_required === true
  );

  let rehydrationPromise: Promise<void> | null = null;

  function setAuthenticatedSession(nextToken: string, nextUser: User): void {
    token.value = nextToken;
    user.value = nextUser;
    status.value = 'authenticated';
    persistToken(nextToken);
  }

  function setAuthenticatedUser(nextUser: User): void {
    user.value = nextUser;
    status.value = 'authenticated';
  }

  function setAnonymous(): void {
    token.value = null;
    user.value = null;
    status.value = 'anonymous';
  }

  function clearStaleSession(): void {
    token.value = null;
    user.value = null;
    status.value = 'anonymous';
    clearPersistedToken();
  }

  /**
   * Rehydrate session from a persisted token.
   * Only one caller runs at a time; concurrent callers await the same promise.
   */
  async function rehydrateSession(): Promise<void> {
    if (status.value === 'authenticated' && token.value && user.value) {
      return;
    }

    if (rehydrationPromise) {
      return rehydrationPromise;
    }

    rehydrationPromise = (async () => {
      const persisted = readPersistedToken();

      if (!persisted) {
        setAnonymous();
        return;
      }

      status.value = 'rehydrating';

      try {
        const me = await getMe();
        token.value = persisted;
        user.value = me;
        status.value = 'authenticated';
      } catch (err: unknown) {
        const httpStatus = (err as any)?.response?.status;

        if (httpStatus === 401 || httpStatus === 403) {
          clearStaleSession();
          return;
        }

        // Keep token for a future retry, but do not leave the app in a falsely
        // authenticated state.
        token.value = persisted;
        user.value = null;
        status.value = 'rehydrating';
      }
    })().finally(() => {
      rehydrationPromise = null;
    });

    return rehydrationPromise;
  }

  /**
   * Bootstrap desktop authentication from backend-issued bootstrap token.
   * Called on Tauri startup if no local token exists.
   * Returns true if bootstrap succeeded and user is now authenticated.
   */
  async function bootstrapDesktopAuth(): Promise<boolean> {
    if (status.value === 'authenticated' && token.value && user.value) {
      return true;
    }

    if (typeof window === 'undefined' || !('__TAURI_INTERNALS__' in window)) {
      return false;
    }

    try {
      const res = await apiRequest.get<ApiEnvelope<BootstrapSessionData>>(
        '/internal/bootstrap-session'
      );

      const bootstrapToken = res.data?.data?.token;
      const bootstrapUser = res.data?.data?.user;

      if (!bootstrapToken || !bootstrapUser) {
        return false;
      }

      setAuthenticatedSession(bootstrapToken, bootstrapUser);
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Login using username/password and persist the resulting session.
   */
  async function doLogin(username: string, password: string): Promise<void> {
    const data = await login({ username, password });

    const accessToken = data.access_token;
    if (!accessToken) {
      throw new Error('login response missing access_token');
    }

    if (data.user) {
      setAuthenticatedSession(accessToken, data.user);
    } else {
      persistToken(accessToken);
      token.value = accessToken;

      try {
        const me = await getMe();
        setAuthenticatedUser(me);
      } catch {
        await rehydrateSession();
      }
    }

    await router.replace('/');
  }

  /**
   * Update the current user's password and refresh the in-memory user.
   * The backend decides whether old_password is required based on user state.
   */
  async function updatePassword(oldPassword: string, newPassword: string): Promise<void> {
    const updatedUser = await changePassword(oldPassword, newPassword);
    user.value = updatedUser;

    // Keep the auth state explicit after forced password change.
    if (token.value) {
      status.value = 'authenticated';
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
