import axios, { type AxiosInstance } from 'axios';
import { getApiBaseUrl } from '@/lib/runtime/tauri';

/**
 * Main API client for general API calls (library, media, etc.)
 * Uses /api/v1 prefix as configured by the backend.
 */
const apiRequest: AxiosInstance = axios.create({
  baseURL: getApiBaseUrl(),
  timeout: 10000,
});

/**
 * Auth-specific API client for authentication endpoints.
 * Uses empty baseURL so relative paths resolve against the origin
 * (e.g., /auth/login, /auth/me) since the backend serves auth endpoints
 * at the root level, not under /api/v1.
 */
const authRequest: AxiosInstance = axios.create({
  baseURL: '',
  timeout: 10000,
});

/**
 * Request interceptor: attach Authorization header from localStorage.
 * Applied to both clients so all requests carry the token when present.
 */
function attachAuthInterceptor(instance: AxiosInstance) {
  instance.interceptors.request.use(
    (config) => {
      const token = localStorage.getItem('token');
      if (token && config.headers) {
        config.headers.Authorization = `Bearer ${token}`;
      }
      return config;
    },
    (error) => Promise.reject(error)
  );
}

attachAuthInterceptor(apiRequest);
attachAuthInterceptor(authRequest);

/**
 * Response interceptor: handle 401/403 by clearing stale session.
 * Applied to both clients.
 */
function attachAuthInvalidationInterceptor(instance: AxiosInstance) {
  instance.interceptors.response.use(
    (response) => response,
    (error) => {
      const status = error?.response?.status;
      if (status === 401 || status === 403) {
        localStorage.removeItem('token');
        window.dispatchEvent(new Event('auth:unauthorized'));
      }
      return Promise.reject(error);
    }
  );
}

attachAuthInvalidationInterceptor(apiRequest);
attachAuthInvalidationInterceptor(authRequest);

export { apiRequest, authRequest };
export { type AxiosInstance } from 'axios';
