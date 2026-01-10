// Package indexer provides the Indexer service for maintaining secondary indexes.
//
// The Indexer service subscribes to change events from the Puller service and
// maintains in-memory btree indexes for fast query execution. It provides:
//
//   - Template-based indexing: Define indexes via YAML configuration
//   - Query-to-Index matching: Automatically select the best index for queries
//   - Pagination support: Cursor-based pagination with OrderKey encoding
//   - Multi-database isolation: Separate index namespaces per database
//
// # Usage
//
//	cfg := indexer.Config{TemplatePath: "config/templates.yaml"}
//	svc := indexer.NewService(cfg, pullerSvc, logger)
//	svc.Start(ctx)
//
//	// Search using the index
//	results, err := svc.Search(ctx, "mydb", plan)
package indexer

import (
	"context"

	"github.com/syntrixbase/syntrix/internal/indexer/internal/manager"
)

// Service defines the interface for the Indexer service.
type Service interface {
	// Search returns document references matching the query plan.
	// Indexer internally selects the best matching template based on
	// plan.OrderBy and plan.Filters using the Query-to-Index matching rules.
	Search(ctx context.Context, database string, plan Plan) ([]DocRef, error)

	// Health returns current health status of the indexer.
	Health(ctx context.Context) (Health, error)

	// Stats returns index statistics.
	Stats(ctx context.Context) (Stats, error)
}

// LocalService extends Service with methods for managing the indexer lifecycle.
type LocalService interface {
	Service

	// Start starts the indexer service, including Puller subscription.
	Start(ctx context.Context) error

	// Stop gracefully stops the indexer service.
	Stop(ctx context.Context) error

	// ApplyEvent applies a single change event to the indexes.
	// This is called by the Puller subscription handler.
	ApplyEvent(ctx context.Context, evt *ChangeEvent) error

	// Manager returns the underlying index manager for advanced operations.
	Manager() *manager.Manager
}

// Plan represents a query plan passed from Query Engine.
type Plan = manager.Plan

// Filter represents a query filter on a field.
type Filter = manager.Filter

// FilterOp represents the type of filter operation.
type FilterOp = manager.FilterOp

// Filter operation constants.
const (
	FilterEq  = manager.FilterEq
	FilterGt  = manager.FilterGt
	FilterLt  = manager.FilterLt
	FilterGte = manager.FilterGte
	FilterLte = manager.FilterLte
)

// OrderField represents an ordering specification.
type OrderField = manager.OrderField

// DocRef represents a document reference with its OrderKey.
type DocRef = manager.DocRef

// Health represents the health status of the indexer.
type Health struct {
	Status      HealthStatus      // Overall status
	ShardHealth map[string]string // Per-shard status (key: database|pattern|templateID)
	LastError   string            // Last error message if any
}

// HealthStatus represents the health status.
type HealthStatus string

const (
	HealthOK        HealthStatus = "ok"
	HealthDegraded  HealthStatus = "degraded"
	HealthUnhealthy HealthStatus = "unhealthy"
)

// Stats represents index statistics.
type Stats struct {
	DatabaseCount int   // Number of databases
	ShardCount    int   // Total number of shards
	TemplateCount int   // Number of loaded templates
	DocumentCount int64 // Total indexed documents
	LastEventTime int64 // Unix timestamp of last processed event
	EventsApplied int64 // Total events applied
}

// ChangeEvent is the change event type from Puller.
type ChangeEvent = manager.ChangeEvent
