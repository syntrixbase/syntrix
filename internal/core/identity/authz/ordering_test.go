package authz

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/syntrixbase/syntrix/internal/core/identity/config"
)

func TestMatchOrdering(t *testing.T) {
	// Rule set with overlapping paths, nested under the default database prefix
	rulesYAML := `
rules_version: "1.0"
service: "test"
match:
  /databases/default/documents:
    match:
      /users/{uid}:
        allow:
          read: "false" # Wildcard should be overridden by concrete
      /users/admin:
        allow:
          read: "true" # Concrete path
      /posts/{pid}:
        allow:
          read: "false"
      /posts/special/featured:
        allow:
          read: "true" # Longer path
`

	engine, err := NewEngine(config.AuthZConfig{}, nil)
	require.NoError(t, err)
	err = engine.UpdateRules([]byte(rulesYAML))
	require.NoError(t, err)

	ctx := context.Background()
	req := Request{}

	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{
			name:     "Concrete path /users/admin should match before /users/{uid}",
			path:     "/users/admin",
			expected: true,
		},
		{
			name:     "Wildcard path /users/other should match /users/{uid}",
			path:     "/users/other",
			expected: false,
		},
		{
			name:     "Longer path /posts/special/featured should match before /posts/{pid}",
			path:     "/posts/special/featured",
			expected: true,
		},
		{
			name:     "Shorter path /posts/other should match /posts/{pid}",
			path:     "/posts/other",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowed, err := engine.Evaluate(ctx, tt.path, "read", req, nil)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, allowed)
		})
	}
}
