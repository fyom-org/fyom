import request from './request';

export interface LoginPayload {
  username: string;
  password: string;
}

export interface LoginData {
  access_token: string;
  token_type: string;
  expires_in: number;
}

export interface MeData {
  user_id: string;
  username: string;
  role: string;
}

export function login(payload: LoginPayload) {
  return request.post<LoginData>('/auth/login', payload);
}

export function getMe() {
  return request.get<MeData>('/auth/me');
}
