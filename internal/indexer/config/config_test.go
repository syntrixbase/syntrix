package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.Equal(t, "config/index_templates.yaml", cfg.TemplatePath)
	assert.Equal(t, "data/indexer/progress", cfg.ProgressPath)
	assert.Equal(t, "indexer", cfg.ConsumerID)
	assert.Equal(t, 5*time.Second, cfg.ReconcileInterval)
}

func TestConfig_Fields(t *testing.T) {
	cfg := Config{
		TemplatePath:      "/custom/path/templates.yaml",
		ProgressPath:      "/data/progress",
		ConsumerID:        "custom-indexer",
		ReconcileInterval: 10 * time.Second,
	}

	assert.Equal(t, "/custom/path/templates.yaml", cfg.TemplatePath)
	assert.Equal(t, "/data/progress", cfg.ProgressPath)
	assert.Equal(t, "custom-indexer", cfg.ConsumerID)
	assert.Equal(t, 10*time.Second, cfg.ReconcileInterval)
}
