package trigger

import (
	"github.com/syntrixbase/syntrix/internal/trigger/types"
)

// LoadTriggersFromFile reads triggers from a JSON or YAML file.
// Deprecated: Use types.LoadTriggersFromFile directly.
func LoadTriggersFromFile(filename string) ([]*Trigger, error) {
	return types.LoadTriggersFromFile(filename)
}
