package trigger

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDeliveryWorker_ProcessTask_NetworkError(t *testing.T) {
	// 1. Setup Worker with default client
	worker := NewDeliveryWorker(nil)

	// 2. Create Task with invalid URL to force network error
	task := &DeliveryTask{
		TriggerID: "trig-1",
		URL:       "http://invalid-url-that-does-not-exist.local",
	}

	// 3. Execute
	err := worker.ProcessTask(context.Background(), task)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "request failed")
}

func TestDeliveryWorker_ProcessTask_InvalidURL(t *testing.T) {
	worker := NewDeliveryWorker(nil)
	task := &DeliveryTask{
		TriggerID: "trig-1",
		URL:       "://invalid-url",
	}
	err := worker.ProcessTask(context.Background(), task)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create request")
}

type MockRoundTripper struct {
	RoundTripFunc func(req *http.Request) (*http.Response, error)
}

func (m *MockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.RoundTripFunc(req)
}
