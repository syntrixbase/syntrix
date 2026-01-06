package storage

import "github.com/syntrixbase/syntrix/internal/storage/types"

var (
	// CalculateTenantID calculates the tenant-aware document ID
	CalculateTenantID = types.CalculateTenantID

	// NewStoredDoc creates a new document instance with initialized metadata
	NewStoredDoc = types.NewStoredDoc
)
