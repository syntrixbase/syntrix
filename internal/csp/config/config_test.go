package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.Equal(t, 8083, cfg.Port)
}

func TestConfig_StructFields(t *testing.T) {
	cfg := Config{
		Port: 9090,
	}

	assert.Equal(t, 9090, cfg.Port)
}
