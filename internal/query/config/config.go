// Package config provides configuration for the Query service.
package config

// Config holds the Query service configuration.
type Config struct {
	// IndexerAddr is the address of the Indexer gRPC service.
	// Used in distributed mode to connect to Indexer.
	// Defaults to "localhost:9000".
	IndexerAddr string `yaml:"indexer_addr"`
}

// DefaultConfig returns the default Query configuration.
func DefaultConfig() Config {
	return Config{
		IndexerAddr: "localhost:9000",
	}
}
