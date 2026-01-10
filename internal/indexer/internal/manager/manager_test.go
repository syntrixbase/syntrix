package manager

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/syntrixbase/syntrix/internal/indexer/internal/encoding"
	"github.com/syntrixbase/syntrix/internal/indexer/internal/shard"
	"github.com/syntrixbase/syntrix/internal/indexer/internal/template"
)

const testTemplatesYAML = `
templates:
  - name: chats_by_timestamp
    collectionPattern: users/{uid}/chats
    fields:
      - { field: timestamp, order: desc }

  - name: chats_by_name_age
    collectionPattern: users/{uid}/chats
    fields:
      - { field: name, order: asc }
      - { field: age, order: desc }

  - name: messages_by_sender
    collectionPattern: rooms/{rid}/messages
    fields:
      - { field: senderId, order: asc }
      - { field: timestamp, order: desc }

  - name: specific_alice_chats
    collectionPattern: users/alice/chats
    fields:
      - { field: timestamp, order: desc }
`

func TestNew(t *testing.T) {
	m := New()
	assert.NotNil(t, m)
	assert.Empty(t, m.ListDatabases())
	assert.Empty(t, m.Templates())
}

func TestManager_LoadTemplatesFromBytes(t *testing.T) {
	m := New()

	err := m.LoadTemplatesFromBytes([]byte(testTemplatesYAML))
	require.NoError(t, err)

	templates := m.Templates()
	assert.Len(t, templates, 4)
}

func TestManager_LoadTemplatesFromBytes_Invalid(t *testing.T) {
	m := New()

	err := m.LoadTemplatesFromBytes([]byte("invalid: yaml: ["))
	assert.ErrorIs(t, err, ErrTemplateLoadFailed)
}

func TestManager_GetDatabase(t *testing.T) {
	m := New()

	// First call creates database
	db1 := m.GetDatabase("myapp")
	assert.NotNil(t, db1)
	assert.Equal(t, "myapp", db1.Name)

	// Second call returns same database
	db2 := m.GetDatabase("myapp")
	assert.Same(t, db1, db2)

	// Different name creates different database
	db3 := m.GetDatabase("other")
	assert.NotSame(t, db1, db3)

	assert.Len(t, m.ListDatabases(), 2)
}

func TestManager_DeleteDatabase(t *testing.T) {
	m := New()

	m.GetDatabase("myapp")
	m.GetDatabase("other")
	assert.Len(t, m.ListDatabases(), 2)

	m.DeleteDatabase("myapp")
	assert.Len(t, m.ListDatabases(), 1)

	// Deleting again is a no-op
	m.DeleteDatabase("myapp")
	assert.Len(t, m.ListDatabases(), 1)
}

func TestManager_MatchTemplatesForCollection(t *testing.T) {
	m := New()
	require.NoError(t, m.LoadTemplatesFromBytes([]byte(testTemplatesYAML)))

	t.Run("matches generic pattern", func(t *testing.T) {
		results := m.MatchTemplatesForCollection("users/bob/chats")
		// Should match: chats_by_timestamp, chats_by_name_age
		assert.Len(t, results, 2)
	})

	t.Run("matches specific over generic", func(t *testing.T) {
		results := m.MatchTemplatesForCollection("users/alice/chats")
		// Should match: specific_alice_chats (priority), chats_by_timestamp, chats_by_name_age
		assert.Len(t, results, 3)
		// First should be the specific one
		assert.Equal(t, "specific_alice_chats", results[0].Template.Name)
	})

	t.Run("no match", func(t *testing.T) {
		results := m.MatchTemplatesForCollection("other/path")
		assert.Len(t, results, 0)
	})
}

func TestManager_SelectBestTemplate(t *testing.T) {
	m := New()
	require.NoError(t, m.LoadTemplatesFromBytes([]byte(testTemplatesYAML)))

	t.Run("select by orderBy", func(t *testing.T) {
		plan := Plan{
			Collection: "users/bob/chats",
			OrderBy:    []OrderField{{Field: "timestamp", Direction: encoding.Desc}},
		}
		tmpl, err := m.SelectBestTemplate(plan)
		require.NoError(t, err)
		assert.Equal(t, "chats_by_timestamp", tmpl.Name)
	})

	t.Run("select by multiple orderBy fields", func(t *testing.T) {
		plan := Plan{
			Collection: "users/bob/chats",
			OrderBy: []OrderField{
				{Field: "name", Direction: encoding.Asc},
				{Field: "age", Direction: encoding.Desc},
			},
		}
		tmpl, err := m.SelectBestTemplate(plan)
		require.NoError(t, err)
		assert.Equal(t, "chats_by_name_age", tmpl.Name)
	})

	t.Run("select specific over generic", func(t *testing.T) {
		plan := Plan{
			Collection: "users/alice/chats",
			OrderBy:    []OrderField{{Field: "timestamp", Direction: encoding.Desc}},
		}
		tmpl, err := m.SelectBestTemplate(plan)
		require.NoError(t, err)
		assert.Equal(t, "specific_alice_chats", tmpl.Name)
	})

	t.Run("no matching collection", func(t *testing.T) {
		plan := Plan{
			Collection: "other/path",
			OrderBy:    []OrderField{{Field: "timestamp", Direction: encoding.Desc}},
		}
		_, err := m.SelectBestTemplate(plan)
		assert.ErrorIs(t, err, ErrNoMatchingIndex)
	})

	t.Run("no compatible template - wrong orderBy", func(t *testing.T) {
		plan := Plan{
			Collection: "users/bob/chats",
			OrderBy:    []OrderField{{Field: "nonexistent", Direction: encoding.Asc}},
		}
		_, err := m.SelectBestTemplate(plan)
		assert.ErrorIs(t, err, ErrNoMatchingIndex)
	})

	t.Run("no compatible template - wrong direction", func(t *testing.T) {
		plan := Plan{
			Collection: "users/bob/chats",
			OrderBy:    []OrderField{{Field: "timestamp", Direction: encoding.Asc}},
		}
		_, err := m.SelectBestTemplate(plan)
		assert.ErrorIs(t, err, ErrNoMatchingIndex)
	})
}

func TestManager_SelectBestTemplate_WithFilters(t *testing.T) {
	m := New()
	require.NoError(t, m.LoadTemplatesFromBytes([]byte(testTemplatesYAML)))

	t.Run("equality filter allows skipping prefix", func(t *testing.T) {
		plan := Plan{
			Collection: "users/bob/chats",
			Filters:    []Filter{{Field: "name", Op: FilterEq, Value: "test"}},
			OrderBy:    []OrderField{{Field: "age", Direction: encoding.Desc}},
		}
		tmpl, err := m.SelectBestTemplate(plan)
		require.NoError(t, err)
		assert.Equal(t, "chats_by_name_age", tmpl.Name)
	})

	t.Run("range filter on next field", func(t *testing.T) {
		plan := Plan{
			Collection: "rooms/room1/messages",
			Filters: []Filter{
				{Field: "senderId", Op: FilterEq, Value: "user1"},
				{Field: "timestamp", Op: FilterGt, Value: 1000},
			},
			OrderBy: []OrderField{},
		}
		tmpl, err := m.SelectBestTemplate(plan)
		require.NoError(t, err)
		assert.Equal(t, "messages_by_sender", tmpl.Name)
	})

	t.Run("multiple range filters not supported", func(t *testing.T) {
		plan := Plan{
			Collection: "rooms/room1/messages",
			Filters: []Filter{
				{Field: "timestamp", Op: FilterGt, Value: 1000},
				{Field: "timestamp", Op: FilterLt, Value: 2000},
			},
			OrderBy: []OrderField{},
		}
		_, err := m.SelectBestTemplate(plan)
		assert.ErrorIs(t, err, ErrNoMatchingIndex)
	})
}

func TestManager_SelectBestTemplate_TieBreaker(t *testing.T) {
	// Create templates with same score
	yaml := `
templates:
  - name: z_template
    collectionPattern: test/{id}/items
    fields:
      - { field: name, order: asc }
  - name: a_template
    collectionPattern: test/{id}/items
    fields:
      - { field: name, order: asc }
`
	m := New()
	require.NoError(t, m.LoadTemplatesFromBytes([]byte(yaml)))

	plan := Plan{
		Collection: "test/123/items",
		OrderBy:    []OrderField{{Field: "name", Direction: encoding.Asc}},
	}
	tmpl, err := m.SelectBestTemplate(plan)
	require.NoError(t, err)
	// Should pick lexicographically smallest name
	assert.Equal(t, "a_template", tmpl.Name)
}

func TestManager_Search(t *testing.T) {
	m := New()
	require.NoError(t, m.LoadTemplatesFromBytes([]byte(testTemplatesYAML)))

	// Create shard and add data
	s := m.GetOrCreateShard("mydb", "users/*/chats", "chats_by_timestamp", "users/{uid}/chats")
	s.Upsert("doc1", []byte{0x01, 0x00, 0x10})
	s.Upsert("doc2", []byte{0x01, 0x00, 0x20})
	s.Upsert("doc3", []byte{0x01, 0x00, 0x30})

	t.Run("basic search", func(t *testing.T) {
		plan := Plan{
			Collection: "users/bob/chats",
			OrderBy:    []OrderField{{Field: "timestamp", Direction: encoding.Desc}},
			Limit:      10,
		}
		results, err := m.Search(context.Background(), "mydb", plan)
		require.NoError(t, err)
		assert.Len(t, results, 3)
		assert.Equal(t, "doc1", results[0].ID)
		assert.Equal(t, "doc2", results[1].ID)
		assert.Equal(t, "doc3", results[2].ID)
	})

	t.Run("search with limit", func(t *testing.T) {
		plan := Plan{
			Collection: "users/bob/chats",
			OrderBy:    []OrderField{{Field: "timestamp", Direction: encoding.Desc}},
			Limit:      2,
		}
		results, err := m.Search(context.Background(), "mydb", plan)
		require.NoError(t, err)
		assert.Len(t, results, 2)
	})

	t.Run("search with cursor", func(t *testing.T) {
		cursor := encoding.EncodeBase64([]byte{0x01, 0x00, 0x10})
		plan := Plan{
			Collection: "users/bob/chats",
			OrderBy:    []OrderField{{Field: "timestamp", Direction: encoding.Desc}},
			StartAfter: cursor,
			Limit:      10,
		}
		results, err := m.Search(context.Background(), "mydb", plan)
		require.NoError(t, err)
		assert.Len(t, results, 2)
		assert.Equal(t, "doc2", results[0].ID)
		assert.Equal(t, "doc3", results[1].ID)
	})

	t.Run("search empty collection", func(t *testing.T) {
		plan := Plan{
			Collection: "users/bob/chats",
			OrderBy:    []OrderField{{Field: "timestamp", Direction: encoding.Desc}},
			Limit:      10,
		}
		// Search in a database where shard doesn't exist
		_, err := m.Search(context.Background(), "otherdb", plan)
		assert.ErrorIs(t, err, ErrIndexNotReady)
	})

	t.Run("search no matching template", func(t *testing.T) {
		plan := Plan{
			Collection: "nonexistent/path",
			OrderBy:    []OrderField{{Field: "timestamp", Direction: encoding.Desc}},
			Limit:      10,
		}
		_, err := m.Search(context.Background(), "mydb", plan)
		assert.ErrorIs(t, err, ErrNoMatchingIndex)
	})

	t.Run("search with invalid cursor", func(t *testing.T) {
		plan := Plan{
			Collection: "users/bob/chats",
			OrderBy:    []OrderField{{Field: "timestamp", Direction: encoding.Desc}},
			StartAfter: "invalid-base64!!!",
			Limit:      10,
		}
		_, err := m.Search(context.Background(), "mydb", plan)
		assert.Error(t, err)
	})

	t.Run("search with empty collection", func(t *testing.T) {
		plan := Plan{
			Collection: "",
			OrderBy:    []OrderField{{Field: "timestamp", Direction: encoding.Desc}},
		}
		_, err := m.Search(context.Background(), "mydb", plan)
		assert.ErrorIs(t, err, ErrInvalidPlan)
	})
}

func TestManager_GetShard(t *testing.T) {
	m := New()

	// Shard doesn't exist
	s := m.GetShard("mydb", "users/*/chats", "ts:desc")
	assert.Nil(t, s)

	// Create shard
	created := m.GetOrCreateShard("mydb", "users/*/chats", "ts:desc", "users/{uid}/chats")
	assert.NotNil(t, created)

	// Now it exists
	found := m.GetShard("mydb", "users/*/chats", "ts:desc")
	assert.Same(t, created, found)
}

func TestManager_Stats(t *testing.T) {
	m := New()

	stats := m.Stats()
	assert.Equal(t, 0, stats.DatabaseCount)
	assert.Equal(t, 0, stats.ShardCount)
	assert.Equal(t, 0, stats.TemplateCount)

	require.NoError(t, m.LoadTemplatesFromBytes([]byte(testTemplatesYAML)))
	m.GetOrCreateShard("db1", "users/*/chats", "ts:desc", "users/{uid}/chats")
	m.GetOrCreateShard("db1", "rooms/*/messages", "ts:desc", "rooms/{rid}/messages")
	m.GetOrCreateShard("db2", "users/*/chats", "ts:desc", "users/{uid}/chats")

	stats = m.Stats()
	assert.Equal(t, 2, stats.DatabaseCount)
	assert.Equal(t, 3, stats.ShardCount)
	assert.Equal(t, 4, stats.TemplateCount)
}

func TestManager_ComputeQueryScore(t *testing.T) {
	m := New()
	require.NoError(t, m.LoadTemplatesFromBytes([]byte(testTemplatesYAML)))

	t.Run("orderBy prefix match scores positive", func(t *testing.T) {
		plan := Plan{
			Collection: "users/bob/chats",
			OrderBy:    []OrderField{{Field: "name", Direction: encoding.Asc}},
		}
		templates := m.Templates()
		// Find chats_by_name_age
		var tmpl *template.Template
		for i := range templates {
			if templates[i].Name == "chats_by_name_age" {
				tmpl = &templates[i]
				break
			}
		}
		require.NotNil(t, tmpl)
		score := m.computeQueryScore(plan, tmpl)
		assert.Greater(t, score, 0)
	})

	t.Run("orderBy exceeds index scores 0", func(t *testing.T) {
		plan := Plan{
			Collection: "users/bob/chats",
			OrderBy: []OrderField{
				{Field: "name", Direction: encoding.Asc},
				{Field: "age", Direction: encoding.Desc},
				{Field: "extra", Direction: encoding.Asc},
			},
		}
		templates := m.Templates()
		var tmpl *template.Template
		for i := range templates {
			if templates[i].Name == "chats_by_name_age" {
				tmpl = &templates[i]
				break
			}
		}
		require.NotNil(t, tmpl)
		score := m.computeQueryScore(plan, tmpl)
		assert.Equal(t, 0, score)
	})
}

func TestManager_LoadTemplates(t *testing.T) {
	t.Run("valid template file", func(t *testing.T) {
		// Create temp template file
		tmpFile, err := os.CreateTemp("", "templates-*.yaml")
		require.NoError(t, err)
		defer os.Remove(tmpFile.Name())

		templateYAML := `
templates:
  - name: test_template
    collectionPattern: users/{uid}/docs
    fields:
      - { field: timestamp, order: desc }
`
		_, err = tmpFile.WriteString(templateYAML)
		require.NoError(t, err)
		tmpFile.Close()

		m := New()
		err = m.LoadTemplates(tmpFile.Name())
		require.NoError(t, err)

		assert.Len(t, m.Templates(), 1)
		assert.Equal(t, "test_template", m.Templates()[0].Name)
	})

	t.Run("nonexistent file", func(t *testing.T) {
		m := New()
		err := m.LoadTemplates("/nonexistent/path/templates.yaml")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrTemplateLoadFailed)
	})
}

func TestManager_ListDatabases(t *testing.T) {
	m := New()

	// Initially empty
	names := m.ListDatabases()
	assert.Empty(t, names)

	// Add databases
	m.GetDatabase("db1")
	m.GetDatabase("db2")
	m.GetDatabase("db3")

	names = m.ListDatabases()
	assert.Len(t, names, 3)
	assert.Contains(t, names, "db1")
	assert.Contains(t, names, "db2")
	assert.Contains(t, names, "db3")
}

func TestManager_SelectBestTemplate_Tie(t *testing.T) {
	m := New()

	// Templates with identical scores - should choose by name
	templateYAML := `
templates:
  - name: zzzz_last
    collectionPattern: test/{id}/docs
    fields:
      - { field: timestamp, order: desc }
  - name: aaaa_first
    collectionPattern: test/{id}/docs
    fields:
      - { field: timestamp, order: desc }
`
	require.NoError(t, m.LoadTemplatesFromBytes([]byte(templateYAML)))

	plan := Plan{
		Collection: "test/123/docs",
		OrderBy:    []OrderField{{Field: "timestamp", Direction: encoding.Desc}},
	}

	tmpl, err := m.SelectBestTemplate(plan)
	require.NoError(t, err)
	// Should pick the lexicographically smallest name
	assert.Equal(t, "aaaa_first", tmpl.Name)
}

func TestManager_Search_InvalidCursor(t *testing.T) {
	m := New()
	templateYAML := `
templates:
  - name: test
    collectionPattern: test/docs
    fields:
      - { field: ts, order: asc }
`
	require.NoError(t, m.LoadTemplatesFromBytes([]byte(templateYAML)))

	// Create shard and add data
	shard := m.GetOrCreateShard("mydb", "test/docs", "test", "test/docs")
	shard.Upsert("doc1", []byte{0x01, 0x02, 0x03})

	t.Run("invalid base64 cursor", func(t *testing.T) {
		plan := Plan{
			Collection: "test/docs",
			OrderBy:    []OrderField{{Field: "ts", Direction: encoding.Asc}},
			StartAfter: "!!!not-valid-base64!!!",
		}
		_, err := m.Search(context.Background(), "mydb", plan)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid cursor")
	})

	t.Run("valid base64 cursor", func(t *testing.T) {
		plan := Plan{
			Collection: "test/docs",
			OrderBy:    []OrderField{{Field: "ts", Direction: encoding.Asc}},
			StartAfter: encoding.EncodeBase64([]byte{0x00}),
		}
		results, err := m.Search(context.Background(), "mydb", plan)
		require.NoError(t, err)
		assert.Len(t, results, 1)
	})
}

func TestManager_GetDatabase_Concurrent(t *testing.T) {
	m := New()

	// Run many goroutines concurrently trying to get/create the same database
	// This should trigger the double-check path
	const numGoroutines = 100
	done := make(chan *shard.Database, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			done <- m.GetDatabase("concurrent-db")
		}()
	}

	// Collect all results
	var dbs []*shard.Database
	for i := 0; i < numGoroutines; i++ {
		dbs = append(dbs, <-done)
	}

	// All should return the same database instance
	first := dbs[0]
	for i, db := range dbs {
		assert.Same(t, first, db, "goroutine %d got different database", i)
	}

	// Should only have one database
	assert.Len(t, m.ListDatabases(), 1)
}

func TestManager_SelectBestTemplate_PatternPriority(t *testing.T) {
	m := New()

	// More specific pattern should win over generic
	templateYAML := `
templates:
  - name: generic
    collectionPattern: "{category}/{id}/items"
    fields:
      - { field: timestamp, order: desc }
  - name: specific
    collectionPattern: "products/{id}/items"
    fields:
      - { field: timestamp, order: desc }
`
	require.NoError(t, m.LoadTemplatesFromBytes([]byte(templateYAML)))

	plan := Plan{
		Collection: "products/123/items",
		OrderBy:    []OrderField{{Field: "timestamp", Direction: encoding.Desc}},
	}

	tmpl, err := m.SelectBestTemplate(plan)
	require.NoError(t, err)
	assert.Equal(t, "specific", tmpl.Name)
}
