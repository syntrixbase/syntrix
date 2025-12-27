package query

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/codetrek/syntrix/internal/storage"
	"github.com/codetrek/syntrix/pkg/model"
)

// Server exposes the Query Engine via HTTP (Internal API).
type Server struct {
	engine *Engine
	mux    *http.ServeMux
}

func tenantOrDefault(t string) string {
	if t == "" {
		return model.DefaultTenantID
	}
	return t
}

// NewServer creates a new Query Server.
func NewServer(engine *Engine) *Server {
	s := &Server{
		engine: engine,
		mux:    http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	// Internal API routes
	s.mux.HandleFunc("POST /internal/v1/document/get", s.handleGetDocument)
	s.mux.HandleFunc("POST /internal/v1/document/create", s.handleCreateDocument)
	s.mux.HandleFunc("POST /internal/v1/document/replace", s.handleReplaceDocument)
	s.mux.HandleFunc("POST /internal/v1/document/patch", s.handlePatchDocument)
	s.mux.HandleFunc("POST /internal/v1/document/delete", s.handleDeleteDocument)
	s.mux.HandleFunc("POST /internal/v1/query/execute", s.handleExecuteQuery)
	s.mux.HandleFunc("POST /internal/v1/watch", s.handleWatchCollection)
	s.mux.HandleFunc("POST /internal/replication/v1/pull", s.handlePull)
	s.mux.HandleFunc("POST /internal/replication/v1/push", s.handlePush)

	// Health Check
	s.mux.HandleFunc("GET /health", s.handleHealth)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (s *Server) handleGetDocument(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path   string `json:"path"`
		Tenant string `json:"tenant"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	tenant := tenantOrDefault(req.Tenant)

	doc, err := s.engine.GetDocument(r.Context(), tenant, req.Path)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			http.Error(w, "Document not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(doc)
}

func (s *Server) handleCreateDocument(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Data   model.Document `json:"data"`
		Tenant string         `json:"tenant"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	tenant := tenantOrDefault(req.Tenant)

	if err := s.engine.CreateDocument(r.Context(), tenant, req.Data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (s *Server) handleReplaceDocument(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Data   model.Document `json:"data"`
		Pred   model.Filters  `json:"pred"`
		Tenant string         `json:"tenant"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	tenant := tenantOrDefault(req.Tenant)

	doc, err := s.engine.ReplaceDocument(r.Context(), tenant, req.Data, req.Pred)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(doc)
}

func (s *Server) handlePatchDocument(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Data   model.Document `json:"data"`
		Pred   model.Filters  `json:"pred"`
		Tenant string         `json:"tenant"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	tenant := tenantOrDefault(req.Tenant)

	doc, err := s.engine.PatchDocument(r.Context(), tenant, req.Data, req.Pred)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			http.Error(w, "Document not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(doc)
}

func (s *Server) handleDeleteDocument(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path   string        `json:"path"`
		Pred   model.Filters `json:"pred"`
		Tenant string        `json:"tenant"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	tenant := tenantOrDefault(req.Tenant)

	if err := s.engine.DeleteDocument(r.Context(), tenant, req.Path, req.Pred); err != nil {
		if errors.Is(err, model.ErrNotFound) {
			http.Error(w, "Document not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, model.ErrPreconditionFailed) {
			http.Error(w, "Version conflict", http.StatusPreconditionFailed)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleExecuteQuery(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Query  model.Query `json:"query"`
		Tenant string      `json:"tenant"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	tenant := tenantOrDefault(req.Tenant)

	docs, err := s.engine.ExecuteQuery(r.Context(), tenant, req.Query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(docs)
}

func (s *Server) handleWatchCollection(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Collection string `json:"collection"`
		Tenant     string `json:"tenant"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Ensure the writer supports flushing
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Flush headers immediately
	flusher.Flush()

	// Start watching
	tenant := tenantOrDefault(req.Tenant)

	stream, err := s.engine.WatchCollection(r.Context(), tenant, req.Collection)
	if err != nil {
		// Headers already sent, can't send error status now.
		// Maybe send a special error event or just close connection.
		return
	}

	encoder := json.NewEncoder(w)

	for {
		select {
		case <-r.Context().Done():
			return
		case evt, ok := <-stream:
			if !ok {
				return
			}
			if err := encoder.Encode(evt); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (s *Server) handlePull(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Request storage.ReplicationPullRequest `json:"request"`
		Tenant  string                         `json:"tenant"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	tenant := tenantOrDefault(req.Tenant)

	resp, err := s.engine.Pull(r.Context(), tenant, req.Request)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handlePush(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Request storage.ReplicationPushRequest `json:"request"`
		Tenant  string                         `json:"tenant"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	tenant := tenantOrDefault(req.Tenant)

	resp, err := s.engine.Push(r.Context(), tenant, req.Request)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
