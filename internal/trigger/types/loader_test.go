package types

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadTriggersFromFile_JSON(t *testing.T) {
	content := `{
		"trigger1": {
			"database": "db1",
			"collection": "users",
			"events": ["create"],
			"url": "https://example.com/webhook"
		}
	}`

	tmpFile := filepath.Join(t.TempDir(), "triggers.json")
	err := os.WriteFile(tmpFile, []byte(content), 0644)
	require.NoError(t, err)

	triggers, err := LoadTriggersFromFile(tmpFile)
	assert.NoError(t, err)
	assert.Len(t, triggers, 1)
	assert.Equal(t, "trigger1", triggers[0].ID)
	assert.Equal(t, "db1", triggers[0].Database)
}

func TestLoadTriggersFromFile_YAML(t *testing.T) {
	content := `
trigger1:
  database: db1
  collection: users
  events:
    - create
  url: https://example.com/webhook
`

	tmpFile := filepath.Join(t.TempDir(), "triggers.yaml")
	err := os.WriteFile(tmpFile, []byte(content), 0644)
	require.NoError(t, err)

	triggers, err := LoadTriggersFromFile(tmpFile)
	assert.NoError(t, err)
	assert.Len(t, triggers, 1)
	assert.Equal(t, "trigger1", triggers[0].ID)
}

func TestLoadTriggersFromFile_YML(t *testing.T) {
	content := `
trigger2:
  database: db2
  collection: orders
  events:
    - update
  url: https://example.com/orders
`

	tmpFile := filepath.Join(t.TempDir(), "triggers.yml")
	err := os.WriteFile(tmpFile, []byte(content), 0644)
	require.NoError(t, err)

	triggers, err := LoadTriggersFromFile(tmpFile)
	assert.NoError(t, err)
	assert.Len(t, triggers, 1)
	assert.Equal(t, "trigger2", triggers[0].ID)
}

func TestLoadTriggersFromFile_UnknownExt_JSON(t *testing.T) {
	content := `{"trigger1": {"database": "db1", "collection": "users", "events": ["create"], "url": "https://example.com"}}`

	tmpFile := filepath.Join(t.TempDir(), "triggers.txt")
	err := os.WriteFile(tmpFile, []byte(content), 0644)
	require.NoError(t, err)

	triggers, err := LoadTriggersFromFile(tmpFile)
	assert.NoError(t, err)
	assert.Len(t, triggers, 1)
}

func TestLoadTriggersFromFile_UnknownExt_YAML(t *testing.T) {
	content := `
trigger1:
  database: db1
  collection: users
  events:
    - create
  url: https://example.com
`

	tmpFile := filepath.Join(t.TempDir(), "triggers.conf")
	err := os.WriteFile(tmpFile, []byte(content), 0644)
	require.NoError(t, err)

	triggers, err := LoadTriggersFromFile(tmpFile)
	assert.NoError(t, err)
	assert.Len(t, triggers, 1)
}

func TestLoadTriggersFromFile_NotFound(t *testing.T) {
	_, err := LoadTriggersFromFile("/nonexistent/file.json")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read trigger rules file")
}

func TestLoadTriggersFromFile_InvalidJSON(t *testing.T) {
	content := `{invalid json`

	tmpFile := filepath.Join(t.TempDir(), "invalid.json")
	err := os.WriteFile(tmpFile, []byte(content), 0644)
	require.NoError(t, err)

	_, err = LoadTriggersFromFile(tmpFile)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse JSON trigger rules")
}

func TestLoadTriggersFromFile_InvalidYAML(t *testing.T) {
	content := `
trigger1:
  database: db1
    invalid: indentation
`

	tmpFile := filepath.Join(t.TempDir(), "invalid.yaml")
	err := os.WriteFile(tmpFile, []byte(content), 0644)
	require.NoError(t, err)

	_, err = LoadTriggersFromFile(tmpFile)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse YAML trigger rules")
}

func TestLoadTriggersFromFile_UnknownExt_InvalidFormat(t *testing.T) {
	// Content that is neither valid JSON nor valid YAML
	content := `<<<not valid anything>>>`

	tmpFile := filepath.Join(t.TempDir(), "invalid.xyz")
	err := os.WriteFile(tmpFile, []byte(content), 0644)
	require.NoError(t, err)

	_, err = LoadTriggersFromFile(tmpFile)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse trigger rules (unknown format)")
}

func TestLoadTriggersFromFile_MultipleTriggers(t *testing.T) {
	content := `{
		"trigger1": {
			"database": "db1",
			"collection": "users",
			"events": ["create"],
			"url": "https://example.com/webhook1"
		},
		"trigger2": {
			"database": "db2",
			"collection": "orders",
			"events": ["update", "delete"],
			"url": "https://example.com/webhook2"
		}
	}`

	tmpFile := filepath.Join(t.TempDir(), "triggers.json")
	err := os.WriteFile(tmpFile, []byte(content), 0644)
	require.NoError(t, err)

	triggers, err := LoadTriggersFromFile(tmpFile)
	assert.NoError(t, err)
	assert.Len(t, triggers, 2)

	// Check that IDs are set correctly from map keys
	ids := make(map[string]bool)
	for _, t := range triggers {
		ids[t.ID] = true
	}
	assert.True(t, ids["trigger1"])
	assert.True(t, ids["trigger2"])
}

func TestLoadTriggersFromFile_EmptyMap(t *testing.T) {
	content := `{}`

	tmpFile := filepath.Join(t.TempDir(), "triggers.json")
	err := os.WriteFile(tmpFile, []byte(content), 0644)
	require.NoError(t, err)

	triggers, err := LoadTriggersFromFile(tmpFile)
	assert.NoError(t, err)
	assert.Len(t, triggers, 0)
}

// Tests for LoadTriggersFromDir

func TestLoadTriggersFromDir_SingleFile(t *testing.T) {
	tmpDir := t.TempDir()
	content := `database: db1
triggers:
  trigger1:
    collection: users
    events:
      - create
    url: https://example.com/webhook
`
	err := os.WriteFile(filepath.Join(tmpDir, "db1.yml"), []byte(content), 0644)
	require.NoError(t, err)

	result, err := LoadTriggersFromDir(tmpDir)
	assert.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Len(t, result["db1"], 1)
	assert.Equal(t, "trigger1", result["db1"][0].ID)
	assert.Equal(t, "db1", result["db1"][0].Database)
	assert.Equal(t, "users", result["db1"][0].Collection)
}

func TestLoadTriggersFromDir_MultipleFiles(t *testing.T) {
	tmpDir := t.TempDir()

	content1 := `database: db1
triggers:
  trigger1:
    collection: users
    events:
      - create
    url: https://example.com/webhook1
`
	content2 := `database: db2
triggers:
  trigger2:
    collection: orders
    events:
      - update
    url: https://example.com/webhook2
`
	err := os.WriteFile(filepath.Join(tmpDir, "db1.yml"), []byte(content1), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tmpDir, "db2.yaml"), []byte(content2), 0644)
	require.NoError(t, err)

	result, err := LoadTriggersFromDir(tmpDir)
	assert.NoError(t, err)
	assert.Len(t, result, 2)
	assert.Len(t, result["db1"], 1)
	assert.Len(t, result["db2"], 1)
	assert.Equal(t, "trigger1", result["db1"][0].ID)
	assert.Equal(t, "trigger2", result["db2"][0].ID)
}

func TestLoadTriggersFromDir_MultipleTriggersSameFile(t *testing.T) {
	tmpDir := t.TempDir()
	content := `database: db1
triggers:
  trigger1:
    collection: users
    events:
      - create
    url: https://example.com/webhook1
  trigger2:
    collection: orders
    events:
      - update
    url: https://example.com/webhook2
`
	err := os.WriteFile(filepath.Join(tmpDir, "db1.yml"), []byte(content), 0644)
	require.NoError(t, err)

	result, err := LoadTriggersFromDir(tmpDir)
	assert.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Len(t, result["db1"], 2)

	ids := make(map[string]bool)
	for _, tr := range result["db1"] {
		ids[tr.ID] = true
		assert.Equal(t, "db1", tr.Database) // All triggers should have file-level database
	}
	assert.True(t, ids["trigger1"])
	assert.True(t, ids["trigger2"])
}

func TestLoadTriggersFromDir_NotDirectory(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "file.yml")
	err := os.WriteFile(tmpFile, []byte("database: db1\ntriggers: {}"), 0644)
	require.NoError(t, err)

	_, err = LoadTriggersFromDir(tmpFile)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrNotDirectory)
}

func TestLoadTriggersFromDir_NotFound(t *testing.T) {
	_, err := LoadTriggersFromDir("/nonexistent/directory")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to stat directory")
}

func TestLoadTriggersFromDir_EmptyDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	content := `triggers:
  trigger1:
    collection: users
    events:
      - create
    url: https://example.com/webhook
`
	err := os.WriteFile(filepath.Join(tmpDir, "missing_db.yml"), []byte(content), 0644)
	require.NoError(t, err)

	_, err = LoadTriggersFromDir(tmpDir)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrEmptyDatabase)
}

func TestLoadTriggersFromDir_DuplicateDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	content1 := `database: db1
triggers:
  trigger1:
    collection: users
    events: [create]
    url: https://example.com/1
`
	content2 := `database: db1
triggers:
  trigger2:
    collection: orders
    events: [update]
    url: https://example.com/2
`
	err := os.WriteFile(filepath.Join(tmpDir, "file1.yml"), []byte(content1), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tmpDir, "file2.yml"), []byte(content2), 0644)
	require.NoError(t, err)

	_, err = LoadTriggersFromDir(tmpDir)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrDuplicateDatabase)
}

func TestLoadTriggersFromDir_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	content := `database: db1
triggers:
  trigger1:
    invalid: [unclosed
`
	err := os.WriteFile(filepath.Join(tmpDir, "invalid.yml"), []byte(content), 0644)
	require.NoError(t, err)

	_, err = LoadTriggersFromDir(tmpDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse YAML trigger file")
}

func TestLoadTriggersFromDir_SkipsNonYAMLFiles(t *testing.T) {
	tmpDir := t.TempDir()
	yamlContent := `database: db1
triggers:
  trigger1:
    collection: users
    events: [create]
    url: https://example.com/webhook
`
	err := os.WriteFile(filepath.Join(tmpDir, "db1.yml"), []byte(yamlContent), 0644)
	require.NoError(t, err)
	// Create non-YAML files that should be ignored
	err = os.WriteFile(filepath.Join(tmpDir, "readme.txt"), []byte("ignore me"), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tmpDir, ".gitkeep"), []byte(""), 0644)
	require.NoError(t, err)

	result, err := LoadTriggersFromDir(tmpDir)
	assert.NoError(t, err)
	assert.Len(t, result, 1) // Only db1 should be loaded
}

func TestLoadTriggersFromDir_EmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	result, err := LoadTriggersFromDir(tmpDir)
	assert.NoError(t, err)
	assert.Len(t, result, 0)
}

func TestLoadTriggersFromDir_JSONFormat(t *testing.T) {
	tmpDir := t.TempDir()
	content := `{
	"database": "db1",
	"triggers": {
		"trigger1": {
			"collection": "users",
			"events": ["create"],
			"url": "https://example.com/webhook"
		}
	}
}`
	err := os.WriteFile(filepath.Join(tmpDir, "db1.json"), []byte(content), 0644)
	require.NoError(t, err)

	result, err := LoadTriggersFromDir(tmpDir)
	assert.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Len(t, result["db1"], 1)
	assert.Equal(t, "trigger1", result["db1"][0].ID)
	assert.Equal(t, "db1", result["db1"][0].Database)
}
