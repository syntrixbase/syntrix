package config

import (
	"fmt"
	"path/filepath"

	services "github.com/syntrixbase/syntrix/internal/services/config"
)

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level    string         `yaml:"level"`  // debug, info, warn, error
	Format   string         `yaml:"format"` // text, json
	Dir      string         `yaml:"dir"`    // log directory path
	Rotation RotationConfig `yaml:"rotation"`
	Console  ConsoleConfig  `yaml:"console"`
	File     FileConfig     `yaml:"file"`
}

// RotationConfig holds log rotation settings
type RotationConfig struct {
	MaxSize    int  `yaml:"max_size"`    // MB
	MaxBackups int  `yaml:"max_backups"` // number of files
	MaxAge     int  `yaml:"max_age"`     // days
	Compress   bool `yaml:"compress"`    // gzip old files
}

// ConsoleConfig holds console output configuration
type ConsoleConfig struct {
	Enabled bool   `yaml:"enabled"`
	Level   string `yaml:"level"`  // optional override
	Format  string `yaml:"format"` // text or json
}

// FileConfig holds file output configuration
type FileConfig struct {
	Enabled bool   `yaml:"enabled"`
	Level   string `yaml:"level"`  // optional override
	Format  string `yaml:"format"` // text or json
}

// DefaultLoggingConfig returns default logging configuration
func DefaultLoggingConfig() LoggingConfig {
	return LoggingConfig{
		Level:  "info",
		Format: "text",
		Dir:    "logs",
		Rotation: RotationConfig{
			MaxSize:    100,
			MaxBackups: 10,
			MaxAge:     30,
			Compress:   true,
		},
		Console: ConsoleConfig{
			Enabled: true,
			Level:   "info",
			Format:  "text",
		},
		File: FileConfig{
			Enabled: true,
			Level:   "info",
			Format:  "text",
		},
	}
}

// ApplyDefaults fills in missing values with defaults
func (c *LoggingConfig) ApplyDefaults() {
	if c.Level == "" {
		c.Level = "info"
	}
	if c.Format == "" {
		c.Format = "text"
	}
	if c.Dir == "" {
		c.Dir = "logs"
	}

	// Rotation defaults
	if c.Rotation.MaxSize == 0 {
		c.Rotation.MaxSize = 100
	}
	if c.Rotation.MaxBackups == 0 {
		c.Rotation.MaxBackups = 10
	}
	if c.Rotation.MaxAge == 0 {
		c.Rotation.MaxAge = 30
	}
	// Note: Compress defaults to false (zero value for bool).
	// DefaultLoggingConfig() returns true as the production default, but
	// ApplyDefaults() cannot distinguish between explicit false and zero value,
	// so users must explicitly set Compress in their config if they want compression.

	// Console defaults - check if console config is completely empty
	if c.Console.Level == "" && c.Console.Format == "" && !c.Console.Enabled {
		c.Console.Enabled = true
	}
	if c.Console.Level == "" {
		c.Console.Level = c.Level
	}
	if c.Console.Format == "" {
		c.Console.Format = c.Format
	}

	// File defaults - check if file config is completely empty
	if c.File.Level == "" && c.File.Format == "" && !c.File.Enabled {
		c.File.Enabled = true
	}
	if c.File.Level == "" {
		c.File.Level = c.Level
	}
	if c.File.Format == "" {
		c.File.Format = c.Format
	}
}

// ApplyEnvOverrides applies environment variable overrides
func (c *LoggingConfig) ApplyEnvOverrides() {
	// Future: Add environment variable support
}

// ResolvePaths resolves relative paths based on config directory
func (c *LoggingConfig) ResolvePaths(configDir string) {
	if c.Dir != "" && !filepath.IsAbs(c.Dir) {
		// Resolve relative paths
		// If path starts with "..", resolve from configDir directly (allows navigating up from config location)
		// Otherwise, resolve from parent of configDir (so logs/ ends up next to configs/, not inside it)
		var resolvedPath string
		if len(c.Dir) >= 2 && c.Dir[0:2] == ".." {
			// Path starts with "..", resolve from configDir
			resolvedPath = filepath.Join(configDir, c.Dir)
		} else {
			// Simple relative path, resolve from parent
			baseDir := filepath.Dir(configDir)
			resolvedPath = filepath.Join(baseDir, c.Dir)
		}
		c.Dir = filepath.Clean(resolvedPath)
	}
}

// Validate validates the configuration
func (c *LoggingConfig) Validate(mode services.DeploymentMode) error {
	validLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
	}

	if !validLevels[c.Level] {
		return fmt.Errorf("invalid log level: %s (must be debug, info, warn, or error)", c.Level)
	}

	validFormats := map[string]bool{
		"text": true,
		"json": true,
	}

	if !validFormats[c.Format] {
		return fmt.Errorf("invalid log format: %s (must be text or json)", c.Format)
	}

	if c.Dir == "" {
		return fmt.Errorf("log directory cannot be empty")
	}

	// Validate console config if enabled
	if c.Console.Enabled {
		if c.Console.Level != "" && !validLevels[c.Console.Level] {
			return fmt.Errorf("invalid console log level: %s", c.Console.Level)
		}
		if c.Console.Format != "" && !validFormats[c.Console.Format] {
			return fmt.Errorf("invalid console log format: %s", c.Console.Format)
		}
	}

	// Validate file config if enabled
	if c.File.Enabled {
		if c.File.Level != "" && !validLevels[c.File.Level] {
			return fmt.Errorf("invalid file log level: %s", c.File.Level)
		}
		if c.File.Format != "" && !validFormats[c.File.Format] {
			return fmt.Errorf("invalid file log format: %s", c.File.Format)
		}
	}

	return nil
}
