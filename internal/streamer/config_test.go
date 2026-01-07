package streamer

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	// Verify Service defaults
	assert.Equal(t, DefaultServiceConfig(), cfg.Service)

	// Verify Client defaults
	assert.Equal(t, DefaultClientConfig(), cfg.Client)
}

func TestDefaultServiceConfig(t *testing.T) {
	cfg := DefaultServiceConfig()

	assert.Empty(t, cfg.PullerAddr)
	assert.Equal(t, 5*time.Second, cfg.SendTimeout)
}

func TestConfig_StructFields(t *testing.T) {
	cfg := Config{
		Service: ServiceConfig{
			PullerAddr:  "localhost:50053",
			SendTimeout: 10 * time.Second,
		},
		Client: ClientConfig{
			StreamerAddr: "custom:50052",
		},
	}

	assert.Equal(t, "localhost:50053", cfg.Service.PullerAddr)
	assert.Equal(t, 10*time.Second, cfg.Service.SendTimeout)
	assert.Equal(t, "custom:50052", cfg.Client.StreamerAddr)
}
