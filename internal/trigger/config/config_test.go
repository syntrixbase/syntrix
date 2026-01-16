package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/syntrixbase/syntrix/internal/trigger/delivery"
	"github.com/syntrixbase/syntrix/internal/trigger/evaluator"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.Equal(t, "nats://localhost:4222", cfg.NatsURL)

	// Evaluator config
	assert.Equal(t, "TRIGGERS", cfg.Evaluator.StreamName)
	assert.Equal(t, 3, cfg.Evaluator.RetryAttempts)
	assert.Equal(t, "file", cfg.Evaluator.StorageType)
	assert.True(t, cfg.Evaluator.StartFromNow)
	assert.Equal(t, "default", cfg.Evaluator.CheckpointDatabase)

	// Delivery config
	assert.Equal(t, "TRIGGERS", cfg.Delivery.StreamName)
	assert.Equal(t, "trigger-delivery", cfg.Delivery.ConsumerName)
	assert.Equal(t, "file", cfg.Delivery.StorageType)
	assert.Equal(t, 16, cfg.Delivery.NumWorkers)
}

func TestConfig_StructFields(t *testing.T) {
	cfg := Config{
		NatsURL: "nats://custom:4222",
		Evaluator: evaluator.Config{
			StreamName:         "custom_stream",
			RulesFile:          "custom_triggers.json",
			CheckpointDatabase: "custom_db",
		},
		Delivery: delivery.Config{
			StreamName:   "custom_stream",
			ConsumerName: "custom-consumer",
			NumWorkers:   8,
		},
	}

	assert.Equal(t, "nats://custom:4222", cfg.NatsURL)
	assert.Equal(t, "custom_stream", cfg.Evaluator.StreamName)
	assert.Equal(t, "custom_triggers.json", cfg.Evaluator.RulesFile)
	assert.Equal(t, "custom_db", cfg.Evaluator.CheckpointDatabase)
	assert.Equal(t, "custom_stream", cfg.Delivery.StreamName)
	assert.Equal(t, "custom-consumer", cfg.Delivery.ConsumerName)
	assert.Equal(t, 8, cfg.Delivery.NumWorkers)
}
