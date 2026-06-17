import axios, {
  AxiosError,
  type AxiosInstance,
  type AxiosRequestConfig,
  type AxiosResponse,
  type InternalAxiosRequestConfig,
} from 'axios';
import { getApiBaseUrl } from '@/lib/runtime/tauri';
import { getCurrentLocale } from '@/composables/useLocale';

export type AuthFailureMode =
  /**
   * Default behavior.
   * Reject the request error and let the caller decide what to do.
   * Does not clear token and does not dispatch global auth events.
   */
  | 'soft'

  /**
   * For optional/background requests.
   * Intended for progress, polling, recommendations, telemetry, etc.
   * Never dispatches global auth events.
   */
  | 'silent'

  /**
   * For explicit auth-critical requests only.
   * A 401 response dispatches auth:unauthorized.
   * A 403 response is still treated as a permission/business error and does
   * not dispatch any global auth event.
   */
  | 'redirect'

  /**
   * For permission-sensitive requests.
   * Intended for admin endpoints where the page should display "forbidden".
   * Never dispatches global auth events.
   */
  | 'forbidden'

  /**
   * For requests that want to ask the auth store to verify /auth/me.
   * A 401 response dispatches auth:session-check-required, but does not clear
   * token and does not redirect by itself.
   */
  | 'session-check';

export interface FyomRequestConfig extends AxiosRequestConfig {
  authFailureMode?: AuthFailureMode;

  /**
   * Backward-compatible escape hatch.
   * When true, this request never dispatches global auth events.
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

interface AuthEventDetail {
  status: number;
  mode: AuthFailureMode;
  method?: string;
  url?: string;
}

const DEFAULT_TIMEOUT_MS = 10000;

const apiBaseUrl = getApiBaseUrl();

const apiRequest: AxiosInstance = axios.create({
  baseURL: apiBaseUrl,
  timeout: DEFAULT_TIMEOUT_MS,
});

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

      setAuthorizationHeader(config, token);

      return config;
    },
    (error: unknown) => Promise.reject(error)
  );
}

/**
 * Attach `Accept-Language` header so the backend can locale-aware log,
 * validate, or (Phase 3+) select error_code metadata.
 *
 * The backend does NOT translate messages today; the header is a forward-
 * compatibility signal. Reading the locale at request time (not at module
 * load) ensures locale switches mid-session propagate to subsequent calls
 * without re-creating the axios instance.
 */
function attachAcceptLanguageInterceptor(instance: AxiosInstance): void {
  instance.interceptors.request.use(
    (config: InternalAxiosRequestConfig) => {
      const locale = getCurrentLocale();

      if (!config.headers) {
        // `Accept-Language` is not a known AxiosHeaders property, so cast
        // through `unknown` to satisfy the strict AxiosRequestHeaders type.
        config.headers = {
          'Accept-Language': locale,
        } as unknown as InternalAxiosRequestConfig['headers'];

        return config;
      }

      const headersWithSet = config.headers as {
        set?: (name: string, value: string) => void;
        'Accept-Language'?: string;
      };

      if (typeof headersWithSet.set === 'function') {
        headersWithSet.set('Accept-Language', locale);
        return config;
      }

      headersWithSet['Accept-Language'] = locale;

      return config;
    },
    (error: unknown) => Promise.reject(error)
  );
}

/**
 * Set Authorization header in a way that is compatible with Axios 1.x
 * AxiosHeaders and plain object headers.
 */
function setAuthorizationHeader(config: InternalAxiosRequestConfig, token: string): void {
  const value = `Bearer ${token}`;

  if (!config.headers) {
    config.headers = {
      Authorization: value,
    } as InternalAxiosRequestConfig['headers'];

    return;
  }

  const headersWithSet = config.headers as {
    set?: (name: string, value: string) => void;
    Authorization?: string;
  };

  if (typeof headersWithSet.set === 'function') {
    headersWithSet.set('Authorization', value);
    return;
  }

  headersWithSet.Authorization = value;
}

/**
 * Attach auth failure policy.
 *
 * This interceptor is intentionally non-destructive by default.
 *
 * Important rules:
 * - 403 never clears session.
 * - 403 never dispatches global auth events.
 * - 401 does not clear session by default.
 * - Only explicit authFailureMode: 'redirect' can dispatch auth:unauthorized.
 * - Only explicit authFailureMode: 'session-check' can dispatch
 *   auth:session-check-required.
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
   * 403 means "forbidden", not "logged out".
   *
   * Do not dispatch anything here. Some app-level listeners may treat auth
   * events as redirect signals, so the safest behavior is complete silence.
   */
  if (status === 403) {
    return;
  }

  if (status !== 401) {
    return;
  }

  if (shouldSuppressAuthEvent(config, mode)) {
    return;
  }

  if (mode === 'session-check') {
    dispatchAuthEvent('auth:session-check-required', {
      status,
      mode,
      method: error.config?.method,
      url: error.config?.url,
    });

    return;
  }

  if (mode === 'redirect') {
    dispatchAuthEvent('auth:unauthorized', {
      status,
      mode,
      method: error.config?.method,
      url: error.config?.url,
    });
  }
}

function shouldSuppressAuthEvent(
  config: FyomRequestConfig | undefined,
  mode: AuthFailureMode
): boolean {
  if (config?.skipAuthRedirect) {
    return true;
  }

  return mode === 'soft' || mode === 'silent' || mode === 'forbidden';
}

function resolveAuthFailureMode(config?: FyomRequestConfig): AuthFailureMode {
  return config?.authFailureMode ?? 'soft';
}

function dispatchAuthEvent(eventName: string, detail: AuthEventDetail): void {
  if (!isBrowser()) return;

  window.dispatchEvent(
    new CustomEvent<AuthEventDetail>(eventName, {
      detail,
    })
  );
}

function isBrowser(): boolean {
  return typeof window !== 'undefined' && typeof window.localStorage !== 'undefined';
}

attachAuthInterceptor(apiRequest);
attachAuthInterceptor(authRequest);

attachAcceptLanguageInterceptor(apiRequest);
attachAcceptLanguageInterceptor(authRequest);

attachAuthFailurePolicyInterceptor(apiRequest);
attachAuthFailurePolicyInterceptor(authRequest);

export { apiRequest, authRequest };
export { type AxiosInstance } from 'axios';
