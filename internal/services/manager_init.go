package services

import (
	"context"
	"fmt"
	"log/slog"

	indexerv1 "github.com/syntrixbase/syntrix/api/gen/indexer/v1"
	pullerv1 "github.com/syntrixbase/syntrix/api/gen/puller/v1"
	pb "github.com/syntrixbase/syntrix/api/gen/query/v1"
	streamerv1 "github.com/syntrixbase/syntrix/api/gen/streamer/v1"
	"github.com/syntrixbase/syntrix/internal/config"
	"github.com/syntrixbase/syntrix/internal/core/identity"
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
	triggerengine "github.com/syntrixbase/syntrix/internal/trigger/engine"
	"github.com/syntrixbase/syntrix/internal/trigger/evaluator"

	"github.com/nats-io/nats.go"
)

var triggerFactoryFactory = func(store storage.DocumentStore, nats *nats.Conn, auth identity.AuthN, opts ...triggerengine.FactoryOption) (triggerengine.TriggerFactory, error) {
	// Default options
	defaultOpts := []triggerengine.FactoryOption{triggerengine.WithStartFromNow(true)}
	return triggerengine.NewFactory(store, nats, auth, append(defaultOpts, opts...)...)
}
var storageFactoryFactory = func(ctx context.Context, cfg *config.Config) (storage.StorageFactory, error) {
	return storage.NewFactory(ctx, cfg.Storage)
}

func (m *Manager) Init(ctx context.Context) error {
	// Common infrastructure initialization
	if err := m.initAuthService(ctx); err != nil {
		return err
	}

	// Initialize Unified Server Service
	server.InitDefault(m.cfg.Server, nil)

	// Mode-specific initialization
	if m.opts.Mode == ModeStandalone {
		return m.initStandalone(ctx)
	}
	return m.initDistributed(ctx)
}

// initStandalone initializes services for standalone deployment mode.
// All services run in a single process without HTTP inter-service communication.
func (m *Manager) initStandalone(ctx context.Context) error {
	// Initialize Puller service (local only, no gRPC)
	if m.opts.RunPuller {
		if err := m.initPullerService(ctx); err != nil {
			return err
		}
	}

	// Initialize Indexer service (local only, no gRPC)
	// Always initialize in standalone because Query Engine requires it
	if err := m.initIndexerService(ctx); err != nil {
		return err
	}

	// Initialize Streamer service (local only, no gRPC)
	if err := m.initStreamerService(); err != nil {
		return err
	}

	// Initialize Query service and API server
	queryService, err := m.initQueryService(ctx)
	if err != nil {
		return err
	}

	// API server is the only HTTP server in standalone mode
	if err := m.initAPIServer(queryService); err != nil {
		return err
	}

	// Initialize trigger services with embedded NATS if configured
	if m.opts.RunTriggerEvaluator || m.opts.RunTriggerWorker {
		useEmbeddedNATS := m.cfg.Deployment.Standalone.EmbeddedNATS
		if err := m.initTriggerServices(ctx, useEmbeddedNATS); err != nil {
			return err
		}
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
		if err := m.initTriggerServices(ctx, false); err != nil {
			return err
		}
	}

	return nil
}

func (m *Manager) getStorageFactory(ctx context.Context) (storage.StorageFactory, error) {
	if !(m.opts.RunQuery || m.opts.RunTriggerEvaluator || m.opts.RunAPI || m.opts.RunTriggerWorker || m.opts.RunPuller) {
		return nil, nil
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

// initQueryService creates and returns a query engine service.
// In standalone mode, uses local Indexer; in distributed mode, uses gRPC client.
func (m *Manager) initQueryService(ctx context.Context) (query.Service, error) {
	sf, err := m.getStorageFactory(ctx)
	if err != nil {
		return nil, err
	}

	if m.opts.Mode == ModeStandalone {
		// Standalone: direct call to local indexer
		if m.indexerService == nil {
			return nil, fmt.Errorf("indexer service required in standalone mode")
		}
		return query.NewService(sf.Document(), m.indexerService), nil
	}

	// Distributed (including --all): must use gRPC client
	if m.cfg.Query.IndexerAddr == "" {
		return nil, fmt.Errorf("query.indexer_addr required in distributed mode")
	}
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

	if m.cfg.Identity.AuthZ.RulesFile != "" {
		var err error
		authzEngine, err = identity.NewAuthZ(m.cfg.Identity.AuthZ, queryService)
		if err != nil {
			return fmt.Errorf("failed to create authz engine: %w", err)
		}

		slog.Info("Loaded authorization rules", "file", m.cfg.Identity.AuthZ.RulesFile)
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

// initStreamerService creates the Streamer service and sets m.streamerService.
// In standalone mode, uses local Puller; in distributed mode, uses gRPC client.
func (m *Manager) initStreamerService() error {
	cfg := m.cfg.Streamer.Server
	var opts []streamer.ServiceConfigOption

	if m.opts.Mode == ModeStandalone {
		// Standalone mode: use local Puller service
		if m.pullerService != nil {
			opts = append(opts, streamer.WithPullerClient(m.pullerService))
			slog.Info("Streamer using local Puller service")
		} else {
			slog.Warn("Streamer running without Puller - realtime events will not be available")
		}
	} else {
		// Distributed mode: connect to Puller via gRPC
		if cfg.PullerAddr != "" {
			pullerClient, err := puller.NewClient(cfg.PullerAddr, nil)
			if err != nil {
				return fmt.Errorf("failed to create Puller gRPC client: %w", err)
			}
			opts = append(opts, streamer.WithPullerClient(pullerClient))
			slog.Info("Streamer using remote Puller service", "url", cfg.PullerAddr)
		} else {
			slog.Warn("Streamer running without Puller - realtime events will not be available")
		}
	}

	svc, err := streamer.NewService(cfg, slog.Default(), opts...)
	if err != nil {
		return fmt.Errorf("failed to create streamer service: %w", err)
	}
	m.streamerService = svc
	slog.Info("Initialized Streamer Service")

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

// initTriggerServices initializes trigger evaluator and worker services.
// useEmbeddedNATS determines whether to use embedded NATS (standalone) or remote NATS (distributed).
func (m *Manager) initTriggerServices(ctx context.Context, useEmbeddedNATS bool) error {
	// Create NATS provider based on parameter
	if useEmbeddedNATS {
		m.natsProvider = trigger.NewEmbeddedNATSProvider(m.cfg.Deployment.Standalone.NATSDataDir)
	} else {
		m.natsProvider = trigger.NewRemoteNATSProvider(m.cfg.Trigger.NatsURL)
	}

	nc, err := m.natsProvider.Connect(context.Background())
	if err != nil {
		return fmt.Errorf("failed to connect to NATS: %w", err)
	}

	opts := []triggerengine.FactoryOption{
		triggerengine.WithStreamName(m.cfg.Trigger.StreamName),
	}
	if m.pullerService != nil {
		opts = append(opts, triggerengine.WithPuller(m.pullerService))
	}
	if m.cfg.Trigger.RulesFile != "" {
		opts = append(opts, triggerengine.WithRulesFile(m.cfg.Trigger.RulesFile))
	}

	sf, err := m.getStorageFactory(ctx)
	if err != nil {
		return err
	}

	factory, err := triggerFactoryFactory(sf.Document(), nc, m.authService, opts...)
	if err != nil {
		return fmt.Errorf("failed to create trigger factory: %w", err)
	}

	if m.opts.RunTriggerEvaluator {
		engine, err := factory.Engine()
		if err != nil {
			return fmt.Errorf("failed to create trigger engine: %w", err)
		}
		m.triggerService = engine

		slog.Info("Initialized Trigger Evaluator Service")
	}

	if m.opts.RunTriggerWorker {
		cons, err := factory.Consumer(m.cfg.Trigger.WorkerCount)
		if err != nil {
			return fmt.Errorf("failed to create trigger consumer: %w", err)
		}
		m.triggerConsumer = cons

		slog.Info("Initialized Trigger Worker Service")
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

// initIndexerService creates the Indexer service.
// In standalone mode, it uses local Puller service if available.
// In distributed mode, it must connect to Puller via gRPC.
func (m *Manager) initIndexerService(ctx context.Context) error {
	slog.Info("Initializing Indexer Service...")

	var pullerSvc puller.Service

	if m.opts.Mode == ModeStandalone {
		// Standalone mode: use local Puller service if available
		if m.pullerService != nil {
			pullerSvc = m.pullerService
			slog.Info("Indexer using local Puller service")
		} else {
			slog.Warn("Indexer running without Puller - index updates will not be available")
		}
	} else {
		// Distributed mode: must connect via gRPC
		if m.cfg.Indexer.PullerAddr != "" {
			client, err := puller.NewClient(m.cfg.Indexer.PullerAddr, nil)
			if err != nil {
				return fmt.Errorf("failed to create Puller gRPC client for Indexer: %w", err)
			}
			pullerSvc = client
			slog.Info("Indexer using remote Puller service", "url", m.cfg.Indexer.PullerAddr)
		} else {
			slog.Warn("Indexer running without Puller - index updates will not be available")
		}
	}

	svc, err := indexer.NewService(m.cfg.Indexer, pullerSvc, slog.Default())
	if err != nil {
		return fmt.Errorf("failed to create indexer service: %w", err)
	}
	m.indexerService = svc
	slog.Info("Initialized Indexer Service")

	return nil
}

// initIndexerGRPCServer registers the Indexer service with the unified gRPC server.
func (m *Manager) initIndexerGRPCServer() {
	grpcServer := indexer.NewGRPCServer(m.indexerService)
	server.Default().RegisterGRPCService(&indexerv1.IndexerService_ServiceDesc, grpcServer)
	slog.Info("Registered Indexer Service (gRPC)")
}

// initTriggerServicesV2 initializes trigger services using the new separated service packages.
// This is an alternative to initTriggerServices that uses evaluator.Service and delivery.Service
// instead of the combined TriggerFactory approach.
// useEmbeddedNATS determines whether to use embedded NATS (standalone) or remote NATS (distributed).
func (m *Manager) initTriggerServicesV2(ctx context.Context, useEmbeddedNATS bool) error {
	// Create NATS provider based on parameter
	if useEmbeddedNATS {
		m.natsProvider = trigger.NewEmbeddedNATSProvider(m.cfg.Deployment.Standalone.NATSDataDir)
	} else {
		m.natsProvider = trigger.NewRemoteNATSProvider(m.cfg.Trigger.NatsURL)
	}

	nc, err := m.natsProvider.Connect(context.Background())
	if err != nil {
		return fmt.Errorf("failed to connect to NATS: %w", err)
	}

	sf, err := m.getStorageFactory(ctx)
	if err != nil {
		return err
	}

	// Initialize Evaluator Service
	if m.opts.RunTriggerEvaluator {
		if m.pullerService == nil {
			return fmt.Errorf("puller service is required for trigger evaluator")
		}

		evalSvc, err := evaluator.NewService(evaluator.Dependencies{
			Store:   sf.Document(),
			Puller:  m.pullerService,
			Nats:    nc,
			Metrics: nil, // TODO: Add metrics when available
		}, evaluator.ServiceOptions{
			Database:     "default",
			StartFromNow: true,
			RulesFile:    m.cfg.Trigger.RulesFile,
			StreamName:   m.cfg.Trigger.StreamName,
		})
		if err != nil {
			return fmt.Errorf("failed to create trigger evaluator service: %w", err)
		}
		m.triggerService = evalSvc
		slog.Info("Initialized Trigger Evaluator Service (v2)")
	}

	// Initialize Delivery Service
	if m.opts.RunTriggerWorker {
		deliverySvc, err := delivery.NewService(delivery.Dependencies{
			Nats:    nc,
			Auth:    m.authService,
			Secrets: nil, // TODO: Add secret provider when available
			Metrics: nil, // TODO: Add metrics when available
		}, delivery.ServiceOptions{
			StreamName: m.cfg.Trigger.StreamName,
			NumWorkers: m.cfg.Trigger.WorkerCount,
		})
		if err != nil {
			return fmt.Errorf("failed to create trigger delivery service: %w", err)
		}
		m.triggerConsumer = deliverySvc
		slog.Info("Initialized Trigger Delivery Service (v2)")
	}

	return nil
}
