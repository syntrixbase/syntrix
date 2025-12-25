package storage

import (
	"context"
	"testing"
	"time"

	"github.com/codetrek/syntrix/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testMongoURI = "mongodb://localhost:27017"
	testDBName   = "syntrix_test_factory"
)

func TestNewDocumentProvider(t *testing.T) {
	cfg := config.StorageConfig{
		Document: config.DocumentStorageConfig{
			Mongo: config.MongoDocConfig{
				URI:            testMongoURI,
				DatabaseName:   testDBName,
				DataCollection: "docs",
				SysCollection:  "sys",
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	provider, err := NewDocumentProvider(ctx, cfg)
	if err != nil {
		t.Skipf("Skipping test: MongoDB not available: %v", err)
	}
	require.NoError(t, err)
	assert.NotNil(t, provider)
	assert.NotNil(t, provider.Document())
}

func TestNewAuthProvider(t *testing.T) {
	cfg := config.StorageConfig{
		User: config.UserStorageConfig{
			Mongo: config.MongoConfig{
				URI:          testMongoURI,
				DatabaseName: testDBName,
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	provider, err := NewAuthProvider(ctx, cfg)
	if err != nil {
		t.Skipf("Skipping test: MongoDB not available: %v", err)
	}
	require.NoError(t, err)
	assert.NotNil(t, provider)
	assert.NotNil(t, provider.Users())
	assert.NotNil(t, provider.Revocations())
}
