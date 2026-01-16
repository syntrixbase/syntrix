package config

import (
	"github.com/syntrixbase/syntrix/internal/trigger/delivery"
	"github.com/syntrixbase/syntrix/internal/trigger/evaluator"
)

// Config is the top-level trigger configuration.
type Config struct {
	NatsURL string `yaml:"nats_url"`

	// Evaluator service configuration
	Evaluator evaluator.Config `yaml:"evaluator"`

	// Delivery service configuration
	Delivery delivery.Config `yaml:"delivery"`
}

// DefaultConfig returns the default trigger configuration.
func DefaultConfig() Config {
	return Config{
		NatsURL:   "nats://localhost:4222",
		Evaluator: evaluator.DefaultConfig(),
		Delivery:  delivery.DefaultConfig(),
	}
}
