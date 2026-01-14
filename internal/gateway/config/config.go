package config

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

func (g *GatewayConfig) Validate() error {
	// Add validation logic if needed
	return nil
}
