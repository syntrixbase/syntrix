package config

import (
	"fmt"
	"os"
)

// DeploymentMode represents the deployment mode of the service.
type DeploymentMode string

const (
	// ModeDistributed is the default mode where services communicate via gRPC/NATS.
	ModeDistributed DeploymentMode = "distributed"
	// ModeStandalone runs all services in a single process with direct function calls.
	ModeStandalone DeploymentMode = "standalone"
)

// IsStandalone returns true if this is standalone mode.
func (m DeploymentMode) IsStandalone() bool {
	return m == ModeStandalone
}

// IsDistributed returns true if this is distributed mode.
// Empty string defaults to distributed.
func (m DeploymentMode) IsDistributed() bool {
	return m == "" || m == ModeDistributed
}

// DeploymentConfig holds deployment mode settings
type DeploymentConfig struct {
	Mode       DeploymentMode   `yaml:"mode"` // "standalone" or "distributed" (default)
	Standalone StandaloneConfig `yaml:"standalone"`
}

// StandaloneConfig holds standalone-specific settings
type StandaloneConfig struct {
	EmbeddedNATS bool   `yaml:"embedded_nats"` // Use embedded NATS server
	NATSDataDir  string `yaml:"nats_data_dir"` // Data directory for embedded NATS
}

func DefaultDeploymentConfig() DeploymentConfig {
	return DeploymentConfig{
		Mode: ModeDistributed, // Default to distributed mode
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
		c.Mode = DeploymentMode(val)
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
	if c.Mode != "" && c.Mode != ModeStandalone && c.Mode != ModeDistributed {
		return fmt.Errorf("deployment.mode must be 'standalone' or 'distributed', got '%s'", c.Mode)
	}

	return nil
}
