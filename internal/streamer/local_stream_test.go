package streamer

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLocalStream_Recv_ContextCanceled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)

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
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

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
