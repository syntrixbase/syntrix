import axios, { AxiosInstance } from 'axios';
import { CollectionReference, DocumentReference } from '../api/reference';
import { StorageClient } from '../internal/interfaces';
import { Query } from '../types';

export class SyntrixClient implements StorageClient {
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

    // --- StorageClient Implementation ---

    async get<T = any>(path: string): Promise<T | null> {
        try {
            const response = await this.client.get<T>(`/v1/${path}`);
            return response.data;
        } catch (error: any) {
            if (error.response && error.response.status === 404) {
                return null;
            }
            throw error;
        }
    }

    async create<T = any>(collectionPath: string, data: any, id?: string): Promise<T> {
        const payload = id ? { ...data, id } : data;
        const response = await this.client.post<T>(`/v1/${collectionPath}`, payload);
        return response.data;
    }

    async update<T = any>(path: string, data: any): Promise<T> {
        const response = await this.client.patch<T>(`/v1/${path}`, data);
        return response.data;
    }

    async replace<T = any>(path: string, data: any): Promise<T> {
        const response = await this.client.put<T>(`/v1/${path}`, data);
        return response.data;
    }

    async delete(path: string): Promise<void> {
        await this.client.delete(`/v1/${path}`);
    }

    async query<T = any>(query: Query): Promise<T[]> {
        const response = await this.client.post<T[]>('/v1/query', query);
        return response.data;
    }
}
