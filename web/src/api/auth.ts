import { authRequest } from './request';

export interface LoginPayload {
	username: string;
	password: string;
}

export interface LoginData {
	access_token: string;
	token_type: string;
	expires_in: number;
	user: User;
}

export interface User {
	user_id: string;
	username: string;
	role: string;
	password_change_required: boolean;
}

export interface MeData {
	user_id: string;
	username: string;
	role: string;
	password_change_required: boolean;
}

export function login(payload: LoginPayload) {
	return authRequest.post<LoginData>('/auth/login', payload);
}

export function getMe() {
	return authRequest.get<MeData>('/auth/me');
}

export function register(payload: { username: string; password: string }) {
	return authRequest.post('/auth/register', payload);
}

export function updatePassword(payload: { old_password: string; new_password: string }) {
	return authRequest.put<{ user: User }>('/auth/me/password', payload);
}
