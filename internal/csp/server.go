package csp

import (
	"encoding/json"
	"net/http"

	"syntrix/internal/storage"
)

type Server struct {
	mux     *http.ServeMux
	storage storage.StorageBackend
}

func NewServer(storage storage.StorageBackend) *Server {
	s := &Server{
		mux:     http.NewServeMux(),
		storage: storage,
	}
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/internal/v1/watch", s.handleWatch)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("CSP Service OK"))
}

func (s *Server) handleWatch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Collection string `json:"collection"`
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

	// Start watching via Storage Backend (Mongo)
	stream, err := s.storage.Watch(r.Context(), req.Collection)
	if err != nil {
		// Headers already sent, can't send error status now.
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
