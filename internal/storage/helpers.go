package storage

import "github.com/codetrek/syntrix/internal/storage/types"

// CalculateTenantID calculates the tenant-aware document ID
func CalculateTenantID(tenant, fullpath string) string {
	return types.CalculateTenantID(tenant, fullpath)
}

// CalculateID calculates the document ID (hash) from the full path
// Deprecated: Use CalculateTenantID instead
func CalculateID(fullpath string) string {
	return types.CalculateID(fullpath)
}

// NewDocument creates a new document instance with initialized metadata
func NewDocument(tenant string, fullpath string, collection string, data map[string]interface{}) *Document {
	return types.NewDocument(tenant, fullpath, collection, data)
}
