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

func TestDefaultStoreConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultStoreConfig()

	if cfg.Path != "data/indexer/indexes.db" {
		t.Errorf("Path = %q, want data/indexer/indexes.db", cfg.Path)
	}
	if cfg.BatchSize != 100 {
		t.Errorf("BatchSize = %d, want 100", cfg.BatchSize)
	}
	if cfg.BatchInterval != 100*time.Millisecond {
		t.Errorf("BatchInterval = %v, want 100ms", cfg.BatchInterval)
	}
	if cfg.QueueSize != 10000 {
		t.Errorf("QueueSize = %d, want 10000", cfg.QueueSize)
	}
	if cfg.BlockCacheSize != 128*1024*1024 {
		t.Errorf("BlockCacheSize = %d, want %d", cfg.BlockCacheSize, 128*1024*1024)
	}
}
