package storage

import "github.com/syntrixbase/syntrix/internal/storage/types"

var (
	// CalculateDatabaseID calculates the database-aware document ID
	CalculateDatabaseID = types.CalculateDatabaseID

	// NewStoredDoc creates a new document instance with initialized metadata
	NewStoredDoc = types.NewStoredDoc
)
