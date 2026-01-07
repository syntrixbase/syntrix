package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultDeploymentConfig(t *testing.T) {
	cfg := DefaultDeploymentConfig()

	assert.Equal(t, "distributed", cfg.Mode)
	assert.True(t, cfg.Standalone.EmbeddedNATS)
	assert.Equal(t, "data/nats", cfg.Standalone.NATSDataDir)
}

func TestDeploymentConfig_StructFields(t *testing.T) {
	cfg := DeploymentConfig{
		Mode: "standalone",
		Standalone: StandaloneConfig{
			EmbeddedNATS: false,
			NATSDataDir:  "custom/nats",
		},
	}

	assert.Equal(t, "standalone", cfg.Mode)
	assert.False(t, cfg.Standalone.EmbeddedNATS)
	assert.Equal(t, "custom/nats", cfg.Standalone.NATSDataDir)
}
