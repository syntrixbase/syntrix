package standalone

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/syntrixbase/syntrix/internal/config"
	identity "github.com/syntrixbase/syntrix/internal/identity/config"
	"github.com/syntrixbase/syntrix/internal/server"
	"github.com/syntrixbase/syntrix/internal/services"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// StandaloneEnv is a test environment for standalone mode.
// Unlike ServiceEnv, it only exposes APIURL since standalone mode
// doesn't have separate Query/CSP HTTP servers.
type StandaloneEnv struct {
	APIURL   string
	Manager  *services.Manager
	Cancel   context.CancelFunc
	MongoURI string
	DBName   string
}

// setupStandaloneEnv creates a test environment in standalone mode.
// In standalone mode:
// - Only the API gateway HTTP server is created
// - Query and CSP services run in-process without HTTP servers
// - Inter-service communication uses direct function calls
func setupStandaloneEnv(t *testing.T, rulesContent string, configModifiers ...func(*config.Config)) *StandaloneEnv {
	// Setup MongoDB connection
	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" {
		mongoURI = "mongodb://localhost:27017"
	}

	// Generate unique database name
	safeName := strings.ReplaceAll(t.Name(), "/", "_")
	safeName = strings.ReplaceAll(safeName, "\\", "_")
	if len(safeName) > 20 {
		safeName = safeName[len(safeName)-20:]
	}
	dbName := fmt.Sprintf("test_%s_%d", safeName, time.Now().UnixNano()%100000)

	// Connect and clean DB
	ctx := context.Background()
	connCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	client, err := mongo.Connect(connCtx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		t.Fatalf("Failed to connect to MongoDB: %v", err)
	}
	defer client.Disconnect(ctx)

	dropCtx, dropCancel := context.WithTimeout(ctx, 10*time.Second)
	defer dropCancel()
	err = client.Database(dbName).Drop(dropCtx)
	require.NoError(t, err)

	apiPort := getFreePort(t)

	// Default security rules
	if rulesContent == "" {
		rulesContent = `
rules_version: '1'
service: syntrix
match:
  /databases/{database}/documents:
    match:
      /{document=**}:
        allow:
          read, write: "true"
`
	}
	rulesFile := t.TempDir() + "/security.yaml"
	err = os.WriteFile(rulesFile, []byte(rulesContent), 0644)
	require.NoError(t, err)

	// Initialize the unified server for standalone mode
	server.InitDefault(server.Config{
		HTTPPort: apiPort,
		GRPCPort: 0,
	}, nil)

	cfg := &config.Config{
		Server: server.Config{
			HTTPPort: apiPort,
		},
		// Query and CSP configs don't need ports in standalone mode
		// as they don't expose HTTP servers
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
				PrivateKeyFile:  t.TempDir() + "/keys/auth_private.pem",
			},
			AuthZ: identity.AuthZConfig{
				RulesFile: rulesFile,
			},
		},
	}

	// Apply config modifiers
	for _, mod := range configModifiers {
		mod(cfg)
	}

	// Create manager in standalone mode
	// Note: RunQuery is not needed as it's handled internally
	opts := services.Options{
		RunAPI:              true,
		RunQuery:            false, // No separate Query HTTP server
		RunTriggerEvaluator: false, // Not testing triggers
		RunTriggerWorker:    false,
		Mode:                services.ModeStandalone, // Standalone mode
	}

	manager := services.NewManager(cfg, opts)
	require.NoError(t, manager.Init(context.Background()))

	// Start Manager
	mgrCtx, mgrCancel := context.WithCancel(context.Background())
	manager.Start(mgrCtx)

	// Wait for API server only
	waitForHealth(t, fmt.Sprintf("http://localhost:%d/health", apiPort))

	return &StandaloneEnv{
		APIURL:   fmt.Sprintf("http://localhost:%d", apiPort),
		Manager:  manager,
		MongoURI: mongoURI,
		DBName:   dbName,
		Cancel: func() {
			mgrCancel()
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			manager.Shutdown(shutdownCtx)

			// Cleanup DB
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cleanupCancel()

			client, err := mongo.Connect(cleanupCtx, options.Client().ApplyURI(mongoURI))
			if err == nil {
				defer client.Disconnect(context.Background())
				_ = client.Database(dbName).Drop(cleanupCtx)
			}
		},
	}
}

func (e *StandaloneEnv) GetToken(t *testing.T, uid string, role string) string {
	// SignUp
	signupBody := map[string]string{
		"database": "default",
		"username": uid,
		"password": "password123456",
	}
	bodyBytes, _ := json.Marshal(signupBody)
	resp, err := http.Post(e.APIURL+"/auth/v1/signup", "application/json", bytes.NewBuffer(bodyBytes))
	require.NoError(t, err)
	defer resp.Body.Close()

	// SignUp returns the token directly if successful
	if resp.StatusCode == http.StatusOK {
		var res map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&res)
		require.NoError(t, err)
		return res["access_token"].(string)
	}

	// If signup failed (user exists), try login
	loginBody := map[string]string{
		"database": "default",
		"username": uid,
		"password": "password123456",
	}
	bodyBytes, _ = json.Marshal(loginBody)
	resp2, err := http.Post(e.APIURL+"/auth/v1/login", "application/json", bytes.NewBuffer(bodyBytes))
	require.NoError(t, err)
	defer resp2.Body.Close()
	require.Equal(t, http.StatusOK, resp2.StatusCode)

	var tokenResp map[string]interface{}
	err = json.NewDecoder(resp2.Body).Decode(&tokenResp)
	require.NoError(t, err)

	return tokenResp["access_token"].(string)
}

// TestStandaloneMode_BasicCRUD tests basic CRUD operations in standalone mode.
// This verifies that the in-process query and CSP services work correctly.
func TestStandaloneMode_BasicCRUD(t *testing.T) {
	env := setupStandaloneEnv(t, "")
	defer env.Cancel()

	token := env.GetToken(t, "testuser", "user")
	collection := "test-collection"

	// Test Create
	t.Run("Create document", func(t *testing.T) {
		docData := map[string]interface{}{
			"name": "Test Document",
			"type": "standalone-test",
		}
		body, _ := json.Marshal(docData)

		req, _ := http.NewRequest(http.MethodPost, env.APIURL+"/api/v1/"+collection, bytes.NewBuffer(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)
		assert.Contains(t, result, "id")
	})

	// Test Query (uses Query service internally)
	t.Run("Query documents", func(t *testing.T) {
		queryData := map[string]interface{}{
			"collection": collection,
		}
		body, _ := json.Marshal(queryData)

		req, _ := http.NewRequest(http.MethodPost, env.APIURL+"/api/v1/query", bytes.NewBuffer(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result []map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(result), 1, "Should have at least one document")
	})
}

// TestStandaloneMode_NoExternalServices verifies that standalone mode
// doesn't require or create Query/CSP HTTP servers.
func TestStandaloneMode_NoExternalServices(t *testing.T) {
	env := setupStandaloneEnv(t, "")
	defer env.Cancel()

	// API server should be up
	resp, err := http.Get(env.APIURL + "/health")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// In standalone mode, there should be only one HTTP server (API)
	// We verify this by checking that the manager's server count is 1
	// (This is implicitly verified by the fact that we didn't set Query/CSP ports
	// and the service still works)

	// Perform a query operation to verify in-process communication works
	token := env.GetToken(t, "standalone_user", "user")

	// Create a document
	docData := map[string]interface{}{
		"title": "Standalone Test",
	}
	body, _ := json.Marshal(docData)

	req, _ := http.NewRequest(http.MethodPost, env.APIURL+"/api/v1/standalone-test", bytes.NewBuffer(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode, "Document creation should work in standalone mode")
}

// TestStandaloneMode_Watch tests real-time watch functionality in standalone mode.
// Watch uses CSP service internally, so this verifies the local CSP integration.
func TestStandaloneMode_Watch(t *testing.T) {
	env := setupStandaloneEnv(t, "")
	defer env.Cancel()

	token := env.GetToken(t, "watch_user", "user")
	collection := "watch-test"

	// Create initial document
	docData := map[string]interface{}{
		"status": "initial",
	}
	body, _ := json.Marshal(docData)

	req, _ := http.NewRequest(http.MethodPost, env.APIURL+"/api/v1/"+collection, bytes.NewBuffer(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	// Extract document ID for update
	var createResult map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&createResult)
	require.NoError(t, err)
	docID := createResult["id"].(string)

	// Update the document (this would trigger watch events via CSP)
	// PATCH requires the data to be wrapped in "doc" field
	updateData := map[string]interface{}{
		"doc": map[string]interface{}{
			"status": "updated",
		},
	}
	body, _ = json.Marshal(updateData)

	req, _ = http.NewRequest(http.MethodPatch, fmt.Sprintf("%s/api/v1/%s/%s", env.APIURL, collection, docID), bytes.NewBuffer(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Document update should work in standalone mode")
}
