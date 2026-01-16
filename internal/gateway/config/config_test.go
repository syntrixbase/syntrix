package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultGatewayConfig(t *testing.T) {
	cfg := DefaultGatewayConfig()

	assert.Equal(t, "localhost:9000", cfg.QueryServiceURL)
	assert.Equal(t, "localhost:9000", cfg.StreamerServiceURL)
	assert.Equal(t, []string{"http://localhost:8080", "http://localhost:3000", "http://localhost:5173"}, cfg.Realtime.AllowedOrigins)
	assert.True(t, cfg.Realtime.AllowDevOrigin)
}

func TestGatewayConfig_StructFields(t *testing.T) {
	cfg := GatewayConfig{
		QueryServiceURL: "http://custom:9090",
		Realtime: RealtimeConfig{
			AllowedOrigins: []string{"https://example.com"},
			AllowDevOrigin: false,
		},
	}

	assert.Equal(t, "http://custom:9090", cfg.QueryServiceURL)
	assert.Equal(t, []string{"https://example.com"}, cfg.Realtime.AllowedOrigins)
	assert.False(t, cfg.Realtime.AllowDevOrigin)
}

func TestGatewayConfig_Validate(t *testing.T) {
	cfg := DefaultGatewayConfig()
	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestGatewayConfig_ApplyDefaults(t *testing.T) {
	cfg := &GatewayConfig{}
	cfg.ApplyDefaults()

	assert.Equal(t, "localhost:9000", cfg.QueryServiceURL)
	assert.Equal(t, "localhost:9000", cfg.StreamerServiceURL)
	assert.Len(t, cfg.Realtime.AllowedOrigins, 3)
}

func TestGatewayConfig_ApplyEnvOverrides(t *testing.T) {
	os.Setenv("GATEWAY_QUERY_SERVICE_URL", "http://env:9090")
	defer os.Unsetenv("GATEWAY_QUERY_SERVICE_URL")

	cfg := DefaultGatewayConfig()
	cfg.ApplyEnvOverrides()

	assert.Equal(t, "http://env:9090", cfg.QueryServiceURL)
}

func TestGatewayConfig_ResolvePaths(t *testing.T) {
	cfg := DefaultGatewayConfig()
	cfg.ResolvePaths("config")
	// No paths to resolve, just verify no panic
}

func TestGatewayConfig_ApplyDefaults_CustomValuesPreserved(t *testing.T) {
	cfg := &GatewayConfig{
		QueryServiceURL:    "custom:9001",
		StreamerServiceURL: "custom:9002",
		Realtime: RealtimeConfig{
			AllowedOrigins: []string{"https://prod.example.com"},
			AllowDevOrigin: false,
		},
	}
	cfg.ApplyDefaults()

	assert.Equal(t, "custom:9001", cfg.QueryServiceURL)
	assert.Equal(t, "custom:9002", cfg.StreamerServiceURL)
	assert.Equal(t, []string{"https://prod.example.com"}, cfg.Realtime.AllowedOrigins)
	assert.False(t, cfg.Realtime.AllowDevOrigin)
}

func TestGatewayConfig_ApplyDefaults_PartialConfig(t *testing.T) {
	cfg := &GatewayConfig{
		QueryServiceURL: "partial:9001",
		// StreamerServiceURL empty, should get default
		// Realtime.AllowedOrigins empty, should get defaults
	}
	cfg.ApplyDefaults()

	assert.Equal(t, "partial:9001", cfg.QueryServiceURL)
	assert.Equal(t, "localhost:9000", cfg.StreamerServiceURL)
	assert.Len(t, cfg.Realtime.AllowedOrigins, 3)
}

func TestGatewayConfig_ApplyEnvOverrides_WithTSetenv(t *testing.T) {
	t.Setenv("GATEWAY_QUERY_SERVICE_URL", "http://testenv:9999")

	cfg := DefaultGatewayConfig()
	cfg.ApplyEnvOverrides()

	assert.Equal(t, "http://testenv:9999", cfg.QueryServiceURL)
}

func TestGatewayConfig_ApplyEnvOverrides_NoEnvVar(t *testing.T) {
	cfg := DefaultGatewayConfig()
	originalURL := cfg.QueryServiceURL

	cfg.ApplyEnvOverrides()

	assert.Equal(t, originalURL, cfg.QueryServiceURL)
}

func TestGatewayConfig_Validate_EmptyConfig(t *testing.T) {
	cfg := GatewayConfig{}
	err := cfg.Validate()
	assert.NoError(t, err)
}
