package nats

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/syntrixbase/syntrix/internal/core/pubsub"
)

func TestNewPublisher_WithMock(t *testing.T) {
	mockJS := new(MockJetStream)
	cleanup := SetJetStreamNew(func(nc *nats.Conn) (jetstream.JetStream, error) {
		return mockJS, nil
	})
	defer cleanup()

	mockJS.On("CreateOrUpdateStream", mock.Anything, mock.MatchedBy(func(cfg jetstream.StreamConfig) bool {
		return cfg.Name == "TEST" && len(cfg.Subjects) > 0 && cfg.Subjects[0] == "PREFIX.>"
	})).Return(nil, nil)

	pub, err := NewPublisher(&nats.Conn{}, pubsub.PublisherOptions{
		StreamName:    "TEST",
		SubjectPrefix: "PREFIX",
	})

	require.NoError(t, err)
	assert.NotNil(t, pub)
	mockJS.AssertExpectations(t)
}

func TestNewPublisher_JetStreamError(t *testing.T) {
	cleanup := SetJetStreamNew(func(nc *nats.Conn) (jetstream.JetStream, error) {
		return nil, errors.New("jetstream error")
	})
	defer cleanup()

	_, err := NewPublisher(&nats.Conn{}, pubsub.PublisherOptions{
		StreamName: "TEST",
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "jetstream error")
}

func TestNewPublisher_StreamCreationError(t *testing.T) {
	mockJS := new(MockJetStream)
	cleanup := SetJetStreamNew(func(nc *nats.Conn) (jetstream.JetStream, error) {
		return mockJS, nil
	})
	defer cleanup()

	mockJS.On("CreateOrUpdateStream", mock.Anything, mock.Anything).Return(nil, errors.New("stream error"))

	_, err := NewPublisher(&nats.Conn{}, pubsub.PublisherOptions{
		StreamName: "TEST",
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "stream error")
}

func TestPublisher_Publish(t *testing.T) {
	mockJS := new(MockJetStream)
	cleanup := SetJetStreamNew(func(nc *nats.Conn) (jetstream.JetStream, error) {
		return mockJS, nil
	})
	defer cleanup()

	mockJS.On("CreateOrUpdateStream", mock.Anything, mock.Anything).Return(nil, nil)
	mockJS.On("Publish", mock.Anything, "PREFIX.test.subject", []byte("hello")).Return(&jetstream.PubAck{}, nil)

	pub, err := NewPublisher(&nats.Conn{}, pubsub.PublisherOptions{
		StreamName:    "TEST",
		SubjectPrefix: "PREFIX",
	})
	require.NoError(t, err)

	err = pub.Publish(context.Background(), "test.subject", []byte("hello"))
	assert.NoError(t, err)
	mockJS.AssertExpectations(t)
}

func TestPublisher_PublishError(t *testing.T) {
	mockJS := new(MockJetStream)
	cleanup := SetJetStreamNew(func(nc *nats.Conn) (jetstream.JetStream, error) {
		return mockJS, nil
	})
	defer cleanup()

	mockJS.On("CreateOrUpdateStream", mock.Anything, mock.Anything).Return(nil, nil)
	mockJS.On("Publish", mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("publish failed"))

	pub, err := NewPublisher(&nats.Conn{}, pubsub.PublisherOptions{
		StreamName:    "TEST",
		SubjectPrefix: "PREFIX",
	})
	require.NoError(t, err)

	err = pub.Publish(context.Background(), "test.subject", []byte("hello"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "publish failed")
}

func TestPublisher_OnPublishCallback_WithMock(t *testing.T) {
	mockJS := new(MockJetStream)
	cleanup := SetJetStreamNew(func(nc *nats.Conn) (jetstream.JetStream, error) {
		return mockJS, nil
	})
	defer cleanup()

	mockJS.On("CreateOrUpdateStream", mock.Anything, mock.Anything).Return(nil, nil)
	mockJS.On("Publish", mock.Anything, mock.Anything, mock.Anything).Return(&jetstream.PubAck{}, nil)

	var calledSubject string
	var calledErr error
	var calledLatency time.Duration

	pub, err := NewPublisher(&nats.Conn{}, pubsub.PublisherOptions{
		StreamName:    "TEST",
		SubjectPrefix: "PREFIX",
		OnPublish: func(subject string, err error, latency time.Duration) {
			calledSubject = subject
			calledErr = err
			calledLatency = latency
		},
	})
	require.NoError(t, err)

	err = pub.Publish(context.Background(), "test.subject", []byte("hello"))
	require.NoError(t, err)

	assert.Equal(t, "PREFIX.test.subject", calledSubject)
	assert.NoError(t, calledErr)
	assert.Greater(t, calledLatency, time.Duration(0))
}

func TestPublisher_Close(t *testing.T) {
	mockJS := new(MockJetStream)
	cleanup := SetJetStreamNew(func(nc *nats.Conn) (jetstream.JetStream, error) {
		return mockJS, nil
	})
	defer cleanup()

	mockJS.On("CreateOrUpdateStream", mock.Anything, mock.Anything).Return(nil, nil)

	pub, err := NewPublisher(&nats.Conn{}, pubsub.PublisherOptions{
		StreamName: "TEST",
	})
	require.NoError(t, err)

	err = pub.Close()
	assert.NoError(t, err)
}

func TestNewConsumer_WithMock(t *testing.T) {
	mockJS := new(MockJetStream)
	cleanup := SetJetStreamNew(func(nc *nats.Conn) (jetstream.JetStream, error) {
		return mockJS, nil
	})
	defer cleanup()

	consumer, err := NewConsumer(&nats.Conn{}, pubsub.ConsumerOptions{
		StreamName:   "TEST",
		ConsumerName: "test-consumer",
		NumWorkers:   4,
	})

	require.NoError(t, err)
	assert.NotNil(t, consumer)
}

func TestNewConsumer_JetStreamError(t *testing.T) {
	cleanup := SetJetStreamNew(func(nc *nats.Conn) (jetstream.JetStream, error) {
		return nil, errors.New("jetstream error")
	})
	defer cleanup()

	_, err := NewConsumer(&nats.Conn{}, pubsub.ConsumerOptions{
		StreamName: "TEST",
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "jetstream error")
}

func TestNewConsumer_RequiresStreamName(t *testing.T) {
	mockJS := new(MockJetStream)
	cleanup := SetJetStreamNew(func(nc *nats.Conn) (jetstream.JetStream, error) {
		return mockJS, nil
	})
	defer cleanup()

	_, err := NewConsumer(&nats.Conn{}, pubsub.ConsumerOptions{
		StreamName: "", // Empty stream name
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "stream name is required")
}

func TestNewConsumer_DefaultOptions(t *testing.T) {
	mockJS := new(MockJetStream)
	cleanup := SetJetStreamNew(func(nc *nats.Conn) (jetstream.JetStream, error) {
		return mockJS, nil
	})
	defer cleanup()

	consumer, err := NewConsumer(&nats.Conn{}, pubsub.ConsumerOptions{
		StreamName: "TEST",
		// All other options left at zero values
	})

	require.NoError(t, err)
	assert.NotNil(t, consumer)

	// Verify defaults were applied
	jsc := consumer.(*jetStreamConsumer)
	assert.Equal(t, 16, jsc.opts.NumWorkers)
	assert.Equal(t, 100, jsc.opts.ChannelBufSize)
	assert.Equal(t, 5*time.Second, jsc.opts.DrainTimeout)
	assert.Equal(t, 30*time.Second, jsc.opts.ShutdownTimeout)
}

func TestPublisher_NoPrefix(t *testing.T) {
	mockJS := new(MockJetStream)
	cleanup := SetJetStreamNew(func(nc *nats.Conn) (jetstream.JetStream, error) {
		return mockJS, nil
	})
	defer cleanup()

	mockJS.On("CreateOrUpdateStream", mock.Anything, mock.Anything).Return(nil, nil)
	// Without prefix, subject should not be prefixed
	mockJS.On("Publish", mock.Anything, "test.subject", []byte("hello")).Return(&jetstream.PubAck{}, nil)

	pub, err := NewPublisher(&nats.Conn{}, pubsub.PublisherOptions{
		StreamName:    "TEST",
		SubjectPrefix: "", // No prefix
	})
	require.NoError(t, err)

	err = pub.Publish(context.Background(), "test.subject", []byte("hello"))
	assert.NoError(t, err)
	mockJS.AssertExpectations(t)
}

func TestPublisher_NoStreamName(t *testing.T) {
	mockJS := new(MockJetStream)
	cleanup := SetJetStreamNew(func(nc *nats.Conn) (jetstream.JetStream, error) {
		return mockJS, nil
	})
	defer cleanup()

	// Without stream name, CreateOrUpdateStream should not be called
	pub, err := NewPublisher(&nats.Conn{}, pubsub.PublisherOptions{
		StreamName: "", // No stream name
	})

	require.NoError(t, err)
	assert.NotNil(t, pub)
	mockJS.AssertNotCalled(t, "CreateOrUpdateStream")
}

func TestConsumer_Start_StreamError(t *testing.T) {
	mockJS := new(MockJetStream)
	cleanup := SetJetStreamNew(func(nc *nats.Conn) (jetstream.JetStream, error) {
		return mockJS, nil
	})
	defer cleanup()

	mockJS.On("CreateOrUpdateStream", mock.Anything, mock.Anything).Return(nil, errors.New("stream creation failed"))

	consumer, err := NewConsumer(&nats.Conn{}, pubsub.ConsumerOptions{
		StreamName:   "TEST",
		ConsumerName: "test-consumer",
		NumWorkers:   2,
	})
	require.NoError(t, err)

	err = consumer.Start(context.Background(), func(ctx context.Context, msg pubsub.Message) error {
		return nil
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to ensure stream")
}

func TestConsumer_Start_ConsumerCreationError(t *testing.T) {
	mockJS := new(MockJetStream)
	cleanup := SetJetStreamNew(func(nc *nats.Conn) (jetstream.JetStream, error) {
		return mockJS, nil
	})
	defer cleanup()

	mockJS.On("CreateOrUpdateStream", mock.Anything, mock.Anything).Return(nil, nil)
	mockJS.On("CreateOrUpdateConsumer", mock.Anything, "TEST", mock.Anything).Return(nil, errors.New("consumer creation failed"))

	consumer, err := NewConsumer(&nats.Conn{}, pubsub.ConsumerOptions{
		StreamName:   "TEST",
		ConsumerName: "test-consumer",
		NumWorkers:   2,
	})
	require.NoError(t, err)

	err = consumer.Start(context.Background(), func(ctx context.Context, msg pubsub.Message) error {
		return nil
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create consumer")
}

func TestConsumer_Start_ConsumeError(t *testing.T) {
	mockJS := new(MockJetStream)
	mockConsumer := new(MockConsumer)
	cleanup := SetJetStreamNew(func(nc *nats.Conn) (jetstream.JetStream, error) {
		return mockJS, nil
	})
	defer cleanup()

	mockJS.On("CreateOrUpdateStream", mock.Anything, mock.Anything).Return(nil, nil)
	mockJS.On("CreateOrUpdateConsumer", mock.Anything, "TEST", mock.Anything).Return(mockConsumer, nil)
	mockConsumer.On("Consume", mock.Anything).Return(nil, errors.New("consume failed"))

	consumer, err := NewConsumer(&nats.Conn{}, pubsub.ConsumerOptions{
		StreamName:   "TEST",
		ConsumerName: "test-consumer",
		NumWorkers:   2,
	})
	require.NoError(t, err)

	err = consumer.Start(context.Background(), func(ctx context.Context, msg pubsub.Message) error {
		return nil
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to start consumer")
}

func TestConsumer_Start_Success(t *testing.T) {
	mockJS := new(MockJetStream)
	mockConsumer := new(MockConsumer)
	mockCC := NewMockConsumeContext()
	cleanup := SetJetStreamNew(func(nc *nats.Conn) (jetstream.JetStream, error) {
		return mockJS, nil
	})
	defer cleanup()

	mockJS.On("CreateOrUpdateStream", mock.Anything, mock.Anything).Return(nil, nil)
	mockJS.On("CreateOrUpdateConsumer", mock.Anything, "TEST", mock.Anything).Return(mockConsumer, nil)
	mockConsumer.On("Consume", mock.Anything).Return(mockCC, nil)
	mockCC.On("Stop").Return()

	consumer, err := NewConsumer(&nats.Conn{}, pubsub.ConsumerOptions{
		StreamName:      "TEST",
		ConsumerName:    "test-consumer",
		NumWorkers:      2,
		DrainTimeout:    100 * time.Millisecond,
		ShutdownTimeout: 100 * time.Millisecond,
	})
	require.NoError(t, err)

	// Start consumer in goroutine
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- consumer.Start(ctx, func(ctx context.Context, msg pubsub.Message) error {
			return nil
		})
	}()

	// Give it time to start
	time.Sleep(50 * time.Millisecond)

	// Cancel context to trigger shutdown
	cancel()

	// Wait for Start to return
	select {
	case err := <-errCh:
		assert.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return in time")
	}
}

func TestConsumer_Start_WithFilterSubject(t *testing.T) {
	mockJS := new(MockJetStream)
	mockConsumer := new(MockConsumer)
	mockCC := NewMockConsumeContext()
	cleanup := SetJetStreamNew(func(nc *nats.Conn) (jetstream.JetStream, error) {
		return mockJS, nil
	})
	defer cleanup()

	mockJS.On("CreateOrUpdateStream", mock.Anything, mock.MatchedBy(func(cfg jetstream.StreamConfig) bool {
		return cfg.Subjects[0] == "custom.filter.>"
	})).Return(nil, nil)
	mockJS.On("CreateOrUpdateConsumer", mock.Anything, "TEST", mock.MatchedBy(func(cfg jetstream.ConsumerConfig) bool {
		return cfg.FilterSubject == "custom.filter.>"
	})).Return(mockConsumer, nil)
	mockConsumer.On("Consume", mock.Anything).Return(mockCC, nil)
	mockCC.On("Stop").Return()

	consumer, err := NewConsumer(&nats.Conn{}, pubsub.ConsumerOptions{
		StreamName:      "TEST",
		ConsumerName:    "test-consumer",
		FilterSubject:   "custom.filter.>",
		NumWorkers:      1,
		DrainTimeout:    100 * time.Millisecond,
		ShutdownTimeout: 100 * time.Millisecond,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- consumer.Start(ctx, func(ctx context.Context, msg pubsub.Message) error {
			return nil
		})
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		assert.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return in time")
	}
}

func TestConsumer_Start_DefaultConsumerName(t *testing.T) {
	mockJS := new(MockJetStream)
	mockConsumer := new(MockConsumer)
	mockCC := NewMockConsumeContext()
	cleanup := SetJetStreamNew(func(nc *nats.Conn) (jetstream.JetStream, error) {
		return mockJS, nil
	})
	defer cleanup()

	mockJS.On("CreateOrUpdateStream", mock.Anything, mock.Anything).Return(nil, nil)
	mockJS.On("CreateOrUpdateConsumer", mock.Anything, "TEST", mock.MatchedBy(func(cfg jetstream.ConsumerConfig) bool {
		return cfg.Durable == "consumer" // Default consumer name
	})).Return(mockConsumer, nil)
	mockConsumer.On("Consume", mock.Anything).Return(mockCC, nil)
	mockCC.On("Stop").Return()

	consumer, err := NewConsumer(&nats.Conn{}, pubsub.ConsumerOptions{
		StreamName:      "TEST",
		ConsumerName:    "", // Empty, should use default
		NumWorkers:      1,
		DrainTimeout:    100 * time.Millisecond,
		ShutdownTimeout: 100 * time.Millisecond,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- consumer.Start(ctx, func(ctx context.Context, msg pubsub.Message) error {
			return nil
		})
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		assert.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return in time")
	}
}

func TestConsumer_Start_WithMessageProcessing(t *testing.T) {
	mockJS := new(MockJetStream)
	mockConsumer := new(MockConsumer)
	mockCC := NewMockConsumeContext()
	cleanup := SetJetStreamNew(func(nc *nats.Conn) (jetstream.JetStream, error) {
		return mockJS, nil
	})
	defer cleanup()

	var messageHandler jetstream.MessageHandler

	mockJS.On("CreateOrUpdateStream", mock.Anything, mock.Anything).Return(nil, nil)
	mockJS.On("CreateOrUpdateConsumer", mock.Anything, "TEST", mock.Anything).Return(mockConsumer, nil)
	mockConsumer.On("Consume", mock.Anything).Run(func(args mock.Arguments) {
		messageHandler = args.Get(0).(jetstream.MessageHandler)
	}).Return(mockCC, nil)
	mockCC.On("Stop").Return()

	processed := make(chan string, 10)
	consumer, err := NewConsumer(&nats.Conn{}, pubsub.ConsumerOptions{
		StreamName:      "TEST",
		ConsumerName:    "test-consumer",
		NumWorkers:      2,
		DrainTimeout:    200 * time.Millisecond,
		ShutdownTimeout: 200 * time.Millisecond,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- consumer.Start(ctx, func(ctx context.Context, msg pubsub.Message) error {
			processed <- msg.Subject()
			msg.Ack()
			return nil
		})
	}()

	// Wait for consumer to start
	time.Sleep(50 * time.Millisecond)

	// Simulate sending messages
	mockMsg1 := NewMockMsg("test.subject1", []byte("data1"))
	mockMsg1.On("Ack").Return(nil)
	mockMsg2 := NewMockMsg("test.subject2", []byte("data2"))
	mockMsg2.On("Ack").Return(nil)

	messageHandler(mockMsg1)
	messageHandler(mockMsg2)

	// Wait for messages to be processed
	time.Sleep(100 * time.Millisecond)

	// Cancel to trigger shutdown
	cancel()

	select {
	case err := <-errCh:
		assert.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return in time")
	}

	// Verify messages were processed
	close(processed)
	subjects := make([]string, 0)
	for subject := range processed {
		subjects = append(subjects, subject)
	}
	assert.Len(t, subjects, 2)
}

func TestConsumer_Start_WithPartitioner(t *testing.T) {
	mockJS := new(MockJetStream)
	mockConsumer := new(MockConsumer)
	mockCC := NewMockConsumeContext()
	cleanup := SetJetStreamNew(func(nc *nats.Conn) (jetstream.JetStream, error) {
		return mockJS, nil
	})
	defer cleanup()

	var messageHandler jetstream.MessageHandler

	mockJS.On("CreateOrUpdateStream", mock.Anything, mock.Anything).Return(nil, nil)
	mockJS.On("CreateOrUpdateConsumer", mock.Anything, "TEST", mock.Anything).Return(mockConsumer, nil)
	mockConsumer.On("Consume", mock.Anything).Run(func(args mock.Arguments) {
		messageHandler = args.Get(0).(jetstream.MessageHandler)
	}).Return(mockCC, nil)
	mockCC.On("Stop").Return()

	processed := make(chan string, 10)
	consumer, err := NewConsumer(&nats.Conn{}, pubsub.ConsumerOptions{
		StreamName:      "TEST",
		ConsumerName:    "test-consumer",
		NumWorkers:      4,
		DrainTimeout:    200 * time.Millisecond,
		ShutdownTimeout: 200 * time.Millisecond,
		Partitioner: func(data []byte) uint32 {
			// Simple partitioner that uses first byte
			if len(data) > 0 {
				return uint32(data[0])
			}
			return 0
		},
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- consumer.Start(ctx, func(ctx context.Context, msg pubsub.Message) error {
			processed <- msg.Subject()
			msg.Ack()
			return nil
		})
	}()

	// Wait for consumer to start
	time.Sleep(50 * time.Millisecond)

	// Send messages with different data to test partitioning
	mockMsg1 := NewMockMsg("test.subject1", []byte("adata"))
	mockMsg1.On("Ack").Return(nil)
	mockMsg2 := NewMockMsg("test.subject2", []byte("bdata"))
	mockMsg2.On("Ack").Return(nil)

	messageHandler(mockMsg1)
	messageHandler(mockMsg2)

	// Wait for messages to be processed
	time.Sleep(100 * time.Millisecond)

	cancel()

	select {
	case err := <-errCh:
		assert.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return in time")
	}

	close(processed)
	subjects := make([]string, 0)
	for subject := range processed {
		subjects = append(subjects, subject)
	}
	assert.Len(t, subjects, 2)
}

func TestConsumer_Start_WithOnMessageCallback(t *testing.T) {
	mockJS := new(MockJetStream)
	mockConsumer := new(MockConsumer)
	mockCC := NewMockConsumeContext()
	cleanup := SetJetStreamNew(func(nc *nats.Conn) (jetstream.JetStream, error) {
		return mockJS, nil
	})
	defer cleanup()

	var messageHandler jetstream.MessageHandler

	mockJS.On("CreateOrUpdateStream", mock.Anything, mock.Anything).Return(nil, nil)
	mockJS.On("CreateOrUpdateConsumer", mock.Anything, "TEST", mock.Anything).Return(mockConsumer, nil)
	mockConsumer.On("Consume", mock.Anything).Run(func(args mock.Arguments) {
		messageHandler = args.Get(0).(jetstream.MessageHandler)
	}).Return(mockCC, nil)
	mockCC.On("Stop").Return()

	callbackCalled := make(chan struct{}, 10)
	consumer, err := NewConsumer(&nats.Conn{}, pubsub.ConsumerOptions{
		StreamName:      "TEST",
		ConsumerName:    "test-consumer",
		NumWorkers:      1,
		DrainTimeout:    200 * time.Millisecond,
		ShutdownTimeout: 200 * time.Millisecond,
		OnMessage: func(subject string, err error, latency time.Duration) {
			callbackCalled <- struct{}{}
		},
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- consumer.Start(ctx, func(ctx context.Context, msg pubsub.Message) error {
			msg.Ack()
			return nil
		})
	}()

	time.Sleep(50 * time.Millisecond)

	mockMsg := NewMockMsg("test.subject", []byte("data"))
	mockMsg.On("Ack").Return(nil)
	messageHandler(mockMsg)

	// Wait for callback to be called
	select {
	case <-callbackCalled:
		// Success
	case <-time.After(500 * time.Millisecond):
		t.Fatal("OnMessage callback was not called")
	}

	cancel()

	select {
	case err := <-errCh:
		assert.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return in time")
	}
}

func TestConsumer_DispatchDuringClosing(t *testing.T) {
	mockJS := new(MockJetStream)
	mockConsumer := new(MockConsumer)
	mockCC := NewMockConsumeContext()
	cleanup := SetJetStreamNew(func(nc *nats.Conn) (jetstream.JetStream, error) {
		return mockJS, nil
	})
	defer cleanup()

	var messageHandler jetstream.MessageHandler

	mockJS.On("CreateOrUpdateStream", mock.Anything, mock.Anything).Return(nil, nil)
	mockJS.On("CreateOrUpdateConsumer", mock.Anything, "TEST", mock.Anything).Return(mockConsumer, nil)
	mockConsumer.On("Consume", mock.Anything).Run(func(args mock.Arguments) {
		messageHandler = args.Get(0).(jetstream.MessageHandler)
	}).Return(mockCC, nil)
	mockCC.On("Stop").Return()

	consumer, err := NewConsumer(&nats.Conn{}, pubsub.ConsumerOptions{
		StreamName:      "TEST",
		ConsumerName:    "test-consumer",
		NumWorkers:      1,
		DrainTimeout:    100 * time.Millisecond,
		ShutdownTimeout: 100 * time.Millisecond,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- consumer.Start(ctx, func(ctx context.Context, msg pubsub.Message) error {
			msg.Ack()
			return nil
		})
	}()

	time.Sleep(50 * time.Millisecond)

	// Cancel context first to trigger closing state
	cancel()

	// Wait a bit for closing flag to be set
	time.Sleep(50 * time.Millisecond)

	// Now send a message while closing - it should be NAK'd
	mockMsg := NewMockMsg("test.subject", []byte("data"))
	mockMsg.On("Nak").Return(nil)

	// This message arrives while closing
	messageHandler(mockMsg)

	select {
	case err := <-errCh:
		assert.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return in time")
	}

	// Verify Nak was called (message was rejected during closing)
	mockMsg.AssertCalled(t, "Nak")
}

func TestConsumer_DrainTimeoutWithInFlight(t *testing.T) {
	mockJS := new(MockJetStream)
	mockConsumer := new(MockConsumer)
	mockCC := NewMockConsumeContext()
	cleanup := SetJetStreamNew(func(nc *nats.Conn) (jetstream.JetStream, error) {
		return mockJS, nil
	})
	defer cleanup()

	var messageHandler jetstream.MessageHandler

	mockJS.On("CreateOrUpdateStream", mock.Anything, mock.Anything).Return(nil, nil)
	mockJS.On("CreateOrUpdateConsumer", mock.Anything, "TEST", mock.Anything).Return(mockConsumer, nil)
	mockConsumer.On("Consume", mock.Anything).Run(func(args mock.Arguments) {
		messageHandler = args.Get(0).(jetstream.MessageHandler)
	}).Return(mockCC, nil)
	mockCC.On("Stop").Return()

	consumer, err := NewConsumer(&nats.Conn{}, pubsub.ConsumerOptions{
		StreamName:      "TEST",
		ConsumerName:    "test-consumer",
		NumWorkers:      1,
		ChannelBufSize:  10,
		DrainTimeout:    100 * time.Millisecond, // Short timeout
		ShutdownTimeout: 500 * time.Millisecond,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- consumer.Start(ctx, func(ctx context.Context, msg pubsub.Message) error {
			// Simulate slow processing that exceeds drain timeout
			time.Sleep(300 * time.Millisecond)
			msg.Ack()
			return nil
		})
	}()

	time.Sleep(50 * time.Millisecond)

	// Send a message that will take a long time to process
	mockMsg := NewMockMsg("test.subject", []byte("data"))
	mockMsg.On("Ack").Return(nil)
	messageHandler(mockMsg)

	// Wait a bit for message to be dispatched
	time.Sleep(50 * time.Millisecond)

	// Cancel while message is still processing
	cancel()

	select {
	case err := <-errCh:
		assert.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("Start did not return in time")
	}
}

func TestConsumer_DrainTimeoutWithBlockedDispatch(t *testing.T) {
	mockJS := new(MockJetStream)
	mockConsumer := new(MockConsumer)
	mockCC := NewMockConsumeContext()
	cleanup := SetJetStreamNew(func(nc *nats.Conn) (jetstream.JetStream, error) {
		return mockJS, nil
	})
	defer cleanup()

	var messageHandler jetstream.MessageHandler

	mockJS.On("CreateOrUpdateStream", mock.Anything, mock.Anything).Return(nil, nil)
	mockJS.On("CreateOrUpdateConsumer", mock.Anything, "TEST", mock.Anything).Return(mockConsumer, nil)
	mockConsumer.On("Consume", mock.Anything).Run(func(args mock.Arguments) {
		messageHandler = args.Get(0).(jetstream.MessageHandler)
	}).Return(mockCC, nil)
	mockCC.On("Stop").Return()

	consumer, err := NewConsumer(&nats.Conn{}, pubsub.ConsumerOptions{
		StreamName:      "TEST",
		ConsumerName:    "test-consumer",
		NumWorkers:      1,
		ChannelBufSize:  1, // Very small buffer to cause blocking
		DrainTimeout:    50 * time.Millisecond,
		ShutdownTimeout: 500 * time.Millisecond,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- consumer.Start(ctx, func(ctx context.Context, msg pubsub.Message) error {
			// Block processing so channel fills up
			time.Sleep(500 * time.Millisecond)
			msg.Ack()
			return nil
		})
	}()

	time.Sleep(50 * time.Millisecond)

	// Send messages to fill the channel buffer
	mockMsg1 := NewMockMsg("test.subject1", []byte("data1"))
	mockMsg1.On("Ack").Return(nil)
	mockMsg1.On("Nak").Return(nil)
	mockMsg2 := NewMockMsg("test.subject2", []byte("data2"))
	mockMsg2.On("Ack").Return(nil)
	mockMsg2.On("Nak").Return(nil)

	// First message goes to processing
	messageHandler(mockMsg1)

	// Second message fills the buffer
	go messageHandler(mockMsg2)

	// Wait a bit
	time.Sleep(30 * time.Millisecond)

	// Third message will block in dispatch because channel is full
	// This keeps inFlight > 0 during drain
	mockMsg3 := NewMockMsg("test.subject3", []byte("data3"))
	mockMsg3.On("Nak").Return(nil)
	go messageHandler(mockMsg3)

	// Let dispatch start and block
	time.Sleep(20 * time.Millisecond)

	// Cancel to trigger shutdown - dispatch is still blocked with inFlight > 0
	cancel()

	select {
	case err := <-errCh:
		assert.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("Start did not return in time")
	}
}
