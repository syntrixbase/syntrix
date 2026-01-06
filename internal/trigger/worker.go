package trigger

import (
	"github.com/syntrixbase/syntrix/internal/identity"
	"github.com/syntrixbase/syntrix/internal/trigger/internal/worker"
)

// DeliveryWorker is an alias for worker.DeliveryWorker interface
type DeliveryWorker = worker.DeliveryWorker

// NewDeliveryWorker creates a new DeliveryWorker.
func NewDeliveryWorker(auth identity.AuthN) DeliveryWorker {
	return worker.NewDeliveryWorker(auth, nil, worker.HTTPClientOptions{}, nil)
}
