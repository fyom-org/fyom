// API response envelope type
export interface ApiEnvelope<T> {
  code: number;
  message: string;
  /** Stable machine-readable error code (omitted on success). See pkg/errors/codes.go. */
  error_code?: string;
  data: T;
}

// User types
export interface User {
  user_id: string;
  username: string;
  role: string;
  password_change_required: boolean;
  preferred_language: string;
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
  default_locale: string;
  supported_locales: string[];
}

export interface UpdatePreferencesPayload {
  preferred_language: string;
}
