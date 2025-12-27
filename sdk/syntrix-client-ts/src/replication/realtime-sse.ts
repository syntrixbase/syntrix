import { BaseMessage, MessageType, RealtimeCallbacks, RealtimeEvent, SnapshotEvent, ConnectionState } from './realtime';
import { TokenProvider } from '../internal/auth/types';

type FetchLike = (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>;

export interface RealtimeSSEOptions {
  collection?: string;
  fetchImpl?: FetchLike;
}

export class RealtimeSSEClient {
  private controller: AbortController | null = null;
  private state: ConnectionState = 'disconnected';

  constructor(private baseUrl: string, private tokenProvider: TokenProvider) {}

  getState(): ConnectionState {
    return this.state;
  }

  disconnect(): void {
    this.controller?.abort();
    this.controller = null;
    this.state = 'disconnected';
  }

  async connect(callbacks: RealtimeCallbacks = {}, options: RealtimeSSEOptions = {}): Promise<void> {
    if (this.controller) {
      return; // already connected
    }

    const fetchImpl: FetchLike = options.fetchImpl || fetch;
    const collection = options.collection || '';
    const url = this.buildUrl(collection);
    const token = await this.tokenProvider.getToken();

    this.controller = new AbortController();
    this.setState('connecting', callbacks);

    try {
      const res = await fetchImpl(url, {
        method: 'GET',
        headers: token ? { Authorization: `Bearer ${token}` } : undefined,
        signal: this.controller.signal,
      });

      if (!res.ok || !res.body) {
        throw new Error(`SSE connection failed: ${res.status}`);
      }

      this.setState('connected', callbacks);
      callbacks.onConnect?.();

      const reader = res.body.getReader();
      const decoder = new TextDecoder();
      let buffer = '';

      while (true) {
        const { value, done } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });
        let idx = buffer.indexOf('\n\n');
        while (idx >= 0) {
          const chunk = buffer.slice(0, idx);
          buffer = buffer.slice(idx + 2);
          this.processChunk(chunk, callbacks);
          idx = buffer.indexOf('\n\n');
        }
      }
    } catch (error) {
      this.setState('error', callbacks);
      callbacks.onError?.(error as Error);
      throw error;
    } finally {
      this.controller = null;
      this.setState('disconnected', callbacks);
      callbacks.onDisconnect?.();
    }
  }

  private processChunk(chunk: string, callbacks: RealtimeCallbacks) {
    const dataLine = chunk.split('\n').find((l) => l.startsWith('data:'));
    if (!dataLine) return;
    const data = dataLine.slice(5).trim();
    if (!data) return;
    try {
      const msg: BaseMessage = JSON.parse(data);
      this.handleMessage(msg, callbacks);
    } catch (err) {
      callbacks.onError?.(err as Error);
    }
  }

  private handleMessage(msg: BaseMessage, callbacks: RealtimeCallbacks) {
    switch (msg.type) {
      case MessageType.Event: {
        const event: RealtimeEvent = typeof msg.payload === 'string' ? JSON.parse(msg.payload) : msg.payload;
        callbacks.onEvent?.(event);
        break;
      }
      case MessageType.Snapshot: {
        const snapshot: SnapshotEvent = typeof msg.payload === 'string' ? JSON.parse(msg.payload) : msg.payload;
        callbacks.onSnapshot?.(snapshot);
        break;
      }
      case MessageType.Error: {
        const errPayload = typeof msg.payload === 'string' ? JSON.parse(msg.payload) : msg.payload;
        callbacks.onError?.(new Error(errPayload?.message || 'Realtime SSE error'));
        break;
      }
      default:
        break;
    }
  }

  private buildUrl(collection: string): string {
    const cleanBase = this.baseUrl.replace(/\/$/, '');
    const path = `${cleanBase}/realtime/sse`;
    if (!collection) return path;
    const encoded = encodeURIComponent(collection);
    return `${path}?collection=${encoded}`;
  }

  private setState(state: ConnectionState, callbacks: RealtimeCallbacks) {
    if (this.state !== state) {
      this.state = state;
      callbacks.onStateChange?.(state);
    }
  }
}
