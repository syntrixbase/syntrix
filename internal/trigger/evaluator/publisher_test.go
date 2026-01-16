package evaluator

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pubsubtesting "github.com/syntrixbase/syntrix/internal/core/pubsub/testing"
	"github.com/syntrixbase/syntrix/internal/trigger/types"
)

func TestNatsPublisher_Publish(t *testing.T) {
	mockPub := pubsubtesting.NewMockPublisher()
	publisher := NewTaskPublisher(mockPub, "TRIGGERS", nil)

	task := &types.DeliveryTask{
		Database:   "acme",
		Collection: "users",
		DocumentID: "user-1",
		TriggerID:  "t1",
	}

	// Subject format: <database>.<collection>.<base64_encoded_doc_id>
	// (prefix is handled by pubsub.Publisher)
	expectedSubject := "acme.users.dXNlci0x" // base64url("user-1")

	err := publisher.Publish(context.Background(), task)
	require.NoError(t, err)

	msgs := mockPub.Messages()
	require.Len(t, msgs, 1)
	assert.Equal(t, expectedSubject, msgs[0].Subject)

	var decoded types.DeliveryTask
	err = json.Unmarshal(msgs[0].Data, &decoded)
	require.NoError(t, err)
	assert.Equal(t, task.TriggerID, decoded.TriggerID)
}

func TestNatsPublisher_Publish_HashedSubject(t *testing.T) {
	mockPub := pubsubtesting.NewMockPublisher()
	publisher := NewTaskPublisher(mockPub, "TRIGGERS", nil)

	// Create a long DocumentID to force hashing
	longDocumentID := strings.Repeat("a", 1000)
	encodedDocumentID := base64.URLEncoding.EncodeToString([]byte(longDocumentID))

	task := &types.DeliveryTask{
		Database:   "acme",
		Collection: "users",
		DocumentID: longDocumentID,
		TriggerID:  "t1",
	}

	// Calculate expected subject
	// Full subject with prefix for length check
	fullSubject := fmt.Sprintf("TRIGGERS.%s.%s.%s", task.Database, task.Collection, encodedDocumentID)
	// Verify it is indeed long enough
	assert.True(t, len(fullSubject) > 1024)

	hash := sha256.Sum256([]byte(fullSubject))
	hashStr := hex.EncodeToString(hash[:16])
	expectedSubject := fmt.Sprintf("hashed.%s", hashStr)

	err := publisher.Publish(context.Background(), task)
	require.NoError(t, err)
	assert.True(t, task.SubjectHashed) // Verify the task was modified in place

	msgs := mockPub.Messages()
	require.Len(t, msgs, 1)
	assert.Equal(t, expectedSubject, msgs[0].Subject)
}

func TestNatsPublisher_Publish_Error(t *testing.T) {
	mockPub := pubsubtesting.NewMockPublisher()
	mockPub.SetError(assert.AnError)
	publisher := NewTaskPublisher(mockPub, "TRIGGERS", nil)

	task := &types.DeliveryTask{
		Database:   "acme",
		Collection: "users",
		DocumentID: "user-1",
		TriggerID:  "t1",
	}

	err := publisher.Publish(context.Background(), task)
	assert.Error(t, err)
}

func TestNatsPublisher_Close_Delegates(t *testing.T) {
	mockPub := pubsubtesting.NewMockPublisher()
	publisher := NewTaskPublisher(mockPub, "TRIGGERS", nil)

	err := publisher.Close()
	assert.NoError(t, err)
	assert.True(t, mockPub.IsClosed())
}

func TestNewTaskPublisher_EmptyStreamName(t *testing.T) {
	mockPub := pubsubtesting.NewMockPublisher()
	publisher := NewTaskPublisher(mockPub, "", nil)
	assert.NotNil(t, publisher)

	// Verify default prefix is used for hashing calculation
	task := &types.DeliveryTask{
		Database:   "acme",
		Collection: "users",
		DocumentID: "user-1",
		TriggerID:  "t1",
	}

	err := publisher.Publish(context.Background(), task)
	require.NoError(t, err)

	msgs := mockPub.Messages()
	require.Len(t, msgs, 1)
	// Subject should be: <database>.<collection>.<base64_encoded_doc_id>
	assert.Equal(t, "acme.users.dXNlci0x", msgs[0].Subject)
}
