// internal/logging/logger_test.go
package logging

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/syntrixbase/syntrix/internal/config"
)

func TestNewLogger_DefaultConfig(t *testing.T) {
	cfg := config.DefaultLoggingConfig()

	// Use temp directory for testing
	tmpDir := t.TempDir()
	cfg.Dir = tmpDir

	logger, err := NewLogger(cfg)
	require.NoError(t, err)
	assert.NotNil(t, logger)

	// Test that logging works
	logger.Info("test message", "key", "value")

	// Verify main log file was created
	mainLogPath := filepath.Join(tmpDir, "syntrix.log")
	assert.FileExists(t, mainLogPath)

	// Note: error log file won't exist yet since we only logged Info
	// and lumberjack doesn't create empty files

	// Cleanup
	Shutdown()
}

func TestNewLogger_JSONFormat(t *testing.T) {
	cfg := config.DefaultLoggingConfig()
	cfg.Format = "json"
	cfg.File.Format = "json"

	tmpDir := t.TempDir()
	cfg.Dir = tmpDir

	logger, err := NewLogger(cfg)
	require.NoError(t, err)

	logger.Info("test json", "key", "value")

	// Read log file and verify JSON format
	mainLogPath := filepath.Join(tmpDir, "syntrix.log")
	content, err := os.ReadFile(mainLogPath)
	require.NoError(t, err)

	assert.Contains(t, string(content), `"msg":"test json"`)
	assert.Contains(t, string(content), `"key":"value"`)

	Shutdown()
}

func TestNewLogger_ErrorLogSeparation(t *testing.T) {
	cfg := config.DefaultLoggingConfig()

	tmpDir := t.TempDir()
	cfg.Dir = tmpDir

	logger, err := NewLogger(cfg)
	require.NoError(t, err)

	// Log different levels
	logger.Info("info message")
	logger.Warn("warning message")
	logger.Error("error message")

	// Shutdown to flush buffers
	Shutdown()

	// Check main log (should have all messages)
	mainLogPath := filepath.Join(tmpDir, "syntrix.log")
	mainContent, err := os.ReadFile(mainLogPath)
	require.NoError(t, err)

	assert.Contains(t, string(mainContent), "info message")
	assert.Contains(t, string(mainContent), "warning message")
	assert.Contains(t, string(mainContent), "error message")

	// Check error log (should only have warn and error)
	errorLogPath := filepath.Join(tmpDir, "errors.log")
	errorContent, err := os.ReadFile(errorLogPath)
	require.NoError(t, err)

	assert.NotContains(t, string(errorContent), "info message")
	assert.Contains(t, string(errorContent), "warning message")
	assert.Contains(t, string(errorContent), "error message")
}

func TestNewLogger_ConsoleDisabled(t *testing.T) {
	cfg := config.DefaultLoggingConfig()
	cfg.Console.Enabled = false

	tmpDir := t.TempDir()
	cfg.Dir = tmpDir

	logger, err := NewLogger(cfg)
	require.NoError(t, err)
	assert.NotNil(t, logger)

	// Logging should still work (to files only)
	logger.Info("test message")

	Shutdown()

	// Verify file was created
	mainLogPath := filepath.Join(tmpDir, "syntrix.log")
	assert.FileExists(t, mainLogPath)
}

func TestInitialize_SetsGlobalLogger(t *testing.T) {
	cfg := config.DefaultLoggingConfig()

	tmpDir := t.TempDir()
	cfg.Dir = tmpDir

	err := Initialize(cfg)
	require.NoError(t, err)

	// Test that global default logger works
	slog.Info("global test message")

	Shutdown()

	// Verify log was written
	mainLogPath := filepath.Join(tmpDir, "syntrix.log")
	content, err := os.ReadFile(mainLogPath)
	require.NoError(t, err)

	assert.Contains(t, string(content), "global test message")
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		name     string
		level    string
		expected slog.Level
	}{
		{
			name:     "debug level",
			level:    "debug",
			expected: slog.LevelDebug,
		},
		{
			name:     "info level",
			level:    "info",
			expected: slog.LevelInfo,
		},
		{
			name:     "warn level",
			level:    "warn",
			expected: slog.LevelWarn,
		},
		{
			name:     "error level",
			level:    "error",
			expected: slog.LevelError,
		},
		{
			name:     "unknown level defaults to info",
			level:    "invalid",
			expected: slog.LevelInfo,
		},
		{
			name:     "empty level defaults to info",
			level:    "",
			expected: slog.LevelInfo,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseLevel(tt.level)
			assert.Equal(t, tt.expected, result)
		})
	}
}
