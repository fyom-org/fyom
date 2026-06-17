import { defineStore } from 'pinia';
import { computed, ref } from 'vue';
import { login, getMe, changePassword, type User } from '@/api/auth';
import { apiRequest } from '@/api/request';
import type { ApiEnvelope } from '@/api/types';

export type AuthStatus = 'unknown' | 'rehydrating' | 'authenticated' | 'anonymous' | 'error';

interface BootstrapSessionData {
  token: string;
  user: User;
}

interface AuthEventDetail {
  status?: number;
  mode?: string;
  method?: string;
  url?: string;
}

function readPersistedToken(): string | null {
  if (!isBrowser()) return null;

  return window.localStorage.getItem('token');
}

function persistToken(value: string): void {
  if (!isBrowser()) return;

  window.localStorage.setItem('token', value);
}

function clearPersistedToken(): void {
  if (!isBrowser()) return;

  window.localStorage.removeItem('token');
}

function isBrowser(): boolean {
  return typeof window !== 'undefined' && typeof window.localStorage !== 'undefined';
}

function isTauriRuntime(): boolean {
  return typeof window !== 'undefined' && '__TAURI_INTERNALS__' in window;
}

function getHttpStatus(error: unknown): number | undefined {
  if (!isRecord(error)) return undefined;

  const response = error.response;

  if (!isRecord(response)) return undefined;

  const status = response.status;

  return typeof status === 'number' ? status : undefined;
}

function isAuthInvalidStatus(status: number | undefined): boolean {
  return status === 401 || status === 403;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null;
}

function normalizeBootstrapResponse(
  response: ApiEnvelope<BootstrapSessionData> | undefined
): BootstrapSessionData | null {
  const data = response?.data;

  if (!data?.token || !data?.user) {
    return null;
  }

  return data;
}

export const useUserStore = defineStore('user', () => {
  const status = ref<AuthStatus>('unknown');
  const token = ref<string | null>(readPersistedToken());
  const user = ref<User | null>(null);
  const lastAuthError = ref<string>('');

  const isAuthenticated = computed(() => {
    return status.value === 'authenticated' && Boolean(token.value && user.value);
  });

  const isAuthReady = computed(() => {
    return (
      status.value === 'authenticated' || status.value === 'anonymous' || status.value === 'error'
    );
  });

  const isAdmin = computed(() => {
    if (status.value !== 'authenticated') return false;

    const role = user.value?.role?.toLowerCase();

    return role === 'admin' || role === 'owner';
  });

  const requiresPasswordChange = computed(() => {
    return status.value === 'authenticated' && user.value?.password_change_required === true;
  });

  let rehydrationPromise: Promise<void> | null = null;
  let verificationPromise: Promise<boolean> | null = null;

  function setAuthenticatedSession(nextToken: string, nextUser: User): void {
    token.value = nextToken;
    user.value = nextUser;
    status.value = 'authenticated';
    lastAuthError.value = '';
    persistToken(nextToken);
  }

  function setAuthenticatedUser(nextUser: User): void {
    const persisted = readPersistedToken();

    token.value = persisted;
    user.value = nextUser;
    status.value = 'authenticated';
    lastAuthError.value = '';
  }

  function setAnonymous(): void {
    token.value = null;
    user.value = null;
    status.value = 'anonymous';
    lastAuthError.value = '';
    clearPersistedToken();
  }

  function clearStaleSession(): void {
    token.value = null;
    user.value = null;
    status.value = 'anonymous';
    lastAuthError.value = '';
    clearPersistedToken();
  }

  function markAuthError(message = 'Unable to verify session.'): void {
    status.value = 'error';
    lastAuthError.value = message;
  }

  /**
   * Rehydrate session from a persisted token.
   *
   * This is the canonical startup session check.
   * Only this flow, explicit verifySession(), or user logout should clear token.
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

      token.value = persisted;
      status.value = 'rehydrating';
      lastAuthError.value = '';

      try {
        const me = await getMe();

        token.value = persisted;
        user.value = me;
        status.value = 'authenticated';
        lastAuthError.value = '';
      } catch (error: unknown) {
        const httpStatus = getHttpStatus(error);

        if (isAuthInvalidStatus(httpStatus)) {
          clearStaleSession();
          return;
        }

        // Keep the token for a future retry. Network/backend failures are not
        // proof that the session is invalid.
        token.value = persisted;
        user.value = null;
        markAuthError('Unable to verify session. Please try again.');
      }
    })().finally(() => {
      rehydrationPromise = null;
    });

    return rehydrationPromise;
  }

  /**
   * Verify the current session without assuming that a random business request
   * failure means logout.
   *
   * Returns true only when /auth/me succeeds or the user is already verified.
   * Returns false when the token is missing, invalid, or verification cannot
   * currently prove the session.
   */
  async function verifySession(): Promise<boolean> {
    if (status.value === 'authenticated' && token.value && user.value) {
      return true;
    }

    if (verificationPromise) {
      return verificationPromise;
    }

    verificationPromise = (async () => {
      const persisted = readPersistedToken();

      if (!persisted) {
        clearStaleSession();
        return false;
      }

      token.value = persisted;

      try {
        const me = await getMe();

        token.value = persisted;
        user.value = me;
        status.value = 'authenticated';
        lastAuthError.value = '';

        return true;
      } catch (error: unknown) {
        const httpStatus = getHttpStatus(error);

        if (isAuthInvalidStatus(httpStatus)) {
          clearStaleSession();
          return false;
        }

        // Temporary verification failures should not destroy a persisted token.
        if (user.value && token.value) {
          status.value = 'authenticated';
          lastAuthError.value = '';
          return true;
        }

        markAuthError('Unable to verify session. Please try again.');
        return false;
      }
    })().finally(() => {
      verificationPromise = null;
    });

    return verificationPromise;
  }

  /**
   * Bootstrap desktop authentication from backend-issued bootstrap token.
   *
   * Called on Tauri startup if no local token exists.
   * Returns true if bootstrap succeeded and user is now authenticated.
   */
  async function bootstrapDesktopAuth(): Promise<boolean> {
    if (status.value === 'authenticated' && token.value && user.value) {
      return true;
    }

    if (!isTauriRuntime()) {
      return false;
    }

    try {
      const response = await apiRequest.get<ApiEnvelope<BootstrapSessionData>>(
        '/internal/bootstrap-session',
        {
          authFailureMode: 'silent',
        }
      );

      const bootstrap = normalizeBootstrapResponse(response.data);

      if (!bootstrap) {
        return false;
      }

      setAuthenticatedSession(bootstrap.token, bootstrap.user);
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Login using username/password and persist the resulting session.
   *
   * Navigation is intentionally not handled here. The caller should decide
   * where to redirect after login.
   */
  async function doLogin(username: string, password: string): Promise<void> {
    const data = await login({
      username,
      password,
    });

    const accessToken = data.access_token;

    if (!accessToken) {
      throw new Error('login response missing access_token');
    }

    if (data.user) {
      setAuthenticatedSession(accessToken, data.user);
      return;
    }

    persistToken(accessToken);
    token.value = accessToken;

    try {
      const me = await getMe();
      setAuthenticatedUser(me);
    } catch (error: unknown) {
      const httpStatus = getHttpStatus(error);

      if (isAuthInvalidStatus(httpStatus)) {
        clearStaleSession();
        throw new Error('Unable to load authenticated user.');
      }

      markAuthError('Signed in, but failed to load user profile.');
      throw error;
    }
  }

  /**
   * Update the current user's password and refresh the in-memory user.
   * The backend decides whether old_password is required based on user state.
   */
  async function updatePassword(oldPassword: string, newPassword: string): Promise<void> {
    const updatedUser = await changePassword(oldPassword, newPassword);

    user.value = updatedUser;

    if (token.value) {
      status.value = 'authenticated';
      lastAuthError.value = '';
    }
  }

  function logout(): void {
    clearStaleSession();
  }

  function handleUnauthorizedEvent(event: Event): void {
    const customEvent = event as CustomEvent<AuthEventDetail>;
    const httpStatus = customEvent.detail?.status;

    // This event should only be emitted by explicit auth-critical requests.
    // Therefore it is safe to clear session here.
    if (httpStatus === undefined || httpStatus === 401) {
      clearStaleSession();
    }
  }

  function handleSessionCheckRequiredEvent(): void {
    void verifySession();
  }

  if (typeof window !== 'undefined') {
    window.addEventListener('auth:unauthorized', handleUnauthorizedEvent);
    window.addEventListener('auth:session-check-required', handleSessionCheckRequiredEvent);
  }

  return {
    status,
    token,
    user,
    lastAuthError,
    isAuthenticated,
    isAuthReady,
    isAdmin,
    requiresPasswordChange,
    rehydrateSession,
    verifySession,
    bootstrapDesktopAuth,
    setAuthenticatedSession,
    setAuthenticatedUser,
    setAnonymous,
    clearStaleSession,
    markAuthError,
    doLogin,
    updatePassword,
    logout,
  };
});
