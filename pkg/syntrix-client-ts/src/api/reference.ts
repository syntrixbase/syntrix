import { StorageClient } from '../internal/interfaces';
import { Filter, Order, Query } from '../types';

export class DocumentReference<T = any> {
    constructor(
        private api: StorageClient,
        public readonly path: string
    ) {}

    collection(path: string): CollectionReference {
        return new CollectionReference(this.api, `${this.path}/${path}`);
    }

    async get(): Promise<T | null> {
        return this.api.get<T>(this.path);
    }

    async set(data: T): Promise<T> {
        // 'set' usually implies overwrite (replace) or create if not exists.
        // In our API, PUT is replace.
        return this.api.replace<T>(this.path, data);
    }

    async update(data: Partial<T>): Promise<T> {
        return this.api.update<T>(this.path, data);
    }

    async delete(): Promise<void> {
        return this.api.delete(this.path);
    }
}

export class CollectionReference<T = any> {
    constructor(
        private api: StorageClient,
        public readonly path: string
    ) {}

    doc(id: string): DocumentReference<T> {
        return new DocumentReference<T>(this.api, `${this.path}/${id}`);
    }

    async add(data: T, id?: string): Promise<DocumentReference<T>> {
        const result = await this.api.create<T>(this.path, data, id);
        // Assuming result has the ID, or we used the passed ID.
        // If the API returns the created object with ID, we can extract it.
        // For now, let's assume if we passed ID, we use it. If not, we hope the API returns it.
        // But our generic create returns T.
        const docId = id || (result as any).id;
        if (!docId) {
            throw new Error("Created document does not have an ID");
        }
        return this.doc(docId);
    }

    // Query methods
    where(field: string, op: string, value: any): QueryBuilder<T> {
        return new QueryBuilder<T>(this.api, this.path).where(field, op, value);
    }

    orderBy(field: string, direction: 'asc' | 'desc' = 'asc'): QueryBuilder<T> {
        return new QueryBuilder<T>(this.api, this.path).orderBy(field, direction);
    }

    limit(limit: number): QueryBuilder<T> {
        return new QueryBuilder<T>(this.api, this.path).limit(limit);
    }

    async get(): Promise<T[]> {
        return new QueryBuilder<T>(this.api, this.path).get();
    }
}

export class QueryBuilder<T = any> {
    private query: Query;

    constructor(
        private api: StorageClient,
        collection: string
    ) {
        this.query = {
            collection,
            filters: [],
            orderBy: []
        };
    }

    where(field: string, op: string, value: any): this {
        this.query.filters = this.query.filters || [];
        this.query.filters.push({ field, op, value });
        return this;
    }

    orderBy(field: string, direction: 'asc' | 'desc' = 'asc'): this {
        this.query.orderBy = this.query.orderBy || [];
        this.query.orderBy.push({ field, direction });
        return this;
    }

    limit(limit: number): this {
        this.query.limit = limit;
        return this;
    }

    startAfter(cursor: string): this {
        this.query.startAfter = cursor;
        return this;
    }

    async get(): Promise<T[]> {
        return this.api.query<T>(this.query);
    }
}
