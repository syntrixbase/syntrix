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

func TestNewPublisher_NilConnection(t *testing.T) {
	cfg := Config{}

	// Test with nil connection - should fail to create JetStream
	pub, err := NewPublisher(nil, cfg)
	assert.Error(t, err)
	assert.Nil(t, pub)
	assert.Contains(t, err.Error(), "failed to create JetStream")
}
