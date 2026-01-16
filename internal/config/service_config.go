package config

// ServiceConfig defines the standard configuration lifecycle methods.
// Each service config should implement this interface to ensure consistent
// configuration handling across the application.
type ServiceConfig interface {
	// ApplyDefaults fills zero values with sensible defaults
	ApplyDefaults()

	// ApplyEnvOverrides applies environment variable overrides
	ApplyEnvOverrides()

	// ResolvePaths resolves relative paths using the given base directory
	ResolvePaths(baseDir string)

	// Validate returns an error if the configuration is invalid
	Validate() error
}

// ApplyServiceConfigs applies the configuration lifecycle to all service configs.
// It calls ApplyDefaults, ApplyEnvOverrides, ResolvePaths, and Validate in order.
func ApplyServiceConfigs(baseDir string, configs ...ServiceConfig) error {
	for _, cfg := range configs {
		cfg.ApplyDefaults()
		cfg.ApplyEnvOverrides()
		cfg.ResolvePaths(baseDir)
		if err := cfg.Validate(); err != nil {
			return err
		}
	}
	return nil
}
