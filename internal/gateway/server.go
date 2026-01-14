package gateway

import (
	"net/http"

	"github.com/syntrixbase/syntrix/internal/core/identity"
	"github.com/syntrixbase/syntrix/internal/gateway/realtime"
	"github.com/syntrixbase/syntrix/internal/gateway/rest"
	"github.com/syntrixbase/syntrix/internal/query"
)

// Server is a route registrar for the API layer.
// It registers REST, realtime, and console routes to a given ServeMux.
type Server struct {
	rest     *rest.Handler
	realtime *realtime.Server
}

// NewServer creates a new API Server (route registrar).
func NewServer(engine query.Service, auth identity.AuthN, authz identity.AuthZ, rt *realtime.Server) *Server {
	return &Server{
		rest:     rest.NewHandler(engine, auth, authz),
		realtime: rt,
	}
}

// RegisterRoutes registers all API routes to the given ServeMux.
// This includes REST API routes, realtime WebSocket/SSE routes, and console static files.
func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	// Register REST routes
	s.rest.RegisterRoutes(mux)

	// Register Realtime routes
	if s.realtime != nil {
		mux.HandleFunc("GET /realtime/ws", s.realtime.HandleWS)
		mux.HandleFunc("GET /realtime/sse", s.realtime.HandleSSE)
	}

	// Console
	mux.Handle("GET /console/", http.StripPrefix("/console/", http.FileServer(http.Dir("console"))))
}
