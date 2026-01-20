import axios from 'axios';
import { AuthConfig, LoginResponse, AuthService } from '../internal/auth/types';
import { DefaultTokenProvider } from '../internal/auth/provider';
import { setupAuthInterceptor } from '../internal/auth/interceptor';
import { RestTransport } from '../internal/transport/rest-transport';
import { StorageClient } from '../internal/storage-client';
import { CollectionReference, DocumentReference } from '../api/types';
import { CollectionReferenceImpl, DocumentReferenceImpl } from '../api/reference';
import { RealtimeClient, RealtimeCallbacks, SubscribeOptions, ConnectionState } from '../replication/realtime';
import { RealtimeSSEClient, RealtimeSSEOptions } from '../replication/realtime-sse';

export interface SyntrixClientConfig {
  database: string;
  auth?: AuthConfig;
}

export class SyntrixClient implements AuthService {
  private storage: StorageClient;
  private tokenProvider: DefaultTokenProvider;
  private realtimeClient: RealtimeClient | null = null;
  private realtimeSseClient: RealtimeSSEClient | null = null;
  private baseUrl: string;
  private database: string;

  constructor(baseUrl: string, config: SyntrixClientConfig) {
    this.baseUrl = baseUrl;
    this.database = config.database;
    const axiosInstance = axios.create({ baseURL: baseUrl });
    this.tokenProvider = new DefaultTokenProvider(config.auth || {}, baseUrl);
    setupAuthInterceptor(axiosInstance, this.tokenProvider);
    this.storage = new RestTransport(axiosInstance, config.database);
  }

  getDatabase(): string {
    return this.database;
  }

  // Auth methods
  async login(username: string, password: string, database?: string): Promise<LoginResponse> {
    return this.tokenProvider.login(username, password, database);
  }

  async logout(): Promise<void> {
    if (this.realtimeClient) {
      this.realtimeClient.disconnect();
      this.realtimeClient = null;
    }
    return this.tokenProvider.logout();
  }

  isAuthenticated(): boolean {
    return this.tokenProvider.isAuthenticated();
  }

  // Data methods
  collection<T>(path: string): CollectionReference<T> {
    return new CollectionReferenceImpl<T>(this.storage, path);
  }

  doc<T>(path: string): DocumentReference<T> {
    const parts = path.split('/');
    const id = parts[parts.length - 1];
    return new DocumentReferenceImpl<T>(this.storage, path, id);
  }

  // Realtime methods
  realtime(): RealtimeClient {
    if (!this.realtimeClient) {
      const wsUrl = this.baseUrl.replace(/^http/, 'ws') + '/realtime/ws';
      this.realtimeClient = new RealtimeClient(wsUrl, this.tokenProvider, this.database);
    }
    return this.realtimeClient;
  }

  realtimeSSE(): RealtimeSSEClient {
    if (!this.realtimeSseClient) {
      this.realtimeSseClient = new RealtimeSSEClient(this.baseUrl, this.tokenProvider, this.database);
    }
    return this.realtimeSseClient;
  }

  // Convenience method for subscribing to a collection
  subscribe(
    collection: string,
    callbacks: {
      onEvent?: RealtimeCallbacks['onEvent'];
      onSnapshot?: RealtimeCallbacks['onSnapshot'];
      onError?: RealtimeCallbacks['onError'];
    },
    options?: Partial<SubscribeOptions>
  ): { subId: string; unsubscribe: () => void } {
    const rt = this.realtime();

    if (callbacks.onEvent) rt.on('onEvent', callbacks.onEvent);
    if (callbacks.onSnapshot) rt.on('onSnapshot', callbacks.onSnapshot);
    if (callbacks.onError) rt.on('onError', callbacks.onError);

    const subId = rt.subscribe({
      query: { collection, filters: options?.query?.filters || [] },
      includeData: options?.includeData ?? true,
      sendSnapshot: options?.sendSnapshot ?? false,
    });

    return {
      subId,
      unsubscribe: () => rt.unsubscribe(subId),
    };
  }
}
