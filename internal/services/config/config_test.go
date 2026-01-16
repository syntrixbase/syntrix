package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultDeploymentConfig(t *testing.T) {
	cfg := DefaultDeploymentConfig()

	assert.Equal(t, "distributed", cfg.Mode)
	assert.True(t, cfg.Standalone.EmbeddedNATS)
	assert.Equal(t, "data/nats", cfg.Standalone.NATSDataDir)
}

func TestDeploymentConfig_StructFields(t *testing.T) {
	cfg := DeploymentConfig{
		Mode: "standalone",
		Standalone: StandaloneConfig{
			EmbeddedNATS: false,
			NATSDataDir:  "custom/nats",
		},
	}

	assert.Equal(t, "standalone", cfg.Mode)
	assert.False(t, cfg.Standalone.EmbeddedNATS)
	assert.Equal(t, "custom/nats", cfg.Standalone.NATSDataDir)
}

func TestDeploymentConfig_ApplyDefaults(t *testing.T) {
	cfg := &DeploymentConfig{}
	cfg.ApplyDefaults()

	assert.Equal(t, "distributed", cfg.Mode)
	assert.Equal(t, "data/nats", cfg.Standalone.NATSDataDir)
}

func TestDeploymentConfig_ApplyEnvOverrides(t *testing.T) {
	os.Setenv("SYNTRIX_DEPLOYMENT_MODE", "standalone")
	os.Setenv("SYNTRIX_EMBEDDED_NATS", "true")
	os.Setenv("SYNTRIX_NATS_DATA_DIR", "/custom/path")
	defer func() {
		os.Unsetenv("SYNTRIX_DEPLOYMENT_MODE")
		os.Unsetenv("SYNTRIX_EMBEDDED_NATS")
		os.Unsetenv("SYNTRIX_NATS_DATA_DIR")
	}()

	cfg := DefaultDeploymentConfig()
	cfg.ApplyEnvOverrides()

	assert.Equal(t, "standalone", cfg.Mode)
	assert.True(t, cfg.Standalone.EmbeddedNATS)
	assert.Equal(t, "/custom/path", cfg.Standalone.NATSDataDir)
}

func TestDeploymentConfig_ApplyEnvOverrides_EmbeddedNATSFalse(t *testing.T) {
	os.Setenv("SYNTRIX_EMBEDDED_NATS", "false")
	defer os.Unsetenv("SYNTRIX_EMBEDDED_NATS")

	cfg := DefaultDeploymentConfig()
	cfg.ApplyEnvOverrides()

	assert.False(t, cfg.Standalone.EmbeddedNATS)
}

func TestDeploymentConfig_ResolvePaths(t *testing.T) {
	cfg := DefaultDeploymentConfig()
	cfg.ResolvePaths("config")
	// No paths to resolve, just verify no panic
}

func TestDeploymentConfig_Validate(t *testing.T) {
	cfg := DefaultDeploymentConfig()
	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestDeploymentConfig_ApplyDefaults_CustomValuesPreserved(t *testing.T) {
	cfg := &DeploymentConfig{
		Mode: "standalone",
		Standalone: StandaloneConfig{
			EmbeddedNATS: false,
			NATSDataDir:  "/custom/nats",
		},
	}
	cfg.ApplyDefaults()

	assert.Equal(t, "standalone", cfg.Mode)
	assert.False(t, cfg.Standalone.EmbeddedNATS)
	assert.Equal(t, "/custom/nats", cfg.Standalone.NATSDataDir)
}

func TestDeploymentConfig_ApplyDefaults_PartialConfig(t *testing.T) {
	cfg := &DeploymentConfig{
		Mode: "custom_mode",
		// NATSDataDir empty, should get default
	}
	cfg.ApplyDefaults()

	assert.Equal(t, "custom_mode", cfg.Mode)
	assert.Equal(t, "data/nats", cfg.Standalone.NATSDataDir)
}

func TestDeploymentConfig_ApplyEnvOverrides_WithTSetenv(t *testing.T) {
	t.Setenv("SYNTRIX_DEPLOYMENT_MODE", "production")
	t.Setenv("SYNTRIX_EMBEDDED_NATS", "1")
	t.Setenv("SYNTRIX_NATS_DATA_DIR", "/prod/nats")

	cfg := DefaultDeploymentConfig()
	cfg.ApplyEnvOverrides()

	assert.Equal(t, "production", cfg.Mode)
	assert.True(t, cfg.Standalone.EmbeddedNATS)
	assert.Equal(t, "/prod/nats", cfg.Standalone.NATSDataDir)
}

func TestDeploymentConfig_ApplyEnvOverrides_NoEnvVars(t *testing.T) {
	cfg := DefaultDeploymentConfig()
	originalMode := cfg.Mode

	cfg.ApplyEnvOverrides()

	assert.Equal(t, originalMode, cfg.Mode)
}

func TestDeploymentConfig_Validate_EmptyConfig(t *testing.T) {
	cfg := DeploymentConfig{}
	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestDeploymentConfig_Validate_InvalidMode(t *testing.T) {
	cfg := DeploymentConfig{Mode: "invalid_mode"}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "deployment.mode must be 'standalone' or 'distributed'")
}

func TestDeploymentConfig_Validate_ValidModes(t *testing.T) {
	tests := []struct {
		name string
		mode string
	}{
		{"empty mode", ""},
		{"standalone mode", "standalone"},
		{"distributed mode", "distributed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DeploymentConfig{Mode: tt.mode}
			err := cfg.Validate()
			assert.NoError(t, err)
		})
	}
}
