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
	"syntrix/internal/csp"
	"syntrix/internal/storage/mongo"
)

func main() {
	// 1. Load Configuration
	cfg := config.LoadConfig()
	log.Printf("Starting Syntrix CSP Service on port %d...", cfg.CSP.Port)

	// Initialize Storage (Mongo)
	ctx := context.Background()
	mongoBackend, err := mongo.NewMongoBackend(ctx, cfg.CSP.Storage.MongoURI, cfg.CSP.Storage.DatabaseName)
	if err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}
	defer mongoBackend.Close(ctx)

	// 2. Initialize CSP Server
	cspServer := csp.NewServer(mongoBackend)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.CSP.Port),
		Handler: cspServer,
	}

	// 3. Graceful Shutdown
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
