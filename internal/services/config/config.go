package config

import (
	"fmt"
	"os"
)

// DeploymentConfig holds deployment mode settings
type DeploymentConfig struct {
	Mode       string           `yaml:"mode"` // "standalone" or "distributed" (default)
	Standalone StandaloneConfig `yaml:"standalone"`
}

// StandaloneConfig holds standalone-specific settings
type StandaloneConfig struct {
	EmbeddedNATS bool   `yaml:"embedded_nats"` // Use embedded NATS server
	NATSDataDir  string `yaml:"nats_data_dir"` // Data directory for embedded NATS
}

func DefaultDeploymentConfig() DeploymentConfig {
	return DeploymentConfig{
		Mode: "distributed", // Default to distributed mode
		Standalone: StandaloneConfig{
			EmbeddedNATS: true,        // Default to embedded NATS in standalone
			NATSDataDir:  "data/nats", // Default data directory
		},
	}
}

// ApplyDefaults fills in zero values with defaults.
func (c *DeploymentConfig) ApplyDefaults() {
	defaults := DefaultDeploymentConfig()
	if c.Mode == "" {
		c.Mode = defaults.Mode
	}
	if c.Standalone.NATSDataDir == "" {
		c.Standalone.NATSDataDir = defaults.Standalone.NATSDataDir
	}
}

// ApplyEnvOverrides applies environment variable overrides.
func (c *DeploymentConfig) ApplyEnvOverrides() {
	if val := os.Getenv("SYNTRIX_DEPLOYMENT_MODE"); val != "" {
		c.Mode = val
	}
	if val := os.Getenv("SYNTRIX_EMBEDDED_NATS"); val != "" {
		c.Standalone.EmbeddedNATS = val == "true" || val == "1"
	}
	if val := os.Getenv("SYNTRIX_NATS_DATA_DIR"); val != "" {
		c.Standalone.NATSDataDir = val
	}
}

// ResolvePaths resolves relative paths using the given base directory.
// No paths to resolve in deployment config.
func (c *DeploymentConfig) ResolvePaths(_ string) { _ = c }

// Validate returns an error if the configuration is invalid.
func (c *DeploymentConfig) Validate() error {

	// Validate Deployment Mode
	mode := c.Mode
	if mode != "" && mode != "standalone" && mode != "distributed" {
		return fmt.Errorf("deployment.mode must be 'standalone' or 'distributed', got '%s'", mode)
	}

	return nil
}
