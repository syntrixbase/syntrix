package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/syntrixbase/syntrix/pkg/model"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestStatusToError(t *testing.T) {
	tests := []struct {
		name    string
		code    codes.Code
		wantErr error
	}{
		{"not found", codes.NotFound, model.ErrNotFound},
		{"precondition failed", codes.FailedPrecondition, model.ErrPreconditionFailed},
		{"already exists", codes.AlreadyExists, model.ErrExists},
		{"invalid argument", codes.InvalidArgument, model.ErrInvalidQuery},
		{"permission denied", codes.PermissionDenied, model.ErrPermissionDenied},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			grpcErr := status.Error(tt.code, "test error")
			result := statusToError(grpcErr)
			assert.Equal(t, tt.wantErr, result)
		})
	}

	t.Run("nil error", func(t *testing.T) {
		result := statusToError(nil)
		assert.Nil(t, result)
	})

	t.Run("unknown code", func(t *testing.T) {
		grpcErr := status.Error(codes.Internal, "internal error")
		result := statusToError(grpcErr)
		assert.Error(t, result)
		assert.Contains(t, result.Error(), "internal error")
	})

	t.Run("non-grpc error", func(t *testing.T) {
		result := statusToError(assert.AnError)
		assert.Equal(t, assert.AnError, result)
	})
}

func TestClient_Close(t *testing.T) {
	t.Run("close nil connection", func(t *testing.T) {
		client := &Client{}
		err := client.Close()
		assert.NoError(t, err)
	})
}

func TestNew_Success(t *testing.T) {
	// Test that New handles connection (gRPC connection is lazy)
	client, err := New("localhost:50051")
	assert.NoError(t, err)
	assert.NotNil(t, client)
	if client != nil {
		client.Close()
	}
}
