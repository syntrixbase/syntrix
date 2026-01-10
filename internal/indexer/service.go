package indexer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/syntrixbase/syntrix/internal/indexer/internal/encoding"
	"github.com/syntrixbase/syntrix/internal/indexer/internal/manager"
	"github.com/syntrixbase/syntrix/internal/indexer/internal/template"
	"github.com/syntrixbase/syntrix/internal/puller"
)

// Config holds the Indexer service configuration.
type Config struct {
	// TemplatePath is the path to the index templates YAML file.
	TemplatePath string

	// PullerAddress is the gRPC address of the Puller service.
	// If empty, uses in-process Puller (LocalService).
	PullerAddress string

	// ProgressPath is the path to store progress markers.
	// Defaults to "data/indexer/progress".
	ProgressPath string

	// ConsumerID is the ID used when subscribing to Puller.
	// Defaults to "indexer".
	ConsumerID string
}

// service implements LocalService.
type service struct {
	cfg     Config
	logger  *slog.Logger
	manager *manager.Manager

	// Puller subscription
	pullerSvc puller.Service

	// State
	mu       sync.RWMutex
	running  bool
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	progress string // last progress marker

	// Stats
	eventsApplied atomic.Int64
	lastEventTime atomic.Int64
}

// NewService creates a new Indexer service.
// pullerSvc can be nil if not subscribing to Puller (for testing).
func NewService(cfg Config, pullerSvc puller.Service, logger *slog.Logger) LocalService {
	if cfg.ConsumerID == "" {
		cfg.ConsumerID = "indexer"
	}
	if cfg.ProgressPath == "" {
		cfg.ProgressPath = "data/indexer/progress"
	}

	return &service{
		cfg:       cfg,
		logger:    logger.With("component", "indexer"),
		manager:   manager.New(),
		pullerSvc: pullerSvc,
	}
}

// Start starts the indexer service.
func (s *service) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return errors.New("service already running")
	}

	// Load templates
	if s.cfg.TemplatePath != "" {
		if err := s.manager.LoadTemplates(s.cfg.TemplatePath); err != nil {
			return fmt.Errorf("failed to load templates: %w", err)
		}
		s.logger.Info("loaded index templates",
			"path", s.cfg.TemplatePath,
			"count", len(s.manager.Templates()))
	}

	// TODO: Load progress marker from disk

	// Start Puller subscription if configured
	if s.pullerSvc != nil {
		subCtx, cancel := context.WithCancel(ctx)
		s.cancel = cancel
		s.running = true

		s.wg.Add(1)
		go s.subscriptionLoop(subCtx)
	} else {
		s.running = true
	}

	s.logger.Info("indexer service started")
	return nil
}

// Stop gracefully stops the indexer service.
func (s *service) Stop(ctx context.Context) error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}

	if s.cancel != nil {
		s.cancel()
	}
	s.running = false
	s.mu.Unlock()

	// Wait for subscription loop to finish
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		s.logger.Info("indexer service stopped")
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// subscriptionLoop runs the Puller subscription.
func (s *service) subscriptionLoop(ctx context.Context) {
	defer s.wg.Done()

	s.logger.Info("starting puller subscription",
		"consumerID", s.cfg.ConsumerID,
		"progress", s.progress)

	events := s.pullerSvc.Subscribe(ctx, s.cfg.ConsumerID, s.progress)

	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-events:
			if !ok {
				s.logger.Warn("puller subscription closed, will reconnect")
				// Reconnect after backoff
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Second):
					events = s.pullerSvc.Subscribe(ctx, s.cfg.ConsumerID, s.progress)
					continue
				}
			}

			if evt.Change != nil {
				if err := s.ApplyEvent(ctx, evt.Change); err != nil {
					s.logger.Error("failed to apply event",
						"eventID", evt.Change.EventID,
						"error", err)
				}
			}

			// Update progress marker
			if evt.Progress != "" {
				s.mu.Lock()
				s.progress = evt.Progress
				s.mu.Unlock()
				// TODO: Persist progress marker to disk periodically
			}
		}
	}
}

// ApplyEvent applies a single change event to the indexes.
func (s *service) ApplyEvent(ctx context.Context, evt *ChangeEvent) error {
	if evt == nil {
		return nil
	}

	// Get collection path from the event
	var collection string
	if evt.FullDocument != nil {
		collection = evt.FullDocument.Collection
	} else {
		// For deletes without FullDocument, we cannot update indexes
		// This is handled by includeDeleted templates only
		return nil
	}

	if collection == "" {
		return nil
	}

	// Match templates for this collection
	matches := s.manager.MatchTemplatesForCollection(collection)
	if len(matches) == 0 {
		// No indexes for this collection
		return nil
	}

	// Process each matching template
	for _, match := range matches {
		if err := s.applyEventToTemplate(ctx, evt, match.Template); err != nil {
			s.logger.Error("failed to apply event to template",
				"template", match.Template.Name,
				"eventID", evt.EventID,
				"error", err)
			// Continue with other templates
		}
	}

	s.eventsApplied.Add(1)
	s.lastEventTime.Store(time.Now().Unix())

	return nil
}

// applyEventToTemplate applies an event to a specific template's shard.
func (s *service) applyEventToTemplate(ctx context.Context, evt *ChangeEvent, tmpl *template.Template) error {
	doc := evt.FullDocument
	if doc == nil {
		return nil
	}

	// Skip deleted documents unless template includes them
	if doc.Deleted && !tmpl.IncludeDeleted {
		// Delete from index
		shard := s.manager.GetOrCreateShard(
			evt.DatabaseID,
			tmpl.NormalizedPattern(),
			tmpl.Identity(),
			tmpl.CollectionPattern,
		)
		shard.Delete(doc.Id)
		return nil
	}

	// Build OrderKey from document fields
	orderKey, err := s.buildOrderKey(doc.Data, tmpl)
	if err != nil {
		return fmt.Errorf("failed to build order key: %w", err)
	}

	// Get or create shard
	shard := s.manager.GetOrCreateShard(
		evt.DatabaseID,
		tmpl.NormalizedPattern(),
		tmpl.Identity(),
		tmpl.CollectionPattern,
	)

	// Upsert document
	shard.Upsert(doc.Id, orderKey)

	return nil
}

// buildOrderKey builds an OrderKey from document data and template fields.
func (s *service) buildOrderKey(data map[string]any, tmpl *template.Template) ([]byte, error) {
	fields := make([]encoding.Field, len(tmpl.Fields))

	for i, tf := range tmpl.Fields {
		value := extractFieldValue(data, tf.Field)

		// Convert template direction to encoding direction
		var dir encoding.Direction
		if tf.Order == template.Desc {
			dir = encoding.Desc
		} else {
			dir = encoding.Asc
		}

		fields[i] = encoding.Field{
			Value:     value,
			Direction: dir,
		}
	}

	// Note: docID is already appended by encoding.Encode as tie-breaker
	// We use empty string here since the shard stores ID separately
	return encoding.Encode(fields, "")
}

// extractFieldValue extracts a field value from document data.
// Supports nested fields with dot notation (e.g., "user.name").
func extractFieldValue(data map[string]any, field string) any {
	// TODO: Support dot notation for nested fields
	return data[field]
}

// Search returns document references matching the query plan.
func (s *service) Search(ctx context.Context, database string, plan Plan) ([]DocRef, error) {
	return s.manager.Search(ctx, database, plan)
}

// Health returns current health status of the indexer.
func (s *service) Health(ctx context.Context) (Health, error) {
	s.mu.RLock()
	running := s.running
	s.mu.RUnlock()

	status := HealthOK
	if !running {
		status = HealthUnhealthy
	}

	// TODO: Check shard health, lag status, etc.

	return Health{
		Status:      status,
		ShardHealth: make(map[string]string),
	}, nil
}

// Stats returns index statistics.
func (s *service) Stats(ctx context.Context) (Stats, error) {
	mgrStats := s.manager.Stats()

	return Stats{
		DatabaseCount: mgrStats.DatabaseCount,
		ShardCount:    mgrStats.ShardCount,
		TemplateCount: mgrStats.TemplateCount,
		EventsApplied: s.eventsApplied.Load(),
		LastEventTime: s.lastEventTime.Load(),
	}, nil
}

// Manager returns the underlying index manager.
func (s *service) Manager() *manager.Manager {
	return s.manager
}
