package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	identity "github.com/syntrixbase/syntrix/internal/core/identity/config"
	services_config "github.com/syntrixbase/syntrix/internal/services/config"
)

func TestLoadConfig_Defaults(t *testing.T) {
	// Ensure no env vars interfere
	os.Unsetenv("MONGO_URI")
	os.Unsetenv("DB_NAME")
	os.Unsetenv("GATEWAY_PORT")

	cfg := LoadConfig()

	assert.Equal(t, "mongodb://localhost:27017", cfg.Storage.Backends["default_mongo"].Mongo.URI)
	assert.Equal(t, "syntrix", cfg.Storage.Backends["default_mongo"].Mongo.DatabaseName)
}

func TestLoadConfig_EnvVars(t *testing.T) {
	os.Setenv("MONGO_URI", "mongodb://test:27017")
	os.Setenv("DB_NAME", "testdb")
	os.Setenv("GATEWAY_QUERY_SERVICE_URL", "http://api-env")
	os.Setenv("TRIGGER_NATS_URL", "nats://env:4222")
	os.Setenv("TRIGGER_RULES_PATH", "custom")
	defer func() {
		os.Unsetenv("MONGO_URI")
		os.Unsetenv("DB_NAME")
		os.Unsetenv("GATEWAY_QUERY_SERVICE_URL")
		os.Unsetenv("TRIGGER_NATS_URL")
		os.Unsetenv("TRIGGER_RULES_PATH")
	}()

	cfg := LoadConfig()

	assert.Equal(t, "mongodb://test:27017", cfg.Storage.Backends["default_mongo"].Mongo.URI)
	assert.Equal(t, "testdb", cfg.Storage.Backends["default_mongo"].Mongo.DatabaseName)
	assert.Equal(t, "http://api-env", cfg.Gateway.QueryServiceURL)
	assert.Equal(t, "nats://env:4222", cfg.Trigger.NatsURL)
	assert.True(t, strings.HasSuffix(cfg.Trigger.Evaluator.RulesPath, filepath.Join("config", "custom")))
}

func TestLoadConfig_LoadFileErrors(t *testing.T) {
	require.NoError(t, os.Mkdir("config", 0755))
	defer os.RemoveAll("config")

	// Create a directory where a file is expected to trigger read error path
	require.NoError(t, os.Mkdir("config/config.yml", 0755))

	// Malformed YAML to trigger parse error path
	require.NoError(t, os.WriteFile("config/config.local.yml", []byte("not: [valid"), 0644))

	cfg := LoadConfig()

	// Defaults should remain when files fail to load/parse
	assert.Equal(t, 8080, cfg.Server.HTTPPort)
	assert.Equal(t, "mongodb://localhost:27017", cfg.Storage.Backends["default_mongo"].Mongo.URI)
}

func TestLoadConfig_File(t *testing.T) {
	err := os.Mkdir("config", 0755)
	require.NoError(t, err)
	defer os.RemoveAll("config")

	// Create a temporary config.yml in the config directory
	configContent := []byte(`
storage:
  backends:
    default_mongo:
      mongo:
        uri: "mongodb://file:27017"
        database_name: "filedb"
gateway:
  port: 7070
`)
	err = os.WriteFile("config/config.yml", configContent, 0644)
	require.NoError(t, err)

	cfg := LoadConfig()

	assert.Equal(t, "mongodb://file:27017", cfg.Storage.Backends["default_mongo"].Mongo.URI)
	assert.Equal(t, "filedb", cfg.Storage.Backends["default_mongo"].Mongo.DatabaseName)
}

func TestServiceConfig_ResolvePaths(t *testing.T) {
	// Test that relative paths are resolved correctly
	// The ResolvePaths function is now part of each service config

	// Test with identity config
	identityCfg := identity.Config{
		AuthZ: identity.AuthZConfig{RulesPath: "security.yaml"},
		AuthN: identity.AuthNConfig{PrivateKeyFile: "keys/auth.pem"},
	}
	identityCfg.ResolvePaths("config")
	assert.Equal(t, filepath.Join("config", "security.yaml"), identityCfg.AuthZ.RulesPath)
	assert.Equal(t, filepath.Join("config", "keys/auth.pem"), identityCfg.AuthN.PrivateKeyFile)

	// Test with absolute path - should not be modified
	absPath := filepath.Join(t.TempDir(), "absolute", "path", "to", "file")
	identityCfg2 := identity.Config{
		AuthZ: identity.AuthZConfig{RulesPath: absPath},
	}
	identityCfg2.ResolvePaths("config")
	assert.Equal(t, absPath, identityCfg2.AuthZ.RulesPath)

	// Test with empty path - should remain empty
	identityCfg3 := identity.Config{
		AuthZ: identity.AuthZConfig{RulesPath: ""},
	}
	identityCfg3.ResolvePaths("config")
	assert.Equal(t, "", identityCfg3.AuthZ.RulesPath)
}

func TestDeploymentMode_IsStandalone_ViaConfig(t *testing.T) {
	cfg := &Config{Deployment: services_config.DeploymentConfig{Mode: services_config.ModeStandalone}}
	assert.True(t, cfg.Deployment.Mode.IsStandalone())

	cfg = &Config{Deployment: services_config.DeploymentConfig{Mode: services_config.ModeDistributed}}
	assert.False(t, cfg.Deployment.Mode.IsStandalone())

	cfg = &Config{Deployment: services_config.DeploymentConfig{Mode: ""}}
	assert.False(t, cfg.Deployment.Mode.IsStandalone())
}

// Note: Config.Validate() has been removed. Mode-dependent validation is now
// performed by individual service configs via their Validate(mode) method.
// Tests for distributed mode address requirements are in the respective
// service config test files (e.g., query/config/config_test.go,
// gateway/config/config_test.go, etc.)
