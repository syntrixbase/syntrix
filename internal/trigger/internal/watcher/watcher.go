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
	puller   puller.Service
	store    storage.DocumentStore
	database string
	opts     WatcherOptions
}

// NewWatcher creates a new DocumentWatcher.
func NewWatcher(p puller.Service, store storage.DocumentStore, database string, opts WatcherOptions) DocumentWatcher {
	return &pullerWatcher{
		puller:   p,
		store:    store,
		database: database,
		opts:     opts,
	}
}

func (w *pullerWatcher) Watch(ctx context.Context) (<-chan events.SyntrixChangeEvent, error) {
	// 1. Load Checkpoint
	// We use "default" database for system checkpoints as per design implication of database-scoped keys.
	// Key format: sys/checkpoints/trigger_evaluator/<database>
	checkpointKey := fmt.Sprintf("sys/checkpoints/trigger_evaluator/%s", w.database)
	checkpointDatabase := "default"

	var resumeToken string
	checkpointDoc, err := w.store.Get(ctx, checkpointDatabase, checkpointKey)
	if err == nil && checkpointDoc != nil {
		if token, ok := checkpointDoc.Data["token"].(string); ok {
			resumeToken = token
			log.Printf("Resuming trigger watcher for database %s from checkpoint: %s", w.database, token)
		}
	} else if err == model.ErrNotFound {
		if !w.opts.StartFromNow {
			return nil, fmt.Errorf("checkpoint not found for database %s and StartFromNow is false", w.database)
		}
		log.Printf("AUDIT: Starting watch from NOW for database %s (checkpoint missing)", w.database)
	} else {
		log.Printf("Failed to load checkpoint for database %s: %v", w.database, err)
		return nil, err
	}

	// 2. Start Watch
	consumerID := fmt.Sprintf("trigger-evaluator-%s", w.database)
	pullerCh := w.puller.Subscribe(ctx, consumerID, resumeToken)

	outCh := make(chan events.SyntrixChangeEvent)

	go func() {
		defer close(outCh)
		for pEvent := range pullerCh {
			// Filter by database
			if pEvent.Change.DatabaseID != w.database {
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
	checkpointKey := fmt.Sprintf("%s/%s", coll, w.database)
	checkpointDatabase := "default"

	data := map[string]interface{}{
		"token":     token,
		"updatedAt": time.Now().Unix(),
	}

	err := w.store.Update(ctx, checkpointDatabase, checkpointKey, data, model.Filters{})
	if err != nil {
		if err == model.ErrNotFound {
			// Create if not exists
			doc := storage.NewStoredDoc(checkpointDatabase, coll, w.database, data)
			return w.store.Create(ctx, checkpointDatabase, doc)
		}
		return err
	}
	return nil
}

// Close releases resources held by the watcher.
func (w *pullerWatcher) Close() error {
	return nil
}
