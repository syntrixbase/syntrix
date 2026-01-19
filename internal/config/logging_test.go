package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	services "github.com/syntrixbase/syntrix/internal/services/config"
	"gopkg.in/yaml.v3"
)

func TestDefaultLoggingConfig(t *testing.T) {
	cfg := DefaultLoggingConfig()

	assert.Equal(t, "info", cfg.Level)
	assert.Equal(t, "text", cfg.Format)
	assert.Equal(t, "logs", cfg.Dir)
	assert.Equal(t, 100, cfg.Rotation.MaxSize)
	assert.Equal(t, 10, cfg.Rotation.MaxBackups)
	assert.Equal(t, 30, cfg.Rotation.MaxAge)
	assert.True(t, cfg.Rotation.Compress)
	assert.True(t, cfg.Console.Enabled)
	assert.True(t, cfg.File.Enabled)
}

func TestLoggingConfigYAMLParsing(t *testing.T) {
	yamlData := `
level: "debug"
format: "json"
dir: "/var/log/syntrix"
rotation:
  max_size: 50
  max_backups: 5
  max_age: 14
  compress: false
console:
  enabled: false
  level: "warn"
  format: "text"
file:
  enabled: true
  level: "info"
  format: "json"
`

	var cfg LoggingConfig
	err := yaml.Unmarshal([]byte(yamlData), &cfg)

	assert.NoError(t, err)
	assert.Equal(t, "debug", cfg.Level)
	assert.Equal(t, "json", cfg.Format)
	assert.Equal(t, "/var/log/syntrix", cfg.Dir)
	assert.Equal(t, 50, cfg.Rotation.MaxSize)
	assert.False(t, cfg.Console.Enabled)
}

func TestLoggingConfigApplyDefaults(t *testing.T) {
	cfg := &LoggingConfig{}
	cfg.ApplyDefaults()

	assert.Equal(t, "info", cfg.Level)
	assert.Equal(t, "text", cfg.Format)
	assert.Equal(t, "logs", cfg.Dir)
	assert.Equal(t, 100, cfg.Rotation.MaxSize)
	assert.Equal(t, 10, cfg.Rotation.MaxBackups)
	assert.Equal(t, 30, cfg.Rotation.MaxAge)
	// Note: Compress defaults to false (zero value), users must explicitly set to true
	assert.False(t, cfg.Rotation.Compress)
	assert.True(t, cfg.Console.Enabled)
	assert.True(t, cfg.File.Enabled)
}

func TestLoggingConfigApplyDefaultsWithPartialConfig(t *testing.T) {
	cfg := &LoggingConfig{
		Level:  "debug",
		Format: "json",
		Console: ConsoleConfig{
			Enabled: true,
			Level:   "warn",
		},
	}
	cfg.ApplyDefaults()

	// Explicitly set values should be preserved
	assert.Equal(t, "debug", cfg.Level)
	assert.Equal(t, "json", cfg.Format)
	assert.Equal(t, "warn", cfg.Console.Level)

	// Missing values should be filled with defaults
	assert.Equal(t, "logs", cfg.Dir)
	assert.Equal(t, 100, cfg.Rotation.MaxSize)
	assert.Equal(t, "json", cfg.Console.Format) // inherits from cfg.Format
	assert.True(t, cfg.File.Enabled)
	assert.Equal(t, "debug", cfg.File.Level) // inherits from cfg.Level
	assert.Equal(t, "json", cfg.File.Format) // inherits from cfg.Format
}

func TestLoggingConfigResolvePaths(t *testing.T) {
	tests := []struct {
		name      string
		configDir string
		dir       string
		expected  string
	}{
		{
			name:      "relative path",
			configDir: "/app/configs",
			dir:       "logs",
			expected:  "/app/logs",
		},
		{
			name:      "absolute path unchanged",
			configDir: "/app/configs",
			dir:       "/var/log/syntrix",
			expected:  "/var/log/syntrix",
		},
		{
			name:      "relative with subdirs",
			configDir: "/app/configs",
			dir:       "../logs/app",
			expected:  "/app/logs/app",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &LoggingConfig{Dir: tt.dir}
			cfg.ResolvePaths(tt.configDir)
			assert.Equal(t, tt.expected, cfg.Dir)
		})
	}
}

func TestLoggingConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		cfg         LoggingConfig
		expectError bool
	}{
		{
			name: "valid config",
			cfg: LoggingConfig{
				Level:  "info",
				Format: "text",
				Dir:    "logs",
			},
			expectError: false,
		},
		{
			name: "invalid level",
			cfg: LoggingConfig{
				Level:  "invalid",
				Format: "text",
				Dir:    "logs",
			},
			expectError: true,
		},
		{
			name: "invalid format",
			cfg: LoggingConfig{
				Level:  "info",
				Format: "xml",
				Dir:    "logs",
			},
			expectError: true,
		},
		{
			name: "empty dir",
			cfg: LoggingConfig{
				Level:  "info",
				Format: "text",
				Dir:    "",
			},
			expectError: true,
		},
		{
			name: "invalid console level when enabled",
			cfg: LoggingConfig{
				Level:  "info",
				Format: "text",
				Dir:    "logs",
				Console: ConsoleConfig{
					Enabled: true,
					Level:   "invalid",
					Format:  "text",
				},
			},
			expectError: true,
		},
		{
			name: "invalid console format when enabled",
			cfg: LoggingConfig{
				Level:  "info",
				Format: "text",
				Dir:    "logs",
				Console: ConsoleConfig{
					Enabled: true,
					Level:   "info",
					Format:  "xml",
				},
			},
			expectError: true,
		},
		{
			name: "invalid file level when enabled",
			cfg: LoggingConfig{
				Level:  "info",
				Format: "text",
				Dir:    "logs",
				File: FileConfig{
					Enabled: true,
					Level:   "invalid",
					Format:  "text",
				},
			},
			expectError: true,
		},
		{
			name: "invalid file format when enabled",
			cfg: LoggingConfig{
				Level:  "info",
				Format: "text",
				Dir:    "logs",
				File: FileConfig{
					Enabled: true,
					Level:   "info",
					Format:  "xml",
				},
			},
			expectError: true,
		},
		{
			name: "valid config with console and file overrides",
			cfg: LoggingConfig{
				Level:  "info",
				Format: "text",
				Dir:    "logs",
				Console: ConsoleConfig{
					Enabled: true,
					Level:   "debug",
					Format:  "text",
				},
				File: FileConfig{
					Enabled: true,
					Level:   "warn",
					Format:  "json",
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate(services.ModeDistributed)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
