package engine

import (
	"strings"
	"testing"

	"github.com/codetrek/syntrix/internal/trigger"
	"github.com/stretchr/testify/assert"
)

func TestValidateTrigger(t *testing.T) {
	tests := []struct {
		name    string
		trigger *trigger.Trigger
		wantErr string
	}{
		{
			name: "valid trigger",
			trigger: &trigger.Trigger{
				ID:         "valid-id",
				Tenant:     "valid-tenant",
				Collection: "users",
				Events:     []string{"create"},
				URL:        "http://example.com",
			},
			wantErr: "",
		},
		{
			name: "missing id",
			trigger: &trigger.Trigger{
				Tenant:     "valid-tenant",
				Collection: "users",
				Events:     []string{"create"},
				URL:        "http://example.com",
			},
			wantErr: "trigger id is required",
		},
		{
			name: "invalid id",
			trigger: &trigger.Trigger{
				ID:         "invalid id",
				Tenant:     "valid-tenant",
				Collection: "users",
				Events:     []string{"create"},
				URL:        "http://example.com",
			},
			wantErr: "invalid trigger id",
		},
		{
			name: "missing tenant",
			trigger: &trigger.Trigger{
				ID:         "valid-id",
				Collection: "users",
				Events:     []string{"create"},
				URL:        "http://example.com",
			},
			wantErr: "tenant is required",
		},
		{
			name: "invalid tenant",
			trigger: &trigger.Trigger{
				ID:         "valid-id",
				Tenant:     "invalid tenant",
				Collection: "users",
				Events:     []string{"create"},
				URL:        "http://example.com",
			},
			wantErr: "invalid tenant",
		},
		{
			name: "tenant too long",
			trigger: &trigger.Trigger{
				ID:         "valid-id",
				Tenant:     strings.Repeat("a", 129),
				Collection: "users",
				Events:     []string{"create"},
				URL:        "http://example.com",
			},
			wantErr: "tenant name too long",
		},
		{
			name: "missing collection",
			trigger: &trigger.Trigger{
				ID:     "valid-id",
				Tenant: "valid-tenant",
				Events: []string{"create"},
				URL:    "http://example.com",
			},
			wantErr: "collection is required",
		},
		{
			name: "collection too long",
			trigger: &trigger.Trigger{
				ID:         "valid-id",
				Tenant:     "valid-tenant",
				Collection: strings.Repeat("a", 129),
				Events:     []string{"create"},
				URL:        "http://example.com",
			},
			wantErr: "collection name too long",
		},
		{
			name: "missing events",
			trigger: &trigger.Trigger{
				ID:         "valid-id",
				Tenant:     "valid-tenant",
				Collection: "users",
				URL:        "http://example.com",
			},
			wantErr: "at least one event is required",
		},
		{
			name: "invalid event",
			trigger: &trigger.Trigger{
				ID:         "valid-id",
				Tenant:     "valid-tenant",
				Collection: "users",
				Events:     []string{"invalid"},
				URL:        "http://example.com",
			},
			wantErr: "invalid event type",
		},
		{
			name: "missing url",
			trigger: &trigger.Trigger{
				ID:         "valid-id",
				Tenant:     "valid-tenant",
				Collection: "users",
				Events:     []string{"create"},
			},
			wantErr: "url is required",
		},
		{
			name: "invalid url scheme",
			trigger: &trigger.Trigger{
				ID:         "valid-id",
				Tenant:     "valid-tenant",
				Collection: "users",
				Events:     []string{"create"},
				URL:        "ftp://example.com",
			},
			wantErr: "url must use http or https scheme",
		},
		{
			name: "url without host",
			trigger: &trigger.Trigger{
				ID:         "valid-id",
				Tenant:     "valid-tenant",
				Collection: "users",
				Events:     []string{"create"},
				URL:        "http://",
			},
			wantErr: "url must have a host",
		},
		{
			name: "valid https url",
			trigger: &trigger.Trigger{
				ID:         "valid-id",
				Tenant:     "valid-tenant",
				Collection: "users",
				Events:     []string{"create"},
				URL:        "https://example.com/webhook",
			},
			wantErr: "",
		},
		{
			name: "invalid url format",
			trigger: &trigger.Trigger{
				ID:         "valid-id",
				Tenant:     "valid-tenant",
				Collection: "users",
				Events:     []string{"create"},
				URL:        "://invalid-url",
			},
			wantErr: "invalid url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTrigger(tt.trigger)
			if tt.wantErr != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
