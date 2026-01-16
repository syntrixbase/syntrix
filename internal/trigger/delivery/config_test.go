package delivery

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

	assert.Equal(t, 16, cfg.NumWorkers)
	assert.Equal(t, 0, cfg.ChannelBufSize)
	assert.Equal(t, "TRIGGERS", cfg.StreamName)
	assert.Equal(t, "trigger-delivery", cfg.ConsumerName)
	assert.Equal(t, "file", cfg.StorageType)
}

func TestConfig_ApplyDefaults(t *testing.T) {
	// Empty config should get all defaults
	cfg := Config{}
	cfg.ApplyDefaults()

	assert.Equal(t, 16, cfg.NumWorkers)
	assert.Equal(t, "TRIGGERS", cfg.StreamName)
	assert.Equal(t, "trigger-delivery", cfg.ConsumerName)
	assert.Equal(t, "file", cfg.StorageType)

	// Custom values should be preserved
	cfg2 := Config{
		NumWorkers:   8,
		StreamName:   "CUSTOM_STREAM",
		ConsumerName: "custom-consumer",
		StorageType:  "memory",
	}
	cfg2.ApplyDefaults()

	assert.Equal(t, 8, cfg2.NumWorkers)
	assert.Equal(t, "CUSTOM_STREAM", cfg2.StreamName)
	assert.Equal(t, "custom-consumer", cfg2.ConsumerName)
	assert.Equal(t, "memory", cfg2.StorageType)
}

func TestNewConsumer_NilConnection(t *testing.T) {
	cfg := Config{}

	// Test with nil connection - should fail to create JetStream
	consumer, err := NewConsumer(nil, cfg)
	assert.Error(t, err)
	assert.Nil(t, consumer)
	assert.Contains(t, err.Error(), "failed to create JetStream")
}
