package watcher

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/syntrixbase/syntrix/internal/puller"
	"github.com/syntrixbase/syntrix/internal/puller/events"
	"github.com/syntrixbase/syntrix/internal/storage"
	"github.com/syntrixbase/syntrix/pkg/model"
)

// WatcherOptions configures the watcher.
type WatcherOptions struct {
	StartFromNow bool
}

type pullerWatcher struct {
	puller puller.Service
	store  storage.DocumentStore
	tenant string
	opts   WatcherOptions
}

// NewWatcher creates a new DocumentWatcher.
func NewWatcher(p puller.Service, store storage.DocumentStore, tenant string, opts WatcherOptions) DocumentWatcher {
	return &pullerWatcher{
		puller: p,
		store:  store,
		tenant: tenant,
		opts:   opts,
	}
}

func (w *pullerWatcher) Watch(ctx context.Context) (<-chan events.SyntrixChangeEvent, error) {
	// 1. Load Checkpoint
	// We use "default" tenant for system checkpoints as per design implication of tenant-scoped keys.
	// Key format: sys/checkpoints/trigger_evaluator/<tenant>
	checkpointKey := fmt.Sprintf("sys/checkpoints/trigger_evaluator/%s", w.tenant)
	checkpointTenant := "default"

	var resumeToken string
	checkpointDoc, err := w.store.Get(ctx, checkpointTenant, checkpointKey)
	if err == nil && checkpointDoc != nil {
		if token, ok := checkpointDoc.Data["token"].(string); ok {
			resumeToken = token
			log.Printf("Resuming trigger watcher for tenant %s from checkpoint: %s", w.tenant, token)
		}
	} else if err == model.ErrNotFound {
		if !w.opts.StartFromNow {
			return nil, fmt.Errorf("checkpoint not found for tenant %s and StartFromNow is false", w.tenant)
		}
		log.Printf("AUDIT: Starting watch from NOW for tenant %s (checkpoint missing)", w.tenant)
	} else {
		log.Printf("Failed to load checkpoint for tenant %s: %v", w.tenant, err)
		return nil, err
	}

	// 2. Start Watch
	consumerID := fmt.Sprintf("trigger-evaluator-%s", w.tenant)
	pullerCh := w.puller.Subscribe(ctx, consumerID, resumeToken)

	outCh := make(chan events.SyntrixChangeEvent)

	go func() {
		defer close(outCh)
		for pEvent := range pullerCh {
			// Filter by tenant
			if pEvent.Change.TenantID != w.tenant {
				continue
			}

			// Convert
			event, err := events.Transform(pEvent)
			if err != nil {
				continue
			}

			select {
			case outCh <- event:
			case <-ctx.Done():
				return
			}
		}
	}()

	return outCh, nil
}

func (w *pullerWatcher) SaveCheckpoint(ctx context.Context, token interface{}) error {
	coll := "sys/checkpoints/trigger_evaluator"
	checkpointKey := fmt.Sprintf("%s/%s", coll, w.tenant)
	checkpointTenant := "default"

	data := map[string]interface{}{
		"token":     token,
		"updatedAt": time.Now().Unix(),
	}

	err := w.store.Update(ctx, checkpointTenant, checkpointKey, data, model.Filters{})
	if err != nil {
		if err == model.ErrNotFound {
			// Create if not exists
			doc := storage.NewStoredDoc(checkpointTenant, coll, w.tenant, data)
			return w.store.Create(ctx, checkpointTenant, doc)
		}
		return err
	}
	return nil
}

// Close releases resources held by the watcher.
func (w *pullerWatcher) Close() error {
	return nil
}
