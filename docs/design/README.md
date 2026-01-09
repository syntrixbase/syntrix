# Design Documents

This directory now splits designs into server-side and SDK-focused documents.

## Arch Overview

```mermaid
graph TB
    Client[Client SDK]
    Gateway[API Gateway: HTTP, Websocket, SSE]
    Streamer[Streamers, Stateful]
    Indexer[A Group of Indexers, Sharded & Copies, Stateful, Presistant]
    QueryServer[A Group of Query Servers, Stateless]
    MongoDB[(MongoDB Storage)]

    Client --> |HTTP/SSE/Websocket|Gateway

    Gateway ---> |gRPC| QueryServer ---> |Get/Put| MongoDB
    QueryServer ---> |Query| Indexer
    Indexer ---> |gRPC Streaming| Puller

    Gateway ---> |Register/Unregister|Streamer
    Streamer ---> |gRPC Streaming| Gateway
    Streamer ---> |gRPC Streaming| Puller

    Puller ---> |ChangeStream| MongoDB


    TriggerEval --->|gRPC Streaming| Puller
    TriggerEval ---> |Pub| NATS2 -->|Sub| TriggerWorker
    TriggerWorker --> External

    subgraph Trigger
        NATS2[(NATS Jetstream)]
        TriggerEval[Sharded Trigger Evaluators]
        TriggerWorker[A Group of Trigger Workers]
    end

    subgraph External
      Webhook[Webhook Worker]
      Lambda[Cloud Lambda]
      Function[Function Compute]
    end

```

## Puller Event Schema

The Puller service emits events with the following characteristics:

- **Event Wrapper**: Events are wrapped in a `PullerEvent` structure containing the `change_event` and a `progress` marker.
- **Change Event**: The core event payload (`ChangeEvent`) reflects the MongoDB change stream event but with normalized fields.
- **Mongo Provenance**: Fields `mgoColl` and `mgoDocId` explicitly indicate the MongoDB collection and document ID (document key).
- **Full Document Contract**:
  - The `fullDoc` field MUST conform to the `storage.StoredDoc` JSON shape (containing `id`, `databaseId`, `fullpath`).
  - Update operations MUST use `UpdateLookup` to ensure `fullDoc` is present.
  - Events without `fullDoc` (except deletes) are rejected.
- **Database Identification**: Database ID is derived SOLELY from `fullDoc.DatabaseID`.
