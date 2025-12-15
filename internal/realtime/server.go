package realtime

import (
	"context"
	"log"
	"net/http"

	"syntrix/internal/query"
)

type Server struct {
	hub          *Hub
	queryService query.Service
	mux          *http.ServeMux
}

func NewServer(qs query.Service) *Server {
	h := NewHub()
	s := &Server{
		hub:          h,
		queryService: qs,
		mux:          http.NewServeMux(),
	}
	s.mux.HandleFunc("/v1/realtime", func(w http.ResponseWriter, r *http.Request) {
		ServeWs(h, w, r)
	})
	return s
}

// StartBackgroundTasks starts the hub and the change stream watcher.
// It returns an error if watching fails to start.
// The background tasks run until ctx is cancelled.
func (s *Server) StartBackgroundTasks(ctx context.Context) error {
	go s.hub.Run()

	stream, err := s.queryService.WatchCollection(ctx, "")
	if err != nil {
		return err
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case evt, ok := <-stream:
				if !ok {
					log.Println("Realtime: Change stream closed")
					return
				}
				s.hub.Broadcast(evt)
			}
		}
	}()

	return nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}
