package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	identity "github.com/syntrixbase/syntrix/internal/core/identity/config"
	storage "github.com/syntrixbase/syntrix/internal/core/storage/config"
	api "github.com/syntrixbase/syntrix/internal/gateway/config"
	indexer "github.com/syntrixbase/syntrix/internal/indexer/config"
	puller "github.com/syntrixbase/syntrix/internal/puller/config"
	query "github.com/syntrixbase/syntrix/internal/query/config"
	"github.com/syntrixbase/syntrix/internal/server"
	services "github.com/syntrixbase/syntrix/internal/services/config"
	"github.com/syntrixbase/syntrix/internal/streamer"
	trigger "github.com/syntrixbase/syntrix/internal/trigger/config"
	"gopkg.in/yaml.v3"
)

// Config holds the application configuration
type Config struct {
	Deployment services.DeploymentConfig `yaml:"deployment"`
	Server     server.Config             `yaml:"server"`

	// Services
	Query    query.Config      `yaml:"query"`
	Indexer  indexer.Config    `yaml:"indexer"`
	Gateway  api.GatewayConfig `yaml:"gateway"`
	Trigger  trigger.Config    `yaml:"trigger"`
	Puller   puller.Config     `yaml:"puller"`
	Streamer streamer.Config   `yaml:"streamer"`

	// Components
	Storage  storage.Config  `yaml:"storage"`
	Identity identity.Config `yaml:"identity"`
}

// LoadConfig loads configuration from files and environment variables
// Order: defaults -> config.yml -> config.local.yml -> env vars
func LoadConfig() *Config {
	// 1. Defaults
	cfg := &Config{
		Storage: storage.Config{
			Backends: map[string]storage.BackendConfig{
				"default_mongo": {
					Type: "mongo",
					Mongo: storage.MongoConfig{
						URI:          "mongodb://localhost:27017",
						DatabaseName: "syntrix",
					},
				},
			},
			Topology: storage.TopologyConfig{
				Document: storage.DocumentTopology{
					BaseTopology: storage.BaseTopology{
						Strategy: "single",
						Primary:  "default_mongo",
					},
					DataCollection:      "documents",
					SysCollection:       "sys",
					SoftDeleteRetention: 5 * time.Minute,
				},
				User: storage.CollectionTopology{
					BaseTopology: storage.BaseTopology{
						Strategy: "single",
						Primary:  "default_mongo",
					},
					Collection: "users",
				},
				Revocation: storage.CollectionTopology{
					BaseTopology: storage.BaseTopology{
						Strategy: "single",
						Primary:  "default_mongo",
					},
					Collection: "revocations",
				},
			},
			Databases: map[string]storage.DatabaseConfig{
				"default": {
					Backend: "default_mongo",
				},
			},
		},
		Identity:   identity.DefaultConfig(),
		Server:     server.DefaultConfig(),
		Query:      query.DefaultConfig(),
		Gateway:    api.DefaultGatewayConfig(),
		Trigger:    trigger.DefaultConfig(),
		Deployment: services.DefaultDeploymentConfig(),
		Puller:     puller.DefaultConfig(),
		Streamer:   streamer.DefaultConfig(),
		Indexer:    indexer.DefaultConfig(),
	}

	// 2. Load config.yml
	loadFile("config/config.yml", cfg)

	// 3. Load config.local.yml
	loadFile("config/config.local.yml", cfg)

	// 4. Override with Env Vars
	if val := os.Getenv("GATEWAY_QUERY_SERVICE_URL"); val != "" {
		cfg.Gateway.QueryServiceURL = val
	}

	if val := os.Getenv("MONGO_URI"); val != "" {
		if backend, ok := cfg.Storage.Backends["default_mongo"]; ok {
			backend.Mongo.URI = val
			cfg.Storage.Backends["default_mongo"] = backend
		}
	}
	if val := os.Getenv("DB_NAME"); val != "" {
		if backend, ok := cfg.Storage.Backends["default_mongo"]; ok {
			backend.Mongo.DatabaseName = val
			cfg.Storage.Backends["default_mongo"] = backend
		}
	}

	if val := os.Getenv("TRIGGER_NATS_URL"); val != "" {
		cfg.Trigger.NatsURL = val
	}
	if val := os.Getenv("TRIGGER_RULES_FILE"); val != "" {
		cfg.Trigger.RulesFile = val
	}

	// Deployment configuration
	if val := os.Getenv("SYNTRIX_DEPLOYMENT_MODE"); val != "" {
		cfg.Deployment.Mode = val
	}
	if val := os.Getenv("SYNTRIX_EMBEDDED_NATS"); val != "" {
		cfg.Deployment.Standalone.EmbeddedNATS = val == "true" || val == "1"
	}
	if val := os.Getenv("SYNTRIX_NATS_DATA_DIR"); val != "" {
		cfg.Deployment.Standalone.NATSDataDir = val
	}

	// 5. Resolve paths relative to config directory
	cfg.resolvePaths()

	// 6. Validate configuration
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Configuration error: %v", err)
	}

	return cfg
}

func (c *Config) Validate() error {
	// Validate Storage Databases
	if _, ok := c.Storage.Databases["default"]; !ok {
		return fmt.Errorf("storage.databases.default is required")
	}
	for tID, tCfg := range c.Storage.Databases {
		if _, ok := c.Storage.Backends[tCfg.Backend]; !ok {
			return fmt.Errorf("database '%s' references unknown backend '%s'", tID, tCfg.Backend)
		}
	}

	// Validate Deployment Mode
	mode := c.Deployment.Mode
	if mode != "" && mode != "standalone" && mode != "distributed" {
		return fmt.Errorf("deployment.mode must be 'standalone' or 'distributed', got '%s'", mode)
	}

	return nil
}

// IsStandaloneMode returns true if the deployment is configured for standalone mode
func (c *Config) IsStandaloneMode() bool {
	return c.Deployment.Mode == "standalone"
}

func (c *Config) resolvePaths() {
	configDir := "config"
	c.Identity.AuthZ.RulesFile = resolvePath(configDir, c.Identity.AuthZ.RulesFile)
	c.Identity.AuthN.PrivateKeyFile = resolvePath(configDir, c.Identity.AuthN.PrivateKeyFile)
	c.Trigger.RulesFile = resolvePath(configDir, c.Trigger.RulesFile)
	c.Indexer.TemplatePath = resolvePath(configDir, c.Indexer.TemplatePath)
}

func resolvePath(base, path string) string {
	if path == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(base, path)
}

func loadFile(filename string, cfg *Config) {
	data, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return // File doesn't exist, skip
		}
		log.Printf("Warning: Error reading %s: %v", filename, err)
		return
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		log.Printf("Warning: Error parsing %s: %v", filename, err)
	}
}
