package integration

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	api_config "github.com/syntrixbase/syntrix/internal/api/config"
	"github.com/syntrixbase/syntrix/internal/config"
	identity "github.com/syntrixbase/syntrix/internal/identity/config"
	puller_config "github.com/syntrixbase/syntrix/internal/puller/config"
	"github.com/syntrixbase/syntrix/internal/server"
	"github.com/syntrixbase/syntrix/internal/services"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// HTTP client with timeout for health checks
var httpClientWithTimeout = &http.Client{Timeout: 5 * time.Second}

// Global service instance shared by all integration tests
var (
	globalEnv   *GlobalTestEnv
	globalEnvMu sync.RWMutex
)

// GlobalTestEnv holds the shared test environment
type GlobalTestEnv struct {
	APIURL    string
	QueryURL  string
	Manager   *services.Manager
	MongoURI  string
	DBName    string
	cancel    context.CancelFunc
	rulesFile string
	keysDir   string
	bufferDir string
}

// TestMain sets up the global test environment before running tests
func TestMain(m *testing.M) {
	// Setup global environment
	env, err := setupGlobalEnv()
	if err != nil {
		log.Fatalf("Failed to setup global test environment: %v", err)
	}
	globalEnv = env

	// Run tests
	code := m.Run()

	// Cleanup
	if globalEnv != nil {
		globalEnv.Shutdown()
	}

	os.Exit(code)
}

// GetGlobalEnv returns the shared global test environment
// Safe for concurrent access
func GetGlobalEnv() *GlobalTestEnv {
	globalEnvMu.RLock()
	defer globalEnvMu.RUnlock()
	return globalEnv
}

// setupGlobalEnv creates and starts the global service instance
func setupGlobalEnv() (*GlobalTestEnv, error) {
	// MongoDB connection
	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" {
		mongoURI = "mongodb://localhost:27017"
	}

	// Use a single database for all tests, with collection-level isolation
	dbName := fmt.Sprintf("integration_test_%d", time.Now().UnixNano())

	// Connect and ensure DB is clean
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %w", err)
	}
	defer client.Disconnect(context.Background())

	if err := client.Database(dbName).Drop(ctx); err != nil {
		return nil, fmt.Errorf("failed to drop database: %w", err)
	}

	// Create temp directories for test files
	tempDir, err := os.MkdirTemp("", "integration_test_")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}

	keysDir := tempDir + "/keys"
	if err := os.MkdirAll(keysDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create keys dir: %w", err)
	}

	bufferDir := tempDir + "/buffer"
	if err := os.MkdirAll(bufferDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create buffer dir: %w", err)
	}

	// Create comprehensive security rules file that supports all test scenarios
	// Different tests use different path prefixes to test different authorization rules
	rulesContent := `
rules_version: '1'
service: syntrix
match:
  /databases/{database}/documents:
    match:
      # Public paths - no auth required (for authz tests)
      /public/{doc=**}:
        allow:
          read, write: "true"
      # Private paths - require authentication (for authz tests)
      /private/{doc=**}:
        allow:
          read, write: "request.auth.userId != null"
      # Admin paths - require admin role (for authz tests)
      /admin/{doc=**}:
        allow:
          read, write: "request.auth.roles != null && 'admin' in request.auth.roles"
      # Default - allow all (for most tests)
      /{document=**}:
        allow:
          read, write: "true"
`
	rulesFile := tempDir + "/security.yaml"
	if err := os.WriteFile(rulesFile, []byte(rulesContent), 0644); err != nil {
		return nil, fmt.Errorf("failed to write rules file: %w", err)
	}

	// Get available ports
	apiPort, err := getAvailablePort()
	if err != nil {
		return nil, fmt.Errorf("failed to get API port: %w", err)
	}
	grpcPort, err := getAvailablePort()
	if err != nil {
		return nil, fmt.Errorf("failed to get gRPC port: %w", err)
	}

	cfg := &config.Config{
		Server: server.Config{
			Host:     "localhost",
			HTTPPort: apiPort,
			GRPCPort: grpcPort,
		},
		Gateway: api_config.GatewayConfig{
			QueryServiceURL:    fmt.Sprintf("localhost:%d", grpcPort),
			PullerServiceURL:   fmt.Sprintf("localhost:%d", grpcPort),
			StreamerServiceURL: fmt.Sprintf("localhost:%d", grpcPort),
		},
		Storage: config.StorageConfig{
			Backends: map[string]config.BackendConfig{
				"default": {
					Type: "mongo",
					Mongo: config.MongoConfig{
						URI:          mongoURI,
						DatabaseName: dbName,
					},
				},
			},
			Topology: config.TopologyConfig{
				Document: config.DocumentTopology{
					BaseTopology: config.BaseTopology{
						Strategy: "single",
						Primary:  "default",
					},
					DataCollection: "documents",
					SysCollection:  "sys",
				},
				User: config.CollectionTopology{
					BaseTopology: config.BaseTopology{
						Strategy: "single",
						Primary:  "default",
					},
					Collection: "users",
				},
				Revocation: config.CollectionTopology{
					BaseTopology: config.BaseTopology{
						Strategy: "single",
						Primary:  "default",
					},
					Collection: "revocations",
				},
			},
		},
		Identity: identity.Config{
			AuthN: identity.AuthNConfig{
				AccessTokenTTL:  15 * time.Minute,
				RefreshTokenTTL: 7 * 24 * time.Hour,
				AuthCodeTTL:     2 * time.Minute,
				PrivateKeyFile:  keysDir + "/auth_private.pem",
			},
			AuthZ: identity.AuthZConfig{
				RulesFile: rulesFile,
			},
		},
		Puller: puller_config.Config{
			Backends: []puller_config.PullerBackendConfig{
				{Name: "default", Collections: []string{"documents"}},
			},
			Cleaner: puller_config.CleanerConfig{
				Interval:  1 * time.Minute,
				Retention: 1 * time.Hour,
			},
			Buffer: puller_config.BufferConfig{
				Path:    bufferDir,
				MaxSize: "100MB",
			},
		},
	}

	// Initialize the unified server
	server.InitDefault(cfg.Server, nil)

	// Check if NATS is available
	natsAvailable := false
	if os.Getenv("NATS_URL") != "" {
		natsAvailable = true
	} else {
		conn, err := net.DialTimeout("tcp", "localhost:4222", 100*time.Millisecond)
		if err == nil {
			conn.Close()
			natsAvailable = true
		}
	}

	opts := services.Options{
		Mode:                services.ModeDistributed, // Services communicate via gRPC even in same process
		RunAPI:              true,
		RunQuery:            true,
		RunStreamer:         true, // Run Streamer service locally
		RunTriggerEvaluator: natsAvailable,
		RunTriggerWorker:    natsAvailable,
		RunPuller:           true,
	}

	manager := services.NewManager(cfg, opts)
	if err := manager.Init(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to init manager: %w", err)
	}

	// Start manager
	mgrCtx, mgrCancel := context.WithCancel(context.Background())
	manager.Start(mgrCtx)

	// Wait for services to be healthy
	if err := waitForHealthWithTimeout(fmt.Sprintf("http://localhost:%d/health", apiPort), 30*time.Second); err != nil {
		mgrCancel()
		return nil, fmt.Errorf("API server failed to start: %w", err)
	}
	// Query service now uses unified gRPC server, no separate health check needed

	log.Printf("[Integration Test] Global environment started (Distributed Mode) - API: %d, gRPC: %d, DB: %s",
		apiPort, grpcPort, dbName)

	return &GlobalTestEnv{
		APIURL:    fmt.Sprintf("http://localhost:%d", apiPort),
		QueryURL:  fmt.Sprintf("localhost:%d", grpcPort),
		Manager:   manager,
		MongoURI:  mongoURI,
		DBName:    dbName,
		cancel:    mgrCancel,
		rulesFile: rulesFile,
		keysDir:   keysDir,
		bufferDir: bufferDir,
	}, nil
}

// Shutdown stops the global service and cleans up resources
func (e *GlobalTestEnv) Shutdown() {
	if e.cancel != nil {
		e.cancel()
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if e.Manager != nil {
		e.Manager.Shutdown(shutdownCtx)
	}

	// Cleanup database
	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cleanupCancel()

	client, err := mongo.Connect(cleanupCtx, options.Client().ApplyURI(e.MongoURI))
	if err == nil {
		defer client.Disconnect(context.Background())
		_ = client.Database(e.DBName).Drop(cleanupCtx)
	}

	// Cleanup temp files
	if e.rulesFile != "" {
		os.RemoveAll(e.rulesFile)
	}

	log.Printf("[Integration Test] Global environment shutdown complete")
}

// GenerateSystemToken generates a system token for admin operations
func (e *GlobalTestEnv) GenerateSystemToken() (string, error) {
	authService := e.Manager.AuthService()
	return authService.GenerateSystemToken("system-worker")
}

// getAvailablePort finds an available TCP port
func getAvailablePort() (int, error) {
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

// waitForHealthWithTimeout waits for a health endpoint to respond
func waitForHealthWithTimeout(url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := httpClientWithTimeout.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("health check timed out for %s", url)
}
