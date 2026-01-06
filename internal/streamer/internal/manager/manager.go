package manager

import (
	"fmt"
	"sync"

	pb "github.com/syntrixbase/syntrix/api/gen/streamer/v1"
	"github.com/syntrixbase/syntrix/pkg/model"
)

// Manager handles subscription registration and matching.
// It maintains two lookup paths:
//   - Fast path: exact Collection+DocumentID matches (O(1))
//   - Slow path: CEL expression evaluation (O(n) per expression)
type Manager struct {
	// exactMatches: tenant -> collection -> docID -> []*Subscriber
	exactMatches map[string]map[string]map[string][]*Subscriber

	// expressions: tenant -> collection -> []*ExpressionSubscriber
	expressions map[string]map[string][]*ExpressionSubscriber

	// gateways: gatewayID -> GatewaySubscriptions
	gateways map[string]*GatewaySubscriptions

	// allSubs: subID -> *Subscriber (for O(1) unsubscribe lookup)
	allSubs map[string]*Subscriber

	// expressionSubs: subID -> *ExpressionSubscriber (for O(1) removal)
	expressionSubs map[string]*ExpressionSubscriber

	mu sync.RWMutex

	// celCompiler compiles CEL expressions
	celCompiler CELCompiler
}

// CELCompiler compiles CEL expressions into executable programs.
type CELCompiler interface {
	Compile(expr string) (*ExpressionSubscriber, error)
	CompileProtoFilters(filters []*pb.Filter) (*ExpressionSubscriber, error)
}

// ManagerOption configures the Manager.
type ManagerOption func(*Manager)

// WithCELCompiler sets the CEL compiler.
func WithCELCompiler(c CELCompiler) ManagerOption {
	return func(m *Manager) {
		m.celCompiler = c
	}
}

// New creates a new subscription Manager.
func New(opts ...ManagerOption) *Manager {
	m := &Manager{
		exactMatches:   make(map[string]map[string]map[string][]*Subscriber),
		expressions:    make(map[string]map[string][]*ExpressionSubscriber),
		gateways:       make(map[string]*GatewaySubscriptions),
		allSubs:        make(map[string]*Subscriber),
		expressionSubs: make(map[string]*ExpressionSubscriber),
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// Subscribe registers a new subscription.
func (m *Manager) Subscribe(gatewayID string, req *pb.SubscribeRequest) (*pb.SubscribeResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check for duplicate subscription ID
	if _, exists := m.allSubs[req.SubscriptionId]; exists {
		return &pb.SubscribeResponse{
			SubscriptionId: req.SubscriptionId,
			Success:        false,
			Error:          "subscription ID already exists",
		}, nil
	}

	sub := &Subscriber{
		ID:         req.SubscriptionId,
		GatewayID:  gatewayID,
		Tenant:     req.Tenant,
		Collection: req.Collection,
		Filters:    req.Filters,
	}

	// Register with gateway
	gw := m.getOrCreateGateway(gatewayID)
	gw.Add(sub)

	// Store in allSubs for O(1) lookup
	m.allSubs[sub.ID] = sub

	// Add to appropriate index based on filter type
	if sub.IsExactMatch() {
		// Fast path: exact document ID match
		m.addExactMatch(sub)
	} else if len(sub.Filters) > 0 {
		// Slow path: CEL expression match
		if err := m.addExpressionMatch(sub); err != nil {
			// Rollback
			gw.Remove(sub.ID)
			delete(m.allSubs, sub.ID)
			return &pb.SubscribeResponse{
				SubscriptionId: req.SubscriptionId,
				Success:        false,
				Error:          fmt.Sprintf("failed to compile filters: %v", err),
			}, nil
		}
	} else {
		// Collection-level subscription (matches all docs in collection)
		m.addExactMatch(sub)
	}

	return &pb.SubscribeResponse{
		SubscriptionId: req.SubscriptionId,
		Success:        true,
	}, nil
}

// Unsubscribe removes a subscription.
func (m *Manager) Unsubscribe(subscriptionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	sub, exists := m.allSubs[subscriptionID]
	if !exists {
		return fmt.Errorf("subscription not found: %s", subscriptionID)
	}

	// Remove from gateway
	if gw, ok := m.gateways[sub.GatewayID]; ok {
		gw.Remove(subscriptionID)
	}

	// Remove from indexes
	if sub.IsExactMatch() || len(sub.Filters) == 0 {
		m.removeExactMatch(sub)
	} else {
		m.removeExpressionMatch(sub)
	}

	delete(m.allSubs, subscriptionID)
	delete(m.expressionSubs, subscriptionID)

	return nil
}

// UnregisterGateway removes all subscriptions for a gateway.
func (m *Manager) UnregisterGateway(gatewayID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	gw, ok := m.gateways[gatewayID]
	if !ok {
		return
	}

	// Remove all subscriptions for this gateway
	for _, sub := range gw.List() {
		if sub.IsExactMatch() || len(sub.Filters) == 0 {
			m.removeExactMatch(sub)
		} else {
			m.removeExpressionMatch(sub)
		}
		delete(m.allSubs, sub.ID)
		delete(m.expressionSubs, sub.ID)
	}

	delete(m.gateways, gatewayID)
}

// Match finds all subscriptions that match the given event.
// Returns a map of gatewayID -> []subscriptionID.
func (m *Manager) Match(tenant, collection, docID string, doc model.Document) map[string][]string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string][]string)

	// Fast path: exact matches
	m.matchExact(tenant, collection, docID, result)

	// Slow path: CEL expressions
	m.matchExpressions(tenant, collection, doc, result)

	return result
}

// GetSubscription returns a subscription by ID.
func (m *Manager) GetSubscription(subID string) (*Subscriber, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	sub, ok := m.allSubs[subID]
	return sub, ok
}

// Stats returns subscription statistics.
func (m *Manager) Stats() (totalSubs, exactMatches, expressions, gateways int) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.allSubs), m.countExactMatches(), len(m.expressionSubs), len(m.gateways)
}

// --- Internal methods ---

func (m *Manager) getOrCreateGateway(gatewayID string) *GatewaySubscriptions {
	gw, ok := m.gateways[gatewayID]
	if !ok {
		gw = NewGatewaySubscriptions()
		m.gateways[gatewayID] = gw
	}
	return gw
}

func (m *Manager) addExactMatch(sub *Subscriber) {
	tenantMap, ok := m.exactMatches[sub.Tenant]
	if !ok {
		tenantMap = make(map[string]map[string][]*Subscriber)
		m.exactMatches[sub.Tenant] = tenantMap
	}

	collMap, ok := tenantMap[sub.Collection]
	if !ok {
		collMap = make(map[string][]*Subscriber)
		tenantMap[sub.Collection] = collMap
	}

	// Use empty string key for collection-level subscriptions
	key := sub.GetDocumentID()
	collMap[key] = append(collMap[key], sub)
}

func (m *Manager) removeExactMatch(sub *Subscriber) {
	tenantMap, ok := m.exactMatches[sub.Tenant]
	if !ok {
		return
	}

	collMap, ok := tenantMap[sub.Collection]
	if !ok {
		return
	}

	key := sub.GetDocumentID()
	subs := collMap[key]
	for i, s := range subs {
		if s.ID == sub.ID {
			collMap[key] = append(subs[:i], subs[i+1:]...)
			break
		}
	}

	// Cleanup empty maps
	if len(collMap[key]) == 0 {
		delete(collMap, key)
	}
	if len(collMap) == 0 {
		delete(tenantMap, sub.Collection)
	}
	if len(tenantMap) == 0 {
		delete(m.exactMatches, sub.Tenant)
	}
}

func (m *Manager) addExpressionMatch(sub *Subscriber) error {
	if m.celCompiler == nil {
		return fmt.Errorf("CEL compiler not configured")
	}

	exprSub, err := m.celCompiler.CompileProtoFilters(sub.Filters)
	if err != nil {
		return err
	}
	exprSub.Subscriber = sub

	tenantMap, ok := m.expressions[sub.Tenant]
	if !ok {
		tenantMap = make(map[string][]*ExpressionSubscriber)
		m.expressions[sub.Tenant] = tenantMap
	}

	tenantMap[sub.Collection] = append(tenantMap[sub.Collection], exprSub)
	m.expressionSubs[sub.ID] = exprSub

	return nil
}

func (m *Manager) removeExpressionMatch(sub *Subscriber) {
	tenantMap, ok := m.expressions[sub.Tenant]
	if !ok {
		return
	}

	subs := tenantMap[sub.Collection]
	for i, s := range subs {
		if s.ID == sub.ID {
			tenantMap[sub.Collection] = append(subs[:i], subs[i+1:]...)
			break
		}
	}

	// Cleanup empty maps
	if len(tenantMap[sub.Collection]) == 0 {
		delete(tenantMap, sub.Collection)
	}
	if len(tenantMap) == 0 {
		delete(m.expressions, sub.Tenant)
	}
}

func (m *Manager) matchExact(tenant, collection, docID string, result map[string][]string) {
	tenantMap, ok := m.exactMatches[tenant]
	if !ok {
		return
	}

	collMap, ok := tenantMap[collection]
	if !ok {
		return
	}

	// Match specific document subscriptions
	for _, sub := range collMap[docID] {
		result[sub.GatewayID] = append(result[sub.GatewayID], sub.ID)
	}

	// Match collection-level subscriptions (empty docID key)
	for _, sub := range collMap[""] {
		result[sub.GatewayID] = append(result[sub.GatewayID], sub.ID)
	}
}

func (m *Manager) matchExpressions(tenant, collection string, doc model.Document, result map[string][]string) {
	tenantMap, ok := m.expressions[tenant]
	if !ok {
		return
	}

	subs, ok := tenantMap[collection]
	if !ok {
		return
	}

	for _, exprSub := range subs {
		if m.evaluateCEL(exprSub, doc) {
			result[exprSub.GatewayID] = append(result[exprSub.GatewayID], exprSub.ID)
		}
	}
}

func (m *Manager) evaluateCEL(exprSub *ExpressionSubscriber, doc model.Document) bool {
	if exprSub.Program == nil {
		return false
	}

	out, _, err := exprSub.Program.Eval(map[string]interface{}{
		"doc": doc,
	})
	if err != nil {
		return false
	}

	result, ok := out.Value().(bool)
	return ok && result
}

func (m *Manager) countExactMatches() int {
	count := 0
	for _, tenantMap := range m.exactMatches {
		for _, collMap := range tenantMap {
			for _, subs := range collMap {
				count += len(subs)
			}
		}
	}
	return count
}
