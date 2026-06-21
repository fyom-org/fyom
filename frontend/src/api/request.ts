import { ofetch } from 'ofetch';
import { getApiBaseUrl } from '@/lib/runtime/desktop';
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

export interface FyomRequestConfig {
  authFailureMode?: AuthFailureMode;

  /**
   * Backward-compatible escape hatch.
   * When true, this request never dispatches global auth events.
   */
  skipAuthRedirect?: boolean;

  // Standard fetch/ofetch passthrough.
  method?: string;
  body?: BodyInit | Record<string, unknown> | unknown[] | null;
  query?: Record<string, unknown>;
  headers?: HeadersInit;
  timeout?: number;
  signal?: AbortSignal;
  credentials?: RequestCredentials;
}

interface AuthEventDetail {
  status: number;
  mode: AuthFailureMode;
  method?: string;
  url?: string;
}

interface HttpClientResponse<T> {
  data: T;
  status: number;
  statusText: string;
}

const DEFAULT_TIMEOUT_MS = 10000;
const apiBaseUrl = getApiBaseUrl();

/**
 * Read the persisted access token.
 */
function readToken(): string | null {
  if (!isBrowser()) {
    return null;
  }

  return window.localStorage.getItem('token');
}

/**
 * Convert HeadersInit into a mutable plain object.
 *
 * Important:
 * - Do not use Headers.entries() here.
 * - Some TS configs lack "DOM.Iterable", which makes entries() unavailable
 *   at type level even though it exists at runtime.
 */
function normalizeHeaders(input?: HeadersInit): Record<string, string> {
  const normalized: Record<string, string> = {};

  if (!input) {
    return normalized;
  }

  if (input instanceof Headers) {
    input.forEach((value, key) => {
      normalized[key] = value;
    });

    return normalized;
  }

  if (Array.isArray(input)) {
    for (const [key, value] of input) {
      normalized[key] = value;
    }

    return normalized;
  }

  for (const [key, value] of Object.entries(input)) {
    normalized[key] = value;
  }

  return normalized;
}

function buildRequestHeaders(
  options: FyomRequestConfig,
  attachAuth: boolean
): Record<string, string> {
  const headers: Record<string, string> = {};

  const locale = getCurrentLocale();
  if (locale) {
    headers['Accept-Language'] = locale;
  }

  const token = attachAuth ? readToken() : null;
  if (token) {
    headers['Authorization'] = `Bearer ${token}`;
  }

  Object.assign(headers, normalizeHeaders(options.headers));

  return headers;
}

async function http<T>(
  baseURL: string,
  url: string,
  options: FyomRequestConfig = {},
  attachAuth = false
): Promise<HttpClientResponse<T>> {
  const headers = buildRequestHeaders(options, attachAuth);

  try {
    const response = await ofetch.raw<T>(url, {
      baseURL,
      method: options.method,
      body: options.body,
      query: options.query,
      headers,
      timeout: options.timeout ?? DEFAULT_TIMEOUT_MS,
      signal: options.signal,
      credentials: options.credentials,
    });

    return {
      data: response._data as T,
      status: response.status,
      statusText: response.statusText,
    };
  } catch (error: unknown) {
    handleAuthFailure(error, options);
    throw error;
  }
}

function createHttpClient(attachAuth: boolean) {
  return {
    async get<T>(url: string, options?: FyomRequestConfig): Promise<HttpClientResponse<T>> {
      return http<T>(
        apiBaseUrl,
        url,
        {
          ...options,
          method: 'GET',
        },
        attachAuth
      );
    },

    async post<T>(
      url: string,
      body?: unknown,
      options?: FyomRequestConfig
    ): Promise<HttpClientResponse<T>> {
      return http<T>(
        apiBaseUrl,
        url,
        {
          ...options,
          method: 'POST',
          body: body as FyomRequestConfig['body'],
        },
        attachAuth
      );
    },

    async put<T>(
      url: string,
      body?: unknown,
      options?: FyomRequestConfig
    ): Promise<HttpClientResponse<T>> {
      return http<T>(
        apiBaseUrl,
        url,
        {
          ...options,
          method: 'PUT',
          body: body as FyomRequestConfig['body'],
        },
        attachAuth
      );
    },

    async patch<T>(
      url: string,
      body?: unknown,
      options?: FyomRequestConfig
    ): Promise<HttpClientResponse<T>> {
      return http<T>(
        apiBaseUrl,
        url,
        {
          ...options,
          method: 'PATCH',
          body: body as FyomRequestConfig['body'],
        },
        attachAuth
      );
    },

    async delete<T>(url: string, options?: FyomRequestConfig): Promise<HttpClientResponse<T>> {
      return http<T>(
        apiBaseUrl,
        url,
        {
          ...options,
          method: 'DELETE',
        },
        attachAuth
      );
    },
  };
}

/**
 * Auth failure policy handler.
 *
 * This is intentionally non-destructive by default.
 *
 * Important rules:
 * - 403 never clears session.
 * - 403 never dispatches global auth events.
 * - 401 does not clear session by default.
 * - Only explicit authFailureMode: 'redirect' can dispatch auth:unauthorized.
 * - Only explicit authFailureMode: 'session-check' can dispatch
 *   auth:session-check-required.
 */
function handleAuthFailure(error: unknown, config?: FyomRequestConfig): void {
  const status = getErrorStatus(error);

  if (!status) {
    return;
  }

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
      method: config?.method,
      url: undefined,
    });

    return;
  }

  if (mode === 'redirect') {
    dispatchAuthEvent('auth:unauthorized', {
      status,
      mode,
      method: config?.method,
      url: undefined,
    });
  }
}

function getErrorStatus(error: unknown): number | undefined {
  if (typeof error === 'object' && error !== null) {
    const err = error as { response?: { status?: number }; status?: number };

    if (err.response?.status !== undefined) {
      return err.response.status;
    }

    if (typeof err.status === 'number') {
      return err.status;
    }
  }

  return undefined;
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
  if (!isBrowser()) {
    return;
  }

  window.dispatchEvent(
    new CustomEvent<AuthEventDetail>(eventName, {
      detail,
    })
  );
}

function isBrowser(): boolean {
  return typeof window !== 'undefined' && typeof window.localStorage !== 'undefined';
}

const apiRequest = createHttpClient(false);
const authRequest = createHttpClient(true);

export { apiRequest, authRequest };
