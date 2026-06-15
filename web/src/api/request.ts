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
    const status = error instanceof Error && 'response' in error ? (error as any).response?.status : undefined;
    const body = error instanceof Error && 'response' in error ? (error as any).response?.data as ApiResponse | undefined : undefined;
    const isUnauthorized = status === 401 || status === 403 || body?.code === 401 || body?.code === 403;
    if (isUnauthorized) {
      localStorage.removeItem('token');
      window.dispatchEvent(new Event('auth:unauthorized'));
    }
    return Promise.reject(error);
  }
);

export default request;
