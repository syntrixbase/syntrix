package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"syntrix/internal/api"
	"syntrix/internal/config"
	"syntrix/internal/query"
)

func main() {
	// 1. Load Configuration
	cfg := config.LoadConfig()
	log.Printf("Starting Syntrix API on port %d...", cfg.API.Port)

	// 2. Initialize Query Service Client
	// Strictly use remote client as per architecture requirements
	if cfg.API.QueryServiceURL == "" {
		log.Fatal("API_QUERY_SERVICE_URL is required")
	}
	log.Printf("Connecting to remote Query Service at %s...", cfg.API.QueryServiceURL)
	queryService := query.NewClient(cfg.API.QueryServiceURL)

	// 3. Setup HTTP Server
	apiServer := api.NewServer(queryService)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.API.Port),
		Handler: apiServer,
	}

	// 4. Graceful Shutdown
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	ctxShutdown, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()

	if err := srv.Shutdown(ctxShutdown); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}

	log.Println("Server exiting")
}
