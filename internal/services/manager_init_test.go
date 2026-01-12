package services

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/syntrixbase/syntrix/internal/config"
	"github.com/syntrixbase/syntrix/internal/identity"
	"github.com/syntrixbase/syntrix/internal/indexer"
	"github.com/syntrixbase/syntrix/internal/puller"
	puller_config "github.com/syntrixbase/syntrix/internal/puller/config"
	"github.com/syntrixbase/syntrix/internal/server"
	"github.com/syntrixbase/syntrix/internal/storage"
	"github.com/syntrixbase/syntrix/pkg/model"

	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestManager_AuthServiceGetter(t *testing.T) {
	t.Parallel()
	cfg := config.LoadConfig()
	mgr := NewManager(cfg, Options{})

	assert.Nil(t, mgr.AuthService())
}

func TestManager_Init_StorageError(t *testing.T) {
	t.Parallel()
	cfg := config.LoadConfig()
	if backend, ok := cfg.Storage.Backends["default_mongo"]; ok {
		backend.Mongo.URI = "mongodb://invalid-host:1"
		cfg.Storage.Backends["default_mongo"] = backend
	}
	opt := Options{RunQuery: true}
	mgr := NewManager(cfg, opt)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := mgr.Init(ctx)
	assert.Error(t, err)
}

func TestManager_Init_TokenServiceError(t *testing.T) {
	cfg := config.LoadConfig()
	cfg.Identity.AuthN.PrivateKeyFile = "/nonexistent/dir/key.pem"
	opt := Options{RunAPI: true}
	mgr := NewManager(cfg, opt)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := mgr.Init(ctx)
	assert.Error(t, err)
}

func TestManager_Init_AuthzRulesLoadError(t *testing.T) {
	cfg := config.LoadConfig()
	cfg.Identity.AuthZ.RulesFile = "__missing_rules_file__"
	opts := Options{RunAPI: true}
	mgr := NewManager(cfg, opts)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := mgr.Init(ctx)
	assert.Error(t, err)
}

func TestManager_InitAuthService_GenerateKey(t *testing.T) {
	cfg := config.LoadConfig()
	cfg.Identity.AuthN.PrivateKeyFile = filepath.Join(t.TempDir(), "private.pem")
	mgr := NewManager(cfg, Options{RunTriggerWorker: true})

	// Mock storage factory
	fakeAuth := &fakeAuthStore{}
	storageFactoryFactory = func(ctx context.Context, cfg *config.Config) (storage.StorageFactory, error) {
		return &fakeStorageFactory{
			usrStore: fakeAuth,
			revStore: fakeAuth,
		}, nil
	}
	defer func() {
		storageFactoryFactory = func(ctx context.Context, cfg *config.Config) (storage.StorageFactory, error) {
			return storage.NewFactory(ctx, cfg)
		}
	}()

	// We need to init storage first because initAuthService depends on it

	err := mgr.initAuthService(context.Background())
	assert.NoError(t, err)
	assert.NotNil(t, mgr.authService)

	_, statErr := os.Stat(cfg.Identity.AuthN.PrivateKeyFile)
	assert.NoError(t, statErr)
}

func TestManager_InitAPIServer_WithRules(t *testing.T) {
	// Initialize a fresh server instance to avoid route conflicts with other tests
	server.InitDefault(server.DefaultConfig(), nil)

	cfg := config.LoadConfig()
	cfg.Server.HTTPPort = 0
	rulesPath := filepath.Join(t.TempDir(), "rules.yaml")
	rulesContent := "match:\n  /databases/{db}/documents/{doc}:\n    allow:\n      get: \"true\"\n"
	assert.NoError(t, os.WriteFile(rulesPath, []byte(rulesContent), 0644))
	cfg.Identity.AuthZ.RulesFile = rulesPath

	mgr := NewManager(cfg, Options{})
	mgr.authService = &stubAuthN{}
	querySvc := &stubQueryService{}

	err := mgr.initAPIServer(querySvc)
	assert.NoError(t, err)
	// API routes are now registered directly to the unified server
}

func TestManager_InitAPIServer_NoRules(t *testing.T) {
	// Initialize a fresh server instance to avoid route conflicts with other tests
	server.InitDefault(server.DefaultConfig(), nil)

	cfg := config.LoadConfig()
	cfg.Server.HTTPPort = 0
	cfg.Identity.AuthZ.RulesFile = ""

	mgr := NewManager(cfg, Options{})
	mgr.authService = &stubAuthN{}
	querySvc := &stubQueryService{}

	err := mgr.initAPIServer(querySvc)
	assert.NoError(t, err)
	// API routes are now registered directly to the unified server
}

func TestManager_InitAPIServer_WithRealtime(t *testing.T) {
	// Initialize a fresh server instance to avoid route conflicts with other tests
	server.InitDefault(server.DefaultConfig(), nil)

	cfg := config.LoadConfig()
	cfg.Server.HTTPPort = 0
	cfg.Identity.AuthZ.RulesFile = ""
	mgr := NewManager(cfg, Options{})
	mgr.authService = &stubAuthN{}

	err := mgr.initAPIServer(&stubQueryService{})
	assert.NoError(t, err)
	assert.NotNil(t, mgr.rtServer)
	// API routes are now registered directly to the unified server
}

func TestManager_InitTriggerServices_NATSFailure(t *testing.T) {
	cfg := config.LoadConfig()
	cfg.Trigger.NatsURL = "nats://127.0.0.1:1"
	mgr := NewManager(cfg, Options{RunTriggerWorker: true})

	err := mgr.initTriggerServices(context.Background(), false)
	assert.Error(t, err)
}

func TestManager_InitStorage_SkipsWhenNoServices(t *testing.T) {
	cfg := config.LoadConfig()
	mgr := NewManager(cfg, Options{})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	sf, err := mgr.getStorageFactory(ctx)
	assert.NoError(t, err)
	assert.Nil(t, sf)
}

func TestManager_Init_RunAuthPath(t *testing.T) {
	origFactory := storageFactoryFactory
	defer func() { storageFactoryFactory = origFactory }()

	fakeDocStore := &fakeDocumentStore{}
	fakeAuth := &fakeAuthStore{}
	storageFactoryFactory = func(ctx context.Context, cfg *config.Config) (storage.StorageFactory, error) {
		return &fakeStorageFactory{
			docStore: fakeDocStore,
			usrStore: fakeAuth,
			revStore: fakeAuth,
		}, nil
	}

	cfg := config.LoadConfig()
	cfg.Identity.AuthN.PrivateKeyFile = filepath.Join(t.TempDir(), "auth.pem")

	// Create a dummy rules file
	rulesFile := filepath.Join(t.TempDir(), "security.yaml")
	os.WriteFile(rulesFile, []byte("rules: []"), 0644)
	cfg.Identity.AuthZ.RulesFile = rulesFile

	mgr := NewManager(cfg, Options{RunAPI: true})

	err := mgr.Init(context.Background())
	assert.NoError(t, err)
	assert.NotNil(t, mgr.authService)
}

func TestManager_Init_RunQueryPath(t *testing.T) {
	origFactory := storageFactoryFactory
	defer func() { storageFactoryFactory = origFactory }()

	// Initialize a fresh server instance
	server.InitDefault(server.DefaultConfig(), nil)

	fakeDocStore := &fakeDocumentStore{}
	storageFactoryFactory = func(ctx context.Context, cfg *config.Config) (storage.StorageFactory, error) {
		return &fakeStorageFactory{
			docStore: fakeDocStore,
		}, nil
	}

	cfg := config.LoadConfig()
	cfg.Server.GRPCPort = 0
	mgr := NewManager(cfg, Options{RunQuery: true})

	err := mgr.Init(context.Background())
	assert.NoError(t, err)
	assert.NotNil(t, mgr.storageFactory)
	assert.Equal(t, fakeDocStore, mgr.storageFactory.Document())
	// Query service now uses unified gRPC server, not separate HTTP server
	// The service is registered on server.Default()
}

func TestManager_Init_RunRealtimePath(t *testing.T) {
	// Initialize a fresh server instance to avoid route conflicts with other tests
	server.InitDefault(server.DefaultConfig(), nil)

	cfg := config.LoadConfig()
	cfg.Server.HTTPPort = 0

	cfg.Identity.AuthZ.RulesFile = ""
	mgr := NewManager(cfg, Options{RunAPI: true})

	err := mgr.Init(context.Background())
	assert.NoError(t, err)
	assert.NotNil(t, mgr.rtServer)
	// API routes are now registered directly to the unified server
}

func TestManager_initPullerService_Success(t *testing.T) {
	fakeSF := &stubStorageFactory{
		dbByName: map[string]string{"primary": "db_primary"},
		client:   &mongo.Client{},
	}

	origFactory := storageFactoryFactory
	defer func() { storageFactoryFactory = origFactory }()
	storageFactoryFactory = func(ctx context.Context, cfg *config.Config) (storage.StorageFactory, error) {
		return fakeSF, nil
	}

	cfg := config.LoadConfig()
	cfg.Puller.Buffer.Path = t.TempDir()
	cfg.Puller.Backends = []puller_config.PullerBackendConfig{{Name: "primary"}}

	mgr := NewManager(cfg, Options{Mode: ModeDistributed, RunPuller: true})
	defer mgr.Shutdown(context.Background())

	err := mgr.initPullerService(context.Background())
	assert.NoError(t, err)
	names := mgr.pullerService.BackendNames()
	assert.Len(t, names, 1)
	assert.Equal(t, "primary", names[0])
	// Note: pullerGRPC is nil here because initPullerService no longer registers gRPC.
	// gRPC registration is done separately by initPullerGRPCServer() in initDistributed().
	assert.Nil(t, mgr.pullerGRPC)
}

func TestManager_initPullerService_GetMongoError(t *testing.T) {
	fakeSF := &stubStorageFactory{
		errByName: map[string]error{"missing": errors.New("no backend")},
		client:    &mongo.Client{},
	}
	origFactory := storageFactoryFactory
	defer func() { storageFactoryFactory = origFactory }()
	storageFactoryFactory = func(ctx context.Context, cfg *config.Config) (storage.StorageFactory, error) {
		return fakeSF, nil
	}

	cfg := config.LoadConfig()
	cfg.Puller.Buffer.Path = t.TempDir()
	cfg.Puller.Backends = []puller_config.PullerBackendConfig{{Name: "missing"}}

	mgr := NewManager(cfg, Options{RunPuller: true})

	err := mgr.initPullerService(context.Background())
	assert.Error(t, err)
}

type fakeDocumentStore struct {
	db        *mongo.Database
	retention time.Duration
}

func (f *fakeDocumentStore) Get(ctx context.Context, database, path string) (*storage.StoredDoc, error) {
	return nil, nil
}
func (f *fakeDocumentStore) Create(ctx context.Context, database string, doc storage.StoredDoc) error {
	return nil
}
func (f *fakeDocumentStore) Update(ctx context.Context, database, path string, data map[string]interface{}, pred model.Filters) error {
	return nil
}
func (f *fakeDocumentStore) Patch(ctx context.Context, database, path string, data map[string]interface{}, pred model.Filters) error {
	return nil
}
func (f *fakeDocumentStore) Delete(ctx context.Context, database, path string, pred model.Filters) error {
	return nil
}
func (f *fakeDocumentStore) Query(ctx context.Context, database string, q model.Query) ([]*storage.StoredDoc, error) {
	return nil, nil
}
func (f *fakeDocumentStore) GetMany(ctx context.Context, database string, paths []string) ([]*storage.StoredDoc, error) {
	return nil, nil
}
func (f *fakeDocumentStore) Watch(ctx context.Context, database, collection string, resumeToken interface{}, opts storage.WatchOptions) (<-chan storage.Event, error) {
	return nil, nil
}
func (f *fakeDocumentStore) Close(ctx context.Context) error { return nil }

type fakeAuthStore struct {
	db           *mongo.Database
	ensureCalled bool
}

func (f *fakeAuthStore) CreateUser(ctx context.Context, database string, user *storage.User) error {
	return nil
}
func (f *fakeAuthStore) GetUserByUsername(ctx context.Context, database, username string) (*storage.User, error) {
	return nil, identity.ErrUserNotFound
}
func (f *fakeAuthStore) GetUserByID(ctx context.Context, database, id string) (*storage.User, error) {
	return nil, identity.ErrUserNotFound
}
func (f *fakeAuthStore) ListUsers(ctx context.Context, database string, limit int, offset int) ([]*storage.User, error) {
	return nil, nil
}
func (f *fakeAuthStore) UpdateUser(ctx context.Context, database string, user *storage.User) error {
	return nil
}
func (f *fakeAuthStore) UpdateUserLoginStats(ctx context.Context, database, id string, lastLogin time.Time, attempts int, lockoutUntil time.Time) error {
	return nil
}
func (f *fakeAuthStore) RevokeToken(ctx context.Context, database, jti string, expiresAt time.Time) error {
	return nil
}
func (f *fakeAuthStore) RevokeTokenImmediate(ctx context.Context, database, jti string, expiresAt time.Time) error {
	return nil
}
func (f *fakeAuthStore) IsRevoked(ctx context.Context, database, jti string, gracePeriod time.Duration) (bool, error) {
	return false, nil
}
func (f *fakeAuthStore) EnsureIndexes(ctx context.Context) error {
	f.ensureCalled = true
	return nil
}
func (f *fakeAuthStore) Close(ctx context.Context) error { return nil }

type fakeDocumentProvider struct {
	store storage.DocumentStore
}

func (f *fakeDocumentProvider) Document() storage.DocumentStore { return f.store }
func (f *fakeDocumentProvider) Close(ctx context.Context) error { return nil }

type fakeAuthProvider struct {
	users       storage.UserStore
	revocations storage.TokenRevocationStore
}

func (f *fakeAuthProvider) Users() storage.UserStore                  { return f.users }
func (f *fakeAuthProvider) Revocations() storage.TokenRevocationStore { return f.revocations }
func (f *fakeAuthProvider) Close(ctx context.Context) error           { return nil }

type stubQueryService struct{}

func (s *stubQueryService) GetDocument(context.Context, string, string) (model.Document, error) {
	return model.Document{}, nil
}

func (s *stubQueryService) CreateDocument(context.Context, string, model.Document) error {
	return nil
}

func (s *stubQueryService) ReplaceDocument(context.Context, string, model.Document, model.Filters) (model.Document, error) {
	return model.Document{}, nil
}

func (s *stubQueryService) PatchDocument(context.Context, string, model.Document, model.Filters) (model.Document, error) {
	return model.Document{}, nil
}

func (s *stubQueryService) DeleteDocument(context.Context, string, string, model.Filters) error {
	return nil
}

func (s *stubQueryService) ExecuteQuery(context.Context, string, model.Query) ([]model.Document, error) {
	return nil, nil
}

func (s *stubQueryService) WatchCollection(context.Context, string, string) (<-chan storage.Event, error) {
	return nil, nil
}

func (s *stubQueryService) Pull(context.Context, string, storage.ReplicationPullRequest) (*storage.ReplicationPullResponse, error) {
	return nil, nil
}

func (s *stubQueryService) Push(context.Context, string, storage.ReplicationPushRequest) (*storage.ReplicationPushResponse, error) {
	return nil, nil
}

type stubStorageFactory struct {
	dbByName  map[string]string
	errByName map[string]error
	client    *mongo.Client
}

func (s *stubStorageFactory) Document() storage.DocumentStore          { return nil }
func (s *stubStorageFactory) User() storage.UserStore                  { return nil }
func (s *stubStorageFactory) Revocation() storage.TokenRevocationStore { return nil }
func (s *stubStorageFactory) GetMongoClient(name string) (*mongo.Client, string, error) {
	if err := s.errByName[name]; err != nil {
		return nil, "", err
	}

	db := s.dbByName[name]
	if db == "" {
		db = name
	}
	return s.client, db, nil
}
func (s *stubStorageFactory) Close() error { return nil }

type fakeStorageFactory struct {
	docStore storage.DocumentStore
	usrStore storage.UserStore
	revStore storage.TokenRevocationStore
}

func (f *fakeStorageFactory) Document() storage.DocumentStore          { return f.docStore }
func (f *fakeStorageFactory) User() storage.UserStore                  { return f.usrStore }
func (f *fakeStorageFactory) Revocation() storage.TokenRevocationStore { return f.revStore }
func (f *fakeStorageFactory) GetMongoClient(name string) (*mongo.Client, string, error) {
	return nil, "", nil
}
func (f *fakeStorageFactory) Close() error { return nil }

type stubAuthN struct{}

func (s *stubAuthN) Middleware(next http.Handler) http.Handler         { return next }
func (s *stubAuthN) MiddlewareOptional(next http.Handler) http.Handler { return next }
func (s *stubAuthN) SignIn(ctx context.Context, req identity.LoginRequest) (*identity.TokenPair, error) {
	return nil, nil
}
func (s *stubAuthN) SignUp(ctx context.Context, req identity.SignupRequest) (*identity.TokenPair, error) {
	return nil, nil
}
func (s *stubAuthN) Refresh(ctx context.Context, req identity.RefreshRequest) (*identity.TokenPair, error) {
	return nil, nil
}
func (s *stubAuthN) ListUsers(ctx context.Context, limit int, offset int) ([]*storage.User, error) {
	return nil, nil
}
func (s *stubAuthN) UpdateUser(ctx context.Context, id string, roles []string, disabled bool) error {
	return nil
}
func (s *stubAuthN) Logout(ctx context.Context, refreshToken string) error      { return nil }
func (s *stubAuthN) GenerateSystemToken(serviceName string) (string, error)     { return "", nil }
func (s *stubAuthN) ValidateToken(tokenString string) (*identity.Claims, error) { return nil, nil }

func TestManager_Init_StandaloneMode(t *testing.T) {
	origFactory := storageFactoryFactory
	defer func() { storageFactoryFactory = origFactory }()

	fakeDocStore := &fakeDocumentStore{}
	fakeAuth := &fakeAuthStore{}
	storageFactoryFactory = func(ctx context.Context, cfg *config.Config) (storage.StorageFactory, error) {
		return &fakeStorageFactory{
			docStore: fakeDocStore,
			usrStore: fakeAuth,
			revStore: fakeAuth,
		}, nil
	}

	cfg := config.LoadConfig()
	cfg.Server.HTTPPort = 0
	cfg.Identity.AuthZ.RulesFile = ""
	mgr := NewManager(cfg, Options{
		Mode:   ModeStandalone,
		RunAPI: true,
	})

	err := mgr.Init(context.Background())
	assert.NoError(t, err)
	// In standalone mode, the Unified Server (server.Default()) handles all HTTP routing.
	// No separate HTTP servers are needed since API routes are registered
	// directly to the unified server mux.
	assert.NotNil(t, mgr.storageFactory)
}

func TestManager_Init_StandaloneMode_NoHTTPForCSP(t *testing.T) {
	origFactory := storageFactoryFactory
	defer func() { storageFactoryFactory = origFactory }()

	fakeDocStore := &fakeDocumentStore{}
	fakeAuth := &fakeAuthStore{}
	storageFactoryFactory = func(ctx context.Context, cfg *config.Config) (storage.StorageFactory, error) {
		return &fakeStorageFactory{
			docStore: fakeDocStore,
			usrStore: fakeAuth,
			revStore: fakeAuth,
		}, nil
	}

	cfg := config.LoadConfig()
	cfg.Server.HTTPPort = 0
	cfg.Identity.AuthZ.RulesFile = ""
	mgr := NewManager(cfg, Options{
		Mode:   ModeStandalone,
		RunAPI: true,
	})

	err := mgr.Init(context.Background())
	assert.NoError(t, err)
	// In standalone mode, CSP and Query services run in-process without separate HTTP servers.
	// The Unified Server (server.Default()) handles all HTTP routing.
}

func TestManager_createQueryService(t *testing.T) {
	fakeDocStore := &fakeDocumentStore{}
	origFactory := storageFactoryFactory
	defer func() { storageFactoryFactory = origFactory }()

	storageFactoryFactory = func(ctx context.Context, cfg *config.Config) (storage.StorageFactory, error) {
		return &fakeStorageFactory{
			docStore: fakeDocStore,
		}, nil
	}

	cfg := config.LoadConfig()
	mgr := NewManager(cfg, Options{RunQuery: true})

	service, err := mgr.createQueryService(context.Background())
	assert.NoError(t, err)
	assert.NotNil(t, service)
}

func TestManager_initQueryGRPCServer(t *testing.T) {
	cfg := config.LoadConfig()
	cfg.Server.GRPCPort = 0
	mgr := NewManager(cfg, Options{})

	// Initialize the unified server first
	server.InitDefault(cfg.Server, nil)

	mockService := &stubQueryService{}
	mgr.initQueryGRPCServer(mockService)
	// gRPC server registration uses the unified server
	// Just verify it doesn't panic
}

func TestManager_initStandalone_APIServerError(t *testing.T) {
	origFactory := storageFactoryFactory
	defer func() { storageFactoryFactory = origFactory }()

	fakeDocStore := &fakeDocumentStore{}
	fakeAuth := &fakeAuthStore{}
	storageFactoryFactory = func(ctx context.Context, cfg *config.Config) (storage.StorageFactory, error) {
		return &fakeStorageFactory{
			docStore: fakeDocStore,
			usrStore: fakeAuth,
			revStore: fakeAuth,
		}, nil
	}

	cfg := config.LoadConfig()
	cfg.Server.HTTPPort = 0
	cfg.Identity.AuthZ.RulesFile = "/nonexistent/rules/file.yaml"
	mgr := NewManager(cfg, Options{
		Mode:   ModeStandalone,
		RunAPI: true,
	})

	err := mgr.Init(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create authz engine")
}

func TestManager_initDistributed_APIServerError(t *testing.T) {
	origFactory := storageFactoryFactory
	defer func() { storageFactoryFactory = origFactory }()

	fakeDocStore := &fakeDocumentStore{}
	fakeAuth := &fakeAuthStore{}
	storageFactoryFactory = func(ctx context.Context, cfg *config.Config) (storage.StorageFactory, error) {
		return &fakeStorageFactory{
			docStore: fakeDocStore,
			usrStore: fakeAuth,
			revStore: fakeAuth,
		}, nil
	}

	cfg := config.LoadConfig()
	cfg.Server.HTTPPort = 0
	cfg.Identity.AuthZ.RulesFile = "/nonexistent/rules/file.yaml"
	mgr := NewManager(cfg, Options{
		Mode:   ModeDistributed,
		RunAPI: true,
	})

	err := mgr.Init(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create authz engine")
}

func TestManager_Init_PullerServiceError(t *testing.T) {
	origFactory := storageFactoryFactory
	defer func() { storageFactoryFactory = origFactory }()

	storageFactoryFactory = func(ctx context.Context, cfg *config.Config) (storage.StorageFactory, error) {
		return &stubStorageFactory{
			// Return an error for the "missing_backend" backend
			errByName: map[string]error{"missing_backend": errors.New("backend not found")},
		}, nil
	}

	cfg := config.LoadConfig()
	cfg.Puller.Buffer.Path = t.TempDir()
	// Configure a backend that will fail during initialization
	cfg.Puller.Backends = []puller_config.PullerBackendConfig{{Name: "missing_backend"}}

	mgr := NewManager(cfg, Options{
		Mode:      ModeDistributed,
		RunPuller: true,
		RunQuery:  true, // Need this to trigger storage initialization
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := mgr.Init(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get mongo client for backend")
}

func TestManager_initDistributed_TriggerServicesError(t *testing.T) {
	origFactory := storageFactoryFactory
	defer func() { storageFactoryFactory = origFactory }()

	fakeDocStore := &fakeDocumentStore{}
	fakeAuth := &fakeAuthStore{}
	storageFactoryFactory = func(ctx context.Context, cfg *config.Config) (storage.StorageFactory, error) {
		return &fakeStorageFactory{
			docStore: fakeDocStore,
			usrStore: fakeAuth,
			revStore: fakeAuth,
		}, nil
	}

	cfg := config.LoadConfig()
	cfg.Server.HTTPPort = 0
	cfg.Identity.AuthZ.RulesFile = ""
	// Configure NATS to fail connection
	cfg.Trigger.NatsURL = "nats://127.0.0.1:1"

	mgr := NewManager(cfg, Options{
		Mode:                ModeDistributed,
		RunTriggerEvaluator: true,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := mgr.Init(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to connect to NATS")
}

// mockPullerService for streamer testing
type mockPullerService struct{}

func (m *mockPullerService) Subscribe(ctx context.Context, consumerID string, after string) <-chan *puller.Event {
	return nil
}
func (m *mockPullerService) AddBackend(name string, client *mongo.Client, dbName string, cfg puller_config.PullerBackendConfig) error {
	return nil
}
func (m *mockPullerService) Start(context.Context) error { return nil }
func (m *mockPullerService) Stop(context.Context) error  { return nil }
func (m *mockPullerService) BackendNames() []string      { return nil }
func (m *mockPullerService) SetEventHandler(handler func(ctx context.Context, backendName string, event *puller.ChangeEvent) error) {
}
func (m *mockPullerService) Replay(ctx context.Context, after map[string]string, streaming bool) (puller.Iterator, error) {
	return nil, nil
}

func TestManager_createStreamerService(t *testing.T) {
	done := make(chan bool)
	go func() {
		cfg := config.LoadConfig()
		// Disable Mongo/Backends for this unit test requiring no IO
		// Use ModeStandalone to avoid gRPC registration
		mgr := NewManager(cfg, Options{Mode: ModeStandalone})

		// Case 1: No Puller Service (already nil)
		svc1, err1 := mgr.createStreamerService()
		assert.NoError(t, err1)
		assert.NotNil(t, svc1)

		// Case 2: With Puller Service
		mgr.pullerService = &mockPullerService{}
		svc2, err2 := mgr.createStreamerService()
		assert.NoError(t, err2)
		assert.NotNil(t, svc2)
		done <- true
	}()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("TestManager_createStreamerService timed out")
	}
}

func TestManager_initStreamerService(t *testing.T) {
	cfg := config.LoadConfig()
	cfg.Server.GRPCPort = 0
	cfg.Gateway.PullerServiceURL = "localhost:50051"

	// Initialize the unified server first
	server.InitDefault(cfg.Server, nil)

	mgr := NewManager(cfg, Options{Mode: ModeDistributed, RunStreamer: true})

	err := mgr.initStreamerService()
	assert.NoError(t, err)
	assert.NotNil(t, mgr.streamerService)
}

func TestManager_initStreamerService_NoPuller(t *testing.T) {
	cfg := config.LoadConfig()
	cfg.Server.GRPCPort = 0
	cfg.Gateway.PullerServiceURL = "" // No Puller URL configured

	// Initialize the unified server first
	server.InitDefault(cfg.Server, nil)

	mgr := NewManager(cfg, Options{Mode: ModeDistributed, RunStreamer: true})

	err := mgr.initStreamerService()
	assert.NoError(t, err)
	assert.NotNil(t, mgr.streamerService)
}

func TestManager_initGateway(t *testing.T) {
	origFactory := storageFactoryFactory
	defer func() { storageFactoryFactory = origFactory }()

	fakeDocStore := &fakeDocumentStore{}
	fakeAuth := &fakeAuthStore{}
	storageFactoryFactory = func(ctx context.Context, cfg *config.Config) (storage.StorageFactory, error) {
		return &fakeStorageFactory{
			docStore: fakeDocStore,
			usrStore: fakeAuth,
			revStore: fakeAuth,
		}, nil
	}

	cfg := config.LoadConfig()
	cfg.Server.HTTPPort = 0
	cfg.Gateway.QueryServiceURL = "localhost:50051"
	cfg.Gateway.StreamerServiceURL = "localhost:50051"
	cfg.Identity.AuthZ.RulesFile = ""

	// Initialize the unified server first
	server.InitDefault(cfg.Server, nil)

	mgr := NewManager(cfg, Options{Mode: ModeDistributed, RunAPI: true})
	mgr.authService = &stubAuthN{}

	err := mgr.initGateway()
	assert.NoError(t, err)
	assert.NotNil(t, mgr.streamerClient)
	assert.NotNil(t, mgr.rtServer)
}

func TestManager_initDistributed_RunStreamer(t *testing.T) {
	cfg := config.LoadConfig()
	cfg.Server.GRPCPort = 0
	cfg.Gateway.PullerServiceURL = "localhost:50051"

	// Initialize the unified server first
	server.InitDefault(cfg.Server, nil)

	mgr := NewManager(cfg, Options{Mode: ModeDistributed, RunStreamer: true})

	err := mgr.initDistributed(context.Background())
	assert.NoError(t, err)
	assert.NotNil(t, mgr.streamerService)
}

func TestManager_initDistributed_RunQueryAndStreamer(t *testing.T) {
	origFactory := storageFactoryFactory
	defer func() { storageFactoryFactory = origFactory }()

	fakeDocStore := &fakeDocumentStore{}
	storageFactoryFactory = func(ctx context.Context, cfg *config.Config) (storage.StorageFactory, error) {
		return &fakeStorageFactory{
			docStore: fakeDocStore,
		}, nil
	}

	cfg := config.LoadConfig()
	cfg.Server.GRPCPort = 0
	cfg.Gateway.PullerServiceURL = "localhost:50051"

	// Initialize the unified server first
	server.InitDefault(cfg.Server, nil)

	mgr := NewManager(cfg, Options{
		Mode:        ModeDistributed,
		RunQuery:    true,
		RunStreamer: true,
	})

	err := mgr.initDistributed(context.Background())
	assert.NoError(t, err)
	assert.NotNil(t, mgr.streamerService)
}

func TestManager_initDistributed_AllServices(t *testing.T) {
	origFactory := storageFactoryFactory
	defer func() { storageFactoryFactory = origFactory }()

	fakeDocStore := &fakeDocumentStore{}
	fakeAuth := &fakeAuthStore{}
	storageFactoryFactory = func(ctx context.Context, cfg *config.Config) (storage.StorageFactory, error) {
		return &fakeStorageFactory{
			docStore: fakeDocStore,
			usrStore: fakeAuth,
			revStore: fakeAuth,
		}, nil
	}

	cfg := config.LoadConfig()
	cfg.Server.GRPCPort = 0
	cfg.Server.HTTPPort = 0
	cfg.Gateway.QueryServiceURL = "localhost:50051"
	cfg.Gateway.PullerServiceURL = "localhost:50051"
	cfg.Gateway.StreamerServiceURL = "localhost:50051"
	cfg.Identity.AuthZ.RulesFile = ""

	// Initialize the unified server first
	server.InitDefault(cfg.Server, nil)

	mgr := NewManager(cfg, Options{
		Mode:        ModeDistributed,
		RunQuery:    true,
		RunStreamer: true,
		RunAPI:      true,
	})

	err := mgr.Init(context.Background())
	assert.NoError(t, err)
	assert.NotNil(t, mgr.streamerService) // Local Streamer service
	assert.NotNil(t, mgr.streamerClient)  // Gateway uses client
	assert.NotNil(t, mgr.rtServer)
}

func TestManager_initStandalone_TriggerServicesError(t *testing.T) {
	origFactory := storageFactoryFactory
	defer func() { storageFactoryFactory = origFactory }()

	fakeDocStore := &fakeDocumentStore{}
	fakeAuth := &fakeAuthStore{}
	storageFactoryFactory = func(ctx context.Context, cfg *config.Config) (storage.StorageFactory, error) {
		return &fakeStorageFactory{
			docStore: fakeDocStore,
			usrStore: fakeAuth,
			revStore: fakeAuth,
		}, nil
	}

	cfg := config.LoadConfig()
	cfg.Server.HTTPPort = 0
	cfg.Identity.AuthZ.RulesFile = ""
	// Configure NATS to fail connection
	cfg.Trigger.NatsURL = "nats://127.0.0.1:1"

	mgr := NewManager(cfg, Options{
		Mode:                ModeStandalone,
		RunAPI:              true,
		RunTriggerEvaluator: true,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := mgr.Init(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to connect to NATS")
}

func TestManager_initStandalone_WithPuller(t *testing.T) {
	origFactory := storageFactoryFactory
	defer func() { storageFactoryFactory = origFactory }()

	fakeSF := &stubStorageFactory{
		dbByName: map[string]string{"default": "test_db"},
		client:   &mongo.Client{},
	}
	storageFactoryFactory = func(ctx context.Context, cfg *config.Config) (storage.StorageFactory, error) {
		return fakeSF, nil
	}

	cfg := config.LoadConfig()
	cfg.Server.HTTPPort = 0
	cfg.Identity.AuthZ.RulesFile = ""
	cfg.Puller.Buffer.Path = t.TempDir()
	cfg.Puller.Backends = []puller_config.PullerBackendConfig{{Name: "default"}}

	mgr := NewManager(cfg, Options{
		Mode:      ModeStandalone,
		RunAPI:    true,
		RunPuller: true,
	})
	defer mgr.Shutdown(context.Background())

	err := mgr.Init(context.Background())
	assert.NoError(t, err)
	assert.NotNil(t, mgr.pullerService)
	// In standalone mode, pullerGRPC should be nil (no gRPC registration)
	assert.Nil(t, mgr.pullerGRPC)
}

func TestManager_initStandalone_PullerError(t *testing.T) {
	origFactory := storageFactoryFactory
	defer func() { storageFactoryFactory = origFactory }()

	storageFactoryFactory = func(ctx context.Context, cfg *config.Config) (storage.StorageFactory, error) {
		return nil, errors.New("storage error")
	}

	cfg := config.LoadConfig()
	cfg.Server.HTTPPort = 0
	cfg.Puller.Backends = []puller_config.PullerBackendConfig{{Name: "default"}}

	mgr := NewManager(cfg, Options{
		Mode:      ModeStandalone,
		RunAPI:    true,
		RunPuller: true,
	})

	err := mgr.Init(context.Background())
	assert.Error(t, err)
}

func TestManager_initPullerGRPCServer(t *testing.T) {
	cfg := config.LoadConfig()
	cfg.Server.HTTPPort = 0
	cfg.Server.GRPCPort = 0
	cfg.Puller.Buffer.Path = t.TempDir()

	// Initialize the unified server first
	server.InitDefault(cfg.Server, nil)

	mgr := NewManager(cfg, Options{
		Mode:      ModeDistributed,
		RunPuller: true,
	})

	// Create a minimal puller service
	mgr.pullerService = puller.NewService(cfg.Puller, nil)

	// Call initPullerGRPCServer
	mgr.initPullerGRPCServer()

	assert.NotNil(t, mgr.pullerGRPC)
}

func TestManager_initIndexerService(t *testing.T) {
	origFactory := storageFactoryFactory
	defer func() { storageFactoryFactory = origFactory }()

	fakeSF := &stubStorageFactory{
		dbByName: map[string]string{"default": "test_db"},
		client:   &mongo.Client{},
	}
	storageFactoryFactory = func(ctx context.Context, cfg *config.Config) (storage.StorageFactory, error) {
		return fakeSF, nil
	}

	cfg := config.LoadConfig()
	cfg.Server.HTTPPort = 0
	cfg.Server.GRPCPort = 0
	cfg.Indexer.TemplatePath = ""

	mgr := NewManager(cfg, Options{
		Mode:       ModeDistributed,
		RunIndexer: true,
		RunPuller:  true, // Indexer uses puller if available
	})

	// Create a minimal puller service (so the indexer can use it)
	mgr.pullerService = &mockPullerService{}

	err := mgr.initIndexerService(context.Background())
	assert.NoError(t, err)
	assert.NotNil(t, mgr.indexerService)
}

func TestManager_initIndexerGRPCServer(t *testing.T) {
	cfg := config.LoadConfig()
	cfg.Server.HTTPPort = 0
	cfg.Server.GRPCPort = 0
	cfg.Indexer.TemplatePath = ""

	// Initialize the unified server first
	server.InitDefault(cfg.Server, nil)

	mgr := NewManager(cfg, Options{
		Mode:       ModeDistributed,
		RunIndexer: true,
	})

	// Create a real indexer service - simpler than mocking the internal interface
	mgr.indexerService = indexer.NewService(indexer.Config{}, nil, slog.Default())

	// Call initIndexerGRPCServer
	mgr.initIndexerGRPCServer()
	// Just verify it doesn't panic - gRPC registration happens on server.Default()
}

func TestManager_initStandalone_WithIndexer(t *testing.T) {
	origFactory := storageFactoryFactory
	defer func() { storageFactoryFactory = origFactory }()

	fakeSF := &stubStorageFactory{
		dbByName: map[string]string{"default": "test_db"},
		client:   &mongo.Client{},
	}
	storageFactoryFactory = func(ctx context.Context, cfg *config.Config) (storage.StorageFactory, error) {
		return fakeSF, nil
	}

	cfg := config.LoadConfig()
	cfg.Server.HTTPPort = 0
	cfg.Identity.AuthZ.RulesFile = ""
	cfg.Indexer.TemplatePath = ""
	cfg.Puller.Buffer.Path = t.TempDir()
	cfg.Puller.Backends = []puller_config.PullerBackendConfig{{Name: "default"}}

	mgr := NewManager(cfg, Options{
		Mode:       ModeStandalone,
		RunAPI:     true,
		RunPuller:  true,
		RunIndexer: true,
	})
	defer mgr.Shutdown(context.Background())

	err := mgr.Init(context.Background())
	assert.NoError(t, err)
	assert.NotNil(t, mgr.indexerService)
}

func TestManager_initDistributed_WithIndexer(t *testing.T) {
	origFactory := storageFactoryFactory
	defer func() { storageFactoryFactory = origFactory }()

	fakeSF := &stubStorageFactory{
		dbByName: map[string]string{"default": "test_db"},
		client:   &mongo.Client{},
	}
	storageFactoryFactory = func(ctx context.Context, cfg *config.Config) (storage.StorageFactory, error) {
		return fakeSF, nil
	}

	cfg := config.LoadConfig()
	cfg.Server.HTTPPort = 0
	cfg.Server.GRPCPort = 0
	cfg.Indexer.TemplatePath = ""
	cfg.Puller.Buffer.Path = t.TempDir()
	cfg.Puller.Backends = []puller_config.PullerBackendConfig{{Name: "default"}}

	// Initialize the unified server first
	server.InitDefault(cfg.Server, nil)

	mgr := NewManager(cfg, Options{
		Mode:       ModeDistributed,
		RunPuller:  true,
		RunIndexer: true,
	})
	defer mgr.Shutdown(context.Background())

	err := mgr.initDistributed(context.Background())
	assert.NoError(t, err)
	assert.NotNil(t, mgr.indexerService)
}
