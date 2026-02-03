package services

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/nats-io/nats.go"

	indexerv1 "github.com/syntrixbase/syntrix/api/gen/indexer/v1"
	pullerv1 "github.com/syntrixbase/syntrix/api/gen/puller/v1"
	pb "github.com/syntrixbase/syntrix/api/gen/query/v1"
	streamerv1 "github.com/syntrixbase/syntrix/api/gen/streamer/v1"
	"github.com/syntrixbase/syntrix/internal/config"
	"github.com/syntrixbase/syntrix/internal/core/database"
	"github.com/syntrixbase/syntrix/internal/core/identity"
	"github.com/syntrixbase/syntrix/internal/core/pubsub"
	"github.com/syntrixbase/syntrix/internal/core/storage"
	"github.com/syntrixbase/syntrix/internal/gateway"
	"github.com/syntrixbase/syntrix/internal/gateway/realtime"
	"github.com/syntrixbase/syntrix/internal/indexer"
	"github.com/syntrixbase/syntrix/internal/puller"
	"github.com/syntrixbase/syntrix/internal/query"
	"github.com/syntrixbase/syntrix/internal/server"
	"github.com/syntrixbase/syntrix/internal/streamer"
	"github.com/syntrixbase/syntrix/internal/trigger"
	"github.com/syntrixbase/syntrix/internal/trigger/delivery"
	"github.com/syntrixbase/syntrix/internal/trigger/evaluator"
)

var storageFactoryFactory = func(ctx context.Context, cfg *config.Config) (storage.StorageFactory, error) {
	return storage.NewFactory(ctx, cfg.Storage)
}

// evaluatorServiceFactory creates evaluator services - injectable for testing
var evaluatorServiceFactory = func(deps evaluator.Dependencies, cfg evaluator.Config) (evaluator.Service, error) {
	return evaluator.NewService(deps, cfg)
}

// deliveryServiceFactory creates delivery services - injectable for testing
var deliveryServiceFactory = func(deps delivery.Dependencies, cfg delivery.Config) (delivery.Service, error) {
	return delivery.NewService(deps, cfg)
}

// pubsubPublisherFactory creates a pubsub.Publisher - injectable for testing
var pubsubPublisherFactory = func(nc *nats.Conn, cfg evaluator.Config) (pubsub.Publisher, error) {
	return evaluator.NewPublisher(nc, cfg)
}

// pubsubConsumerFactory creates a pubsub.Consumer - injectable for testing
var pubsubConsumerFactory = func(nc *nats.Conn, cfg delivery.Config) (pubsub.Consumer, error) {
	return delivery.NewConsumer(nc, cfg)
}

func (m *Manager) Init(ctx context.Context) error {
	// Common infrastructure initialization
	if err := m.initAuthService(ctx); err != nil {
		return err
	}

	// Ensure admin user exists
	if err := m.ensureAdminUser(ctx); err != nil {
		return err
	}

	// Initialize Unified Server Service
	server.InitDefault(m.cfg.Server, nil)

	// Mode-specific initialization
	if m.opts.Mode.IsStandalone() {
		return m.initStandalone(ctx)
	}
	return m.initDistributed(ctx)
}

// initStandalone initializes services for standalone deployment mode.
// All services run in a single process without HTTP inter-service communication.
// In standalone mode, services use direct in-process references instead of gRPC clients.
func (m *Manager) initStandalone(ctx context.Context) error {
	// Initialize Puller service first - required by other services
	if err := m.initPullerService(ctx); err != nil {
		return err
	}

	// Initialize Indexer service - uses local Puller directly
	indexerSvc, err := indexer.NewService(m.cfg.Indexer, m.pullerService, slog.Default())
	if err != nil {
		return fmt.Errorf("failed to create indexer service: %w", err)
	}
	m.indexerService = indexerSvc
	slog.Info("Initialized Indexer Service (standalone)")

	// Initialize Streamer service - uses local Puller directly
	opts := []streamer.ServiceConfigOption{streamer.WithPullerClient(m.pullerService)}
	streamerSvc, err := streamer.NewService(m.cfg.Streamer.Server, slog.Default(), opts...)
	if err != nil {
		return fmt.Errorf("failed to create streamer service: %w", err)
	}
	m.streamerService = streamerSvc
	slog.Info("Initialized Streamer Service (standalone)")

	// Initialize Query service - uses local Indexer directly
	sf, err := m.getStorageFactory(ctx)
	if err != nil {
		return err
	}
	queryService := query.NewService(sf.Document(), m.indexerService)
	slog.Info("Initialized Query Service (standalone)")

	// API server is the only HTTP server in standalone mode
	if err := m.initAPIServer(queryService); err != nil {
		return err
	}

	// Initialize trigger services with embedded NATS if configured
	if err := m.initTriggerServicesStandalone(ctx); err != nil {
		return err
	}

	// Initialize database service and deletion worker
	if err := m.initDatabaseService(ctx); err != nil {
		return err
	}

	return nil
}

// initDistributed initializes services for distributed deployment mode.
// Services communicate via gRPC and can run on separate machines.
// Key principle: In distributed mode, ALWAYS use gRPC clients for inter-service
// communication, never direct in-process calls.
func (m *Manager) initDistributed(ctx context.Context) error {
	// Initialize Puller service and register gRPC server
	if m.opts.RunPuller {
		if err := m.initPullerService(ctx); err != nil {
			return err
		}
		m.initPullerGRPCServer()
	}

	// Initialize Indexer service and register gRPC server
	if m.opts.RunIndexer {
		if err := m.initIndexerService(ctx); err != nil {
			return err
		}
		m.initIndexerGRPCServer()
	}

	// Initialize Query service and register gRPC server
	if m.opts.RunQuery {
		queryService, err := m.initQueryService(ctx)
		if err != nil {
			return err
		}
		m.initQueryGRPCServer(queryService)
	}

	// Initialize Streamer service and register gRPC server
	if m.opts.RunStreamer {
		if err := m.initStreamerService(); err != nil {
			return err
		}
		m.initStreamerGRPCServer()
	}

	// Initialize API Gateway - uses gRPC clients to connect to remote services
	if m.opts.RunAPI {
		if err := m.initGateway(); err != nil {
			return err
		}
	}

	// Initialize trigger services with remote NATS
	if m.opts.RunTriggerEvaluator || m.opts.RunTriggerWorker {
		if err := m.initTriggerServices(ctx); err != nil {
			return err
		}
	}

	// Initialize database service and deletion worker
	// In distributed mode, deletion worker typically runs on API nodes
	if m.opts.RunAPI || m.opts.RunDeletionWorker {
		if err := m.initDatabaseService(ctx); err != nil {
			return err
		}
	}

	return nil
}

func (m *Manager) getStorageFactory(ctx context.Context) (storage.StorageFactory, error) {
	// In standalone mode, storage factory is always needed
	// In distributed mode, check if any service that needs storage is enabled
	if m.opts.Mode.IsDistributed() {
		if !(m.opts.RunQuery || m.opts.RunTriggerEvaluator || m.opts.RunAPI || m.opts.RunTriggerWorker || m.opts.RunPuller) {
			return nil, nil
		}
	}

	m.storageFactoryOnce.Do(func() {
		m.storageFactory, m.storageFactoryErr = storageFactoryFactory(ctx, m.cfg)
		if m.storageFactoryErr == nil {
			slog.Info("Connected to Storage successfully")
		}
	})

	if m.storageFactoryErr != nil {
		return nil, fmt.Errorf("failed to initialize storage factory: %w", m.storageFactoryErr)
	}

	return m.storageFactory, nil
}

func (m *Manager) initAuthService(ctx context.Context) error {
	if !m.opts.RunAPI && !m.opts.RunTriggerWorker {
		return nil
	}

	sf, err := m.getStorageFactory(ctx)
	if err != nil {
		return err
	}

	var authErr error
	m.authService, authErr = identity.NewAuthN(m.cfg.Identity.AuthN, sf.User(), sf.Revocation())
	if authErr != nil {
		return fmt.Errorf("failed to create auth service: %w", authErr)
	}

	slog.Info("Initialized Auth Service")
	return nil
}

// ensureAdminUser creates the system admin user if it doesn't exist.
// The admin username and initial password are read from the identity.admin config.
func (m *Manager) ensureAdminUser(ctx context.Context) error {
	// Skip if auth service is not initialized (no API or trigger worker)
	if m.authService == nil {
		return nil
	}

	adminCfg := m.cfg.Identity.Admin
	if adminCfg.Username == "" {
		slog.Debug("Admin user initialization skipped: no username configured")
		return nil
	}

	if adminCfg.Password == "" {
		slog.Warn("Admin user initialization skipped: no password configured",
			"username", adminCfg.Username)
		return nil
	}

	// Try to create admin user via SignUp
	// SignUp will return error if user already exists
	_, err := m.authService.SignUp(ctx, identity.SignupRequest{
		Username: adminCfg.Username,
		Password: adminCfg.Password,
	})

	if err != nil {
		// User already exists is expected, not an error
		if err.Error() == "user already exists" {
			slog.Debug("Admin user already exists", "username", adminCfg.Username)
			return nil
		}
		return fmt.Errorf("failed to create admin user: %w", err)
	}

	slog.Info("Created admin user", "username", adminCfg.Username)
	return nil
}

// initQueryService creates and returns a query engine service for distributed mode.
// Uses gRPC client to connect to remote Indexer service.
func (m *Manager) initQueryService(ctx context.Context) (query.Service, error) {
	sf, err := m.getStorageFactory(ctx)
	if err != nil {
		return nil, err
	}

	// Distributed mode: use gRPC client to connect to remote Indexer
	client, err := indexer.NewClient(m.cfg.Query.IndexerAddr, slog.Default())
	if err != nil {
		return nil, fmt.Errorf("failed to connect to indexer: %w", err)
	}
	slog.Info("Query Engine using remote Indexer service", "url", m.cfg.Query.IndexerAddr)

	return query.NewService(sf.Document(), client), nil
}

// initQueryGRPCServer registers the query service with the unified gRPC server.
// In standalone mode, this is not called since query service runs in-process.
func (m *Manager) initQueryGRPCServer(service query.Service) {
	grpcServer := query.NewGRPCServer(service)
	server.Default().RegisterGRPCService(&pb.QueryService_ServiceDesc, grpcServer)
	slog.Info("Registered Query Service (gRPC)")
}

func (m *Manager) initAPIServer(queryService query.Service) error {
	var authzEngine identity.AuthZ

	if m.cfg.Identity.AuthZ.RulesPath != "" {
		var err error
		authzEngine, err = identity.NewAuthZ(m.cfg.Identity.AuthZ, queryService)
		if err != nil {
			return fmt.Errorf("failed to create authz engine: %w", err)
		}

		slog.Info("Loaded authorization rules", "path", m.cfg.Identity.AuthZ.RulesPath)
	}

	// Determine which Streamer interface to use
	// In distributed mode, use the gRPC client; in standalone mode, use local service
	var streamerSvc streamer.Service
	if m.streamerClient != nil {
		streamerSvc = m.streamerClient
	} else {
		streamerSvc = m.streamerService
	}

	// Always initialize realtime server as part of gateway
	m.rtServer = realtime.NewServer(queryService, streamerSvc, m.cfg.Storage.Topology.Document.DataCollection,
		m.authService, m.cfg.Gateway.Realtime)

	// Register API routes to the unified server
	gateway := gateway.NewServer(queryService, m.authService, authzEngine, m.rtServer)
	gateway.RegisterRoutes(server.Default().HTTPMux())

	return nil
}

// initStreamerService creates the Streamer service for distributed mode.
// Uses gRPC client to connect to remote Puller service.
func (m *Manager) initStreamerService() error {
	cfg := m.cfg.Streamer.Server

	// Distributed mode: connect to Puller via gRPC
	pullerClient, err := puller.NewClient(cfg.PullerAddr, nil)
	if err != nil {
		return fmt.Errorf("failed to create Puller gRPC client: %w", err)
	}
	slog.Info("Streamer using remote Puller service", "url", cfg.PullerAddr)

	opts := []streamer.ServiceConfigOption{streamer.WithPullerClient(pullerClient)}
	svc, err := streamer.NewService(cfg, slog.Default(), opts...)
	if err != nil {
		return fmt.Errorf("failed to create streamer service: %w", err)
	}
	m.streamerService = svc
	slog.Info("Initialized Streamer Service (distributed)")

	return nil
}

// initStreamerGRPCServer registers the Streamer service with the unified gRPC server.
func (m *Manager) initStreamerGRPCServer() {
	grpcServer := streamer.NewGRPCServer(m.streamerService)
	server.Default().RegisterGRPCService(&streamerv1.StreamerService_ServiceDesc, grpcServer)
	slog.Info("Registered Streamer Service (gRPC)")
}

// initGateway initializes the API Gateway with gRPC clients for remote services.
// In distributed mode, Gateway always connects to Query and Streamer via gRPC clients.
func (m *Manager) initGateway() error {
	// Create Query gRPC client
	queryClient, err := query.NewClient(m.cfg.Gateway.QueryServiceURL)
	if err != nil {
		return fmt.Errorf("failed to create Query gRPC client: %w", err)
	}
	slog.Info("Gateway using remote Query service", "url", m.cfg.Gateway.QueryServiceURL)

	// Create Streamer gRPC client using streamer.client config
	streamerCfg := m.cfg.Streamer.Client
	if m.cfg.Gateway.StreamerServiceURL != "" {
		streamerCfg.StreamerAddr = m.cfg.Gateway.StreamerServiceURL
	}
	streamerClient, err := streamer.NewClient(streamerCfg, slog.Default())
	if err != nil {
		return fmt.Errorf("failed to create Streamer gRPC client: %w", err)
	}
	m.streamerClient = streamerClient
	slog.Info("Gateway using remote Streamer service", "url", streamerCfg.StreamerAddr)

	// Initialize API server with gRPC clients
	if err := m.initAPIServer(queryClient); err != nil {
		return err
	}

	return nil
}

// initPullerService creates the Puller service and adds backends.
// Does NOT register gRPC server - that's done separately in distributed mode.
func (m *Manager) initPullerService(ctx context.Context) error {
	slog.Info("Initializing Change Stream Puller Service...")

	sf, err := m.getStorageFactory(ctx)
	if err != nil {
		return err
	}

	// 1. Create Puller Service
	m.pullerService = puller.NewService(m.cfg.Puller, nil) // Logger is nil for now

	// 2. Add Backends
	for _, backendCfg := range m.cfg.Puller.Backends {
		client, dbName, err := sf.GetMongoClient(backendCfg.Name)
		if err != nil {
			return fmt.Errorf("failed to get mongo client for backend %s: %w", backendCfg.Name, err)
		}

		if err := m.pullerService.AddBackend(backendCfg.Name, client, dbName, backendCfg); err != nil {
			return fmt.Errorf("failed to add backend %s: %w", backendCfg.Name, err)
		}
		slog.Info("Added puller backend", "name", backendCfg.Name, "database", dbName)
	}

	return nil
}

// initPullerGRPCServer registers the Puller service with the unified gRPC server.
func (m *Manager) initPullerGRPCServer() {
	grpcServer := puller.NewGRPCServerWithInit(m.cfg.Puller.GRPC, m.pullerService, nil)
	m.pullerGRPC = grpcServer
	server.Default().RegisterGRPCService(&pullerv1.PullerService_ServiceDesc, grpcServer)
	slog.Info("Registered Puller Service (gRPC)")
}

// initIndexerService creates the Indexer service for distributed mode.
// Uses gRPC client to connect to remote Puller service.
func (m *Manager) initIndexerService(ctx context.Context) error {
	slog.Info("Initializing Indexer Service...")

	// Distributed mode: connect to Puller via gRPC
	pullerClient, err := puller.NewClient(m.cfg.Indexer.PullerAddr, nil)
	if err != nil {
		return fmt.Errorf("failed to create Puller gRPC client for Indexer: %w", err)
	}
	slog.Info("Indexer using remote Puller service", "url", m.cfg.Indexer.PullerAddr)

	svc, err := indexer.NewService(m.cfg.Indexer, pullerClient, slog.Default())
	if err != nil {
		return fmt.Errorf("failed to create indexer service: %w", err)
	}
	m.indexerService = svc
	slog.Info("Initialized Indexer Service (distributed)")

	return nil
}

// initIndexerGRPCServer registers the Indexer service with the unified gRPC server.
func (m *Manager) initIndexerGRPCServer() {
	grpcServer := indexer.NewGRPCServer(m.indexerService)
	server.Default().RegisterGRPCService(&indexerv1.IndexerService_ServiceDesc, grpcServer)
	slog.Info("Registered Indexer Service (gRPC)")
}

// initTriggerServicesStandalone initializes trigger services for standalone mode.
// Uses local Puller service and optionally embedded NATS.
// In standalone mode, all trigger services are always initialized (no RunXXX checks).
func (m *Manager) initTriggerServicesStandalone(ctx context.Context) error {
	// Standalone mode requires Puller service
	if m.pullerService == nil {
		return fmt.Errorf("puller service is required for trigger evaluator in standalone mode")
	}

	// Initialize NATS provider based on configuration
	m.natsProvider = trigger.NewRemoteNATSProvider(m.cfg.Trigger.NatsURL)

	nc, err := m.natsProvider.Connect(context.Background())
	if err != nil {
		return fmt.Errorf("failed to connect to NATS: %w", err)
	}

	sf, err := m.getStorageFactory(ctx)
	if err != nil {
		return err
	}

	// Initialize Evaluator Service
	publisher, err := pubsubPublisherFactory(nc, m.cfg.Trigger.Evaluator)
	if err != nil {
		return fmt.Errorf("failed to create trigger publisher: %w", err)
	}

	evalSvc, err := evaluatorServiceFactory(evaluator.Dependencies{
		Store:     sf.Document(),
		Puller:    m.pullerService,
		Publisher: publisher,
		Metrics:   nil,
	}, m.cfg.Trigger.Evaluator)
	if err != nil {
		return fmt.Errorf("failed to create trigger evaluator service: %w", err)
	}
	m.triggerService = evalSvc
	slog.Info("Initialized Trigger Evaluator Service (standalone)")

	// Initialize Delivery Service
	consumer, err := pubsubConsumerFactory(nc, m.cfg.Trigger.Delivery)
	if err != nil {
		return fmt.Errorf("failed to create trigger consumer: %w", err)
	}

	deliverySvc, err := deliveryServiceFactory(delivery.Dependencies{
		Consumer: consumer,
		Auth:     m.authService,
		Secrets:  nil,
		Metrics:  nil,
	}, m.cfg.Trigger.Delivery)
	if err != nil {
		return fmt.Errorf("failed to create trigger delivery service: %w", err)
	}
	m.triggerConsumer = deliverySvc
	slog.Info("Initialized Trigger Delivery Service (standalone)")

	return nil
}

// initTriggerServices initializes trigger services for distributed mode.
// Uses gRPC client to connect to remote Puller and remote NATS.
func (m *Manager) initTriggerServices(ctx context.Context) error {
	// Distributed mode always uses remote NATS
	m.natsProvider = trigger.NewRemoteNATSProvider(m.cfg.Trigger.NatsURL)

	nc, err := m.natsProvider.Connect(context.Background())
	if err != nil {
		return fmt.Errorf("failed to connect to NATS: %w", err)
	}

	sf, err := m.getStorageFactory(ctx)
	if err != nil {
		return err
	}

	// Initialize Evaluator Service - uses gRPC client to remote Puller
	if m.opts.RunTriggerEvaluator {
		pullerClient, err := puller.NewClient(m.cfg.Trigger.Evaluator.PullerAddr, nil)
		if err != nil {
			return fmt.Errorf("failed to create Puller gRPC client for Trigger Evaluator: %w", err)
		}
		slog.Info("Trigger Evaluator using remote Puller service", "url", m.cfg.Trigger.Evaluator.PullerAddr)

		publisher, err := pubsubPublisherFactory(nc, m.cfg.Trigger.Evaluator)
		if err != nil {
			return fmt.Errorf("failed to create trigger publisher: %w", err)
		}

		evalSvc, err := evaluatorServiceFactory(evaluator.Dependencies{
			Store:     sf.Document(),
			Puller:    pullerClient,
			Publisher: publisher,
			Metrics:   nil,
		}, m.cfg.Trigger.Evaluator)
		if err != nil {
			return fmt.Errorf("failed to create trigger evaluator service: %w", err)
		}
		m.triggerService = evalSvc
		slog.Info("Initialized Trigger Evaluator Service (distributed)")
	}

	// Initialize Delivery Service
	if m.opts.RunTriggerWorker {
		consumer, err := pubsubConsumerFactory(nc, m.cfg.Trigger.Delivery)
		if err != nil {
			return fmt.Errorf("failed to create trigger consumer: %w", err)
		}

		deliverySvc, err := deliveryServiceFactory(delivery.Dependencies{
			Consumer: consumer,
			Auth:     m.authService,
			Secrets:  nil,
			Metrics:  nil,
		}, m.cfg.Trigger.Delivery)
		if err != nil {
			return fmt.Errorf("failed to create trigger delivery service: %w", err)
		}
		m.triggerConsumer = deliverySvc
		slog.Info("Initialized Trigger Delivery Service (distributed)")
	}

	return nil
}

// initDatabaseService initializes the database service and deletion worker.
func (m *Manager) initDatabaseService(ctx context.Context) error {
	sf, err := m.getStorageFactory(ctx)
	if err != nil {
		return err
	}

	// Get or create database store
	dbStore := sf.Database()
	if dbStore == nil {
		slog.Debug("Database store not available, skipping database service initialization")
		return nil
	}

	// Create database service
	svcCfg := database.ServiceConfig{
		MaxDatabasesPerUser: m.cfg.Database.MaxDatabasesPerUser,
	}
	m.databaseService = database.NewService(dbStore, svcCfg, slog.Default())
	slog.Info("Initialized Database Service")

	// Initialize deletion worker if enabled
	if m.cfg.Database.Deletion.Enabled && m.opts.RunDeletionWorker {
		workerCfg := m.cfg.Database.Deletion.ToDeletionWorkerConfig()
		m.deletionWorker = database.NewDeletionWorker(
			dbStore,
			sf.Document(),
			m.indexerService,
			workerCfg,
			slog.Default(),
		)
		slog.Info("Initialized Deletion Worker")
	}

	return nil
}
