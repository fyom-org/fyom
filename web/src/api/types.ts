// API response envelope type
export interface ApiEnvelope<T> {
  code: number;
  message: string;
  data: T;
}

// User types
export interface User {
  user_id: string;
  username: string;
  role: string;
  password_change_required: boolean;
}

export interface LoginData {
  access_token: string;
  token_type: string;
  expires_in: number;
  user: User;
}

export interface SystemStatusData {
  initialized: boolean;
  allow_registration: boolean;
}
