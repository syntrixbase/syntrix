package config

import "os"

type GatewayConfig struct {
	QueryServiceURL    string         `yaml:"query_service_url"`
	StreamerServiceURL string         `yaml:"streamer_service_url"`
	Realtime           RealtimeConfig `yaml:"realtime"`
}

type RealtimeConfig struct {
	AllowedOrigins []string `yaml:"allowed_origins"`
	AllowDevOrigin bool     `yaml:"allow_dev_origin"`
}

func DefaultGatewayConfig() GatewayConfig {
	return GatewayConfig{
		QueryServiceURL:    "localhost:9000",
		StreamerServiceURL: "localhost:9000",
		Realtime: RealtimeConfig{
			AllowedOrigins: []string{"http://localhost:8080", "http://localhost:3000", "http://localhost:5173"},
			AllowDevOrigin: true,
		},
	}
}

// ApplyDefaults fills in zero values with defaults.
func (g *GatewayConfig) ApplyDefaults() {
	defaults := DefaultGatewayConfig()
	if g.QueryServiceURL == "" {
		g.QueryServiceURL = defaults.QueryServiceURL
	}
	if g.StreamerServiceURL == "" {
		g.StreamerServiceURL = defaults.StreamerServiceURL
	}
	if len(g.Realtime.AllowedOrigins) == 0 {
		g.Realtime.AllowedOrigins = defaults.Realtime.AllowedOrigins
	}
}

// ApplyEnvOverrides applies environment variable overrides.
func (g *GatewayConfig) ApplyEnvOverrides() {
	if val := os.Getenv("GATEWAY_QUERY_SERVICE_URL"); val != "" {
		g.QueryServiceURL = val
	}
}

// ResolvePaths resolves relative paths using the given base directory.
// No paths to resolve in gateway config.
func (g *GatewayConfig) ResolvePaths(_ string) { _ = g }

// Validate returns an error if the configuration is invalid.
func (g *GatewayConfig) Validate() error {
	// Add validation logic if needed
	return nil
}
