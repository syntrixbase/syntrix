package services

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/syntrixbase/syntrix/internal/api/realtime"
	"github.com/syntrixbase/syntrix/internal/config"
	"github.com/syntrixbase/syntrix/internal/identity"
	"github.com/syntrixbase/syntrix/internal/puller"
	puller_config "github.com/syntrixbase/syntrix/internal/puller/config"
	"github.com/syntrixbase/syntrix/internal/puller/events"
	"github.com/syntrixbase/syntrix/internal/server"
	"github.com/syntrixbase/syntrix/internal/storage"
	"github.com/syntrixbase/syntrix/internal/streamer"
	"github.com/syntrixbase/syntrix/internal/trigger"
	"github.com/syntrixbase/syntrix/pkg/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.mongodb.org/mongo-driver/mongo"
	"google.golang.org/grpc"
)

func TestManager_Start_Shutdown_WithServer(t *testing.T) {
	cfg := config.LoadConfig()
	mgr := NewManager(cfg, Options{})

	calls := atomic.Int32{}
	addr := freeAddr()
	server := &http.Server{Addr: addr, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNoContent)
	})}
	mgr.servers = append(mgr.servers, server)
	mgr.serverNames = append(mgr.serverNames, "test-server")

	bgCtx, bgCancel := context.WithCancel(context.Background())
	defer bgCancel()

	mgr.Start(bgCtx)
	assert.NoError(t, waitForServer(addr, 500*time.Millisecond))

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
	defer shutdownCancel()

	mgr.Shutdown(shutdownCtx)

	err := waitForServer(addr, 200*time.Millisecond)
	assert.Error(t, err)
	assert.GreaterOrEqual(t, calls.Load(), int32(1))
}

type MockQueryService struct {
	mock.Mock
}

func (m *MockQueryService) GetDocument(ctx context.Context, tenant string, path string) (model.Document, error) {
	return nil, nil
}
func (m *MockQueryService) CreateDocument(ctx context.Context, tenant string, doc model.Document) error {
	return nil
}
func (m *MockQueryService) ReplaceDocument(ctx context.Context, tenant string, data model.Document, pred model.Filters) (model.Document, error) {
	return nil, nil
}
func (m *MockQueryService) PatchDocument(ctx context.Context, tenant string, data model.Document, pred model.Filters) (model.Document, error) {
	return nil, nil
}
func (m *MockQueryService) DeleteDocument(ctx context.Context, tenant string, path string, pred model.Filters) error {
	return nil
}
func (m *MockQueryService) ExecuteQuery(ctx context.Context, tenant string, q model.Query) ([]model.Document, error) {
	return nil, nil
}
func (m *MockQueryService) Pull(ctx context.Context, tenant string, req storage.ReplicationPullRequest) (*storage.ReplicationPullResponse, error) {
	return nil, nil
}
func (m *MockQueryService) Push(ctx context.Context, tenant string, req storage.ReplicationPushRequest) (*storage.ReplicationPushResponse, error) {
	return nil, nil
}

// Removed tests that depended on WatchCollection and old Realtime retry logic
type storageBackendStub struct {
	watchCalls atomic.Int32
}

func (s *storageBackendStub) Get(context.Context, string, string) (*storage.StoredDoc, error) {
	return nil, model.ErrNotFound
}
func (s *storageBackendStub) Create(context.Context, string, *storage.StoredDoc) error { return nil }
func (s *storageBackendStub) Update(context.Context, string, string, map[string]interface{}, model.Filters) error {
	return nil
}
func (s *storageBackendStub) Patch(context.Context, string, string, map[string]interface{}, model.Filters) error {
	return nil
}
func (s *storageBackendStub) Delete(context.Context, string, string, model.Filters) error { return nil }
func (s *storageBackendStub) Query(context.Context, string, model.Query) ([]*storage.StoredDoc, error) {
	return nil, nil
}
func (s *storageBackendStub) Watch(context.Context, string, string, interface{}, storage.WatchOptions) (<-chan storage.Event, error) {
	s.watchCalls.Add(1)
	ch := make(chan storage.Event)
	close(ch)
	return ch, nil
}
func (s *storageBackendStub) Close(context.Context) error { return nil }
func (s *storageBackendStub) DB() *mongo.Database         { return nil }

type triggerConsumerStub struct {
	called atomic.Int32
}

func (c *triggerConsumerStub) Start(ctx context.Context) error {
	c.called.Add(1)
	<-ctx.Done()
	return nil
}

type mockTriggerService struct {
	mock.Mock
}

func (m *mockTriggerService) Start(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *mockTriggerService) LoadTriggers(triggers []*trigger.Trigger) error {
	args := m.Called(triggers)
	return args.Error(0)
}

func TestManager_Start_TriggerEvaluator_CallsStart(t *testing.T) {
	cfg := config.LoadConfig()
	mgr := NewManager(cfg, Options{RunTriggerEvaluator: true})

	mockTS := new(mockTriggerService)
	mgr.triggerService = mockTS

	bgCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockTS.On("Start", bgCtx).Return(nil)

	mgr.Start(bgCtx)

	// Wait for Start to be called
	time.Sleep(50 * time.Millisecond)
	mockTS.AssertExpectations(t)
}

func TestManager_Start_TriggerWorker_CallsStart(t *testing.T) {
	cfg := config.LoadConfig()
	mgr := NewManager(cfg, Options{RunTriggerWorker: true})

	worker := &triggerConsumerStub{}
	mgr.triggerConsumer = worker

	bgCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgr.Start(bgCtx)

	assert.Eventually(t, func() bool {
		return worker.called.Load() == 1
	}, 1*time.Second, 10*time.Millisecond, "Should call Start exactly once")
}

func TestManager_Start_PullerAndGRPC(t *testing.T) {
	cfg := config.LoadConfig()
	mgr := NewManager(cfg, Options{RunPuller: true})

	pullerSvc := &stubPullerService{}
	mgr.pullerService = pullerSvc
	mgr.pullerGRPC = puller.NewGRPCServer(puller_config.GRPCConfig{Address: "127.0.0.1:0"}, pullerSvc, nil)

	bgCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgr.Start(bgCtx)
	mgr.Shutdown(context.Background())

	assert.Equal(t, int32(1), pullerSvc.startCount.Load())
}

type rtQueryStub struct {
	calls      atomic.Int32
	failFirst  bool
	failAlways bool
}

func (s *rtQueryStub) GetDocument(context.Context, string, string) (model.Document, error) {
	return model.Document{}, nil
}
func (s *rtQueryStub) CreateDocument(context.Context, string, model.Document) error { return nil }
func (s *rtQueryStub) ReplaceDocument(context.Context, string, model.Document, model.Filters) (model.Document, error) {
	return model.Document{}, nil
}
func (s *rtQueryStub) PatchDocument(context.Context, string, model.Document, model.Filters) (model.Document, error) {
	return model.Document{}, nil
}
func (s *rtQueryStub) DeleteDocument(context.Context, string, string, model.Filters) error {
	return nil
}
func (s *rtQueryStub) ExecuteQuery(context.Context, string, model.Query) ([]model.Document, error) {
	return nil, nil
}
func (s *rtQueryStub) Pull(context.Context, string, storage.ReplicationPullRequest) (*storage.ReplicationPullResponse, error) {
	return nil, nil
}
func (s *rtQueryStub) Push(context.Context, string, storage.ReplicationPushRequest) (*storage.ReplicationPushResponse, error) {
	return nil, nil
}

type stubPullerService struct {
	startCount atomic.Int32
	stopCount  atomic.Int32
	mu         sync.Mutex
	addCalls   []pullerBackendCall
}

type pullerBackendCall struct {
	name string
	db   string
}

func (s *stubPullerService) AddBackend(name string, _ *mongo.Client, dbName string, _ puller_config.PullerBackendConfig) error {
	s.mu.Lock()
	s.addCalls = append(s.addCalls, pullerBackendCall{name: name, db: dbName})
	s.mu.Unlock()
	return nil
}
func (s *stubPullerService) Start(ctx context.Context) error {
	s.startCount.Add(1)
	return nil
}
func (s *stubPullerService) Stop(ctx context.Context) error {
	s.stopCount.Add(1)
	return nil
}
func (s *stubPullerService) BackendNames() []string { return nil }
func (s *stubPullerService) SetEventHandler(func(ctx context.Context, backendName string, event *events.StoreChangeEvent) error) {
}
func (s *stubPullerService) Replay(ctx context.Context, after map[string]string, coalesce bool) (events.Iterator, error) {
	return nil, nil
}
func (s *stubPullerService) Subscribe(ctx context.Context, consumerID string, after string) <-chan *events.PullerEvent {
	return make(chan *events.PullerEvent)
}

func freeAddr() string {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "127.0.0.1:0"
	}
	addr := ln.Addr().String()
	ln.Close()
	return addr
}

func waitForServer(addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 100 * time.Millisecond}
	for time.Now().Before(deadline) {
		resp, err := client.Get("http://" + addr)
		if err == nil {
			resp.Body.Close()
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return errors.New("server not reachable")
}

func TestManager_Start_AllServices(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cfg := config.LoadConfig()
	mgr := NewManager(cfg, Options{
		RunTriggerEvaluator: true,
		RunTriggerWorker:    true,
	})

	// 1. Mock Server Service (Global Default)
	mockSrv := new(MockServerService)
	// Start blocks, so we verify it was called and return nil.
	// Since it runs in a goroutine, it might race if we don't mock blocking,
	// but the test context cancellation should handle it.
	mockSrv.On("Start", mock.Anything).Return(nil).Maybe()
	server.SetDefault(mockSrv)
	defer server.SetDefault(nil)

	// 2. Mock Streamer Service
	mockStreamer := new(MockStreamerService)
	mockStreamer.On("Start", mock.Anything).Return(nil)

	mockStream := new(MockStream)
	// Recv blocks or returns error. Simulate cancellation or immediate return to avoid hanging?
	// Realtime's listener loop handles error and sleeps/retries or exits.
	// We return context canceled to stop the loop.
	mockStream.On("Recv").Return((*streamer.EventDelivery)(nil), context.Canceled)
	// Realtime calls Stream(ctx)
	mockStreamer.On("Stream", mock.Anything).Return(mockStream, nil)
	mgr.streamerService = mockStreamer

	// 3. Mock Realtime Server
	mockQuery := new(MockQueryService)
	mockAuth := new(MockAuthService)
	rtCfg := realtime.Config{EnableAuth: false}
	// We need to pass the real realtime.Server to manager
	rtSrv := realtime.NewServer(mockQuery, mockStreamer, "data", mockAuth, rtCfg)
	mgr.rtServer = rtSrv

	// 4. Mock Trigger Service
	mockTrigger := new(MockTriggerService)
	mockTrigger.On("Start", mock.Anything).Return(nil)
	mgr.triggerService = mockTrigger

	// 5. Mock Trigger Consumer
	mockTriggerConsumer := new(MockTriggerConsumer)
	mockTriggerConsumer.On("Start", mock.Anything).Return(nil)
	mgr.triggerConsumer = mockTriggerConsumer

	// Start Manager
	// This launches multiple goroutines.
	mgr.Start(ctx)

	// Give time for goroutines to initialize
	time.Sleep(200 * time.Millisecond)

	// Verify expectations
	mockTrigger.AssertCalled(t, "Start", mock.Anything)
	mockTriggerConsumer.AssertCalled(t, "Start", mock.Anything)
	mockStreamer.AssertCalled(t, "Start", mock.Anything)

	// mockSrv.AssertCalled(t, "Start", mock.Anything) // Might not be called if server.Default() logic isn't hit or racy
	// Wait, we set SetDefault, so it SHOULD be called.
	// But it is behind if s := server.Default(); s != nil
	// We did SetDefault(mockSrv).
	mockSrv.AssertCalled(t, "Start", mock.Anything)
}

// --- Mocks ---

type MockServerService struct {
	mock.Mock
}

func (m *MockServerService) Start(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}
func (m *MockServerService) Stop(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}
func (m *MockServerService) RegisterHTTPHandler(pattern string, handler http.Handler) {
	m.Called(pattern, handler)
}
func (m *MockServerService) RegisterGRPCService(desc *grpc.ServiceDesc, impl interface{}) {
	m.Called(desc, impl)
}
func (m *MockServerService) HTTPMux() *http.ServeMux {
	m.Called()
	return http.NewServeMux()
}

type MockStreamerService struct {
	mock.Mock
}

func (m *MockStreamerService) Start(ctx context.Context) error {
	return m.Called(ctx).Error(0)
}
func (m *MockStreamerService) Stop(ctx context.Context) error {
	return m.Called(ctx).Error(0)
}
func (m *MockStreamerService) Stream(ctx context.Context) (streamer.Stream, error) {
	args := m.Called(ctx)
	if s := args.Get(0); s != nil {
		return s.(streamer.Stream), args.Error(1)
	}
	return nil, args.Error(1)
}

// MockStream implements streamer.Stream interface
type MockStream struct {
	mock.Mock
}

func (m *MockStream) Subscribe(tenant, collection string, filters []model.Filter) (string, error) {
	args := m.Called(tenant, collection, filters)
	return args.String(0), args.Error(1)
}
func (m *MockStream) Unsubscribe(subscriptionID string) error {
	return m.Called(subscriptionID).Error(0)
}
func (m *MockStream) Recv() (*streamer.EventDelivery, error) {
	args := m.Called()
	if e := args.Get(0); e != nil {
		return e.(*streamer.EventDelivery), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *MockStream) Close() error {
	return m.Called().Error(0)
}

type MockTriggerService struct {
	mock.Mock
}

func (m *MockTriggerService) Start(ctx context.Context) error {
	return m.Called(ctx).Error(0)
}
func (m *MockTriggerService) LoadTriggers(triggers []*trigger.Trigger) error {
	return m.Called(triggers).Error(0)
}

type MockTriggerConsumer struct {
	mock.Mock
}

func (m *MockTriggerConsumer) Start(ctx context.Context) error {
	return m.Called(ctx).Error(0)
}

type MockAuthService struct {
	mock.Mock
}

func (m *MockAuthService) Middleware(next http.Handler) http.Handler {
	return next
}
func (m *MockAuthService) MiddlewareOptional(next http.Handler) http.Handler {
	return next
}
func (m *MockAuthService) SignIn(ctx context.Context, req identity.LoginRequest) (*identity.TokenPair, error) {
	return nil, nil
}
func (m *MockAuthService) SignUp(ctx context.Context, req identity.SignupRequest) (*identity.TokenPair, error) {
	return nil, nil
}
func (m *MockAuthService) Refresh(ctx context.Context, req identity.RefreshRequest) (*identity.TokenPair, error) {
	return nil, nil
}
func (m *MockAuthService) ListUsers(ctx context.Context, limit int, offset int) ([]*identity.User, error) {
	return nil, nil
}
func (m *MockAuthService) UpdateUser(ctx context.Context, id string, roles []string, disabled bool) error {
	return nil
}
func (m *MockAuthService) Logout(ctx context.Context, refreshToken string) error {
	return nil
}
func (m *MockAuthService) GenerateSystemToken(serviceName string) (string, error) {
	return "", nil
}
func (m *MockAuthService) ValidateToken(tokenString string) (*identity.Claims, error) {
	return nil, nil
}

func TestManager_Start_RealtimeRetry(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	// Shorter timeout for retry test
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	// We don't defer cancel here immediately because we want to wait for timeout in the test logic naturally?
	// or we just let it run.
	defer cancel()

	cfg := config.LoadConfig()
	mgr := NewManager(cfg, Options{})

	mockStreamer := new(MockStreamerService)
	// Expect Start to be called
	mockStreamer.On("Start", mock.Anything).Return(nil)
	// Always fail
	mockStreamer.On("Stream", mock.Anything).Return(nil, errors.New("connection failed"))
	mgr.streamerService = mockStreamer

	mockQuery := new(MockQueryService)
	mockAuth := new(MockAuthService)
	rtCfg := realtime.Config{EnableAuth: false}
	rtSrv := realtime.NewServer(mockQuery, mockStreamer, "data", mockAuth, rtCfg)
	mgr.rtServer = rtSrv

	// Start
	mgr.Start(ctx)

	// Wait for context timeout (200ms)
	<-ctx.Done()

	// Stream should have been called multiple times due to retry
	// 50ms initial sleep + 50ms retry wait.
	// 0ms: Start (sleep 50ms)
	// 50ms: Attempt 1 -> Stream -> Error -> Wait 50ms
	// 100ms: Attempt 2 -> Stream -> Error -> Wait 50ms
	// 150ms: Attempt 3 -> Stream -> Error -> Wait 50ms
	// 200ms: Context done.
	// Expect at least 2 calls.
	assert.GreaterOrEqual(t, len(mockStreamer.Calls), 2, "Should attempt retry at least once")
}
