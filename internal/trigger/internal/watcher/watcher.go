package watcher

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/codetrek/syntrix/internal/puller"
	"github.com/codetrek/syntrix/internal/puller/events"
	"github.com/codetrek/syntrix/internal/storage"
	"github.com/codetrek/syntrix/internal/storage/types"
	triggertypes "github.com/codetrek/syntrix/internal/trigger/types"
	"github.com/codetrek/syntrix/pkg/model"
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

func (w *pullerWatcher) Watch(ctx context.Context) (<-chan triggertypes.TriggerEvent, error) {
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
	pullerCh, err := w.puller.Subscribe(ctx, consumerID, resumeToken)
	if err != nil {
		return nil, err
	}

	outCh := make(chan triggertypes.TriggerEvent)

	go func() {
		defer close(outCh)
		for pEvent := range pullerCh {
			// Filter by tenant
			if pEvent.Change.TenantID != w.tenant {
				continue
			}

			// Convert
			event := w.convertEvent(pEvent)

			select {
			case outCh <- event:
			case <-ctx.Done():
				return
			}
		}
	}()

	return outCh, nil
}

func (w *pullerWatcher) convertEvent(pEvent *events.PullerEvent) triggertypes.TriggerEvent {
	evt := triggertypes.TriggerEvent{
		Id:          pEvent.Change.MgoDocID,
		TenantID:    pEvent.Change.TenantID,
		Timestamp:   pEvent.Change.Timestamp,
		ResumeToken: pEvent.Progress,
	}

	// Map OpType
	switch pEvent.Change.OpType {
	case events.OperationInsert:
		evt.Type = triggertypes.EventCreate
		evt.Document = pEvent.Change.FullDocument
	case events.OperationUpdate, events.OperationReplace:
		if pEvent.Change.FullDocument != nil && pEvent.Change.FullDocument.Deleted {
			evt.Type = triggertypes.EventDelete
			// For soft delete, we might have the document
			evt.Document = pEvent.Change.FullDocument
		} else if pEvent.Change.OpType == events.OperationReplace {
			evt.Type = triggertypes.EventCreate
			evt.Document = pEvent.Change.FullDocument
		} else {
			evt.Type = triggertypes.EventUpdate
			evt.Document = pEvent.Change.FullDocument
		}
	case events.OperationDelete:
		evt.Type = triggertypes.EventDelete
		// Construct minimal Before document for Delete events
		evt.Before = &types.Document{
			Collection: pEvent.Change.MgoColl,
			Id:         pEvent.Change.MgoDocID,
			TenantID:   pEvent.Change.TenantID,
		}
	}

	return evt
}

func (w *pullerWatcher) SaveCheckpoint(ctx context.Context, token interface{}) error {
	checkpointKey := fmt.Sprintf("sys/checkpoints/trigger_evaluator/%s", w.tenant)
	checkpointTenant := "default"

	data := map[string]interface{}{
		"token":     token,
		"updatedAt": time.Now().Unix(),
	}

	err := w.store.Update(ctx, checkpointTenant, checkpointKey, data, model.Filters{})
	if err != nil {
		if err == model.ErrNotFound {
			// Create if not exists
			doc := storage.NewDocument(checkpointTenant, checkpointKey, "sys/checkpoints/trigger_evaluator", data)
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
