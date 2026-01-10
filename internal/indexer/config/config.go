// Package config provides configuration for the Indexer service.
package config

import "time"

// Config holds the Indexer service configuration.
type Config struct {
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
}

// DefaultConfig returns the default Indexer configuration.
func DefaultConfig() Config {
	return Config{
		TemplatePath:      "config/index_templates.yaml",
		ProgressPath:      "data/indexer/progress",
		ConsumerID:        "indexer",
		ReconcileInterval: 5 * time.Second,
	}
}
