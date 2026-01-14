package testing

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/syntrixbase/syntrix/internal/core/pubsub"
)

func TestMockPublisher_Publish(t *testing.T) {
	pub := NewMockPublisher()

	err := pub.Publish(context.Background(), "test.subject", []byte("hello"))
	require.NoError(t, err)

	msgs := pub.Messages()
	require.Len(t, msgs, 1)
	assert.Equal(t, "test.subject", msgs[0].Subject)
	assert.Equal(t, []byte("hello"), msgs[0].Data)
}

func TestMockPublisher_PublishMultiple(t *testing.T) {
	pub := NewMockPublisher()

	pub.Publish(context.Background(), "s1", []byte("d1"))
	pub.Publish(context.Background(), "s2", []byte("d2"))

	msgs := pub.Messages()
	require.Len(t, msgs, 2)
	assert.Equal(t, "s1", msgs[0].Subject)
	assert.Equal(t, "s2", msgs[1].Subject)
}

func TestMockPublisher_Error(t *testing.T) {
	pub := NewMockPublisher()
	expectedErr := errors.New("publish failed")
	pub.SetError(expectedErr)

	err := pub.Publish(context.Background(), "test", []byte("data"))
	assert.Equal(t, expectedErr, err)
	assert.Empty(t, pub.Messages())
}

func TestMockPublisher_Close(t *testing.T) {
	pub := NewMockPublisher()
	assert.False(t, pub.IsClosed())

	err := pub.Close()
	require.NoError(t, err)
	assert.True(t, pub.IsClosed())
}

func TestMockPublisher_Reset(t *testing.T) {
	pub := NewMockPublisher()
	pub.Publish(context.Background(), "test", []byte("data"))
	pub.SetError(errors.New("err"))
	pub.Close()

	pub.Reset()

	assert.Empty(t, pub.Messages())
	assert.False(t, pub.IsClosed())
	err := pub.Publish(context.Background(), "test", []byte("data"))
	require.NoError(t, err)
}

func TestMockMessage_Data(t *testing.T) {
	msg := NewMockMessage("test.subject", []byte("payload"))

	assert.Equal(t, []byte("payload"), msg.Data())
	assert.Equal(t, "test.subject", msg.Subject())
}

func TestMockMessage_Ack(t *testing.T) {
	msg := NewMockMessage("test", []byte("data"))
	assert.False(t, msg.IsAcked())

	err := msg.Ack()
	require.NoError(t, err)
	assert.True(t, msg.IsAcked())
}

func TestMockMessage_Nak(t *testing.T) {
	msg := NewMockMessage("test", []byte("data"))
	assert.False(t, msg.IsNaked())

	err := msg.Nak()
	require.NoError(t, err)
	assert.True(t, msg.IsNaked())
}

func TestMockMessage_NakWithDelay(t *testing.T) {
	msg := NewMockMessage("test", []byte("data"))

	err := msg.NakWithDelay(5 * time.Second)
	require.NoError(t, err)
	assert.True(t, msg.IsNaked())
	assert.Equal(t, 5*time.Second, msg.NakDelay())
}

func TestMockMessage_Term(t *testing.T) {
	msg := NewMockMessage("test", []byte("data"))
	assert.False(t, msg.IsTermed())

	err := msg.Term()
	require.NoError(t, err)
	assert.True(t, msg.IsTermed())
}

func TestMockMessage_Metadata(t *testing.T) {
	msg := NewMockMessage("test", []byte("data"))

	md, err := msg.Metadata()
	require.NoError(t, err)
	assert.Equal(t, uint64(1), md.NumDelivered)
	assert.Equal(t, "test", md.Subject)
}

func TestMockMessage_SetMetadata(t *testing.T) {
	msg := NewMockMessage("test", []byte("data"))
	msg.SetMetadata(pubsub.MessageMetadata{
		NumDelivered: 5,
		Subject:      "custom",
		Stream:       "mystream",
	})

	md, err := msg.Metadata()
	require.NoError(t, err)
	assert.Equal(t, uint64(5), md.NumDelivered)
	assert.Equal(t, "custom", md.Subject)
	assert.Equal(t, "mystream", md.Stream)
}

func TestMockMessage_Errors(t *testing.T) {
	msg := NewMockMessage("test", []byte("data"))
	msg.SetErrors(
		errors.New("ack err"),
		errors.New("nak err"),
		errors.New("term err"),
		errors.New("metadata err"),
	)

	err := msg.Ack()
	assert.EqualError(t, err, "ack err")

	err = msg.Nak()
	assert.EqualError(t, err, "nak err")

	err = msg.Term()
	assert.EqualError(t, err, "term err")

	_, err = msg.Metadata()
	assert.EqualError(t, err, "metadata err")
}

func TestMockConsumer_Start(t *testing.T) {
	consumer := NewMockConsumer()
	assert.False(t, consumer.IsStarted())

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		consumer.Start(ctx, func(ctx context.Context, msg pubsub.Message) error {
			return nil
		})
		close(done)
	}()

	// Wait a bit for Start to be called
	time.Sleep(10 * time.Millisecond)
	assert.True(t, consumer.IsStarted())

	cancel()
	<-done
}

func TestMockConsumer_SimulateMessage(t *testing.T) {
	consumer := NewMockConsumer()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var receivedData []byte
	go consumer.Start(ctx, func(ctx context.Context, msg pubsub.Message) error {
		receivedData = msg.Data()
		return nil
	})

	time.Sleep(10 * time.Millisecond)

	msg := NewMockMessage("test", []byte("hello"))
	err := consumer.SimulateMessage(context.Background(), msg)
	require.NoError(t, err)

	assert.Equal(t, []byte("hello"), receivedData)
}

func TestMockConsumer_Error(t *testing.T) {
	consumer := NewMockConsumer()
	expectedErr := errors.New("start failed")
	consumer.SetError(expectedErr)

	err := consumer.Start(context.Background(), nil)
	assert.Equal(t, expectedErr, err)
}
