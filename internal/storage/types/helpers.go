package types

import (
	"encoding/hex"
	"strings"
	"time"

	"github.com/syntrixbase/syntrix/pkg/model"
	"github.com/zeebo/blake3"
)

// CalculateTenantID calculates the tenant-aware document ID
// Format: tenant_id:hash(fullpath)
func CalculateTenantID(tenant, fullpath string) string {
	hash := blake3.Sum256([]byte(fullpath))
	hashStr := hex.EncodeToString(hash[:16])
	return tenant + ":" + hashStr
}

// CalculateID calculates the document ID (hash) from the full path
// Deprecated: Use CalculateTenantID instead
func CalculateID(fullpath string) string {
	hash := blake3.Sum256([]byte(fullpath))
	return hex.EncodeToString(hash[:16])
}

// CalculateCollectionHash calculates a stable hash for a collection name.
// A prefix keeps the namespace distinct from document IDs.
func CalculateCollectionHash(collection string) string {
	hash := blake3.Sum256([]byte("collection:" + collection))
	return hex.EncodeToString(hash[:16])
}

func NewStoredDoc(tenant, collection, docid string, data map[string]interface{}) StoredDoc {
	// Calculate Parent from collection path
	parent := ""
	if idx := strings.LastIndex(collection, "/"); idx != -1 {
		parent = collection[:idx]
	}

	if data == nil {
		data = make(map[string]interface{})
	}

	if id, exists := data["id"]; !exists || id != docid {
		data["id"] = docid
	}

	model.StripProtectedFields(data)

	fullpath := collection + "/" + docid
	id := CalculateTenantID(tenant, fullpath)
	collectionHash := CalculateCollectionHash(collection)

	now := time.Now().UnixMilli()

	return StoredDoc{
		Id:             id,
		TenantID:       tenant,
		Fullpath:       fullpath,
		Collection:     collection,
		CollectionHash: collectionHash,
		Parent:         parent,
		Data:           data,
		UpdatedAt:      now,
		CreatedAt:      now,
		Version:        1,
	}
}
