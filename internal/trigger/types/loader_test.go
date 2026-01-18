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
