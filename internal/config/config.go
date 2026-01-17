package config

import (
	"log"
	"os"

	identity "github.com/syntrixbase/syntrix/internal/core/identity/config"
	storage "github.com/syntrixbase/syntrix/internal/core/storage/config"
	api "github.com/syntrixbase/syntrix/internal/gateway/config"
	indexer "github.com/syntrixbase/syntrix/internal/indexer/config"
	puller "github.com/syntrixbase/syntrix/internal/puller/config"
	query "github.com/syntrixbase/syntrix/internal/query/config"
	server "github.com/syntrixbase/syntrix/internal/server"
	services "github.com/syntrixbase/syntrix/internal/services/config"
	streamer "github.com/syntrixbase/syntrix/internal/streamer"
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
// Order: defaults -> config.yml -> config.local.yml -> ApplyEnvOverrides -> ResolvePaths -> Validate
func LoadConfig() *Config {
	// 1. Start with default values (so YAML can override them, including bool fields)
	cfg := &Config{
		Storage:    storage.DefaultConfig(),
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

	// 2. Load config.yml (overrides defaults)
	loadFile("config/config.yml", cfg)

	// 3. Load config.local.yml (overrides config.yml)
	loadFile("config/config.local.yml", cfg)

	// 4. Apply configuration lifecycle: ApplyDefaults fills gaps, ApplyEnvOverrides, ResolvePaths, Validate
	// Note: We need to process Deployment first to get the mode, then pass it to other configs
	configDir := "config"
	cfg.Deployment.ApplyDefaults()
	cfg.Deployment.ApplyEnvOverrides()
	cfg.Deployment.ResolvePaths(configDir)
	if err := cfg.Deployment.Validate(cfg.Deployment.Mode); err != nil {
		log.Fatalf("Configuration error: %v", err)
	}

	// Apply other configs with the now-known deployment mode
	if err := ApplyServiceConfigs(configDir, cfg.Deployment.Mode,
		&cfg.Server,
		&cfg.Query,
		&cfg.Indexer,
		&cfg.Gateway,
		&cfg.Trigger,
		&cfg.Puller,
		&cfg.Streamer,
		&cfg.Storage,
		&cfg.Identity,
	); err != nil {
		log.Fatalf("Configuration error: %v", err)
	}

	return cfg
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
