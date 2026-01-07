package config

import "time"

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
