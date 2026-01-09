package services

import (
	"context"
	"sync"

	"github.com/syntrixbase/syntrix/internal/api/realtime"
	"github.com/syntrixbase/syntrix/internal/config"
	"github.com/syntrixbase/syntrix/internal/identity"
	"github.com/syntrixbase/syntrix/internal/puller"
	"github.com/syntrixbase/syntrix/internal/storage"
	"github.com/syntrixbase/syntrix/internal/streamer"
	"github.com/syntrixbase/syntrix/internal/trigger"
)

// DeploymentMode represents the deployment mode of the service.
type DeploymentMode int

const (
	// ModeDistributed is the default mode where services communicate via HTTP/NATS.
	ModeDistributed DeploymentMode = iota
	// ModeStandalone runs all services in a single process with direct function calls.
	ModeStandalone
)

type Options struct {
	RunAPI              bool
	RunQuery            bool
	RunStreamer         bool
	RunTriggerEvaluator bool
	RunTriggerWorker    bool
	RunPuller           bool

	ForceQueryClient bool

	// Mode specifies the deployment mode (distributed or standalone).
	Mode DeploymentMode
}

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
	rtServer        *realtime.Server
	streamerService streamer.StreamerServer // local Streamer service (when RunStreamer=true)
	streamerClient  streamer.Service        // remote Streamer client (for Gateway in distributed mode)
	triggerConsumer triggerConsumer
	triggerService  triggerService
	natsProvider    trigger.NATSProvider
	pullerService   puller.LocalService
	pullerGRPC      *puller.GRPCServer

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
