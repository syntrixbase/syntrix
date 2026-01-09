export interface AuthConfig {
  token?: string;
  refreshToken?: string;
  refreshUrl?: string;
  databaseId?: string;
  onTokenRefresh?: (newToken: string) => void;
  onAuthError?: (error: Error) => void;
}

export interface TokenProvider {
  getToken(): Promise<string | null>;
  setToken(token: string): void;
  setRefreshToken(token: string): void;
  refreshToken(): Promise<string>;
}

export interface LoginResponse {
  access_token: string;
  refresh_token: string;
  expires_in: number;
}

export interface AuthService {
  login(username: string, password: string, databaseId?: string): Promise<LoginResponse>;
  logout(): Promise<void>;
  isAuthenticated(): boolean;
}
