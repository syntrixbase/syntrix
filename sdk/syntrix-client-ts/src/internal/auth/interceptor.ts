import { AxiosInstance, AxiosError, InternalAxiosRequestConfig } from 'axios';
import { TokenProvider } from './types';
import { SyntrixError } from '../../api/errors';

export function setupAuthInterceptor(axiosInstance: AxiosInstance, provider: TokenProvider) {
  axiosInstance.interceptors.request.use(async (config) => {
    const token = await provider.getToken();
    if (token) {
      config.headers.Authorization = `Bearer ${token}`;
    }
    return config;
  });

  axiosInstance.interceptors.response.use(
    (response) => response,
    async (error: AxiosError) => {
      const config = error.config as InternalAxiosRequestConfig & { _retry?: boolean };

      if (!config || !error.response) {
        return Promise.reject(error);
      }

      const status = error.response.status;
      const data = error.response.data as any;

      // Handle rate limiting (429)
      if (status === 429) {
        const retryAfterHeader = error.response.headers['retry-after'];
        const retryAfter = retryAfterHeader ? parseInt(retryAfterHeader, 10) : 60;
        const syntrixError = SyntrixError.fromResponse(status, data, retryAfter);
        return Promise.reject(syntrixError);
      }

      // Handle auth errors with token refresh
      if ((status === 401 || status === 403) && !config._retry) {
        config._retry = true;
        try {
          const newToken = await provider.refreshToken();
          config.headers.Authorization = `Bearer ${newToken}`;
          return axiosInstance(config);
        } catch (refreshError) {
          // Convert to SyntrixError for consistent error handling
          if (refreshError instanceof SyntrixError) {
            return Promise.reject(refreshError);
          }
          return Promise.reject(SyntrixError.fromResponse(401, { code: 'UNAUTHORIZED', message: 'Authentication failed' }));
        }
      }

      // Convert all API errors to SyntrixError
      if (data && (data.code || data.message || data.errors)) {
        return Promise.reject(SyntrixError.fromResponse(status, data));
      }

      return Promise.reject(error);
    }
  );
}
