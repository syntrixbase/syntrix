import axios, { AxiosInstance } from 'axios';
import { CollectionReference, DocumentReference } from '../api/reference';
import { WriteOp, Query, GetResponse } from '../types';
import { StorageClient } from '../internal/interfaces';

export class TriggerClient implements StorageClient {
    private client: AxiosInstance;

    constructor(baseURL: string, token: string) {
        this.client = axios.create({
            baseURL,
            headers: {
                'Content-Type': 'application/json',
                'Authorization': `Bearer ${token}`
            }
        });
    }

    collection<T = any>(path: string): CollectionReference<T> {
        return new CollectionReference<T>(this, path);
    }

    doc<T = any>(path: string): DocumentReference<T> {
        return new DocumentReference<T>(this, path);
    }

    /**
     * Execute a batch of write operations.
     * This is a special capability of the Trigger API.
     */
    async batch(writes: WriteOp[]): Promise<void> {
        await this.client.post('/v1/trigger/write', { writes });
    }

    // --- StorageClient Implementation ---

    async get<T = any>(path: string): Promise<T | null> {
        try {
            const response = await this.client.post<GetResponse<T>>('/v1/trigger/get', { paths: [path] });
            if (response.data.documents && response.data.documents.length > 0) {
                return response.data.documents[0];
            }
            return null;
        } catch (error: any) {
            // Trigger API might return 200 with empty list or error.
            // Assuming standard error handling for network issues.
            return null;
        }
    }

    async create<T = any>(collectionPath: string, data: any, id?: string): Promise<T> {
        if (!id) {
            throw new Error("ID is required for create in trigger mode");
        }
        const path = collectionPath.endsWith('/') ? `${collectionPath}${id}` : `${collectionPath}/${id}`;
        await this.batch([{ type: 'create', path, data }]);
        return { ...data, id } as T;
    }

    async update<T = any>(path: string, data: any): Promise<T> {
        await this.batch([{ type: 'update', path, data }]);
        return data as T;
    }

    async replace<T = any>(path: string, data: any): Promise<T> {
        await this.batch([{ type: 'replace', path, data }]);
        return data as T;
    }

    async delete(path: string): Promise<void> {
        await this.batch([{ type: 'delete', path }]);
    }

    async query<T = any>(query: Query): Promise<T[]> {
        const response = await this.client.post<T[]>('/v1/trigger/query', query);
        return response.data;
    }
}
