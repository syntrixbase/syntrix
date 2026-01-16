package server

import "time"

// Config holds the configuration for the unified server module.
type Config struct {
	Host string `yaml:"host"`

	// HTTP Configuration
	HTTPPort         int           `yaml:"http_port"`
	HTTPReadTimeout  time.Duration `yaml:"http_read_timeout"`
	HTTPWriteTimeout time.Duration `yaml:"http_write_timeout"`
	HTTPIdleTimeout  time.Duration `yaml:"http_idle_timeout"`
	EnableCORS       bool          `yaml:"enable_cors"`

	// gRPC Configuration
	GRPCPort          int  `yaml:"grpc_port"`
	GRPCMaxConcurrent uint `yaml:"grpc_max_concurrent"`
	EnableReflection  bool `yaml:"enable_reflection"`

	// Lifecycle Configuration
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout"`
}

// DefaultConfig returns safe defaults for development.
func DefaultConfig() Config {
	return Config{
		Host:             "localhost",
		HTTPPort:         8080,
		HTTPReadTimeout:  10 * time.Second,
		HTTPWriteTimeout: 10 * time.Second,
		HTTPIdleTimeout:  60 * time.Second,
		GRPCPort:         9000,
		ShutdownTimeout:  10 * time.Second,
	}
}

// ApplyDefaults fills in zero values with defaults.
func (c *Config) ApplyDefaults() {
	defaults := DefaultConfig()
	if c.Host == "" {
		c.Host = defaults.Host
	}
	if c.HTTPPort == 0 {
		c.HTTPPort = defaults.HTTPPort
	}
	if c.HTTPReadTimeout == 0 {
		c.HTTPReadTimeout = defaults.HTTPReadTimeout
	}
	if c.HTTPWriteTimeout == 0 {
		c.HTTPWriteTimeout = defaults.HTTPWriteTimeout
	}
	if c.HTTPIdleTimeout == 0 {
		c.HTTPIdleTimeout = defaults.HTTPIdleTimeout
	}
	if c.GRPCPort == 0 {
		c.GRPCPort = defaults.GRPCPort
	}
	if c.ShutdownTimeout == 0 {
		c.ShutdownTimeout = defaults.ShutdownTimeout
	}
}

// ApplyEnvOverrides applies environment variable overrides.
// No env vars for server config currently.
func (c *Config) ApplyEnvOverrides() { _ = c }

// ResolvePaths resolves relative paths using the given base directory.
// No paths to resolve in server config.
func (c *Config) ResolvePaths(_ string) { _ = c }

// Validate returns an error if the configuration is invalid.
func (c *Config) Validate() error {
	return nil
}
