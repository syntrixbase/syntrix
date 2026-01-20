import { describe, it, expect, mock } from 'bun:test';
import { RealtimeSSEClient } from './realtime-sse';
import { MessageType } from './realtime';

function buildSseResponse(chunks: string[]) {
  const encoder = new TextEncoder();
  const stream = new ReadableStream<Uint8Array>({
    start(controller) {
      chunks.forEach((c) => controller.enqueue(encoder.encode(c)));
      controller.close();
    },
  });
  return new Response(stream, { status: 200 });
}

describe('RealtimeSSEClient', () => {
  it('should emit events and snapshots from SSE stream', async () => {
    const eventMsg = `data: ${JSON.stringify({
      type: MessageType.Event,
      payload: { subId: 'default', delta: { type: 'create', id: '1', timestamp: 1, document: { foo: 'bar' } } },
    })}\n\n`;
    const snapshotMsg = `data: ${JSON.stringify({
      type: MessageType.Snapshot,
      payload: { subId: 'default', documents: [{ foo: 'bar' }] },
    })}\n\n`;

    const fetchMock = mock(async () => buildSseResponse([eventMsg, snapshotMsg]));
    const tokenProvider = { getToken: async () => 'token' } as any;
    const client = new RealtimeSSEClient('http://example.com', tokenProvider, 'test-db');

    let eventSeen = false;
    let snapshotSeen = false;

    await client.connect({
      onEvent: (evt) => {
        eventSeen = true;
        expect(evt.delta.id).toBe('1');
      },
      onSnapshot: (snap) => {
        snapshotSeen = true;
        expect(snap.documents[0].foo).toBe('bar');
      },
    }, { fetchImpl: fetchMock, collection: 'users' });

    expect(eventSeen).toBe(true);
    expect(snapshotSeen).toBe(true);
  });

  it('should surface fetch errors', async () => {
    const fetchMock = mock(async () => new Response(null, { status: 401 }));
    const tokenProvider = { getToken: async () => 'token' } as any;
    const client = new RealtimeSSEClient('http://example.com', tokenProvider, 'test-db');

    try {
      await client.connect({ onError: () => {} }, { fetchImpl: fetchMock });
      expect(true).toBe(false);
    } catch (err: any) {
      expect(err.message).toContain('SSE connection failed');
    }
  });
});
