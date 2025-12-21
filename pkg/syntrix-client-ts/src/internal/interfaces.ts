import { Query } from '../types';

/**
 * @internal
 */
export interface StorageClient {
    get<T = any>(path: string): Promise<T | null>;
    create<T = any>(collectionPath: string, data: any, id?: string): Promise<T>;
    update<T = any>(path: string, data: any): Promise<T>;
    replace<T = any>(path: string, data: any): Promise<T>;
    delete(path: string): Promise<void>;
    query<T = any>(query: Query): Promise<T[]>;
}
