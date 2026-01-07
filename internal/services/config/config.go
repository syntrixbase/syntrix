package config

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
