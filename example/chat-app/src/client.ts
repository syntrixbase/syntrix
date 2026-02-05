import { SyntrixClient } from '@syntrix/client';
import { API_URL } from './constants';

const DATABASE = 'default';

let client: SyntrixClient | null = null;

const buildClient = (token?: string | null, refreshToken?: string | null) => {
  client = new SyntrixClient(API_URL, {
    database: DATABASE,
    auth: {
      token: token || undefined,
      refreshToken: refreshToken || undefined,
      refreshUrl: `${API_URL}/auth/v1/refresh`,
      onTokenRefresh: (newToken: string) => {
        localStorage.setItem('access_token', newToken);
      },
      onAuthError: () => {
        localStorage.removeItem('access_token');
        localStorage.removeItem('refresh_token');
      }
    }
  });
  return client;
};

export const getSyntrixClient = (): SyntrixClient => {
  if (client) return client;
  const token = localStorage.getItem('access_token');
  const refreshToken = localStorage.getItem('refresh_token');
  return buildClient(token, refreshToken);
};

export const resetSyntrixClient = (token?: string | null, refreshToken?: string | null): SyntrixClient => {
  return buildClient(token || null, refreshToken || null);
};
