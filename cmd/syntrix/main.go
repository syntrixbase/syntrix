package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/syntrixbase/syntrix/internal/config"
	"github.com/syntrixbase/syntrix/internal/services"
)

func main() {
	// 0. Parse Command Line Flags
	runAPI := flag.Bool("api", false, "Run API Gateway (REST + Realtime)")
	runQuery := flag.Bool("query", false, "Run Query Service")
	runTriggerEvaluator := flag.Bool("trigger-evaluator", false, "Run Trigger Evaluator Service")
	runTriggerWorker := flag.Bool("trigger-worker", false, "Run Trigger Worker Service")
	runIndexer := flag.Bool("indexer", false, "Run Indexer Service")
	runPuller := flag.Bool("puller", false, "Run Change Stream Puller Service")
	runStreamer := flag.Bool("streamer", false, "Run Streamer Service")
	runAll := flag.Bool("all", false, "Run All Services")
	standalone := flag.Bool("standalone", false, "Run in standalone mode (single process, no inter-service HTTP)")
	flag.Parse()

	// 1. Load Configuration early to check deployment mode from config
	cfg := config.LoadConfig()

	// Standalone mode: from CLI flag or config file
	// CLI flag takes precedence over config file
	if *standalone || cfg.IsStandaloneMode() {
		log.Println("Starting Syntrix in Standalone Mode...")
		log.Println("- All services running in-process")
		opts := services.Options{
			Mode:       services.ModeStandalone,
			RunAPI:     true,
			RunPuller:  true,
			RunIndexer: true,
			RunQuery:   true,
		}
		runServer(cfg, opts)
		return
	}

	// Default to running all if no specific flags are provided or if --all is set
	if *runAll || (!*runAPI && !*runQuery && !*runTriggerEvaluator && !*runTriggerWorker && !*runPuller) {
		*runAPI = true
		*runQuery = true
		*runTriggerEvaluator = true
		*runTriggerWorker = true
		*runPuller = true
		*runIndexer = true
		*runStreamer = true
	}

	log.Println("Starting Syntrix Services...")
	if *runAPI {
		log.Println("- API Gateway (REST + Realtime): Enabled")
	}
	if *runQuery {
		log.Println("- Query Service: Enabled")
	}
	if *runTriggerEvaluator {
		log.Println("- Trigger Evaluator Service: Enabled")
	}
	if *runTriggerWorker {
		log.Println("- Trigger Worker Service: Enabled")
	}
	if *runPuller {
		log.Println("- Change Stream Puller Service: Enabled")
	}
	if *runIndexer {
		log.Println("- Indexer Service: Enabled")
	}

	// 2. Initialize Service Manager
	opts := services.Options{
		RunAPI:              *runAPI,
		RunQuery:            *runQuery,
		RunTriggerEvaluator: *runTriggerEvaluator,
		RunTriggerWorker:    *runTriggerWorker,
		RunPuller:           *runPuller,
		RunIndexer:          *runIndexer,
		RunStreamer:         *runStreamer,
	}
	runServer(cfg, opts)
}

// runServer starts the service manager with the given configuration and options.
func runServer(cfg *config.Config, opts services.Options) {
	mgr := services.NewManager(cfg, opts)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := mgr.Init(ctx); err != nil {
		log.Fatalf("Failed to initialize services: %v", err)
	}

	// 3. Start Services
	// Context for background tasks
	bgCtx, bgCancel := context.WithCancel(context.Background())
	defer bgCancel()

	mgr.Start(bgCtx)

	// 4. Wait for Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down services...")

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	// Cancel background tasks first
	bgCancel()

	mgr.Shutdown(shutdownCtx)

	log.Println("All services stopped.")
}
