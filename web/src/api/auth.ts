import { authRequest } from './request';
import type { ApiEnvelope, User, LoginData } from './types';

// Re-export User type for external use
export type { User, LoginData };

export interface LoginPayload {
  username: string;
  password: string;
}

// Login and return the actual data (unwrapped from envelope)
export async function login(payload: LoginPayload): Promise<LoginData> {
  const res = await authRequest.post<ApiEnvelope<LoginData>>('/auth/login', payload);
  return res.data.data;
}

// Get current user (unwrapped from envelope)
export async function getMe(): Promise<User> {
  const res = await authRequest.get<ApiEnvelope<User>>('/auth/me');
  return res.data.data;
}

export async function register(payload: { username: string; password: string }): Promise<User> {
  const res = await authRequest.post<ApiEnvelope<User>>('/auth/register', payload);
  return res.data.data;
}

export async function changePassword(oldPassword: string, newPassword: string): Promise<User> {
  const res = await authRequest.put<ApiEnvelope<{ user: User }>>('/auth/me/password', {
    old_password: oldPassword,
    new_password: newPassword,
  });
  return res.data.data.user;
}
