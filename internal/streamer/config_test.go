package streamer

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	// Verify Service defaults
	assert.Equal(t, DefaultServiceConfig(), cfg.Server)

	// Verify Client defaults
	assert.Equal(t, DefaultClientConfig(), cfg.Client)
}

func TestConfig_StructFields(t *testing.T) {
	cfg := Config{
		Server: ServerConfig{
			PullerAddr:  "localhost:50053",
			SendTimeout: 10 * time.Second,
		},
		Client: ClientConfig{
			StreamerAddr: "custom:50052",
		},
	}

	assert.Equal(t, "localhost:50053", cfg.Server.PullerAddr)
	assert.Equal(t, 10*time.Second, cfg.Server.SendTimeout)
	assert.Equal(t, "custom:50052", cfg.Client.StreamerAddr)
}
