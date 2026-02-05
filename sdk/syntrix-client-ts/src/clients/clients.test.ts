import { describe, it, expect, mock, afterEach, beforeEach } from 'bun:test';
import { SyntrixClient } from './syntrix-client';
import axios from 'axios';

const originalPost = axios.post;

describe('SyntrixClient', () => {
  afterEach(() => {
    axios.post = originalPost;
  });

  it('should create collection reference', () => {
    const client = new SyntrixClient('http://localhost', { database: 'test-db' });
    const col = client.collection('users');
    expect(col.path).toBe('users');
  });

  it('should create doc reference from path', () => {
    const client = new SyntrixClient('http://localhost', { database: 'test-db' });
    const doc = client.doc('users/123');
    expect(doc.path).toBe('users/123');
    expect(doc.id).toBe('123');
  });

  it('should return database name', () => {
    const client = new SyntrixClient('http://localhost', { database: 'my-database' });
    expect(client.getDatabase()).toBe('my-database');
  });

  describe('Auth methods', () => {
    let client: SyntrixClient;
    let postMock: ReturnType<typeof mock>;

    beforeEach(() => {
      postMock = mock(async () => ({
        data: { access_token: 'test-token', refresh_token: 'test-refresh', expires_in: 3600 }
      })) as any;
      axios.post = postMock;
      client = new SyntrixClient('http://localhost', { database: 'test-db' });
    });

    it('should call signup', async () => {
      const result = await client.signup('testuser', 'testpass');
      expect(postMock).toHaveBeenCalledWith(
        'http://localhost/auth/v1/signup',
        { username: 'testuser', password: 'testpass' }
      );
      expect(result.access_token).toBe('test-token');
    });

    it('should call login', async () => {
      const result = await client.login('testuser', 'testpass');
      expect(postMock).toHaveBeenCalledWith(
        'http://localhost/auth/v1/login',
        { username: 'testuser', password: 'testpass' }
      );
      expect(result.access_token).toBe('test-token');
    });

    it('should call logout', async () => {
      // First login to get refresh token
      await client.login('testuser', 'testpass');

      // Then logout
      await client.logout();
      // Should have been called twice: once for login, once for logout
      expect(postMock.mock.calls.length).toBe(2);
      expect(postMock.mock.calls[1]).toEqual([
        'http://localhost/auth/v1/logout',
        { refresh_token: 'test-refresh' }
      ]);
    });

    it('should check isAuthenticated', async () => {
      expect(client.isAuthenticated()).toBe(false);
      await client.login('testuser', 'testpass');
      expect(client.isAuthenticated()).toBe(true);
    });
  });

  describe('Realtime methods', () => {
    it('should create realtime client', () => {
      const client = new SyntrixClient('http://localhost', { database: 'test-db' });
      const rt = client.realtime();
      expect(rt).toBeDefined();
      // Should return same instance on subsequent calls
      expect(client.realtime()).toBe(rt);
    });

    it('should create realtime SSE client', () => {
      const client = new SyntrixClient('http://localhost', { database: 'test-db' });
      const sse = client.realtimeSSE();
      expect(sse).toBeDefined();
      // Should return same instance on subsequent calls
      expect(client.realtimeSSE()).toBe(sse);
    });
  });
});
