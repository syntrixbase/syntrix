package config

import (
	"errors"
	"path/filepath"
	"time"

	services "github.com/syntrixbase/syntrix/internal/services/config"
)

type Config struct {
	AuthN AuthNConfig `yaml:"authn"`
	AuthZ AuthZConfig `yaml:"authz"`
}

type AuthNConfig struct {
	AccessTokenTTL  time.Duration `yaml:"access_token_ttl"`
	RefreshTokenTTL time.Duration `yaml:"refresh_token_ttl"`
	AuthCodeTTL     time.Duration `yaml:"auth_code_ttl"`
	PrivateKeyFile  string        `yaml:"private_key_file"`
}

type AuthZConfig struct {
	RulesFile string `yaml:"rules_file"`
}

func DefaultConfig() Config {
	return Config{
		AuthN: AuthNConfig{
			AccessTokenTTL:  15 * time.Minute,
			RefreshTokenTTL: 7 * 24 * time.Hour,
			AuthCodeTTL:     2 * time.Minute,
			PrivateKeyFile:  "keys/auth_private.pem",
		},
		AuthZ: AuthZConfig{
			RulesFile: "security.yaml",
		},
	}
}

// ApplyDefaults fills in zero values with defaults.
func (c *Config) ApplyDefaults() {
	defaults := DefaultConfig()
	if c.AuthN.AccessTokenTTL == 0 {
		c.AuthN.AccessTokenTTL = defaults.AuthN.AccessTokenTTL
	}
	if c.AuthN.RefreshTokenTTL == 0 {
		c.AuthN.RefreshTokenTTL = defaults.AuthN.RefreshTokenTTL
	}
	if c.AuthN.AuthCodeTTL == 0 {
		c.AuthN.AuthCodeTTL = defaults.AuthN.AuthCodeTTL
	}
	if c.AuthN.PrivateKeyFile == "" {
		c.AuthN.PrivateKeyFile = defaults.AuthN.PrivateKeyFile
	}
	if c.AuthZ.RulesFile == "" {
		c.AuthZ.RulesFile = defaults.AuthZ.RulesFile
	}
}

// ApplyEnvOverrides applies environment variable overrides.
// No env vars for identity config currently.
func (c *Config) ApplyEnvOverrides() { _ = c }

// ResolvePaths resolves relative paths using the given base directory.
func (c *Config) ResolvePaths(baseDir string) {
	if c.AuthZ.RulesFile != "" && !filepath.IsAbs(c.AuthZ.RulesFile) {
		c.AuthZ.RulesFile = filepath.Join(baseDir, c.AuthZ.RulesFile)
	}
	if c.AuthN.PrivateKeyFile != "" && !filepath.IsAbs(c.AuthN.PrivateKeyFile) {
		c.AuthN.PrivateKeyFile = filepath.Join(baseDir, c.AuthN.PrivateKeyFile)
	}
}

// Validate returns an error if the configuration is invalid.
func (c *Config) Validate(_ services.DeploymentMode) error {
	if c.AuthZ.RulesFile == "" {
		return errors.New("identity.authz.rules_file is required")
	}
	return nil
}
