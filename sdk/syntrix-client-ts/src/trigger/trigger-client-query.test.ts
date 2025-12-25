import { describe, it, expect, mock, beforeEach, afterEach } from 'bun:test';
import { TriggerClient } from './trigger-client';
import axios from 'axios';

// Mock axios
const originalCreate = axios.create;
let mockAxiosInstance: any;

describe('TriggerClient Query Operations', () => {
  beforeEach(() => {
    mockAxiosInstance = {
      post: mock(async () => ({ data: {} })),
      get: mock(async () => ({ data: {} })),
      put: mock(async () => ({ data: {} })),
      patch: mock(async () => ({ data: {} })),
      delete: mock(async () => ({ data: {} })),
    };
    axios.create = mock(() => mockAxiosInstance);
  });

  afterEach(() => {
    axios.create = originalCreate;
  });

  it('should support chainable update by query', async () => {
    const client = new TriggerClient('http://localhost', 'token');
    mockAxiosInstance.post.mockResolvedValue({ data: { docs: [{ id: '1' }, { id: '2' }] } });

    await client.collection('users')
      .where('age', '>', 18)
      .update({ active: true });

    // First, it queries
    expect(mockAxiosInstance.post).toHaveBeenCalledWith('/query', {
      collection: 'users',
      filters: [{ field: 'age', op: '>', value: 18 }],
      orderBy: [],
      limit: undefined
    });

    // Then it updates each doc
    expect(mockAxiosInstance.post).toHaveBeenCalledWith('/write', {
      writes: [{ type: 'update', path: 'users/1', data: { active: true } }]
    });
    expect(mockAxiosInstance.post).toHaveBeenCalledWith('/write', {
      writes: [{ type: 'update', path: 'users/2', data: { active: true } }]
    });
  });

  it('should support chainable delete by query', async () => {
    const client = new TriggerClient('http://localhost', 'token');
    mockAxiosInstance.post.mockResolvedValue({ data: { docs: [{ id: '1' }] } });

    await client.collection('users')
      .where('status', '==', 'inactive')
      .delete();

    // First, it queries
    expect(mockAxiosInstance.post).toHaveBeenCalledWith('/query', {
      collection: 'users',
      filters: [{ field: 'status', op: '==', value: 'inactive' }],
      orderBy: [],
      limit: undefined
    });

    // Then it deletes
    expect(mockAxiosInstance.post).toHaveBeenCalledWith('/write', {
      writes: [{ type: 'delete', path: 'users/1' }]
    });
  });

  it('should support multiple where clauses', async () => {
    const client = new TriggerClient('http://localhost', 'token');
    mockAxiosInstance.post.mockResolvedValue({ data: { docs: [] } });

    await client.collection('users')
      .where('age', '>', 18)
      .where('status', '==', 'active')
      .get();

    expect(mockAxiosInstance.post).toHaveBeenCalledWith('/query', {
      collection: 'users',
      filters: [
        { field: 'age', op: '>', value: 18 },
        { field: 'status', op: '==', value: 'active' }
      ],
      orderBy: [],
      limit: undefined
    });
  });
});
