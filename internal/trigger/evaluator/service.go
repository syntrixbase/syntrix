package evaluator

import (
	"context"
	"log/slog"
	"sync"

	"github.com/syntrixbase/syntrix/internal/trigger/evaluator/watcher"
	"github.com/syntrixbase/syntrix/internal/trigger/types"
)

// Service evaluates document changes against trigger rules and publishes matched tasks.
type Service interface {
	// LoadTriggers validates and loads trigger rules.
	LoadTriggers(triggers []*types.Trigger) error

	// Start begins watching for changes and evaluating triggers.
	// Blocks until context is cancelled.
	Start(ctx context.Context) error

	// Close releases resources.
	Close() error
}

// service implements the Service interface.
type service struct {
	evaluator Evaluator
	watcher   watcher.DocumentWatcher
	publisher TaskPublisher
	triggers  []*types.Trigger
	mu        sync.RWMutex
}

// LoadTriggers validates and loads the given triggers.
func (s *service) LoadTriggers(triggers []*types.Trigger) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, t := range triggers {
		if err := ValidateTrigger(t); err != nil {
			return err
		}
	}

	s.triggers = triggers
	return nil
}

// Start begins the trigger processing loop.
func (s *service) Start(ctx context.Context) error {
	stream, err := s.watcher.Watch(ctx)
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case evt, ok := <-stream:
			if !ok {
				return nil
			}

			// Guard: skip events with no document data
			if evt.Document == nil && evt.Before == nil {
				slog.Warn("Skipping event with nil Document and Before")
				continue
			}

			s.mu.RLock()
			currentTriggers := s.triggers
			s.mu.RUnlock()

			for _, t := range currentTriggers {
				matched, err := s.evaluator.Evaluate(ctx, t, evt)
				if err != nil {
					slog.Error("Evaluation failed for trigger", "trigger_id", t.ID, "error", err)
					continue
				}
				if matched {
					var collection string
					var documentID string
					var payload map[string]interface{}

					if evt.Document != nil {
						collection = evt.Document.Collection
						documentID = evt.Document.Id
						payload = evt.Document.Data
					} else if evt.Before != nil {
						collection = evt.Before.Collection
						documentID = evt.Before.Id
						payload = evt.Before.Data
					}

					task := &types.DeliveryTask{
						TriggerID:   t.ID,
						Database:    t.Database,
						Event:       string(evt.Type),
						Collection:  collection,
						DocumentID:  documentID,
						Payload:     payload,
						URL:         t.URL,
						Headers:     t.Headers,
						SecretsRef:  t.SecretsRef,
						RetryPolicy: t.RetryPolicy,
						Timeout:     types.Duration(types.DefaultTaskTimeout),
					}
					if s.publisher != nil {
						if err := s.publisher.Publish(ctx, task); err != nil {
							slog.Error("Failed to publish task for trigger", "trigger_id", t.ID, "error", err)
						}
					}
				}
			}

			if evt.Progress != "" {
				if err := s.watcher.SaveCheckpoint(ctx, evt.Progress); err != nil {
					slog.Error("Failed to save checkpoint", "error", err)
				}
			}
		}
	}
}

// Close stops the service and releases resources.
func (s *service) Close() error {
	var errs []error

	if s.watcher != nil {
		if err := s.watcher.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if s.publisher != nil {
		if err := s.publisher.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return errs[0] // Return first error
	}
	return nil
}
