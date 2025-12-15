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
	"syntrix/internal/storage/mongo"
)

func main() {
	// 1. Load Configuration
	cfg := config.LoadConfig()
	log.Printf("Starting Syntrix Query Service on port %d...", cfg.Query.Port)

	// 2. Initialize Storage Backend
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	storageBackend, err := mongo.NewMongoBackend(ctx, cfg.Query.Storage.MongoURI, cfg.Query.Storage.DatabaseName)
	if err != nil {
		log.Fatalf("Failed to connect to storage backend: %v", err)
	}
	defer func() {
		if err := storageBackend.Close(context.Background()); err != nil {
			log.Printf("Error closing storage backend: %v", err)
		}
	}()
	log.Println("Connected to MongoDB successfully.")

	// 3. Initialize Query Engine
	cspURL := fmt.Sprintf("http://localhost:%d", cfg.CSP.Port)
	engine := query.NewEngine(storageBackend, cspURL)

	// 4. Setup HTTP Server (Internal API)
	queryServer := query.NewServer(engine)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Query.Port),
		Handler: queryServer,
	}

	// 5. Graceful Shutdown
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}

	log.Println("Server exiting")
}
