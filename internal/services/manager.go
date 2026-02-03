package services

import (
	"context"
	"sync"

	"github.com/syntrixbase/syntrix/internal/config"
	"github.com/syntrixbase/syntrix/internal/core/database"
	"github.com/syntrixbase/syntrix/internal/core/identity"
	"github.com/syntrixbase/syntrix/internal/core/storage"
	"github.com/syntrixbase/syntrix/internal/gateway"
	"github.com/syntrixbase/syntrix/internal/gateway/realtime"
	"github.com/syntrixbase/syntrix/internal/indexer"
	"github.com/syntrixbase/syntrix/internal/puller"
	services_config "github.com/syntrixbase/syntrix/internal/services/config"
	"github.com/syntrixbase/syntrix/internal/streamer"
	"github.com/syntrixbase/syntrix/internal/trigger"
)

type Options struct {
	RunAPI              bool
	RunQuery            bool
	RunStreamer         bool
	RunTriggerEvaluator bool
	RunTriggerWorker    bool
	RunPuller           bool
	RunIndexer          bool

	// Mode specifies the deployment mode (distributed or standalone).
	Mode services_config.DeploymentMode
}

// Re-export DeploymentMode constants for backwards compatibility.
const (
	ModeDistributed = services_config.ModeDistributed
	ModeStandalone  = services_config.ModeStandalone
)

type triggerService interface {
	Start(ctx context.Context) error
}

type triggerConsumer interface {
	Start(ctx context.Context) error
}

type Manager struct {
	cfg  *config.Config
	opts Options

	storageFactory     storage.StorageFactory
	storageFactoryOnce sync.Once
	storageFactoryErr  error

	authService     identity.AuthN
	gatewayServer   *gateway.Server
	rtServer        *realtime.Server
	streamerService streamer.StreamerServer // local Streamer service (when RunStreamer=true)
	streamerClient  streamer.Service        // remote Streamer client (for Gateway in distributed mode)
	triggerConsumer triggerConsumer
	triggerService  triggerService
	natsProvider    trigger.NATSProvider
	pullerService   puller.LocalService
	pullerGRPC      *puller.GRPCServer
	indexerService  indexer.LocalService

	// Database management
	databaseService database.Service
	deletionWorker  *database.DeletionWorker

	wg sync.WaitGroup
}

func NewManager(cfg *config.Config, opts Options) *Manager {
	return &Manager{
		cfg:  cfg,
		opts: opts,
	}
}

func (m *Manager) AuthService() identity.AuthN {
	return m.authService
}

func (m *Manager) DatabaseService() database.Service {
	return m.databaseService
}
