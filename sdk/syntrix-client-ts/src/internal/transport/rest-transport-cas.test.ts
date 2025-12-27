import { describe, it, expect, mock, beforeEach, afterEach } from 'bun:test';
import { RestTransport } from './rest-transport';
import axios from 'axios';

// Mock axios
const originalCreate = axios.create;
let mockAxiosInstance: any;

describe('RestTransport CAS', () => {
  beforeEach(() => {
    mockAxiosInstance = {
      put: mock(async () => ({ data: {} })),
      patch: mock(async () => ({ data: {} })),
      delete: mock(async () => ({ data: {} })),
    };
    axios.create = mock(() => mockAxiosInstance);
  });

  afterEach(() => {
    axios.create = originalCreate;
  });

  it('should include ifMatch in set (PUT)', async () => {
    const transport = new RestTransport(mockAxiosInstance);
    const ifMatch = [{ field: 'version', op: '==', value: 1 }];

    await transport.set('users/1', { name: 'Alice' }, ifMatch);

    expect(mockAxiosInstance.put).toHaveBeenCalledWith('/api/v1/users/1', {
      doc: { name: 'Alice' },
      ifMatch
    });
  });

  it('should include ifMatch in update (PATCH)', async () => {
    const transport = new RestTransport(mockAxiosInstance);
    const ifMatch = [{ field: 'version', op: '==', value: 1 }];

    await transport.update('users/1', { age: 30 }, ifMatch);

    expect(mockAxiosInstance.patch).toHaveBeenCalledWith('/api/v1/users/1', {
      doc: { age: 30 },
      ifMatch
    });
  });

  it('should include ifMatch in delete (DELETE)', async () => {
    const transport = new RestTransport(mockAxiosInstance);
    const ifMatch = [{ field: 'version', op: '==', value: 1 }];

    await transport.delete('users/1', ifMatch);

    expect(mockAxiosInstance.delete).toHaveBeenCalledWith('/api/v1/users/1', {
      data: { ifMatch }
    });
  });
});
