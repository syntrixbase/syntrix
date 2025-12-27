import { describe, it, expect, mock } from 'bun:test';
import { RestTransport } from './rest-transport';

describe('RestTransport buildPath', () => {
  it('should prefix non-api paths with /api/v1', async () => {
    const axiosMock: any = { get: mock(async () => ({ data: { ok: true } })) };
    const transport = new RestTransport(axiosMock);

    await transport.get('users/1');

    expect(axiosMock.get).toHaveBeenCalledWith('/api/v1/users/1');
  });

  it('should keep paths that already include /api/v1', async () => {
    const axiosMock: any = { get: mock(async () => ({ data: { ok: true } })) };
    const transport = new RestTransport(axiosMock);

    await transport.get('/api/v1/users/1');

    expect(axiosMock.get).toHaveBeenCalledWith('/api/v1/users/1');
  });
});
