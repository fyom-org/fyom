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
    // Distinguish real auth failure from transient/network errors.
    // Only HTTP 401/403 means "session is invalid now".
    // Network errors, timeouts, 5xx, etc. mean "try again later".
    const axiosError = error as any;
    const status: number | undefined = axiosError?.response?.status;

    if (status === 401 || status === 403) {
      // Real auth rejection from server — clear session immediately.
      localStorage.removeItem('token');
      window.dispatchEvent(new Event('auth:unauthorized'));
    }
    // For all other errors (network, timeout, 5xx) do NOT touch the token.
    // The session may still be valid.

    return Promise.reject(error);
  }
);

export default request;
