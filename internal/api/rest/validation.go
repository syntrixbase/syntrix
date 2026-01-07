package rest

import (
	"errors"
	"fmt"
	"strings"

	"github.com/syntrixbase/syntrix/internal/helper"
	"github.com/syntrixbase/syntrix/internal/storage"
	"github.com/syntrixbase/syntrix/pkg/model"
)

// ValidationConfig holds configurable limits for validation
type ValidationConfig struct {
	MaxQueryLimit       int // Maximum allowed limit for queries (default: 1000)
	MaxReplicationLimit int // Maximum allowed limit for replication (default: 1000)
	MaxPathLength       int // Maximum allowed path length (default: 1024)
	MaxIDLength         int // Maximum allowed document ID length (default: 64)
}

// DefaultValidationConfig returns the default validation configuration
func DefaultValidationConfig() ValidationConfig {
	return ValidationConfig{
		MaxQueryLimit:       1000,
		MaxReplicationLimit: 1000,
		MaxPathLength:       1024,
		MaxIDLength:         64,
	}
}

// validationConfig is the package-level configuration used by validation functions
var validationConfig = DefaultValidationConfig()

// SetValidationConfig updates the validation configuration
func SetValidationConfig(cfg ValidationConfig) {
	if cfg.MaxQueryLimit <= 0 {
		cfg.MaxQueryLimit = DefaultValidationConfig().MaxQueryLimit
	}
	if cfg.MaxReplicationLimit <= 0 {
		cfg.MaxReplicationLimit = DefaultValidationConfig().MaxReplicationLimit
	}
	if cfg.MaxPathLength <= 0 {
		cfg.MaxPathLength = DefaultValidationConfig().MaxPathLength
	}
	if cfg.MaxIDLength <= 0 {
		cfg.MaxIDLength = DefaultValidationConfig().MaxIDLength
	}
	validationConfig = cfg
}

func validateQuery(q model.Query) error {
	if err := helper.CheckCollectionPath(q.Collection); err != nil {
		return fmt.Errorf("invalid collection: %w", err)
	}
	if q.Limit < 0 {
		return errors.New("limit cannot be negative")
	}
	if q.Limit > validationConfig.MaxQueryLimit {
		return fmt.Errorf("limit cannot exceed %d", validationConfig.MaxQueryLimit)
	}
	for _, f := range q.Filters {
		if f.Field == "" {
			return errors.New("filter field cannot be empty")
		}
		if f.Op == "" {
			return errors.New("filter op cannot be empty")
		}
		if !f.Op.IsValid() {
			return fmt.Errorf("unsupported filter operator: %s", f.Op)
		}
	}
	for _, o := range q.OrderBy {
		if o.Field == "" {
			return errors.New("orderby field cannot be empty")
		}
		if o.Direction != "asc" && o.Direction != "desc" {
			return errors.New("orderby direction must be 'asc' or 'desc'")
		}
	}
	return nil
}

func validateReplicationPull(req storage.ReplicationPullRequest) error {
	if err := helper.CheckCollectionPath(req.Collection); err != nil {
		return fmt.Errorf("invalid collection: %w", err)
	}
	if req.Limit < 0 {
		return errors.New("limit cannot be negative")
	}
	if req.Limit > validationConfig.MaxReplicationLimit {
		return fmt.Errorf("limit cannot exceed %d", validationConfig.MaxReplicationLimit)
	}
	return nil
}

func validateReplicationPush(req storage.ReplicationPushRequest) error {
	if err := helper.CheckCollectionPath(req.Collection); err != nil {
		return fmt.Errorf("invalid collection: %w", err)
	}
	for _, change := range req.Changes {
		if change.Doc == nil {
			return errors.New("change document cannot be nil")
		}
		if err := helper.CheckDocumentPath(change.Doc.Fullpath); err != nil {
			return fmt.Errorf("invalid document path in change: %w", err)
		}
		// Ensure document path matches collection prefix
		if !strings.HasPrefix(change.Doc.Fullpath, req.Collection+"/") {
			return fmt.Errorf("document path %s does not belong to collection %s", change.Doc.Fullpath, req.Collection)
		}
	}
	return nil
}
