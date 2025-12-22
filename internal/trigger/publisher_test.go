package trigger

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func (m *MockJetStream) Publish(ctx context.Context, subject string, data []byte, opts ...jetstream.PublishOpt) (*jetstream.PubAck, error) {
	args := m.Called(ctx, subject, data, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*jetstream.PubAck), args.Error(1)
}

func TestNatsPublisher_Publish(t *testing.T) {
	mockJS := new(MockJetStream)
	publisher := NewEventPublisherFromJS(mockJS)

	task := &DeliveryTask{
		Tenant:     "acme",
		Collection: "users",
		DocKey:     "user-1",
		TriggerID:  "t1",
	}

	expectedSubject := "triggers.acme.users.user-1"
	expectedData, _ := json.Marshal(task)

	mockJS.On("Publish", mock.Anything, expectedSubject, expectedData, mock.Anything).Return(&jetstream.PubAck{}, nil)

	err := publisher.Publish(context.Background(), task)
	assert.NoError(t, err)

	mockJS.AssertExpectations(t)
}

func TestNatsPublisher_Publish_Error(t *testing.T) {
	mockJS := new(MockJetStream)
	publisher := NewEventPublisherFromJS(mockJS)

	task := &DeliveryTask{
		Tenant:     "acme",
		Collection: "users",
		DocKey:     "user-1",
	}

	mockJS.On("Publish", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("publish failed"))

	err := publisher.Publish(context.Background(), task)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "publish failed")

	mockJS.AssertExpectations(t)
}
