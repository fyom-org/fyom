import { authRequest } from './request';
import type { ApiEnvelope, LoginData, User } from './types';

export type { LoginData, User };

export interface LoginPayload {
  username: string;
  password: string;
}

export interface RegisterPayload {
  username: string;
  password: string;
}

interface ChangePasswordResponse {
  user?: User;
}

type MaybeEnvelope<T> = ApiEnvelope<T> | T;

function unwrapEnvelope<T>(value: MaybeEnvelope<T>): T {
  if (isRecord(value) && 'data' in value) {
    return value.data as T;
  }

  return value as T;
}

function normalizeLoginData(value: unknown): LoginData {
  const data = unwrapUnknownEnvelope(value);

  if (!isRecord(data)) {
    throw new Error('login response is invalid');
  }

  const accessToken = data.access_token;

  if (typeof accessToken !== 'string' || !accessToken.trim()) {
    throw new Error('login response missing access_token');
  }

  return data as unknown as LoginData;
}

function normalizeUser(value: unknown): User {
  const data = unwrapUnknownEnvelope(value);

  if (!isRecord(data)) {
    throw new Error('user response is invalid');
  }

  if (isRecord(data.user)) {
    return data.user as unknown as User;
  }

  return data as unknown as User;
}

function unwrapUnknownEnvelope(value: unknown): unknown {
  if (isRecord(value) && 'data' in value) {
    return value.data;
  }

  return value;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null;
}

/**
 * Login using username/password.
 *
 * This request must not trigger global auth invalidation on failure.
 * Login errors are handled by LoginView.
 */
export async function login(payload: LoginPayload): Promise<LoginData> {
  const response = await authRequest.post<MaybeEnvelope<LoginData>>(
    '/auth/login',
    {
      username: payload.username,
      password: payload.password,
    },
    {
      authFailureMode: 'silent',
    }
  );

  return normalizeLoginData(response.data);
}

/**
 * Get current authenticated user.
 *
 * This is an auth truth-source request, but it should not directly clear
 * session in the request layer. The user store decides how to handle failures.
 */
export async function getMe(): Promise<User> {
  const response = await authRequest.get<MaybeEnvelope<User>>('/auth/me', {
    authFailureMode: 'soft',
  });

  return normalizeUser(response.data);
}

/**
 * Register a new user.
 *
 * Registration failures are handled by RegisterView. They should not dispatch
 * global auth events.
 */
export async function register(payload: RegisterPayload): Promise<User> {
  const response = await authRequest.post<MaybeEnvelope<User>>(
    '/auth/register',
    {
      username: payload.username,
      password: payload.password,
    },
    {
      authFailureMode: 'silent',
    }
  );

  return normalizeUser(response.data);
}

/**
 * Change the current user's password.
 *
 * The backend decides whether old_password is required based on user state.
 * 401 responses should ask the store to verify session rather than causing
 * request-layer logout.
 */
export async function changePassword(oldPassword: string, newPassword: string): Promise<User> {
  const response = await authRequest.put<MaybeEnvelope<ChangePasswordResponse | User>>(
    '/auth/me/password',
    {
      old_password: oldPassword,
      new_password: newPassword,
    },
    {
      authFailureMode: 'session-check',
    }
  );

  const data = unwrapEnvelope(response.data);

  if (isRecord(data) && isRecord(data.user)) {
    return data.user as unknown as User;
  }

  return normalizeUser(data);
}
