// Package manager provides the index manager that routes events to indexes
// and matches queries to templates.
package manager

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/syntrixbase/syntrix/internal/indexer/internal/encoding"
	"github.com/syntrixbase/syntrix/internal/indexer/internal/index"
	"github.com/syntrixbase/syntrix/internal/indexer/internal/template"
	"github.com/syntrixbase/syntrix/internal/puller/events"
)

// ChangeEvent is an alias for the Puller's StoreChangeEvent.
// This is the event type that the Indexer receives from the Puller subscription.
type ChangeEvent = events.StoreChangeEvent

// Errors
var (
	ErrDatabaseNotFound   = errors.New("database not found")
	ErrNoMatchingIndex    = errors.New("no matching index for query")
	ErrIndexNotFound      = errors.New("index not found")
	ErrIndexRebuilding    = errors.New("index is rebuilding")
	ErrIndexNotReady      = errors.New("index not ready")
	ErrTemplateLoadFailed = errors.New("failed to load templates")
	ErrInvalidPlan        = errors.New("invalid query plan")
)

// FilterOp represents the type of filter operation.
type FilterOp string

const (
	FilterEq  FilterOp = "eq"  // equality
	FilterGt  FilterOp = "gt"  // greater than
	FilterLt  FilterOp = "lt"  // less than
	FilterGte FilterOp = "gte" // greater than or equal
	FilterLte FilterOp = "lte" // less than or equal
)

// Filter represents a query filter on a field.
type Filter struct {
	Field string
	Op    FilterOp
	Value any
}

// OrderField represents an ordering specification.
type OrderField struct {
	Field     string
	Direction encoding.Direction
}

// Plan represents a query plan passed from Query Engine.
type Plan struct {
	Collection string       // Concrete collection path (e.g., "users/alice/chats")
	Filters    []Filter     // Prefix/range filters
	OrderBy    []OrderField // Ordering specification
	Limit      int          // Max results
	StartAfter string       // Cursor for pagination (base64-encoded OrderKey)
}

// DocRef represents a document reference with its OrderKey.
type DocRef struct {
	ID       string // Document ID within collection
	OrderKey []byte // Encoded sort key
}

// Manager manages index databases and indexes.
type Manager struct {
	mu        sync.RWMutex
	databases map[string]*index.Database
	templates []template.Template
}

// New creates a new index manager.
func New() *Manager {
	return &Manager{
		databases: make(map[string]*index.Database),
	}
}

// LoadTemplates loads templates from a YAML file.
func (m *Manager) LoadTemplates(path string) error {
	templates, err := template.LoadFromFile(path)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrTemplateLoadFailed, err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.templates = templates
	return nil
}

// LoadTemplatesFromBytes loads templates from YAML bytes.
func (m *Manager) LoadTemplatesFromBytes(data []byte) error {
	templates, err := template.LoadFromBytes(data)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrTemplateLoadFailed, err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.templates = templates
	return nil
}

// Templates returns the loaded templates.
func (m *Manager) Templates() []template.Template {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.templates
}

// GetDatabase returns a database by name, creating it if needed.
func (m *Manager) GetDatabase(name string) *index.Database {
	m.mu.RLock()
	if db, ok := m.databases[name]; ok {
		m.mu.RUnlock()
		return db
	}
	m.mu.RUnlock()

	m.mu.Lock()
	defer m.mu.Unlock()
	if db, ok := m.databases[name]; ok {
		return db
	}
	db := index.NewDatabase(name)
	m.databases[name] = db
	return db
}

// DeleteDatabase removes a database and all its indexes.
func (m *Manager) DeleteDatabase(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.databases, name)
}

// ListDatabases returns all database names.
func (m *Manager) ListDatabases() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := make([]string, 0, len(m.databases))
	for name := range m.databases {
		names = append(names, name)
	}
	return names
}

// MatchTemplatesForCollection returns templates that match a collection path.
func (m *Manager) MatchTemplatesForCollection(collection string) []template.MatchResult {
	m.mu.RLock()
	templates := m.templates
	m.mu.RUnlock()
	return template.MatchTemplates(collection, templates)
}

// SelectBestTemplate selects the best template for a query plan.
// Implements Query-to-Index matching rules.
func (m *Manager) SelectBestTemplate(plan Plan) (*template.Template, error) {
	fmt.Printf("DEBUG: SelectBestTemplate for collection=%s filters=%v orderBy=%v\n", plan.Collection, plan.Filters, plan.OrderBy)
	// Get all matching templates for the collection
	matches := m.MatchTemplatesForCollection(plan.Collection)
	if len(matches) == 0 {
		fmt.Println("DEBUG: No matches for collection")
		return nil, ErrNoMatchingIndex
	}

	// Filter by query compatibility and include pattern score
	var compatible []matchCandidate
	for _, match := range matches {
		if queryScore := m.computeQueryScore(plan, match.Template); queryScore > 0 {
			fmt.Printf("DEBUG: Compatible template %s score=%d\n", match.Template.Name, queryScore)
			compatible = append(compatible, matchCandidate{
				template:     match.Template,
				queryScore:   queryScore,
				patternScore: match.Score,
			})
		} else {
			fmt.Printf("DEBUG: Incompatible template %s score=0\n", match.Template.Name)
		}
	}

	if len(compatible) == 0 {
		fmt.Println("DEBUG: No compatible templates")
		return nil, ErrNoMatchingIndex
	}

	// Select best by: pattern specificity first, then query score, then name (tie-breaker)
	best := compatible[0]
	for _, c := range compatible[1:] {
		// More specific pattern wins (higher fixed segments)
		if best.patternScore.Less(c.patternScore) {
			best = c
		} else if c.patternScore.Less(best.patternScore) {
			// best is still better
		} else if c.queryScore > best.queryScore {
			// Same pattern specificity, higher query score wins
			best = c
		} else if c.queryScore == best.queryScore && c.template.Name < best.template.Name {
			// Same scores, lexicographically smaller name wins
			best = c
		}
	}

	return best.template, nil
}

type matchCandidate struct {
	template     *template.Template
	queryScore   int
	patternScore template.PatternScore
}

// computeQueryScore computes how well a template serves a query.
// Returns 0 if the template cannot serve the query.
// Higher score = better match.
func (m *Manager) computeQueryScore(plan Plan, tmpl *template.Template) int {
	// Step 1: Extract equality filters on index prefix
	eqFields := make(map[string]bool)
	for _, f := range tmpl.Fields {
		found := false
		for _, filter := range plan.Filters {
			if filter.Field == f.Field && filter.Op == FilterEq {
				found = true
				break
			}
		}
		if !found {
			break
		}
		eqFields[f.Field] = true
	}

	// Step 2: Check range filter (at most one, on next field)
	var rangeField string
	rangeCount := 0
	for _, filter := range plan.Filters {
		if eqFields[filter.Field] {
			continue
		}
		if filter.Op == FilterGt || filter.Op == FilterLt || filter.Op == FilterGte || filter.Op == FilterLte {
			rangeCount++
			rangeField = filter.Field
		}
	}
	if rangeCount > 1 {
		return 0 // Multiple range filters not supported
	}

	// Step 3: Determine usable index prefix
	usablePrefixLen := len(eqFields)
	if rangeField != "" {
		// Range filter must be on the next index field
		if usablePrefixLen >= len(tmpl.Fields) {
			return 0
		}
		if tmpl.Fields[usablePrefixLen].Field != rangeField {
			return 0
		}
		usablePrefixLen++
	}

	// Step 4: Check orderBy is prefix of remaining index fields
	orderStart := usablePrefixLen
	for i, orderField := range plan.OrderBy {
		templateIdx := orderStart + i
		if templateIdx >= len(tmpl.Fields) {
			return 0 // orderBy exceeds index
		}
		if tmpl.Fields[templateIdx].Field != orderField.Field {
			return 0 // field mismatch
		}
		// Compare directions (encoding.Direction is int, template.Direction is string)
		var expectedDir template.Direction
		if orderField.Direction == encoding.Asc {
			expectedDir = template.Asc
		} else {
			expectedDir = template.Desc
		}
		if tmpl.Fields[templateIdx].Order != expectedDir {
			return 0 // direction mismatch
		}
	}

	// Score: coverage of filters + orderBy fields
	score := len(eqFields)*10 + len(plan.OrderBy)*5
	if rangeField != "" {
		score += 3
	}
	// Prefer exact matches
	if orderStart+len(plan.OrderBy) == len(tmpl.Fields) {
		score += 1
	}

	return score
}

// Search executes a search on the index.
func (m *Manager) Search(ctx context.Context, database string, plan Plan) ([]DocRef, error) {
	if err := m.validatePlan(plan); err != nil {
		return nil, err
	}

	// Select best template
	tmpl, err := m.SelectBestTemplate(plan)
	if err != nil {
		return nil, err
	}

	// Get or create database
	db := m.GetDatabase(database)

	// Get index
	pattern := tmpl.NormalizedPattern()
	idx := db.GetIndex(pattern, tmpl.Identity())
	if idx == nil {
		return nil, ErrIndexNotReady
	}

	// Build search options
	opts, err := m.buildSearchOptions(plan, tmpl)
	if err != nil {
		return nil, err
	}

	// Execute search
	results := idx.Search(opts)

	// Convert to DocRef
	docRefs := make([]DocRef, len(results))
	for i, r := range results {
		docRefs[i] = DocRef{ID: r.ID, OrderKey: r.OrderKey}
	}

	return docRefs, nil
}

func (m *Manager) validatePlan(plan Plan) error {
	if plan.Collection == "" {
		return fmt.Errorf("%w: collection is required", ErrInvalidPlan)
	}
	return nil
}

func (m *Manager) buildSearchOptions(plan Plan, tmpl *template.Template) (index.SearchOptions, error) {
	opts := index.SearchOptions{
		Limit: plan.Limit,
	}
	if opts.Limit <= 0 {
		opts.Limit = 100 // default
	}

	// TODO: Build lower/upper bounds from filters using OrderKey encoding
	// For now, just handle startAfter
	if plan.StartAfter != "" {
		// Decode base64 cursor
		key, err := encoding.DecodeBase64(plan.StartAfter)
		if err != nil {
			return opts, fmt.Errorf("invalid cursor: %w", err)
		}
		opts.StartAfter = key
	}

	return opts, nil
}

// GetIndex returns an index for the given database, pattern, and template.
func (m *Manager) GetIndex(database, pattern, templateID string) *index.Index {
	db := m.GetDatabase(database)
	return db.GetIndex(pattern, templateID)
}

// GetOrCreateIndex returns or creates an index.
func (m *Manager) GetOrCreateIndex(database, pattern, templateID, rawPattern string) *index.Index {
	db := m.GetDatabase(database)
	return db.GetOrCreateIndex(pattern, templateID, rawPattern)
}

// Stats returns manager statistics.
type Stats struct {
	DatabaseCount int
	IndexCount    int
	TemplateCount int
	DocumentCount int64 // Total indexed documents
	LastEventTime int64 // Unix timestamp of last processed event
	EventsApplied int64 // Total events applied
}

// HealthStatus represents the health status.
type HealthStatus string

const (
	HealthOK        HealthStatus = "ok"
	HealthDegraded  HealthStatus = "degraded"
	HealthUnhealthy HealthStatus = "unhealthy"
)

// IndexHealth represents the health of a single index.
type IndexHealth struct {
	State    string
	DocCount int64
}

// Health represents the health status of the indexer.
type Health struct {
	Status    string                 // Overall status
	Indexes   map[string]IndexHealth // Per-index status (key: database|pattern|templateID)
	LastError string                 // Last error message if any
}

// Stats returns current statistics.
func (m *Manager) Stats() Stats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	indexCount := 0
	for _, db := range m.databases {
		indexCount += db.IndexCount()
	}

	return Stats{
		DatabaseCount: len(m.databases),
		IndexCount:    indexCount,
		TemplateCount: len(m.templates),
	}
}
