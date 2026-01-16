package evaluator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/syntrixbase/syntrix/internal/core/pubsub"
)

func TestConfig_StorageTypeValue(t *testing.T) {
	tests := []struct {
		name        string
		storageType string
		expected    pubsub.StorageType
	}{
		{
			name:        "memory storage",
			storageType: "memory",
			expected:    pubsub.MemoryStorage,
		},
		{
			name:        "file storage",
			storageType: "file",
			expected:    pubsub.FileStorage,
		},
		{
			name:        "empty defaults to file",
			storageType: "",
			expected:    pubsub.FileStorage,
		},
		{
			name:        "unknown defaults to file",
			storageType: "unknown",
			expected:    pubsub.FileStorage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{StorageType: tt.storageType}
			assert.Equal(t, tt.expected, cfg.StorageTypeValue())
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.True(t, cfg.StartFromNow)
	assert.Equal(t, "triggers.example.json", cfg.RulesFile)
	assert.Equal(t, "default", cfg.CheckpointDatabase)
	assert.Equal(t, "TRIGGERS", cfg.StreamName)
	assert.Equal(t, 3, cfg.RetryAttempts)
	assert.Equal(t, "file", cfg.StorageType)
}

func TestConfig_ApplyDefaults(t *testing.T) {
	// Empty config should get all defaults
	cfg := Config{}
	cfg.ApplyDefaults()

	assert.Equal(t, "default", cfg.CheckpointDatabase)
	assert.Equal(t, "TRIGGERS", cfg.StreamName)
	assert.Equal(t, 3, cfg.RetryAttempts)
	assert.Equal(t, "file", cfg.StorageType)

	// Custom values should be preserved
	cfg2 := Config{
		CheckpointDatabase: "custom_db",
		StreamName:         "CUSTOM_STREAM",
		RetryAttempts:      5,
		StorageType:        "memory",
	}
	cfg2.ApplyDefaults()

	assert.Equal(t, "custom_db", cfg2.CheckpointDatabase)
	assert.Equal(t, "CUSTOM_STREAM", cfg2.StreamName)
	assert.Equal(t, 5, cfg2.RetryAttempts)
	assert.Equal(t, "memory", cfg2.StorageType)
}

func TestConfig_ApplyEnvOverrides(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ApplyEnvOverrides()
	// No env vars, just verify no panic
}

func TestConfig_ResolvePaths(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ResolvePaths("config")
	// No paths to resolve at evaluator level, just verify no panic
}

func TestConfig_Validate(t *testing.T) {
	cfg := DefaultConfig()
	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestNewPublisher_NilConnection(t *testing.T) {
	cfg := Config{}

	// Test with nil connection - should fail to create JetStream
	pub, err := NewPublisher(nil, cfg)
	assert.Error(t, err)
	assert.Nil(t, pub)
	assert.Contains(t, err.Error(), "failed to create JetStream")
}

func TestConfig_ApplyDefaults_PartialConfig(t *testing.T) {
	cfg := Config{
		RulesFile:          "my_rules.json",
		CheckpointDatabase: "my_db",
		// StreamName empty, should get default
		// RetryAttempts 0, should get default
	}
	cfg.ApplyDefaults()

	assert.Equal(t, "my_rules.json", cfg.RulesFile)
	assert.Equal(t, "my_db", cfg.CheckpointDatabase)
	assert.Equal(t, "TRIGGERS", cfg.StreamName)
	assert.Equal(t, 3, cfg.RetryAttempts)
	assert.Equal(t, "file", cfg.StorageType)
}

func TestConfig_Validate_EmptyConfig(t *testing.T) {
	cfg := Config{}
	err := cfg.Validate()
	// Empty StreamName should fail validation
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "stream_name is required")
}

func TestConfig_Validate_Errors(t *testing.T) {
	tests := []struct {
		name   string
		cfg    Config
		errMsg string
	}{
		{
			name:   "empty stream name",
			cfg:    Config{StreamName: ""},
			errMsg: "stream_name is required",
		},
		{
			name:   "negative retry attempts",
			cfg:    Config{StreamName: "TEST", RetryAttempts: -1},
			errMsg: "retry_attempts must be non-negative",
		},
		{
			name:   "invalid storage type",
			cfg:    Config{StreamName: "TEST", StorageType: "invalid"},
			errMsg: "storage_type must be 'file' or 'memory'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.errMsg)
		})
	}
}

func TestConfig_Validate_ValidConfigs(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
	}{
		{
			name: "default config",
			cfg:  DefaultConfig(),
		},
		{
			name: "memory storage",
			cfg:  Config{StreamName: "TEST", StorageType: "memory"},
		},
		{
			name: "file storage",
			cfg:  Config{StreamName: "TEST", StorageType: "file"},
		},
		{
			name: "empty storage type (defaults allowed)",
			cfg:  Config{StreamName: "TEST", StorageType: ""},
		},
		{
			name: "zero retry attempts",
			cfg:  Config{StreamName: "TEST", RetryAttempts: 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			assert.NoError(t, err)
		})
	}
}

func TestConfig_StructFields(t *testing.T) {
	cfg := Config{
		StartFromNow:       false,
		RulesFile:          "custom_rules.json",
		CheckpointDatabase: "custom_db",
		StreamName:         "CUSTOM_STREAM",
		RetryAttempts:      10,
		StorageType:        "memory",
	}

	assert.False(t, cfg.StartFromNow)
	assert.Equal(t, "custom_rules.json", cfg.RulesFile)
	assert.Equal(t, "custom_db", cfg.CheckpointDatabase)
	assert.Equal(t, "CUSTOM_STREAM", cfg.StreamName)
	assert.Equal(t, 10, cfg.RetryAttempts)
	assert.Equal(t, "memory", cfg.StorageType)
}
