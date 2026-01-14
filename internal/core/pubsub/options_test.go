package pubsub

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDefaultConsumerOptions(t *testing.T) {
	defaults := DefaultConsumerOptions()

	assert.Equal(t, 16, defaults.NumWorkers)
	assert.Equal(t, 100, defaults.ChannelBufSize)
	assert.Equal(t, 5*time.Second, defaults.DrainTimeout)
	assert.Equal(t, 30*time.Second, defaults.ShutdownTimeout)
}

func TestConsumerOptionsFields(t *testing.T) {
	opts := ConsumerOptions{
		StreamName:      "TEST_STREAM",
		ConsumerName:    "test-consumer",
		FilterSubject:   "events.>",
		NumWorkers:      8,
		ChannelBufSize:  50,
		DrainTimeout:    10 * time.Second,
		ShutdownTimeout: 60 * time.Second,
		Partitioner: func(data []byte) uint32 {
			return 0
		},
		OnMessage: func(subject string, err error, latency time.Duration) {},
	}

	assert.Equal(t, "TEST_STREAM", opts.StreamName)
	assert.Equal(t, "test-consumer", opts.ConsumerName)
	assert.Equal(t, "events.>", opts.FilterSubject)
	assert.Equal(t, 8, opts.NumWorkers)
	assert.Equal(t, 50, opts.ChannelBufSize)
	assert.Equal(t, 10*time.Second, opts.DrainTimeout)
	assert.Equal(t, 60*time.Second, opts.ShutdownTimeout)
	assert.NotNil(t, opts.Partitioner)
	assert.NotNil(t, opts.OnMessage)
}

func TestPublisherOptionsFields(t *testing.T) {
	opts := PublisherOptions{
		StreamName:    "TEST_STREAM",
		SubjectPrefix: "PREFIX",
		OnPublish:     func(subject string, err error, latency time.Duration) {},
	}

	assert.Equal(t, "TEST_STREAM", opts.StreamName)
	assert.Equal(t, "PREFIX", opts.SubjectPrefix)
	assert.NotNil(t, opts.OnPublish)
}
