package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/syntrixbase/syntrix/internal/trigger/types"
)

func TestNewFactory_Coverage(t *testing.T) {
	t.Parallel()
	// Case 1: No options
	f, err := NewFactory(nil, nil, nil)
	assert.NoError(t, err)
	assert.NotNil(t, f)

	// Case 2: With options
	f, err = NewFactory(nil, nil, nil, WithMetrics(&types.NoopMetrics{}))
	assert.NoError(t, err)
	assert.NotNil(t, f)
}
