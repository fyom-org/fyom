import axios, {
  type AxiosInstance,
  type InternalAxiosRequestConfig,
  type AxiosResponse,
} from 'axios';
import { getApiBaseUrl } from '@/lib/runtime/tauri';

interface ApiResponse<T = unknown> {
  code: number;
  message: string;
  data: T;
}

const request: AxiosInstance = axios.create({
  baseURL: getApiBaseUrl(),
  timeout: 10000,
});

request.interceptors.request.use(
  (config: InternalAxiosRequestConfig) => {
    const token = localStorage.getItem('token');
    if (token && config.headers) {
      config.headers.Authorization = `Bearer ${token}`;
    }
    return config;
  },
  (error: unknown) => {
    return Promise.reject(error);
  }
);

/**
 * Decide whether an error response should trigger global session clearing.
 *
 * Auth-truth endpoints (where 401/403 means "session is invalid"):
 *   - /auth/me  (session self-validation)
 *   - /auth/login  (login failure)
 *
 * Business/resource endpoints (where 403 means "you lack permission"):
 *   - /admin/*  (RequireAdmin: non-admin user, but session is fine)
 *   - any resource-specific 403
 *
 * Transport failures (network, timeout, 5xx):
 *   never clear session.
 */
function shouldInvalidateSession(error: unknown): boolean {
  const axiosError = error as any;
  const status: number | undefined = axiosError?.response?.status;
  const url: string = axiosError?.config?.url ?? '';

  // Only 401/403 can trigger invalidation.
  if (status !== 401 && status !== 403) return false;

  // Only auth-truth endpoints trigger global session clearing.
  if (url.startsWith('/auth/me') || url.startsWith('/auth/login')) {
    return true;
  }

  // All other 401/403 are endpoint-specific permission denials.
  // Do NOT clear the session.
  return false;
}

request.interceptors.response.use(
  (response: AxiosResponse<ApiResponse>) => {
    const body = response.data;
    // 204 No Content or non-JSON responses — pass through
    if (typeof body !== 'object' || body === null) {
      return body as any;
    }
    if (body.code !== 0) {
      console.error('[API Error]', body.code, body.message);
      return Promise.reject(new Error(body.message));
    }
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    return response.data as any;
  },
  (error: unknown) => {
    // Distinguish real auth-truth failures from business/resource denials.
    // Only /auth/me and /auth/login 401/403 mean "session is globally invalid".
    // /admin/* 403, resource-specific 403, etc. must NOT clear session.
    // Network errors, timeouts, 5xx are always preserved.
    if (shouldInvalidateSession(error)) {
      localStorage.removeItem('token');
      window.dispatchEvent(new Event('auth:unauthorized'));
    }

    return Promise.reject(error);
  }
);

export default request;
