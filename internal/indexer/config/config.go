// Package config provides configuration for the Indexer service.
package config

import "time"

type StorageMode string

const (
	StorageModeMemory StorageMode = "memory"
	StorageModePebble StorageMode = "pebble"
)

// Config holds the Indexer service configuration.
type Config struct {
	// PullerAddr is the address of the Puller gRPC service.
	// Used in distributed mode to connect to Puller.
	// Defaults to "localhost:9000".
	PullerAddr string `yaml:"puller_addr"`

	// TemplatePath is the path to the index templates YAML file.
	TemplatePath string `yaml:"template_path"`

	// ProgressPath is the path to store progress markers.
	// Defaults to "data/indexer/progress".
	ProgressPath string `yaml:"progress_path"`

	// ConsumerID is the ID used when subscribing to Puller.
	// Defaults to "indexer".
	ConsumerID string `yaml:"consumer_id"`

	// ReconcileInterval is the interval between reconciliation loops.
	// Defaults to 5s.
	ReconcileInterval time.Duration `yaml:"reconcile_interval"`

	// StorageMode selects the storage backend: "memory" or "pebble".
	// Defaults to "memory".
	StorageMode StorageMode `yaml:"storage_mode"`

	// Store configures the PebbleDB storage backend.
	// Only used when StorageMode is "pebble".
	Store StoreConfig `yaml:"store"`
}

// StoreConfig holds the configuration for PebbleDB storage.
type StoreConfig struct {
	// Path is the directory to store the database.
	// Defaults to "data/indexer/indexes.db".
	Path string `yaml:"path"`

	// BatchSize is the max number of operations per batch.
	// Defaults to 100.
	BatchSize int `yaml:"batch_size"`

	// BatchInterval is the max time to wait before flushing a batch.
	// Defaults to 100ms.
	BatchInterval time.Duration `yaml:"batch_interval"`

	// QueueSize is the buffer for pending writes.
	// Defaults to 10000.
	QueueSize int `yaml:"queue_size"`

	// BlockCacheSize is the size of the block cache in bytes.
	// Defaults to 64MB.
	BlockCacheSize int64 `yaml:"block_cache_size"`
}

// DefaultConfig returns the default Indexer configuration.
func DefaultConfig() Config {
	return Config{
		PullerAddr:        "localhost:9000",
		TemplatePath:      "config/index_templates.yaml",
		ProgressPath:      "data/indexer/progress",
		ConsumerID:        "indexer",
		ReconcileInterval: 5 * time.Second,
		StorageMode:       StorageModeMemory,
		Store:             DefaultStoreConfig(),
	}
}

// DefaultStoreConfig returns the default StoreConfig.
func DefaultStoreConfig() StoreConfig {
	return StoreConfig{
		Path:           "data/indexer/indexes.db",
		BatchSize:      100,
		BatchInterval:  100 * time.Millisecond,
		QueueSize:      10000,
		BlockCacheSize: 128 * 1024 * 1024, // 128MB
	}
}
