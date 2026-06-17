import axios, {
  AxiosError,
  type AxiosInstance,
  type AxiosRequestConfig,
  type AxiosResponse,
  type InternalAxiosRequestConfig,
} from 'axios';
import { getApiBaseUrl } from '@/lib/runtime/tauri';

export type AuthFailureMode =
  /**
   * Default behavior.
   * Reject the request error and let the caller decide what to do.
   * Does not clear token and does not redirect.
   */
  | 'soft'

  /**
   * For optional/background requests.
   * Reject the request error, but never dispatch global auth events.
   * Intended for progress, polling, recommendations, telemetry, etc.
   */
  | 'silent'

  /**
   * For routes or requests that intentionally want global login redirect behavior.
   * Only 401 can dispatch auth:unauthorized.
   * 403 is still treated as permission denied, not as logout.
   */
  | 'redirect'

  /**
   * For admin/permission-sensitive requests.
   * 403 should be handled by the page as "no permission".
   */
  | 'forbidden'

  /**
   * For future session-check flows.
   * This mode dispatches a non-destructive session-check event on 401,
   * but does not clear token by itself.
   */
  | 'session-check';

export interface FyomRequestConfig extends AxiosRequestConfig {
  /**
   * Controls how authentication and authorization failures should be handled.
   *
   * Important:
   * - 403 must not clear session.
   * - 401 should not clear session unless the request explicitly opts in.
   */
  authFailureMode?: AuthFailureMode;

  /**
   * Backward-compatible escape hatch.
   * If true, no global auth event will be dispatched for this request.
   */
  skipAuthRedirect?: boolean;
}

declare module 'axios' {
  export interface AxiosRequestConfig {
    authFailureMode?: AuthFailureMode;
    skipAuthRedirect?: boolean;
  }

  export interface InternalAxiosRequestConfig {
    authFailureMode?: AuthFailureMode;
    skipAuthRedirect?: boolean;
  }
}

interface AuthFailureEventDetail {
  status: number;
  mode: AuthFailureMode;
  method?: string;
  url?: string;
}

const DEFAULT_TIMEOUT_MS = 10000;

const apiBaseUrl = getApiBaseUrl();

/**
 * Main API client for general API calls.
 *
 * This client uses the runtime-aware API base URL so it works in both
 * browser and Tauri environments.
 */
const apiRequest: AxiosInstance = axios.create({
  baseURL: apiBaseUrl,
  timeout: DEFAULT_TIMEOUT_MS,
});

/**
 * Auth-capable API client.
 *
 * It intentionally uses the same runtime-aware base URL as apiRequest.
 * Keeping both clients on the same base avoids browser/Tauri drift.
 */
const authRequest: AxiosInstance = axios.create({
  baseURL: apiBaseUrl,
  timeout: DEFAULT_TIMEOUT_MS,
});

/**
 * Read the persisted access token.
 */
function readToken(): string | null {
  if (!isBrowser()) return null;

  return window.localStorage.getItem('token');
}

/**
 * Attach Authorization header when a token exists.
 */
function attachAuthInterceptor(instance: AxiosInstance): void {
  instance.interceptors.request.use(
    (config: InternalAxiosRequestConfig) => {
      const token = readToken();

      if (!token) {
        return config;
      }

      config.headers = config.headers ?? {};
      config.headers.Authorization = `Bearer ${token}`;

      return config;
    },
    (error: unknown) => Promise.reject(error)
  );
}

/**
 * Attach auth failure policy.
 *
 * This interceptor deliberately does not clear localStorage by default.
 * Session invalidation must be decided by auth-critical flows such as:
 * - route guards
 * - store.rehydrateSession()
 * - explicit /auth/me verification
 * - user-initiated logout
 */
function attachAuthFailurePolicyInterceptor(instance: AxiosInstance): void {
  instance.interceptors.response.use(
    (response: AxiosResponse) => response,
    (error: AxiosError) => {
      handleAuthFailure(error);

      return Promise.reject(error);
    }
  );
}

function handleAuthFailure(error: AxiosError): void {
  const status = error.response?.status;

  if (!status) return;

  const config = error.config as FyomRequestConfig | undefined;
  const mode = resolveAuthFailureMode(config);

  /**
   * 403 means the user may be authenticated but does not have permission.
   * It must never clear session globally.
   */
  if (status === 403) {
    dispatchForbiddenEvent(error, mode);
    return;
  }

  /**
   * Only 401 can be considered an authentication problem.
   * Even then, the default behavior is non-destructive.
   */
  if (status !== 401) {
    return;
  }

  if (config?.skipAuthRedirect || mode === 'silent' || mode === 'soft' || mode === 'forbidden') {
    return;
  }

  if (mode === 'session-check') {
    dispatchSessionCheckRequiredEvent(error, mode);
    return;
  }

  if (mode === 'redirect') {
    dispatchUnauthorizedEvent(error, mode);
  }
}

function resolveAuthFailureMode(config?: FyomRequestConfig): AuthFailureMode {
  return config?.authFailureMode ?? 'soft';
}

function dispatchUnauthorizedEvent(error: AxiosError, mode: AuthFailureMode): void {
  dispatchAuthEvent('auth:unauthorized', {
    status: error.response?.status ?? 401,
    mode,
    method: error.config?.method,
    url: error.config?.url,
  });
}

function dispatchSessionCheckRequiredEvent(error: AxiosError, mode: AuthFailureMode): void {
  dispatchAuthEvent('auth:session-check-required', {
    status: error.response?.status ?? 401,
    mode,
    method: error.config?.method,
    url: error.config?.url,
  });
}

function dispatchForbiddenEvent(error: AxiosError, mode: AuthFailureMode): void {
  dispatchAuthEvent('auth:forbidden', {
    status: error.response?.status ?? 403,
    mode,
    method: error.config?.method,
    url: error.config?.url,
  });
}

function dispatchAuthEvent(eventName: string, detail: AuthFailureEventDetail): void {
  if (!isBrowser()) return;

  window.dispatchEvent(
    new CustomEvent<AuthFailureEventDetail>(eventName, {
      detail,
    })
  );
}

function isBrowser(): boolean {
  return typeof window !== 'undefined' && typeof window.localStorage !== 'undefined';
}

attachAuthInterceptor(apiRequest);
attachAuthInterceptor(authRequest);

attachAuthFailurePolicyInterceptor(apiRequest);
attachAuthFailurePolicyInterceptor(authRequest);

export { apiRequest, authRequest };
export { type AxiosInstance } from 'axios';
