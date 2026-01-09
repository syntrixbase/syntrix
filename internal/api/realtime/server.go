package realtime

import (
	"context"
	"log"
	"net/http"

	"github.com/syntrixbase/syntrix/internal/api/config"
	"github.com/syntrixbase/syntrix/internal/identity"
	"github.com/syntrixbase/syntrix/internal/query"
	"github.com/syntrixbase/syntrix/internal/streamer"
)

type Server struct {
	hub            *Hub
	queryService   query.Service
	streamer       streamer.Service
	dataCollection string
	auth           identity.AuthN
	cfg            config.RealtimeConfig
}

func NewServer(qs query.Service, str streamer.Service, dataCollection string, auth identity.AuthN, cfg config.RealtimeConfig) *Server {
	h := NewHub()
	s := &Server{
		hub:            h,
		dataCollection: dataCollection,
		queryService:   qs,
		streamer:       str,
		auth:           auth,
		cfg:            cfg,
	}
	return s
}

func (s *Server) HandleWS(w http.ResponseWriter, r *http.Request) {
	s.wrapWS(w, r)
}

func (s *Server) HandleSSE(w http.ResponseWriter, r *http.Request) {
	s.wrapSSE(w, r)
}

func (s *Server) wrapWS(w http.ResponseWriter, r *http.Request) {
	if tokenFromQueryParam(r) != "" {
		http.Error(w, "Query token not allowed", http.StatusUnauthorized)
		return
	}

	if s.auth != nil {
		s.auth.MiddlewareOptional(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ServeWs(s.hub, s.queryService, s.auth, s.cfg, w, r)
		})).ServeHTTP(w, r)
		return
	}

	ServeWs(s.hub, s.queryService, s.auth, s.cfg, w, r)
}

func (s *Server) wrapSSE(w http.ResponseWriter, r *http.Request) {
	if s.auth != nil {
		s.auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ServeSSE(s.hub, s.queryService, s.auth, s.cfg, w, r)
		})).ServeHTTP(w, r)
		return
	}

	ServeSSE(s.hub, s.queryService, s.auth, s.cfg, w, r)
}

func tokenFromQueryParam(r *http.Request) string {
	if r == nil {
		return ""
	}
	q := r.URL.Query()
	if v := q.Get("access_token"); v != "" {
		return v
	}
	if v := q.Get("token"); v != "" {
		return v
	}
	return ""
}

// StartBackgroundTasks starts the hub and the change stream watcher.
// It returns an error if watching fails to start.
// The background tasks run until ctx is cancelled.
func (s *Server) StartBackgroundTasks(ctx context.Context) error {
	go s.hub.Run(ctx)

	// Stream from Streamer service
	stream, err := s.streamer.Stream(ctx)
	if err != nil {
		return err
	}

	// Set the stream on the hub so subscriptions can be sent
	s.hub.SetStream(stream)

	go func() {
		log.Println("[Realtime] Started watching change stream")
		for {
			delivery, err := stream.Recv()
			if err != nil {
				// If context is done, we exit gracefully
				select {
				case <-ctx.Done():
					log.Println("[Realtime] Context cancelled, stopping background tasks")
				default:
					log.Printf("[Realtime] Stream error: %v", err)
				}
				return
			}

			s.hub.BroadcastDelivery(delivery)
		}
	}()

	return nil
}
