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

	"syntrix/internal/config"
	"syntrix/internal/query"
	"syntrix/internal/realtime"
)

func main() {
	// 1. Load Configuration
	cfg := config.LoadConfig()
	log.Println("Starting Syntrix Realtime Service...")

	// 2. Initialize Query Service Client
	if cfg.Realtime.QueryServiceURL == "" {
		log.Fatal("REALTIME_QUERY_SERVICE_URL is required")
	}
	log.Printf("Connecting to remote Query Service at %s...", cfg.Realtime.QueryServiceURL)
	queryService := query.NewClient(cfg.Realtime.QueryServiceURL)

	// 3. Initialize Realtime Server
	rtServer := realtime.NewServer(queryService)

	// 4. Start Background Tasks (Hub & Watcher)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := rtServer.StartBackgroundTasks(ctx); err != nil {
		log.Fatalf("Failed to start background tasks: %v", err)
	}
	log.Println("Listening for changes...")

	// 5. Start HTTP Server
	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Realtime.Port),
		Handler: rtServer,
	}

	go func() {
		log.Printf("Realtime service listening on port %d", cfg.Realtime.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("ListenAndServe: %v", err)
		}
	}()

	// 6. Wait for Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down service...")

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}
	log.Println("Server exiting")
}
