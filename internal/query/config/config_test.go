package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	services "github.com/syntrixbase/syntrix/internal/services/config"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.Equal(t, "localhost:9000", cfg.IndexerAddr)
}

func TestConfig_ApplyDefaults(t *testing.T) {
	cfg := &Config{}
	cfg.ApplyDefaults()

	assert.Equal(t, "localhost:9000", cfg.IndexerAddr)
}

func TestConfig_ApplyEnvOverrides(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ApplyEnvOverrides()
	// No env vars, just verify no panic
}

func TestConfig_ResolvePaths(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ResolvePaths("config")
	// No paths to resolve, just verify no panic
}

func TestConfig_Validate(t *testing.T) {
	cfg := DefaultConfig()
	err := cfg.Validate(services.ModeDistributed)
	assert.NoError(t, err)
}

func TestConfig_ApplyDefaults_CustomValuesPreserved(t *testing.T) {
	cfg := &Config{
		IndexerAddr: "custom:9001",
	}
	cfg.ApplyDefaults()

	assert.Equal(t, "custom:9001", cfg.IndexerAddr)
}

func TestConfig_Validate_EmptyConfig(t *testing.T) {
	cfg := Config{}
	err := cfg.Validate(services.ModeStandalone)
	assert.NoError(t, err)
}

func TestConfig_StructFields(t *testing.T) {
	cfg := Config{
		IndexerAddr: "custom-indexer:9002",
	}

	assert.Equal(t, "custom-indexer:9002", cfg.IndexerAddr)
}
