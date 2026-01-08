import { api } from './api';

export interface Document {
  id: string;
  _collection?: string;
  _createdAt?: string;
  _updatedAt?: string;
  [key: string]: unknown;
}

export interface Filter {
  field: string;
  op: 'eq' | 'ne' | 'gt' | 'gte' | 'lt' | 'lte' | 'in' | 'contains';
  value: unknown;
}

export interface QueryOptions {
  collection: string;
  filters?: Filter[];
  limit?: number;
  startAfter?: string;
  orderBy?: { field: string; dir: 'asc' | 'desc' }[];
}

export interface QueryResponse {
  documents: Document[];
  hasMore: boolean;
}

export interface CollectionInfo {
  name: string;
  count?: number;
}

export const documentsApi = {
  /**
   * Query documents from a collection with optional filters
   */
  query: async (options: QueryOptions): Promise<QueryResponse> => {
    // Backend returns array directly, not wrapped object
    const response = await api.post<Document[]>('/api/v1/query', {
      collection: options.collection,
      filters: options.filters || [],
      limit: (options.limit || 50) + 1, // Request one extra to check if there's more
      startAfter: options.startAfter,
      orderBy: options.orderBy || [],
    });
    
    const docs = response.data || [];
    const limit = options.limit || 50;
    const hasMore = docs.length > limit;
    
    return {
      documents: hasMore ? docs.slice(0, limit) : docs,
      hasMore,
    };
  },

  /**
   * Get a single document by path (collection/id)
   */
  get: async (collection: string, id: string): Promise<Document> => {
    const response = await api.get<Document>(`/api/v1/${collection}/${id}`);
    return response.data;
  },

  /**
   * Create a new document
   */
  create: async (collection: string, data: Record<string, unknown>, id?: string): Promise<Document> => {
    const path = id ? `/api/v1/${collection}/${id}` : `/api/v1/${collection}`;
    const response = await api.post<Document>(path, data);
    return response.data;
  },

  /**
   * Update an existing document
   */
  update: async (collection: string, id: string, data: Record<string, unknown>): Promise<Document> => {
    const response = await api.patch<Document>(`/api/v1/${collection}/${id}`, data);
    return response.data;
  },

  /**
   * Delete a document
   */
  delete: async (collection: string, id: string): Promise<void> => {
    await api.delete(`/api/v1/${collection}/${id}`);
  },

  /**
   * List all collections (queries with empty filter to discover collections)
   * Note: This is a workaround - ideally there would be a /collections endpoint
   */
  listCollections: async (): Promise<CollectionInfo[]> => {
    try {
      // Try to get collections from a metadata endpoint if available
      const response = await api.get<{ collections: CollectionInfo[] }>('/api/v1/collections');
      return response.data.collections || [];
    } catch {
      // Fallback: return known collections
      // In a real implementation, this could scan recent queries or use a discovery endpoint
      return [
        { name: 'messages' },
        { name: 'users' },
        { name: 'settings' },
      ];
    }
  },
};
