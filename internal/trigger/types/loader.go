package types

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// LoadTriggersFromFile reads triggers from a JSON or YAML file.
// The file format is a map where keys are trigger IDs:
//
//	{
//	  "trigger-id": { "version": "1.0", "database": "default", ... }
//	}
func LoadTriggersFromFile(filename string) ([]*Trigger, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read trigger rules file: %w", err)
	}

	var triggerMap map[string]*Trigger
	ext := filepath.Ext(filename)

	switch ext {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &triggerMap); err != nil {
			return nil, fmt.Errorf("failed to parse YAML trigger rules: %w", err)
		}
	case ".json":
		if err := json.Unmarshal(data, &triggerMap); err != nil {
			return nil, fmt.Errorf("failed to parse JSON trigger rules: %w", err)
		}
	default:
		// Try JSON first, then YAML
		if err := json.Unmarshal(data, &triggerMap); err != nil {
			if err := yaml.Unmarshal(data, &triggerMap); err != nil {
				return nil, fmt.Errorf("failed to parse trigger rules (unknown format): %w", err)
			}
		}
	}

	// Convert map to slice and set ID from key
	triggers := make([]*Trigger, 0, len(triggerMap))
	for id, trigger := range triggerMap {
		trigger.ID = id
		triggers = append(triggers, trigger)
	}

	return triggers, nil
}
