// internal/logging/logger.go
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/syntrixbase/syntrix/internal/config"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	// Global state for cleanup
	logFiles   []*lumberjack.Logger
	logFilesMu sync.Mutex
)

// Initialize sets up the global logger based on configuration
func Initialize(cfg config.LoggingConfig) error {
	logger, err := NewLogger(cfg)
	if err != nil {
		return fmt.Errorf("failed to create logger: %w", err)
	}

	// Set as global default logger
	slog.SetDefault(logger)

	// Log startup message with configuration
	slog.Info("Logging initialized",
		"level", cfg.Level,
		"format", cfg.Format,
		"dir", cfg.Dir,
		"console_enabled", cfg.Console.Enabled,
		"file_enabled", cfg.File.Enabled,
	)

	return nil
}

// NewLogger creates a new logger instance with the given configuration
func NewLogger(cfg config.LoggingConfig) (*slog.Logger, error) {
	// Create log directory if it doesn't exist
	if err := os.MkdirAll(cfg.Dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	var handlers []slog.Handler

	// Add console handler if enabled
	if cfg.Console.Enabled {
		level := parseLevel(cfg.Console.Level)
		handler := createHandler(os.Stdout, cfg.Console.Format, level)
		handlers = append(handlers, handler)
	}

	// Add file handlers if enabled
	if cfg.File.Enabled {
		// Main log file (all levels)
		mainLogPath := filepath.Join(cfg.Dir, "syntrix.log")
		mainFile := &lumberjack.Logger{
			Filename:   mainLogPath,
			MaxSize:    cfg.Rotation.MaxSize,
			MaxBackups: cfg.Rotation.MaxBackups,
			MaxAge:     cfg.Rotation.MaxAge,
			Compress:   cfg.Rotation.Compress,
		}
		registerLogFile(mainFile)

		level := parseLevel(cfg.File.Level)
		mainHandler := createHandler(mainFile, cfg.File.Format, level)
		handlers = append(handlers, mainHandler)

		// Error log file (warn and error only)
		errorLogPath := filepath.Join(cfg.Dir, "errors.log")
		errorFile := &lumberjack.Logger{
			Filename:   errorLogPath,
			MaxSize:    cfg.Rotation.MaxSize,
			MaxBackups: cfg.Rotation.MaxBackups,
			MaxAge:     cfg.Rotation.MaxAge,
			Compress:   cfg.Rotation.Compress,
		}
		registerLogFile(errorFile)

		errorHandler := createHandler(errorFile, cfg.File.Format, slog.LevelWarn)
		errorHandler = NewLevelFilter(errorHandler, slog.LevelWarn)
		handlers = append(handlers, errorHandler)
	}

	// Combine all handlers
	var handler slog.Handler
	if len(handlers) == 1 {
		handler = handlers[0]
	} else {
		handler = NewMultiHandler(handlers...)
	}

	return slog.New(handler), nil
}

// Shutdown gracefully closes all log files and flushes buffers
func Shutdown() error {
	logFilesMu.Lock()
	defer logFilesMu.Unlock()

	for _, logFile := range logFiles {
		if err := logFile.Close(); err != nil {
			return fmt.Errorf("failed to close log file: %w", err)
		}
	}

	logFiles = nil
	return nil
}

// Helper functions

func registerLogFile(logFile *lumberjack.Logger) {
	logFilesMu.Lock()
	defer logFilesMu.Unlock()
	logFiles = append(logFiles, logFile)
}

func parseLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func createHandler(w io.Writer, format string, level slog.Level) slog.Handler {
	opts := &slog.HandlerOptions{
		Level: level,
	}

	if format == "json" {
		return slog.NewJSONHandler(w, opts)
	}
	return slog.NewTextHandler(w, opts)
}
