import { describe, it, expect, mock, afterEach } from 'bun:test';
import { SyntrixClient } from './syntrix-client';
import axios from 'axios';

const originalCreate = axios.create;

describe('SyntrixClient', () => {
  afterEach(() => {
    axios.create = originalCreate;
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
});
