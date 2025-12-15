package realtime

import (
	"encoding/json"
	"syntrix/internal/storage"
)

// Message types
const (
	TypeAuth           = "auth"
	TypeAuthAck        = "auth_ack"
	TypeSubscribe      = "subscribe"
	TypeUnsubscribe    = "unsubscribe"
	TypeUnsubscribeAck = "unsubscribe_ack"
	TypeStream         = "stream"
	TypeEvent          = "event"
	TypeStreamEvent    = "stream-event"
	TypeSnapshot       = "snapshot"
	TypeError          = "error"
)

// BaseMessage is the envelope for all messages
type BaseMessage struct {
	ID      string          `json:"id,omitempty"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// AuthPayload
type AuthPayload struct {
	Token string `json:"token"`
}

// SubscribePayload
type SubscribePayload struct {
	Query storage.Query `json:"query"`
}

// StreamPayload (RxDB Replication)
type StreamPayload struct {
	Collection string `json:"collection"`
	Checkpoint int64  `json:"checkpoint"` // Simplified for now, doc says object
}

// UnsubscribePayload
type UnsubscribePayload struct {
	ID string `json:"id"`
}

// EventPayload (Server -> Client)
type EventPayload struct {
	SubID string        `json:"subId"`
	Delta storage.Event `json:"delta"`
}

// StreamEventPayload (Server -> Client)
type StreamEventPayload struct {
	StreamID   string              `json:"streamId"`
	Documents  []*storage.Document `json:"documents"`
	Checkpoint int64               `json:"checkpoint"`
}

// ErrorPayload
type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
