package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/syntrixbase/syntrix/internal/core/identity"
	"github.com/syntrixbase/syntrix/internal/query"
	"github.com/syntrixbase/syntrix/internal/server"
	"github.com/syntrixbase/syntrix/pkg/model"
)

// Context keys for request-scoped values
type contextKey string

const (
	contextKeyParsedBody contextKey = "parsed_body"
)

// getParsedBody retrieves the cached parsed body from the context.
// Returns nil if no cached body exists.
func getParsedBody(ctx context.Context) map[string]interface{} {
	if data, ok := ctx.Value(contextKeyParsedBody).(map[string]interface{}); ok {
		return data
	}
	return nil
}

type Handler struct {
	engine query.Service
	auth   identity.AuthN
	authz  identity.AuthZ
}

func NewHandler(engine query.Service, auth identity.AuthN, authz identity.AuthZ) *Handler {
	if auth == nil {
		panic("AuthN service cannot be nil")
	}

	return &Handler{
		engine: engine,
		auth:   auth,
		authz:  authz,
	}
}

// Default body size limits
const (
	DefaultMaxBodySize = 1 << 20  // 1MB
	LargeMaxBodySize   = 10 << 20 // 10MB for admin operations
)

// Default request timeout
const (
	DefaultRequestTimeout = 30 * time.Second
	LongRequestTimeout    = 60 * time.Second // For replication and admin operations
)

// APIError represents a structured error response
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Error codes
const (
	ErrCodeBadRequest         = "BAD_REQUEST"
	ErrCodeUnauthorized       = "UNAUTHORIZED"
	ErrCodeForbidden          = "FORBIDDEN"
	ErrCodeNotFound           = "NOT_FOUND"
	ErrCodeConflict           = "CONFLICT"
	ErrCodePreconditionFailed = "PRECONDITION_FAILED"
	ErrCodeRequestTooLarge    = "REQUEST_TOO_LARGE"
	ErrCodeInternalError      = "INTERNAL_ERROR"
)

// writeError writes a structured JSON error response
func writeError(w http.ResponseWriter, status int, code string, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(APIError{Code: code, Message: message}); err != nil {
		slog.Warn("Failed to encode error response", "error", err)
	}
}

// writeStorageError writes an appropriate error response for storage errors
func writeStorageError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, model.ErrNotFound):
		writeError(w, http.StatusNotFound, ErrCodeNotFound, "Document not found")
	case errors.Is(err, model.ErrExists):
		writeError(w, http.StatusConflict, ErrCodeConflict, "Document already exists")
	case errors.Is(err, model.ErrPreconditionFailed):
		writeError(w, http.StatusPreconditionFailed, ErrCodePreconditionFailed, "Version conflict")
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		// Client closed the connection - use 499 (Nginx convention) or just return
		// since client won't receive the response anyway
		w.WriteHeader(499) // Client Closed Request
	default:
		slog.Error("Internal storage error", "error", err)
		writeError(w, http.StatusInternalServerError, ErrCodeInternalError, "Internal server error")
	}
}

// writeJSON writes a JSON response with proper error handling
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Warn("Failed to encode JSON response", "error", err)
	}
}

// logRequest logs request information using structured logging
func logRequest(r *http.Request, status int, duration time.Duration) {
	slog.Info("HTTP request",
		"method", r.Method,
		"path", r.URL.Path,
		"status", status,
		"duration_ms", duration.Milliseconds(),
		"request_id", server.GetRequestID(r.Context()),
	)
}

// maxBodySize wraps a handler with request body size limiting
func maxBodySize(next http.HandlerFunc, maxBytes int64) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
		}
		next(w, r)
	}
}

// withTimeout wraps a handler with a context timeout
// If the handler takes longer than the timeout, the context is cancelled
func withTimeout(next http.HandlerFunc, timeout time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		next(w, r.WithContext(ctx))
	}
}

func (h *Handler) getDatabase(r *http.Request) (string, error) {
	if database, ok := r.Context().Value(ContextKeyDatabase).(string); ok && database != "" {
		return database, nil
	}

	return "", identity.ErrDatabaseRequired
}

func (h *Handler) databaseOrError(w http.ResponseWriter, r *http.Request) (string, bool) {
	database, err := h.getDatabase(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, ErrCodeUnauthorized, "Database identification required")
		return "", false
	}
	return database, true
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// Document Operations (with body size limit for write operations)
	// Note: Request ID and panic recovery are handled by the unified server middleware
	mux.HandleFunc("GET /api/v1/{path...}", withTimeout(h.maybeProtected(h.authorized(h.handleGetDocument, "read")), DefaultRequestTimeout))
	mux.HandleFunc("POST /api/v1/{path...}", withTimeout(maxBodySize(h.maybeProtected(h.authorized(h.handleCreateDocument, "create")), DefaultMaxBodySize), DefaultRequestTimeout))
	mux.HandleFunc("PUT /api/v1/{path...}", withTimeout(maxBodySize(h.maybeProtected(h.authorized(h.handleReplaceDocument, "update")), DefaultMaxBodySize), DefaultRequestTimeout))
	mux.HandleFunc("PATCH /api/v1/{path...}", withTimeout(maxBodySize(h.maybeProtected(h.authorized(h.handlePatchDocument, "update")), DefaultMaxBodySize), DefaultRequestTimeout))
	mux.HandleFunc("DELETE /api/v1/{path...}", withTimeout(maxBodySize(h.maybeProtected(h.authorized(h.handleDeleteDocument, "delete")), DefaultMaxBodySize), DefaultRequestTimeout))

	// Query Operations
	mux.HandleFunc("POST /api/v1/query", withTimeout(maxBodySize(h.protected(h.handleQuery), DefaultMaxBodySize), DefaultRequestTimeout))

	// Replication Operations (use longer timeout for potentially large data transfers)
	mux.HandleFunc("GET /replication/v1/pull", withTimeout(h.protected(h.handlePull), LongRequestTimeout))
	mux.HandleFunc("POST /replication/v1/push", withTimeout(maxBodySize(h.protected(h.handlePush), LargeMaxBodySize), LongRequestTimeout))

	// Trigger Internal Operations
	mux.HandleFunc("POST /trigger/v1/get", withTimeout(maxBodySize(h.triggerProtected(h.handleTriggerGet), DefaultMaxBodySize), DefaultRequestTimeout))
	mux.HandleFunc("POST /trigger/v1/query", withTimeout(maxBodySize(h.triggerProtected(h.handleQuery), DefaultMaxBodySize), DefaultRequestTimeout))
	mux.HandleFunc("POST /trigger/v1/write", withTimeout(maxBodySize(h.triggerProtected(h.handleTriggerWrite), DefaultMaxBodySize), DefaultRequestTimeout))

	// Auth Operations
	if h.auth != nil {
		mux.HandleFunc("POST /auth/v1/signup", withTimeout(maxBodySize(h.handleSignUp, DefaultMaxBodySize), DefaultRequestTimeout))
		mux.HandleFunc("POST /auth/v1/login", withTimeout(maxBodySize(h.handleLogin, DefaultMaxBodySize), DefaultRequestTimeout))
		mux.HandleFunc("POST /auth/v1/refresh", withTimeout(maxBodySize(h.handleRefresh, DefaultMaxBodySize), DefaultRequestTimeout))
		mux.HandleFunc("POST /auth/v1/logout", withTimeout(maxBodySize(h.handleLogout, DefaultMaxBodySize), DefaultRequestTimeout))

		// Admin Operations (use longer timeout)
		mux.HandleFunc("GET /admin/users", withTimeout(h.adminOnly(h.handleAdminListUsers), LongRequestTimeout))
		mux.HandleFunc("PATCH /admin/users/{id}", withTimeout(maxBodySize(h.adminOnly(h.handleAdminUpdateUser), DefaultMaxBodySize), DefaultRequestTimeout))
		mux.HandleFunc("GET /admin/rules", withTimeout(h.adminOnly(h.handleAdminGetRules), DefaultRequestTimeout))
		mux.HandleFunc("POST /admin/rules/push", withTimeout(maxBodySize(h.adminOnly(h.handleAdminPushRules), LargeMaxBodySize), LongRequestTimeout))
		mux.HandleFunc("GET /admin/health", withTimeout(h.adminOnly(h.handleAdminHealth), DefaultRequestTimeout))
	}

	// Health Check (no auth, minimal timeout)
	mux.HandleFunc("GET /health", withTimeout(h.handleHealth, 5*time.Second))
}

func (h *Handler) protected(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h.auth.Middleware(handler).ServeHTTP(w, r)
	}
}

func (h *Handler) maybeProtected(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h.auth.MiddlewareOptional(handler).ServeHTTP(w, r)
	}
}

func (h *Handler) authorized(handler http.HandlerFunc, action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.authz == nil {
			handler(w, r)
			return
		}

		path := r.PathValue("path")

		// Extract database from context (defaults to "default")
		database := "default"
		if db, ok := r.Context().Value(ContextKeyDatabase).(string); ok && db != "" {
			database = db
		}

		// Build Request Context
		reqCtx := identity.AuthzRequest{
			Time: time.Now(),
		}

		// Extract Auth
		if uid, ok := r.Context().Value(identity.ContextKeyUserID).(string); ok {
			reqCtx.Auth.UID = uid
		}
		if username, ok := r.Context().Value(identity.ContextKeyUsername).(string); ok {
			reqCtx.Auth.Username = username
		}
		if roles, ok := r.Context().Value(identity.ContextKeyRoles).([]string); ok {
			reqCtx.Auth.Roles = append([]string{}, roles...)
		}
		if claims, ok := r.Context().Value(identity.ContextKeyClaims).(*identity.Claims); ok {
			reqCtx.Auth.Claims = claimsToMap(claims)
		}

		// Fetch Existing Resource if needed
		var existingRes *identity.Resource
		if action != "create" {
			doc, err := h.engine.GetDocument(r.Context(), database, path)
			if err == nil {
				data := model.Document{}
				for k, v := range doc {
					data[k] = v
				}
				data.StripProtectedFields()
				existingRes = &identity.Resource{
					Data: data,
					ID:   doc.GetID(),
				}
			} else if err != model.ErrNotFound {
				writeError(w, http.StatusInternalServerError, ErrCodeInternalError, "Failed to check resource")
				return
			}
		}

		// For Create/Update, extract new data and cache parsed body
		if action == "create" || action == "update" {
			bodyBytes, err := io.ReadAll(r.Body)
			if err != nil {
				writeError(w, http.StatusBadRequest, ErrCodeBadRequest, "Failed to read request body")
				return
			}
			r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

			var data map[string]interface{}
			if err := json.Unmarshal(bodyBytes, &data); err == nil {
				reqCtx.Resource = &identity.Resource{Data: data}
				// Cache parsed body in context to avoid double parsing in handlers
				ctx := context.WithValue(r.Context(), contextKeyParsedBody, data)
				r = r.WithContext(ctx)
			}
			// Note: If JSON parsing fails here, we don't cache and let the handler
			// report the error with more specific context
		}

		allowed, err := h.authz.Evaluate(r.Context(), database, path, action, reqCtx, existingRes)
		if err != nil {
			slog.Warn("Authorization rule evaluation error",
				"database", database,
				"path", path,
				"action", action,
				"error", err,
				"request_id", server.GetRequestID(r.Context()),
			)
			writeError(w, http.StatusForbidden, ErrCodeForbidden, "Authorization check failed")
			return
		}

		if !allowed {
			writeError(w, http.StatusForbidden, ErrCodeForbidden, "Access denied")
			return
		}

		handler(w, r)
	}
}

func claimsToMap(claims *identity.Claims) map[string]interface{} {
	if claims == nil {
		return nil
	}

	toTime := func(nd *jwt.NumericDate) interface{} {
		if nd == nil {
			return nil
		}
		return nd.Time
	}

	return map[string]interface{}{
		"sub":      claims.Subject,
		"database": claims.Database,
		"tid":      claims.TenantID,
		"oid":      claims.UserID,
		"username": claims.Username,
		"roles":    append([]string{}, claims.Roles...),
		"disabled": claims.Disabled,
		"aud":      claims.Audience,
		"iss":      claims.Issuer,
		"jti":      claims.ID,
		"nbf":      toTime(claims.NotBefore),
		"exp":      toTime(claims.ExpiresAt),
		"iat":      toTime(claims.IssuedAt),
	}
}

func (h *Handler) triggerProtected(handler http.HandlerFunc) http.HandlerFunc {
	if h.auth == nil {
		return handler
	}
	return func(w http.ResponseWriter, r *http.Request) {
		// First, run standard auth middleware to validate token
		h.auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check roles
			roles, ok := r.Context().Value(identity.ContextKeyRoles).([]string)
			if !ok {
				writeError(w, http.StatusForbidden, ErrCodeForbidden, "Access denied")
				return
			}

			isSystem := false
			for _, role := range roles {
				if role == "system" {
					isSystem = true
					break
				}
			}

			if !isSystem {
				writeError(w, http.StatusForbidden, ErrCodeForbidden, "System access required")
				return
			}

			handler(w, r)
		})).ServeHTTP(w, r)
	}
}

func (h *Handler) adminOnly(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// First, run standard auth middleware to validate token
		h.auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check roles
			roles, ok := r.Context().Value(identity.ContextKeyRoles).([]string)
			if !ok {
				writeError(w, http.StatusForbidden, ErrCodeForbidden, "Access denied")
				return
			}

			isAdmin := false
			for _, role := range roles {
				if role == "admin" || role == "system" {
					isAdmin = true
					break
				}
			}

			if !isAdmin {
				writeError(w, http.StatusForbidden, ErrCodeForbidden, "Admin access required")
				return
			}

			handler(w, r)
		})).ServeHTTP(w, r)
	}
}
