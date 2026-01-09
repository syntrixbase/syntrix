package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultGatewayConfig(t *testing.T) {
	cfg := DefaultGatewayConfig()

	assert.Equal(t, "localhost:50051", cfg.QueryServiceURL)
	assert.Equal(t, "localhost:50051", cfg.PullerServiceURL)
	assert.Equal(t, "localhost:50051", cfg.StreamerServiceURL)
	assert.Equal(t, []string{"http://localhost:8080", "http://localhost:3000", "http://localhost:5173"}, cfg.Realtime.AllowedOrigins)
	assert.True(t, cfg.Realtime.AllowDevOrigin)
	assert.True(t, cfg.Realtime.EnableAuth)
}

func TestGatewayConfig_StructFields(t *testing.T) {
	cfg := GatewayConfig{
		QueryServiceURL: "http://custom:9090",
		Realtime: RealtimeConfig{
			AllowedOrigins: []string{"https://example.com"},
			AllowDevOrigin: false,
			EnableAuth:     false,
		},
	}

	assert.Equal(t, "http://custom:9090", cfg.QueryServiceURL)
	assert.Equal(t, []string{"https://example.com"}, cfg.Realtime.AllowedOrigins)
	assert.False(t, cfg.Realtime.AllowDevOrigin)
	assert.False(t, cfg.Realtime.EnableAuth)
}

func TestGatewayConfig_Validate(t *testing.T) {
	cfg := DefaultGatewayConfig()
	err := cfg.Validate()
	assert.NoError(t, err)
}
