package streamer

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLocalStream_Recv_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	ls := &localStream{
		ctx:      ctx,
		outgoing: make(chan *EventDelivery), // unbuffered
	}

	// Cancel the context immediately
	cancel()

	// Recv should return context error
	_, err := ls.Recv()
	assert.Error(t, err)
}

func TestLocalStream_Recv_ClosedChannel(t *testing.T) {
	ctx := context.Background()

	ls := &localStream{
		ctx:      ctx,
		outgoing: make(chan *EventDelivery, 10),
	}

	// Close outgoing channel
	close(ls.outgoing)

	// Recv should return EOF
	_, err := ls.Recv()
	assert.ErrorIs(t, err, io.EOF)
}
