package types

import (
	"github.com/codetrek/syntrix/internal/storage/types"
)

// EventType represents the type of change operation.
type EventType string

const (
	EventCreate EventType = "create"
	EventUpdate EventType = "update"
	EventDelete EventType = "delete"
)

// TriggerEvent represents an event that triggers a rule evaluation.
// It is decoupled from the underlying storage event to allow different sources (e.g. Puller).
type TriggerEvent struct {
	Id          string
	TenantID    string
	Type        EventType
	Document    *types.Document // Using storage.Document as the common data model
	Before      *types.Document
	Timestamp   int64
	ResumeToken string // String token from Puller
}
