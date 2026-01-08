package config

type GatewayConfig struct {
	QueryServiceURL    string         `yaml:"query_service_url"`
	PullerServiceURL   string         `yaml:"puller_service_url"`
	StreamerServiceURL string         `yaml:"streamer_service_url"`
	Realtime           RealtimeConfig `yaml:"realtime"`
}

type RealtimeConfig struct {
	AllowedOrigins []string `yaml:"allowed_origins"`
	AllowDevOrigin bool     `yaml:"allow_dev_origin"`
	EnableAuth     bool     `yaml:"enable_auth"`
}

func DefaultGatewayConfig() GatewayConfig {
	return GatewayConfig{
		QueryServiceURL:    "localhost:50051",
		PullerServiceURL:   "localhost:50051",
		StreamerServiceURL: "localhost:50051",
		Realtime: RealtimeConfig{
			AllowedOrigins: []string{"http://localhost:8080", "http://localhost:3000", "http://localhost:5173"},
			AllowDevOrigin: true,
			EnableAuth:     true,
		},
	}
}
