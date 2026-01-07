# Phase 7 Dry Run Analysis

**Date**: 2026-01-06
**Task**: 027 - Streamer Service Implementation
**Phase**: 7 - Refactor Realtime Server

## Executive Summary

✅ **Overall Assessment**: Design is sound, but several implementation details need clarification before proceeding.

**Key Issues Found**:
1. 🔴 **Critical**: Subscription ID mapping between Client and Streamer
2. 🟡 **Medium**: Type conversion from `streamer.EventDelivery` to `storage.Event` format
3. 🟡 **Medium**: Hub routing logic needs redesign for subscription-based delivery
4. 🟡 **Medium**: SSE client subscription sync
5. 🟢 **Low**: Test compatibility (14 test files need updates)

---

## 1. Subscription ID Mapping Issue 🔴

### Problem
Current design assumes Hub can route events by `SubscriptionIDs`, but:
- Client maintains local `subscriptions map[string]Subscription` with message IDs as keys
- Streamer returns `SubscriptionIDs` from its own subscription manager
- **Mismatch**: Client's subscription ID (from WebSocket message) ≠ Streamer's subscription ID

### Current Flow
```
Client.handleMessage()
  → c.subscriptions[msg.ID] = Subscription{...}  // msg.ID is WebSocket message ID
  → Hub routes by iterating client.subscriptions
```

### Proposed Flow (Unclear)
```
Client.handleMessage()
  → stream.Subscribe() → returns streamerSubID
  → Need to map: clientSubID (msg.ID) ↔ streamerSubID
  → Hub receives EventDelivery with streamerSubIDs
  → Hub needs to map back to clientSubID
```

### Solution Options

**Option A: Client maintains mapping**
```go
type Client struct {
    subscriptions map[string]Subscription  // clientSubID -> Subscription
    streamerSubIDs map[string]string       // clientSubID -> streamerSubID
    streamerSubIDsReverse map[string]string // streamerSubID -> clientSubID
}
```

**Option B: Use Streamer subscription ID as client subscription ID**
- Change Client to use Streamer's subscription ID directly
- Requires modifying protocol (subscription ID in SubscribeAck)

**Option C: Hub maintains mapping**
- Hub tracks clientSubID → streamerSubID mapping
- More complex, but keeps Client simpler

**Recommendation**: Option A (Client maintains mapping) - simplest and least invasive.

---

## 2. Type Conversion: EventDelivery → storage.Event 🟡

### Problem
Hub currently expects `storage.Event`, but Streamer delivers `streamer.EventDelivery`.

### Type Differences

| Aspect | storage.Event | streamer.EventDelivery |
|--------|---------------|------------------------|
| Type | `storage.Event` struct | `*streamer.EventDelivery` |
| Event Type | `storage.EventType` (string: "create", "update", "delete") | `streamer.OperationType` (int: Insert, Update, Delete) |
| Document | `*storage.StoredDoc` | `model.Document` (map[string]interface{}) |
| ID Format | `Id` (full path like "users/1") | `DocumentID` (just ID) + `Collection` |
| Tenant | `TenantID` | `Tenant` |

### Conversion Function Needed

```go
func eventDeliveryToStorageEvent(delivery *streamer.EventDelivery) storage.Event {
    evt := storage.Event{
        Id:        fmt.Sprintf("%s/%s", delivery.Event.Collection, delivery.Event.DocumentID),
        TenantID:  delivery.Event.Tenant,
        Type:      operationToEventType(delivery.Event.Operation),
        Timestamp: delivery.Event.Timestamp,
    }

    if delivery.Event.Document != nil {
        // Convert model.Document to storage.StoredDoc
        evt.Document = documentToStoredDoc(delivery.Event.Document, delivery.Event.Collection)
    }

    return evt
}

func operationToEventType(op streamer.OperationType) storage.EventType {
    switch op {
    case streamer.OperationInsert:
        return storage.EventCreate
    case streamer.OperationUpdate:
        return storage.EventUpdate
    case streamer.OperationDelete:
        return storage.EventDelete
    default:
        return ""
    }
}

func documentToStoredDoc(doc model.Document, collection string) *storage.StoredDoc {
    // Extract system fields
    id := doc.GetID()
    version := doc.GetVersion()
    updatedAt, _ := doc["updatedAt"].(int64)
    createdAt, _ := doc["createdAt"].(int64)

    // Build Fullpath
    fullpath := fmt.Sprintf("%s/%s", collection, id)

    return &storage.StoredDoc{
        Fullpath:   fullpath,
        Collection: collection,
        Data:       doc, // model.Document is map[string]interface{}
        Version:    version,
        UpdatedAt:  updatedAt,
        CreatedAt:  createdAt,
    }
}
```

**Note**: `model.Document` is `map[string]interface{}`, which matches `storage.StoredDoc.Data`, so conversion is straightforward.

---

## 3. Hub Routing Logic Redesign 🟡

### Current Logic (Inefficient)
```go
// Hub receives storage.Event
// For each client:
//   For each subscription:
//     If collection matches:
//       If CEL program exists:
//         Evaluate CEL locally
//       If matches:
//         Send to client
```

### New Logic (Subscription-Based)
```go
// Hub receives streamer.EventDelivery
// For each subscriptionID in delivery.SubscriptionIDs:
//   Find client and subscription by subscriptionID
//   Send to client
```

### Implementation Challenge

**Problem**: Hub needs to map `streamerSubID` → `(client, clientSubID)`.

**Current State**: Hub only knows `clients map[*Client]bool`, no subscription tracking.

### Solution: Add Subscription Registry to Hub

```go
type Hub struct {
    clients map[*Client]bool
    broadcast chan *streamer.EventDelivery  // Changed from storage.Event

    // NEW: Subscription tracking
    subscriptions map[string]*SubscriptionInfo  // streamerSubID -> info
    subscriptionsMu sync.RWMutex
}

type SubscriptionInfo struct {
    Client     *Client
    ClientSubID string  // Client's subscription ID (for protocol)
}

func (h *Hub) RegisterSubscription(streamerSubID string, client *Client, clientSubID string) {
    h.subscriptionsMu.Lock()
    h.subscriptions[streamerSubID] = &SubscriptionInfo{
        Client:     client,
        ClientSubID: clientSubID,
    }
    h.subscriptionsMu.Unlock()
}

func (h *Hub) UnregisterSubscription(streamerSubID string) {
    h.subscriptionsMu.Lock()
    delete(h.subscriptions, streamerSubID)
    h.subscriptionsMu.Unlock()
}

// New routing logic
func (h *Hub) BroadcastDelivery(delivery *streamer.EventDelivery) {
    h.subscriptionsMu.RLock()
    defer h.subscriptionsMu.RUnlock()

    // Convert to storage.Event for compatibility
    storageEvt := eventDeliveryToStorageEvent(delivery)

    // Route to matching subscriptions
    for _, streamerSubID := range delivery.SubscriptionIDs {
        if info, ok := h.subscriptions[streamerSubID]; ok {
            // Send to client using clientSubID
            info.Client.mu.Lock()
            if sub, ok := info.Client.subscriptions[info.ClientSubID]; ok {
                // Build event payload
                var doc map[string]interface{}
                if sub.IncludeData {
                    doc = flattenDocument(storageEvt.Document)
                }

                payload := EventPayload{
                    SubID: info.ClientSubID,  // Use client's subscription ID
                    Delta: PublicEvent{
                        Type:      storageEvt.Type,
                        Document:  doc,
                        ID:        storageEvt.Id,
                        Timestamp: storageEvt.Timestamp,
                    },
                }
                msg := BaseMessage{Type: TypeEvent, Payload: mustMarshal(payload)}

                select {
                case info.Client.send <- msg:
                default:
                    // Non-blocking, drop if full
                }
            }
            info.Client.mu.Unlock()
        }
    }
}
```

---

## 4. SSE Client Subscription Sync 🟡

### Problem
SSE clients subscribe via query params in `ServeSSE()`:
```go
collection := r.URL.Query().Get("collection")
client.subscriptions["default"] = Subscription{
    Query: model.Query{Collection: collection},
    IncludeData: true,
}
```

This happens **before** Streamer connection is established.

### Solution
SSE clients need to:
1. Create Client struct
2. Establish Streamer stream (via Server's streamer service)
3. Subscribe to Streamer
4. Register subscription with Hub

**Challenge**: `ServeSSE()` doesn't have access to `streamer.Service`.

### Fix: Pass streamer to ServeSSE
```go
func ServeSSE(hub *Hub, qs engine.Service, streamer streamer.Service, auth identity.AuthN, cfg Config, w http.ResponseWriter, r *http.Request) {
    // ... existing code ...

    // Create stream
    stream, err := streamer.Stream(ctx)
    if err != nil {
        http.Error(w, "failed to connect to streamer", http.StatusInternalServerError)
        return
    }
    defer stream.Close()

    // Subscribe
    streamerSubID, err := stream.Subscribe(tenant, collection, nil)
    if err != nil {
        http.Error(w, "failed to subscribe", http.StatusInternalServerError)
        return
    }

    // Register with Hub
    hub.RegisterSubscription(streamerSubID, client, "default")

    // ... rest of SSE handling ...
}
```

---

## 5. Client Subscription Flow 🟡

### Current Flow (WebSocket)
```
Client.handleMessage(TypeSubscribe)
  → compileFiltersToCEL()  // Local CEL compilation
  → c.subscriptions[msg.ID] = Subscription{...}
  → Send SubscribeAck
```

### New Flow (With Streamer)
```
Client.handleMessage(TypeSubscribe)
  → Convert model.Query.Filters to streamer.Filter[]
  → stream.Subscribe(tenant, collection, filters) → streamerSubID
  → Map: clientSubID (msg.ID) ↔ streamerSubID
  → c.subscriptions[msg.ID] = Subscription{...}  // Keep for IncludeData flag
  → hub.RegisterSubscription(streamerSubID, client, msg.ID)
  → Send SubscribeAck
```

### Filter Conversion Needed

```go
func modelFiltersToStreamerFilters(filters []model.Filter) []streamer.Filter {
    result := make([]streamer.Filter, len(filters))
    for i, f := range filters {
        result[i] = streamer.Filter{
            Field: f.Field,
            Op:    modelOpToStreamerOp(f.Op),  // "==" -> "eq", etc.
            Value: f.Value,
        }
    }
    return result
}

func modelOpToStreamerOp(op string) string {
    switch op {
    case "==":
        return "eq"  // Verified: streamer supports "eq"
    case "!=":
        return "ne"  // Verified: streamer supports "ne"
    case ">":
        return "gt"  // Verified: streamer supports "gt"
    case ">=":
        return "gte" // Verified: streamer supports "gte"
    case "<":
        return "lt"  // Verified: streamer supports "lt"
    case "<=":
        return "lte" // Verified: streamer supports "lte"
    case "in":
        return "in"  // Verified: streamer supports "in"
    case "array-contains":
        return "contains" // Verified: streamer supports "contains"
    default:
        return op
    }
}
```

**✅ Verified**: All operators are supported. See `internal/streamer/internal/cel/cel.go:206-224`.

---

## 6. Server.go Changes ✅

### Current
```go
func (s *Server) StartBackgroundTasks(ctx context.Context) error {
    go s.hub.Run(ctx)

    stream, err := s.queryService.WatchCollection(ctx, "", "")
    // ... handle events ...
}
```

### Proposed (Looks Good)
```go
func (s *Server) StartBackgroundTasks(ctx context.Context) error {
    go s.hub.Run(ctx)

    stream, err := s.streamer.Stream(ctx)
    if err != nil {
        return err
    }

    go func() {
        for {
            delivery, err := stream.Recv()
            if err != nil {
                return
            }
            s.hub.BroadcastDelivery(delivery)
        }
    }()
    return nil
}
```

**Issue**: Only one Stream per Server? What if multiple Gateways?

**Clarification Needed**:
- Task says "Realtime Server is Gateway"
- One Server instance = one Gateway?
- Or one Server can handle multiple Gateway connections?

**Assumption**: One Server instance = one Gateway, so one Stream is correct.

---

## 7. Test Compatibility 🟢

### Files to Update (14 files)
- `server_background_test.go` - Mock `streamer.Service` instead of `engine.Service.WatchCollection`
- `hub_*.go` tests - Use `streamer.EventDelivery` instead of `storage.Event`
- `client_*.go` tests - Mock `streamer.Stream` for subscription tests
- `tenant_isolation_test.go` - Update event creation
- `server_sse_test.go` - Add streamer mock

### Strategy
1. Create mock `streamer.Service` and `streamer.Stream`
2. Update test helpers to create `streamer.EventDelivery`
3. Update assertions to check subscription IDs

**Estimated Effort**: ~300 lines of test code changes (as noted in task).

---

## 8. Missing System Fields in streamer.Event ✅

### Status: VERIFIED - All System Fields Included

**Conversion Flow**:
```
SyntrixChangeEvent.Document (storage.StoredDoc)
  → helper.FlattenStorageDocument()  // internal/helper/convert.go:18
  → model.Document with:
     - id (from Fullpath)
     - collection
     - version
     - updatedAt
     - createdAt
     - deleted (if applicable)
     - All user data fields from Data map
  → syntrixEventToDelivery() uses this doc
  → streamer.Event.Document contains all fields
```

**✅ Verified**: `FlattenStorageDocument()` in `internal/helper/convert.go:18-35` includes all system fields:
- `id` (via `SetID()`)
- `collection` (via `SetCollection()`)
- `version`
- `updatedAt`
- `createdAt`
- `deleted` (if true)

**No action needed** - system fields are properly preserved.

---

## 9. Error Handling 🟡

### Stream Errors
```go
for {
    delivery, err := stream.Recv()
    if err != nil {
        // What happens here?
        // - EOF: Stream closed normally
        // - Context cancelled: Shutdown
        // - Network error: Reconnect?
        return
    }
    s.hub.BroadcastDelivery(delivery)
}
```

**Question**: Should Server reconnect on stream errors? Or is that handled by streamer client?

**From task 030**: Streamer client has auto-reconnect. So Server should just log and return.

---

## 10. Summary of Required Changes

### Files to Modify

1. **server.go**
   - Add `streamer streamer.Service` field
   - Update `StartBackgroundTasks()` to use `streamer.Stream()`
   - Update constructor to accept `streamer.Service`

2. **hub.go**
   - Change `broadcast chan storage.Event` → `broadcast chan *streamer.EventDelivery`
   - Add subscription registry: `subscriptions map[string]*SubscriptionInfo`
   - Add `BroadcastDelivery()` method
   - Add `RegisterSubscription()` / `UnregisterSubscription()` methods
   - Add conversion function: `eventDeliveryToStorageEvent()`

3. **client.go**
   - Add `stream streamer.Stream` field
   - Add `streamerSubIDs map[string]string` (clientSubID → streamerSubID)
   - Update `handleMessage(TypeSubscribe)` to call `stream.Subscribe()`
   - Update `handleMessage(TypeUnsubscribe)` to call `stream.Unsubscribe()`
   - Remove `compileFiltersToCEL()` call
   - Add filter conversion: `modelFiltersToStreamerFilters()`

4. **cel.go**
   - Delete entire file (106 lines)

5. **protocol.go**
   - No changes needed (uses `storage.EventType` which is compatible)

6. **server.go** (ServeSSE)
   - Add `streamer streamer.Service` parameter
   - Create stream and subscribe in `ServeSSE()`

---

## Recommendations

### Before Implementation

1. ✅ **Verify filter operator mapping**: **DONE** - All operators supported, conversion function needed
2. ✅ **Verify system fields**: **DONE** - All system fields included via `FlattenStorageDocument()`
3. ✅ **Clarify Gateway model**: **CONFIRMED** - One Server = one Gateway = one Stream
4. ✅ **Review streamer.Filter operators**: **DONE** - Mapping function designed and verified

### Implementation Order

1. **Phase 7.1**: Update `server.go` (add streamer field, update StartBackgroundTasks)
2. **Phase 7.2**: Update `hub.go` (add subscription registry, BroadcastDelivery)
3. **Phase 7.3**: Update `client.go` (add stream, sync subscriptions)
4. **Phase 7.4**: Delete `cel.go`
5. **Phase 7.5**: Update `ServeSSE()` to use streamer
6. **Phase 7.6**: Update all tests

### Risk Mitigation

- **Backward Compatibility**: Keep `storage.Event` conversion for protocol compatibility
- **Testing**: Update tests incrementally, run after each phase
- **Rollback**: Keep old code commented until Phase 8 cleanup

---

## Open Questions (RESOLVED)

1. ✅ **Q**: Does streamer.Filter support all model.Filter operators?
   **A**: **YES** - Verified in `internal/streamer/internal/cel/cel.go`:
   - `model.Filter` operators: `==`, `>`, `>=`, `<`, `<=`, `in`, `array-contains`
   - `streamer.Filter` operators: `eq`, `ne`, `gt`, `gte`, `lt`, `lte`, `in`, `contains`
   - Mapping needed: `==` → `eq`, `array-contains` → `contains`
   - **Note**: `model.Filter` uses `==` but streamer uses `eq`. Need conversion function.

2. ✅ **Q**: Does streamer.Event.Document include system fields (version, timestamps)?
   **A**: **YES** - Verified in `internal/helper/convert.go`:
   - `FlattenStorageDocument()` includes: `id`, `collection`, `version`, `updatedAt`, `createdAt`, `deleted`
   - Used in `service.go:237`: `doc := helper.FlattenStorageDocument(event.Document)`
   - All system fields are preserved in `model.Document`

3. ✅ **Q**: Should Server handle stream reconnection, or rely on streamer client?
   **A**: **Client handles reconnection** - From task 030, streamer client has auto-reconnect. Server should log error and return (let client reconnect).

4. ⚠️ **Q**: One Stream per Server, or one Stream per Gateway connection?
   **A**: **One Stream per Server instance** - Based on design:
   - `Server.StartBackgroundTasks()` creates one Stream
   - All clients share the same Stream
   - Hub routes events to clients based on subscription IDs
   - **This is correct** - one Gateway (Server) = one Stream connection to Streamer

---

## Conclusion

✅ **READY TO PROCEED** - All open questions resolved:

### ✅ Resolved Issues
1. **Filter operators**: All supported, conversion function designed
2. **System fields**: Verified - all fields included via `FlattenStorageDocument()`
3. **Gateway model**: Confirmed - one Server = one Gateway = one Stream
4. **Type conversions**: Clear path defined with helper functions

### ⚠️ Critical Implementation Points

1. **Subscription ID Mapping** (🔴 High Priority)
   - Client must maintain mapping: `clientSubID` ↔ `streamerSubID`
   - Hub must track subscriptions: `streamerSubID` → `(client, clientSubID)`
   - See Section 1 for detailed solution

2. **Hub Routing Redesign** (🟡 Medium Priority)
   - Change from broadcast-to-all to subscription-based routing
   - Add subscription registry to Hub
   - See Section 3 for implementation

3. **SSE Client Integration** (🟡 Medium Priority)
   - SSE clients need Streamer stream access
   - Update `ServeSSE()` signature to include `streamer.Service`
   - See Section 4 for details

### Implementation Readiness

**Estimated Complexity**: Medium
**Estimated Lines Changed**: ~500 lines
**Estimated Test Updates**: ~300 lines
**Risk Level**: Medium (well-defined interfaces, clear conversion path)

**Blockers**: None
**Dependencies**: All resolved
**Next Step**: Begin Phase 7.1 (Update server.go)
