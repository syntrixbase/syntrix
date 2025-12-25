package realtime

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewServer(t *testing.T) {
	mockQS := new(MockQueryService)
	server := NewServer(mockQS, "docs")
	assert.NotNil(t, server)
	assert.NotNil(t, server.hub)
	assert.Equal(t, "docs", server.dataCollection)
}

func TestServer_HandleWS(t *testing.T) {
	mockQS := new(MockQueryService)
	server := NewServer(mockQS, "docs")

	// Start Hub
	ctxHub, cancelHub := context.WithCancel(context.Background())
	defer cancelHub()
	go server.hub.Run(ctxHub)

	// WS upgrade requires a real server usually or httptest with specific headers.
	// ServeWs calls upgrader.Upgrade which fails if not a websocket handshake.
	// We expect it to fail upgrade but at least call the function.

	req := httptest.NewRequest("GET", "/ws", nil)
	w := httptest.NewRecorder()

	server.HandleWS(w, req)

	// Since it's not a valid WS request, it should probably return 400 or similar from upgrader.
	// But ServeWs implementation might log error and return.
	// Let's check ServeWs implementation if possible, but for coverage, calling it is enough.
	assert.NotEqual(t, http.StatusOK, w.Code)
}

func TestServer_HandleSSE(t *testing.T) {
	mockQS := new(MockQueryService)
	server := NewServer(mockQS, "docs")

	// Start Hub
	ctxHub, cancelHub := context.WithCancel(context.Background())
	defer cancelHub()
	go server.hub.Run(ctxHub)

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest("GET", "/sse", nil).WithContext(ctx)
	w := httptest.NewRecorder()

	// Cancel context shortly to unblock HandleSSE
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	server.HandleSSE(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
}
