import { TriggerClient } from '../clients/trigger';
import { TriggerPayload } from '../types';

export class TriggerHandler {
    private client: TriggerClient;
    public readonly payload: TriggerPayload;

    constructor(payload: TriggerPayload, baseURL: string) {
        this.payload = payload;
        if (!payload.preIssuedToken) {
            throw new Error("Trigger payload missing preIssuedToken");
        }
        this.client = new TriggerClient(baseURL, payload.preIssuedToken);
    }

    get syntrix(): TriggerClient {
        return this.client;
    }
}
