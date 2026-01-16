package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	// Verify backends
	assert.Contains(t, cfg.Backends, "default_mongo")
	assert.Equal(t, "mongo", cfg.Backends["default_mongo"].Type)
	assert.Equal(t, "mongodb://localhost:27017", cfg.Backends["default_mongo"].Mongo.URI)
	assert.Equal(t, "syntrix", cfg.Backends["default_mongo"].Mongo.DatabaseName)

	// Verify topology - Document
	assert.Equal(t, "single", cfg.Topology.Document.Strategy)
	assert.Equal(t, "default_mongo", cfg.Topology.Document.Primary)
	assert.Equal(t, "documents", cfg.Topology.Document.DataCollection)
	assert.Equal(t, "sys", cfg.Topology.Document.SysCollection)
	assert.Equal(t, 5*time.Minute, cfg.Topology.Document.SoftDeleteRetention)

	// Verify topology - User
	assert.Equal(t, "single", cfg.Topology.User.Strategy)
	assert.Equal(t, "default_mongo", cfg.Topology.User.Primary)
	assert.Equal(t, "users", cfg.Topology.User.Collection)

	// Verify topology - Revocation
	assert.Equal(t, "single", cfg.Topology.Revocation.Strategy)
	assert.Equal(t, "default_mongo", cfg.Topology.Revocation.Primary)
	assert.Equal(t, "revocations", cfg.Topology.Revocation.Collection)

	// Verify databases
	assert.Contains(t, cfg.Databases, "default")
	assert.Equal(t, "default_mongo", cfg.Databases["default"].Backend)
}

func TestConfig_Validate(t *testing.T) {
	cfg := DefaultConfig()

	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestConfig_Validate_EmptyConfig(t *testing.T) {
	cfg := Config{}

	err := cfg.Validate()
	assert.NoError(t, err)
}
