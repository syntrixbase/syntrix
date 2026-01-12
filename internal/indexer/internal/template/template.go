// Package template provides index template loading, validation, and matching.
package template

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Direction specifies sort order for a field.
type Direction string

const (
	Asc  Direction = "asc"
	Desc Direction = "desc"
)

// Field represents an indexed field with its sort direction.
type Field struct {
	Field string    `yaml:"field"`
	Order Direction `yaml:"order"`
}

// Template defines an index template.
type Template struct {
	Name              string  `yaml:"name"`
	CollectionPattern string  `yaml:"collectionPattern"`
	Fields            []Field `yaml:"fields"`
	IncludeDeleted    bool    `yaml:"includeDeleted"`
}

// Identity returns a unique identifier for the template.
// Uses name if provided, otherwise generates from fields signature.
func (t *Template) Identity() string {
	if t.Name != "" {
		return t.Name
	}
	// Generate from fields
	var parts []string
	for _, f := range t.Fields {
		parts = append(parts, fmt.Sprintf("%s:%s", f.Field, f.Order))
	}
	return strings.Join(parts, ",")
}

// NormalizedPattern returns the pattern with variables replaced by *.
func (t *Template) NormalizedPattern() string {
	return NormalizePattern(t.CollectionPattern)
}

// Errors
var (
	ErrEmptyPattern       = errors.New("collection pattern cannot be empty")
	ErrInvalidPattern     = errors.New("invalid collection pattern")
	ErrEmptySegment       = errors.New("pattern contains empty segment")
	ErrDocumentLevel      = errors.New("pattern targets document level, not collection")
	ErrNoFields           = errors.New("template must have at least one field")
	ErrInvalidDirection   = errors.New("field order must be 'asc' or 'desc'")
	ErrDuplicateField     = errors.New("duplicate field in template")
	ErrDuplicateTemplate  = errors.New("duplicate template definition")
	ErrConflictingPattern = errors.New("conflicting patterns with same priority")
)

// varPattern matches {anything} in collection patterns.
var varPattern = regexp.MustCompile(`^\{[^}]+\}$`)

// Config represents the templates configuration file.
type Config struct {
	Templates []Template `yaml:"templates"`
}

// LoadFromFile loads templates from a YAML file.
func LoadFromFile(path string) ([]Template, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read template file: %w", err)
	}
	return LoadFromBytes(data)
}

// LoadFromBytes parses templates from YAML bytes.
func LoadFromBytes(data []byte) ([]Template, error) {
	fmt.Printf("DEBUG: Loading templates from bytes: %s\n", string(data))
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse templates: %w", err)
	}
	fmt.Printf("DEBUG: Loaded %d templates from bytes\n", len(cfg.Templates))

	// Validate each template
	for i := range cfg.Templates {
		if err := ValidateTemplate(&cfg.Templates[i]); err != nil {
			return nil, fmt.Errorf("template %q: %w", cfg.Templates[i].Name, err)
		}
	}

	// Validate no duplicates
	if err := ValidateTemplates(cfg.Templates); err != nil {
		return nil, err
	}

	return cfg.Templates, nil
}

// ValidateTemplate validates a single template definition.
func ValidateTemplate(t *Template) error {
	// Validate pattern
	if t.CollectionPattern == "" {
		return ErrEmptyPattern
	}

	segments := strings.Split(t.CollectionPattern, "/")
	for _, seg := range segments {
		if seg == "" {
			return ErrEmptySegment
		}
	}

	// Check if pattern targets document level (even number of segments typically means document)
	// Actually, collection patterns should have odd number of segments or end with variable
	// For simplicity, we allow any valid segment pattern

	// Validate fields
	if len(t.Fields) == 0 {
		return ErrNoFields
	}

	seen := make(map[string]bool)
	for _, f := range t.Fields {
		if f.Field == "" {
			return fmt.Errorf("field name cannot be empty")
		}
		if f.Order != Asc && f.Order != Desc {
			return fmt.Errorf("field %q: %w", f.Field, ErrInvalidDirection)
		}
		if seen[f.Field] {
			return fmt.Errorf("field %q: %w", f.Field, ErrDuplicateField)
		}
		seen[f.Field] = true
	}

	return nil
}

// ValidateTemplates checks for duplicate template definitions.
// Duplicate = same (normalizedPattern, templateIdentity).
func ValidateTemplates(templates []Template) error {
	seen := make(map[string]*Template)
	for i := range templates {
		t := &templates[i]
		norm := t.NormalizedPattern()
		identity := t.Identity()
		key := norm + "|" + identity

		if existing, ok := seen[key]; ok {
			return fmt.Errorf("%w: %q and %q have same pattern and identity",
				ErrDuplicateTemplate, existing.Name, t.Name)
		}
		seen[key] = t
	}
	return nil
}

// NormalizePattern replaces {var} placeholders with * for comparison.
func NormalizePattern(pattern string) string {
	segments := strings.Split(pattern, "/")
	for i, seg := range segments {
		if varPattern.MatchString(seg) {
			segments[i] = "*"
		}
	}
	return strings.Join(segments, "/")
}

// MatchResult contains a matched template with its priority score.
type MatchResult struct {
	Template *Template
	Score    PatternScore
}

// PatternScore represents the priority of a pattern match.
type PatternScore struct {
	FixedSegments int // Number of non-variable segments
	TotalSegments int // Total number of segments
}

// Less returns true if this score is lower priority than other.
func (s PatternScore) Less(other PatternScore) bool {
	if s.FixedSegments != other.FixedSegments {
		return s.FixedSegments < other.FixedSegments
	}
	return s.TotalSegments < other.TotalSegments
}

// Equal returns true if scores are equal.
func (s PatternScore) Equal(other PatternScore) bool {
	return s.FixedSegments == other.FixedSegments && s.TotalSegments == other.TotalSegments
}

// MatchTemplates finds all templates that match a collection path.
// Returns templates sorted by priority (highest first).
func MatchTemplates(path string, templates []Template) []MatchResult {
	fmt.Printf("DEBUG: MatchTemplates path=%s templates=%d\n", path, len(templates))
	pathSegments := strings.Split(path, "/")
	var results []MatchResult

	for i := range templates {
		t := &templates[i]
		patternSegments := strings.Split(t.CollectionPattern, "/")

		// Must have same number of segments
		if len(pathSegments) != len(patternSegments) {
			continue
		}

		// Check each segment matches
		if !segmentsMatch(pathSegments, patternSegments) {
			continue
		}

		// Calculate score
		score := calculateScore(patternSegments)
		results = append(results, MatchResult{Template: t, Score: score})
	}
	fmt.Printf("DEBUG: MatchTemplates found %d matches\n", len(results))

	// Sort by score (highest first)
	sort.Slice(results, func(i, j int) bool {
		return results[j].Score.Less(results[i].Score)
	})

	return results
}

// segmentsMatch checks if path segments match pattern segments.
func segmentsMatch(path, pattern []string) bool {
	for i := range path {
		if varPattern.MatchString(pattern[i]) {
			// Variable matches any single segment
			continue
		}
		if path[i] != pattern[i] {
			return false
		}
	}
	return true
}

// calculateScore computes the priority score for a pattern.
func calculateScore(segments []string) PatternScore {
	fixed := 0
	for _, seg := range segments {
		if !varPattern.MatchString(seg) {
			fixed++
		}
	}
	return PatternScore{
		FixedSegments: fixed,
		TotalSegments: len(segments),
	}
}

// SelectBestTemplates returns the templates with highest priority.
// If multiple templates have the same priority, all are returned.
func SelectBestTemplates(results []MatchResult) []MatchResult {
	if len(results) == 0 {
		return nil
	}

	bestScore := results[0].Score
	var best []MatchResult
	for _, r := range results {
		if r.Score.Equal(bestScore) {
			best = append(best, r)
		} else {
			break // Results are sorted, so we can stop
		}
	}
	return best
}
