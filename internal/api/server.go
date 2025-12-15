package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"syntrix/internal/query"
	"syntrix/internal/storage"

	"github.com/google/uuid"
)

type Server struct {
	engine query.Service
	mux    *http.ServeMux
}

func NewServer(engine query.Service) *Server {
	s := &Server{
		engine: engine,
		mux:    http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	// Document Operations
	s.mux.HandleFunc("GET /v1/{path...}", s.handleGetDocument)
	s.mux.HandleFunc("POST /v1/{path...}", s.handleCreateDocument)
	s.mux.HandleFunc("PUT /v1/{path...}", s.handleReplaceDocument)
	s.mux.HandleFunc("PATCH /v1/{path...}", s.handleUpdateDocument)
	s.mux.HandleFunc("DELETE /v1/{path...}", s.handleDeleteDocument)

	// Query Operations
	s.mux.HandleFunc("POST /v1/query", s.handleQuery)

	// Replication Operations
	s.mux.HandleFunc("GET /v1/replication/pull", s.handlePull)
	s.mux.HandleFunc("POST /v1/replication/push", s.handlePush)

	// Health Check
	s.mux.HandleFunc("GET /health", s.handleHealth)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (s *Server) handleGetDocument(w http.ResponseWriter, r *http.Request) {
	path := r.PathValue("path")

	if err := validateDocumentPath(path); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	doc, err := s.engine.GetDocument(r.Context(), path)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
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
	collection := r.PathValue("path")

	if err := validateCollection(collection); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var req struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := validateData(req.Data); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	docID := uuid.New().String()
	path := collection + "/" + docID

	doc := storage.NewDocument(path, collection, req.Data)

	if err := s.engine.CreateDocument(r.Context(), doc); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(doc)
}

func (s *Server) handleReplaceDocument(w http.ResponseWriter, r *http.Request) {
	path := r.PathValue("path")

	if err := validateDocumentPath(path); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Extract collection from path
	i := strings.LastIndex(path, "/")
	if i == -1 {
		http.Error(w, "Invalid document path", http.StatusBadRequest)
		return
	}
	collection := path[:i]

	var req struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := validateData(req.Data); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	doc, err := s.engine.ReplaceDocument(r.Context(), path, collection, req.Data)
	if err != nil {
		if errors.Is(err, storage.ErrVersionConflict) {
			http.Error(w, "Version conflict", http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(doc)
}

func (s *Server) handleUpdateDocument(w http.ResponseWriter, r *http.Request) {
	path := r.PathValue("path")

	if err := validateDocumentPath(path); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var req struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := validateData(req.Data); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	doc, err := s.engine.PatchDocument(r.Context(), path, req.Data)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			http.Error(w, "Document not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, storage.ErrVersionConflict) {
			http.Error(w, "Version conflict", http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(doc)
}

func (s *Server) handleDeleteDocument(w http.ResponseWriter, r *http.Request) {
	path := r.PathValue("path")

	if err := validateDocumentPath(path); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.engine.DeleteDocument(r.Context(), path); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			http.Error(w, "Document not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleQuery(w http.ResponseWriter, r *http.Request) {
	var q storage.Query
	if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := validateQuery(q); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	docs, err := s.engine.ExecuteQuery(r.Context(), q)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(docs)
}

func (s *Server) handlePull(w http.ResponseWriter, r *http.Request) {
	collection := r.URL.Query().Get("collection")
	if collection == "" {
		http.Error(w, "collection is required", http.StatusBadRequest)
		return
	}
	checkpointStr := r.URL.Query().Get("checkpoint")
	limitStr := r.URL.Query().Get("limit")

	checkpoint := int64(0)
	if checkpointStr != "" {
		var err error
		checkpoint, err = strconv.ParseInt(checkpointStr, 10, 64)
		if err != nil {
			http.Error(w, "Invalid checkpoint", http.StatusBadRequest)
			return
		}
	}

	limit := 100
	if limitStr != "" {
		var err error
		limit, err = strconv.Atoi(limitStr)
		if err != nil {
			http.Error(w, "Invalid limit", http.StatusBadRequest)
			return
		}
	}

	req := storage.ReplicationPullRequest{
		Collection: collection,
		Checkpoint: checkpoint,
		Limit:      limit,
	}

	if err := validateReplicationPull(req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	resp, err := s.engine.Pull(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handlePush(w http.ResponseWriter, r *http.Request) {
	collection := r.URL.Query().Get("collection")
	if collection == "" {
		http.Error(w, "collection is required", http.StatusBadRequest)
		return
	}

	var req storage.ReplicationPushRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	req.Collection = collection

	if err := validateReplicationPush(req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	resp, err := s.engine.Push(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
