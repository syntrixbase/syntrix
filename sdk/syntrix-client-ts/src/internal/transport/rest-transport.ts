import { AxiosInstance } from 'axios';
import { StorageClient } from '../storage-client';

const API_PREFIX = '/api/v1';

export class RestTransport implements StorageClient {
  constructor(private axios: AxiosInstance) {}

  private buildPath(path: string): string {
    // If path already starts with /api/v1, use as-is
    if (path.startsWith('/api/v1')) {
      return path;
    }
    // Remove leading slash if present, then add prefix
    const cleanPath = path.startsWith('/') ? path.slice(1) : path;
    return `${API_PREFIX}/${cleanPath}`;
  }

  async get<T>(path: string): Promise<T | null> {
    try {
      const response = await this.axios.get(this.buildPath(path));
      return response.data;
    } catch (error: any) {
      if (error.response && error.response.status === 404) {
        return null;
      }
      throw error;
    }
  }

  async create<T>(path: string, data: T): Promise<T> {
    const response = await this.axios.post(this.buildPath(path), data);
    return response.data;
  }

  async set<T>(path: string, data: T, ifMatch?: any[]): Promise<T> {
    const payload: any = { doc: data };
    if (ifMatch) {
      payload.ifMatch = ifMatch;
    }
    const response = await this.axios.put(this.buildPath(path), payload);
    return response.data;
  }

  async update<T>(path: string, data: Partial<T>, ifMatch?: any[]): Promise<T> {
    const payload: any = { doc: data };
    if (ifMatch) {
      payload.ifMatch = ifMatch;
    }
    const response = await this.axios.patch(this.buildPath(path), payload);
    return response.data;
  }

  async delete(path: string, ifMatch?: any[]): Promise<void> {
    const config: any = {};
    if (ifMatch) {
      config.data = { ifMatch };
    }
    await this.axios.delete(this.buildPath(path), config);
  }

  async query<T>(path: string, query: any): Promise<T[]> {
    const response = await this.axios.post(this.buildPath(path), query);
    if (response.data && Array.isArray(response.data.docs)) {
        return response.data.docs;
    }
    return response.data;
  }
}
