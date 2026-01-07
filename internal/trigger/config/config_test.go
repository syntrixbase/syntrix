package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.Equal(t, "nats://localhost:4222", cfg.NatsURL)
	assert.Equal(t, "triggers.json", cfg.RulesFile)
	assert.Equal(t, 16, cfg.WorkerCount)
	assert.Empty(t, cfg.StreamName) // Not set in DefaultConfig
}

func TestConfig_StructFields(t *testing.T) {
	cfg := Config{
		NatsURL:     "nats://custom:4222",
		RulesFile:   "custom_triggers.json",
		WorkerCount: 8,
		StreamName:  "custom_stream",
	}

	assert.Equal(t, "nats://custom:4222", cfg.NatsURL)
	assert.Equal(t, "custom_triggers.json", cfg.RulesFile)
	assert.Equal(t, 8, cfg.WorkerCount)
	assert.Equal(t, "custom_stream", cfg.StreamName)
}
