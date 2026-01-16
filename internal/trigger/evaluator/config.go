package evaluator

import "github.com/syntrixbase/syntrix/internal/core/pubsub"

// Config contains all configuration for the evaluator service.
type Config struct {
	// Service behavior
	StartFromNow       bool   `yaml:"start_from_now"`
	RulesFile          string `yaml:"rules_file"`
	CheckpointDatabase string `yaml:"checkpoint_database"`

	// Pubsub configuration
	StreamName    string `yaml:"stream_name"`
	RetryAttempts int    `yaml:"retry_attempts"`
	StorageType   string `yaml:"storage_type"`
}

// DefaultConfig returns default evaluator configuration.
func DefaultConfig() Config {
	return Config{
		StartFromNow:       true,
		RulesFile:          "triggers.example.json",
		CheckpointDatabase: "default",
		StreamName:         "TRIGGERS",
		RetryAttempts:      3,
		StorageType:        "file",
	}
}

// StorageTypeValue returns the pubsub.StorageType from the config string.
func (c Config) StorageTypeValue() pubsub.StorageType {
	if c.StorageType == "memory" {
		return pubsub.MemoryStorage
	}
	return pubsub.FileStorage
}

// ApplyDefaults fills in zero values with defaults.
func (c *Config) ApplyDefaults() {
	defaults := DefaultConfig()
	if c.CheckpointDatabase == "" {
		c.CheckpointDatabase = defaults.CheckpointDatabase
	}
	if c.StreamName == "" {
		c.StreamName = defaults.StreamName
	}
	if c.RetryAttempts <= 0 {
		c.RetryAttempts = defaults.RetryAttempts
	}
	if c.StorageType == "" {
		c.StorageType = defaults.StorageType
	}
}
