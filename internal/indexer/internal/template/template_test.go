package template

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadFromBytes(t *testing.T) {
	yaml := `
templates:
  - name: chats_by_timestamp
    collectionPattern: users/{uid}/chats
    fields:
      - { field: timestamp, order: desc }
    includeDeleted: true
  - name: messages_by_sender
    collectionPattern: rooms/{rid}/messages
    fields:
      - { field: senderId, order: asc }
      - { field: timestamp, order: desc }
`

	templates, err := LoadFromBytes([]byte(yaml))
	require.NoError(t, err)
	assert.Len(t, templates, 2)

	assert.Equal(t, "chats_by_timestamp", templates[0].Name)
	assert.Equal(t, "users/{uid}/chats", templates[0].CollectionPattern)
	assert.Len(t, templates[0].Fields, 1)
	assert.Equal(t, "timestamp", templates[0].Fields[0].Field)
	assert.Equal(t, Desc, templates[0].Fields[0].Order)

	assert.Equal(t, "messages_by_sender", templates[1].Name)
	assert.Len(t, templates[1].Fields, 2)
}

func TestLoadFromBytes_InvalidYAML(t *testing.T) {
	_, err := LoadFromBytes([]byte("invalid: yaml: ["))
	assert.Error(t, err)
}

func TestValidateTemplate(t *testing.T) {
	tests := []struct {
		name    string
		tmpl    Template
		wantErr error
	}{
		{
			name: "valid template",
			tmpl: Template{
				Name:              "test",
				CollectionPattern: "users/{uid}/chats",
				Fields:            []Field{{Field: "name", Order: Asc}},
			},
			wantErr: nil,
		},
		{
			name: "empty pattern",
			tmpl: Template{
				Name:              "test",
				CollectionPattern: "",
				Fields:            []Field{{Field: "name", Order: Asc}},
			},
			wantErr: ErrEmptyPattern,
		},
		{
			name: "empty segment",
			tmpl: Template{
				Name:              "test",
				CollectionPattern: "users//chats",
				Fields:            []Field{{Field: "name", Order: Asc}},
			},
			wantErr: ErrEmptySegment,
		},
		{
			name: "no fields",
			tmpl: Template{
				Name:              "test",
				CollectionPattern: "users/{uid}/chats",
				Fields:            []Field{},
			},
			wantErr: ErrNoFields,
		},
		{
			name: "invalid direction",
			tmpl: Template{
				Name:              "test",
				CollectionPattern: "users/{uid}/chats",
				Fields:            []Field{{Field: "name", Order: "invalid"}},
			},
			wantErr: ErrInvalidDirection,
		},
		{
			name: "duplicate field",
			tmpl: Template{
				Name:              "test",
				CollectionPattern: "users/{uid}/chats",
				Fields: []Field{
					{Field: "name", Order: Asc},
					{Field: "name", Order: Desc},
				},
			},
			wantErr: ErrDuplicateField,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTemplate(&tt.tmpl)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateTemplates_Duplicates(t *testing.T) {
	templates := []Template{
		{
			Name:              "tmpl1",
			CollectionPattern: "users/{uid}/chats",
			Fields:            []Field{{Field: "name", Order: Asc}},
		},
		{
			Name:              "tmpl1", // Same name = same identity
			CollectionPattern: "users/{user_id}/chats",
			Fields:            []Field{{Field: "age", Order: Desc}},
		},
	}

	err := ValidateTemplates(templates)
	assert.ErrorIs(t, err, ErrDuplicateTemplate)
}

func TestValidateTemplates_SamePatternDifferentFields(t *testing.T) {
	// Same pattern but different fields = different identity = OK
	templates := []Template{
		{
			Name:              "by_name",
			CollectionPattern: "users/{uid}/chats",
			Fields:            []Field{{Field: "name", Order: Asc}},
		},
		{
			Name:              "by_age",
			CollectionPattern: "users/{user_id}/chats",
			Fields:            []Field{{Field: "age", Order: Desc}},
		},
	}

	err := ValidateTemplates(templates)
	assert.NoError(t, err)
}

func TestNormalizePattern(t *testing.T) {
	tests := []struct {
		pattern string
		want    string
	}{
		{"users/{uid}/chats", "users/*/chats"},
		{"users/{user_id}/chats", "users/*/chats"},
		{"rooms/{rid}/messages", "rooms/*/messages"},
		{"fixed/path/only", "fixed/path/only"},
		{"{a}/{b}/{c}", "*/*/*"},
	}

	for _, tt := range tests {
		got := NormalizePattern(tt.pattern)
		assert.Equal(t, tt.want, got, "pattern: %s", tt.pattern)
	}
}

func TestTemplateIdentity(t *testing.T) {
	t.Run("with name", func(t *testing.T) {
		tmpl := Template{Name: "my_template", Fields: []Field{{Field: "a", Order: Asc}}}
		assert.Equal(t, "my_template", tmpl.Identity())
	})

	t.Run("without name", func(t *testing.T) {
		tmpl := Template{
			Fields: []Field{
				{Field: "name", Order: Asc},
				{Field: "age", Order: Desc},
			},
		}
		assert.Equal(t, "name:asc,age:desc", tmpl.Identity())
	})
}

func TestMatchTemplates(t *testing.T) {
	templates := []Template{
		{
			Name:              "generic_chats",
			CollectionPattern: "users/{uid}/chats",
			Fields:            []Field{{Field: "ts", Order: Desc}},
		},
		{
			Name:              "specific_chats",
			CollectionPattern: "users/alice/chats",
			Fields:            []Field{{Field: "ts", Order: Desc}},
		},
		{
			Name:              "messages",
			CollectionPattern: "rooms/{rid}/messages",
			Fields:            []Field{{Field: "ts", Order: Desc}},
		},
	}

	t.Run("matches generic", func(t *testing.T) {
		results := MatchTemplates("users/bob/chats", templates)
		require.Len(t, results, 1)
		assert.Equal(t, "generic_chats", results[0].Template.Name)
	})

	t.Run("matches specific over generic", func(t *testing.T) {
		results := MatchTemplates("users/alice/chats", templates)
		require.Len(t, results, 2)
		// Specific should be first (higher priority)
		assert.Equal(t, "specific_chats", results[0].Template.Name)
		assert.Equal(t, "generic_chats", results[1].Template.Name)
	})

	t.Run("no match", func(t *testing.T) {
		results := MatchTemplates("other/path", templates)
		assert.Len(t, results, 0)
	})

	t.Run("segment count mismatch", func(t *testing.T) {
		results := MatchTemplates("users/alice/chats/extra", templates)
		assert.Len(t, results, 0)
	})
}

func TestPatternScore(t *testing.T) {
	// More fixed segments = higher priority
	score1 := PatternScore{FixedSegments: 2, TotalSegments: 3}
	score2 := PatternScore{FixedSegments: 1, TotalSegments: 3}

	assert.True(t, score2.Less(score1), "fewer fixed < more fixed")
	assert.False(t, score1.Less(score2))

	// Same fixed, longer total = higher priority
	score3 := PatternScore{FixedSegments: 2, TotalSegments: 4}
	assert.True(t, score1.Less(score3), "shorter < longer when fixed equal")
}

func TestSelectBestTemplates(t *testing.T) {
	t.Run("single best", func(t *testing.T) {
		results := []MatchResult{
			{Template: &Template{Name: "best"}, Score: PatternScore{3, 3}},
			{Template: &Template{Name: "worse"}, Score: PatternScore{2, 3}},
		}
		best := SelectBestTemplates(results)
		require.Len(t, best, 1)
		assert.Equal(t, "best", best[0].Template.Name)
	})

	t.Run("multiple tied", func(t *testing.T) {
		results := []MatchResult{
			{Template: &Template{Name: "a"}, Score: PatternScore{2, 3}},
			{Template: &Template{Name: "b"}, Score: PatternScore{2, 3}},
			{Template: &Template{Name: "c"}, Score: PatternScore{1, 3}},
		}
		best := SelectBestTemplates(results)
		require.Len(t, best, 2)
	})

	t.Run("empty", func(t *testing.T) {
		best := SelectBestTemplates(nil)
		assert.Nil(t, best)
	})
}

func TestSegmentsMatch(t *testing.T) {
	tests := []struct {
		path    string
		pattern string
		want    bool
	}{
		{"users/alice/chats", "users/{uid}/chats", true},
		{"users/alice/chats", "users/alice/chats", true},
		{"users/bob/chats", "users/alice/chats", false},
		{"users/alice/messages", "users/{uid}/chats", false},
	}

	for _, tt := range tests {
		path := splitPath(tt.path)
		pattern := splitPath(tt.pattern)
		got := segmentsMatch(path, pattern)
		assert.Equal(t, tt.want, got, "path=%s pattern=%s", tt.path, tt.pattern)
	}
}

func splitPath(p string) []string {
	return splitBySlash(p)
}

func splitBySlash(s string) []string {
	var result []string
	for _, part := range splitString(s, '/') {
		result = append(result, part)
	}
	return result
}

func splitString(s string, sep rune) []string {
	var result []string
	var current []rune
	for _, r := range s {
		if r == sep {
			result = append(result, string(current))
			current = nil
		} else {
			current = append(current, r)
		}
	}
	result = append(result, string(current))
	return result
}

func TestLoadFromFile(t *testing.T) {
	t.Run("file not found", func(t *testing.T) {
		_, err := LoadFromFile("/nonexistent/path/templates.yaml")
		assert.Error(t, err)
	})

	t.Run("valid file", func(t *testing.T) {
		// Create a temporary file
		tmpDir := t.TempDir()
		tmpFile := tmpDir + "/templates.yaml"
		yaml := `
templates:
  - name: test
    collectionPattern: users/{uid}/docs
    fields:
      - { field: created, order: desc }
`
		err := writeFile(tmpFile, []byte(yaml))
		require.NoError(t, err)

		templates, err := LoadFromFile(tmpFile)
		require.NoError(t, err)
		assert.Len(t, templates, 1)
		assert.Equal(t, "test", templates[0].Name)
	})
}

func writeFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0644)
}

func TestLoadFromBytes_ValidationError(t *testing.T) {
	// Template with empty field name
	yaml := `
templates:
  - name: bad
    collectionPattern: users/{uid}/chats
    fields:
      - { field: "", order: asc }
`
	_, err := LoadFromBytes([]byte(yaml))
	assert.Error(t, err)
}

func TestValidateTemplate_EmptyFieldName(t *testing.T) {
	tmpl := Template{
		Name:              "test",
		CollectionPattern: "users/{uid}/chats",
		Fields:            []Field{{Field: "", Order: Asc}},
	}
	err := ValidateTemplate(&tmpl)
	assert.Error(t, err)
}
