package query

import (
	"context"
	"net/http"

	"github.com/syntrixbase/syntrix/internal/query/internal/client"
	"github.com/syntrixbase/syntrix/internal/query/internal/core"
	"github.com/syntrixbase/syntrix/internal/query/internal/httphandler"
	"github.com/syntrixbase/syntrix/internal/storage"
	"github.com/syntrixbase/syntrix/pkg/model"
)

// Service defines the interface for the Query Engine.
// Both the local Engine and the remote Client implement this interface.
type Service interface {
	GetDocument(ctx context.Context, tenant string, path string) (model.Document, error)
	CreateDocument(ctx context.Context, tenant string, doc model.Document) error
	ReplaceDocument(ctx context.Context, tenant string, data model.Document, pred model.Filters) (model.Document, error)
	PatchDocument(ctx context.Context, tenant string, data model.Document, pred model.Filters) (model.Document, error)
	DeleteDocument(ctx context.Context, tenant string, path string, pred model.Filters) error
	ExecuteQuery(ctx context.Context, tenant string, q model.Query) ([]model.Document, error)
	Pull(ctx context.Context, tenant string, req storage.ReplicationPullRequest) (*storage.ReplicationPullResponse, error)
	Push(ctx context.Context, tenant string, req storage.ReplicationPushRequest) (*storage.ReplicationPushResponse, error)
}

// NewService creates a new local Query Service.
func NewService(store storage.DocumentStore) Service {
	return core.New(store)
}

// NewClient creates a new remote Query Service client (gRPC client).
// Use this when the query service is running remotely.
func NewClient(address string) (Service, error) {
	return client.New(address)
}

// NewHTTPHandler creates an HTTP handler for the Query Service.
// The handler exposes the Service interface over HTTP.
func NewHTTPHandler(s Service) http.Handler {
	return httphandler.New(s)
}

// ============================================================================
// Deprecated type aliases and functions - for backward compatibility.
// These will be removed in a future release.
// ============================================================================

// Engine is a deprecated alias. Use NewService instead.
// Deprecated: Use NewService to get a Service interface.
type Engine = core.Engine
