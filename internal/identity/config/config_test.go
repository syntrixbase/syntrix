package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	// Verify AuthN defaults
	assert.Equal(t, 15*time.Minute, cfg.AuthN.AccessTokenTTL)
	assert.Equal(t, 7*24*time.Hour, cfg.AuthN.RefreshTokenTTL)
	assert.Equal(t, 2*time.Minute, cfg.AuthN.AuthCodeTTL)
	assert.Equal(t, "keys/auth_private.pem", cfg.AuthN.PrivateKeyFile)

	// Verify AuthZ defaults
	assert.Equal(t, "security.yaml", cfg.AuthZ.RulesFile)
}

func TestConfig_StructFields(t *testing.T) {
	cfg := Config{
		AuthN: AuthNConfig{
			AccessTokenTTL:  30 * time.Minute,
			RefreshTokenTTL: 24 * time.Hour,
			AuthCodeTTL:     5 * time.Minute,
			PrivateKeyFile:  "custom/key.pem",
		},
		AuthZ: AuthZConfig{
			RulesFile: "custom_rules.yaml",
		},
	}

	assert.Equal(t, 30*time.Minute, cfg.AuthN.AccessTokenTTL)
	assert.Equal(t, 24*time.Hour, cfg.AuthN.RefreshTokenTTL)
	assert.Equal(t, 5*time.Minute, cfg.AuthN.AuthCodeTTL)
	assert.Equal(t, "custom/key.pem", cfg.AuthN.PrivateKeyFile)
	assert.Equal(t, "custom_rules.yaml", cfg.AuthZ.RulesFile)
}
