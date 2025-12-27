package query

import (
	"context"
	"testing"

	"github.com/codetrek/syntrix/internal/storage"
	"github.com/codetrek/syntrix/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestEngine_ExecuteQuery(t *testing.T) {
	type testCase struct {
		name        string
		query       model.Query
		mockSetup   func(*MockStorageBackend)
		expectedDocs []model.Document
		expectError bool
	}

	tests := []testCase{
		{
			name: "Success",
			query: model.Query{
				Collection: "test",
				Filters: []model.Filter{
					{Field: "foo", Op: "==", Value: "bar"},
				},
			},
			mockSetup: func(m *MockStorageBackend) {
				storedDocs := []*storage.Document{
					{
						Fullpath:   "test/1",
						Collection: "test",
						Data:       map[string]interface{}{"foo": "bar"},
						Version:    1,
						UpdatedAt:  100,
						CreatedAt:  90,
					},
				}
				m.On("Query", mock.Anything, "default", mock.MatchedBy(func(q model.Query) bool {
					return q.Collection == "test" && q.Filters[0].Value == "bar"
				})).Return(storedDocs, nil)
			},
			expectedDocs: []model.Document{
				{
					"id":         "1",
					"collection": "test",
					"foo":        "bar",
					"version":    int64(1),
					"updatedAt":  int64(100),
					"createdAt":  int64(90),
				},
			},
			expectError: false,
		},
		{
			name: "Storage Error",
			query: model.Query{
				Collection: "test",
			},
			mockSetup: func(m *MockStorageBackend) {
				m.On("Query", mock.Anything, "default", mock.Anything).Return(nil, assert.AnError)
			},
			expectError: true,
		},
		{
			name: "Empty Result",
			query: model.Query{
				Collection: "test",
			},
			mockSetup: func(m *MockStorageBackend) {
				m.On("Query", mock.Anything, "default", mock.Anything).Return([]*storage.Document{}, nil)
			},
			expectedDocs: []model.Document{},
			expectError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockStorage := new(MockStorageBackend)
			if tc.mockSetup != nil {
				tc.mockSetup(mockStorage)
			}
			engine := NewEngine(mockStorage, "http://mock-csp")
			ctx := context.Background()

			docs, err := engine.ExecuteQuery(ctx, "default", tc.query)

			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, len(tc.expectedDocs), len(docs))
				for i, d := range tc.expectedDocs {
					assert.Equal(t, d, docs[i])
				}
			}
			mockStorage.AssertExpectations(t)
		})
	}
}

func TestEngine_Push_Coverage(t *testing.T) {
	type testCase struct {
		name              string
		req               storage.ReplicationPushRequest
		mockSetup         func(*MockStorageBackend)
		expectedConflicts []*storage.Document
		expectError       bool
	}

	tests := []testCase{
		{
			name: "Conflict (Version Mismatch)",
			req: storage.ReplicationPushRequest{
				Collection: "test",
				Changes: []storage.ReplicationPushChange{
					{
						Doc:         &storage.Document{Id: "test/1", Fullpath: "test/1", Collection: "test", Data: map[string]interface{}{"foo": "bar"}, Version: 2},
						BaseVersion: ptr(int64(1)),
					},
				},
			},
			mockSetup: func(m *MockStorageBackend) {
				// Existing doc has version 3, but we expect base version 1 -> Conflict
				existingDoc := &storage.Document{Id: "test/1", Fullpath: "test/1", Version: 3, Data: map[string]interface{}{"foo": "old"}}
				m.On("Get", mock.Anything, "default", "test/1").Return(existingDoc, nil)
			},
			expectedConflicts: []*storage.Document{
				{Id: "test/1", Fullpath: "test/1", Version: 3, Data: map[string]interface{}{"foo": "old"}},
			},
			expectError: false,
		},
		{
			name: "Delete Success",
			req: storage.ReplicationPushRequest{
				Collection: "test",
				Changes: []storage.ReplicationPushChange{
					{
						Doc:         &storage.Document{Id: "test/1", Fullpath: "test/1", Collection: "test", Deleted: true, Version: 2},
						BaseVersion: ptr(int64(1)),
					},
				},
			},
			mockSetup: func(m *MockStorageBackend) {
				existingDoc := &storage.Document{Id: "test/1", Fullpath: "test/1", Version: 1}
				m.On("Get", mock.Anything, "default", "test/1").Return(existingDoc, nil)
				m.On("Delete", mock.Anything, "default", "test/1", mock.MatchedBy(func(f model.Filters) bool {
					return f[0].Field == "version" && f[0].Value == int64(1)
				})).Return(nil)
			},
			expectedConflicts: nil,
			expectError:       false,
		},
		{
			name: "Delete Conflict (Precondition Failed)",
			req: storage.ReplicationPushRequest{
				Collection: "test",
				Changes: []storage.ReplicationPushChange{
					{
						Doc:         &storage.Document{Id: "test/1", Fullpath: "test/1", Collection: "test", Deleted: true, Version: 2},
						BaseVersion: ptr(int64(1)),
					},
				},
			},
			mockSetup: func(m *MockStorageBackend) {
				existingDoc := &storage.Document{Id: "test/1", Fullpath: "test/1", Version: 1}
				m.On("Get", mock.Anything, "default", "test/1").Return(existingDoc, nil).Once()

				// Delete fails with PreconditionFailed
				m.On("Delete", mock.Anything, "default", "test/1", mock.Anything).Return(model.ErrPreconditionFailed)

				// Fetch latest for conflict
				latestDoc := &storage.Document{Id: "test/1", Fullpath: "test/1", Version: 3}
				m.On("Get", mock.Anything, "default", "test/1").Return(latestDoc, nil).Once()
			},
			expectedConflicts: []*storage.Document{
				{Id: "test/1", Fullpath: "test/1", Version: 3},
			},
			expectError: false,
		},
		{
			name: "Create Conflict (Already Exists)",
			req: storage.ReplicationPushRequest{
				Collection: "test",
				Changes: []storage.ReplicationPushChange{
					{
						Doc: &storage.Document{Id: "test/1", Fullpath: "test/1", Collection: "test", Version: 1},
					},
				},
			},
			mockSetup: func(m *MockStorageBackend) {
				// Get returns NotFound, so we try to Create
				m.On("Get", mock.Anything, "default", "test/1").Return(nil, model.ErrNotFound)
				// Create fails (maybe race condition)
				m.On("Create", mock.Anything, "default", mock.Anything).Return(assert.AnError)
			},
			expectedConflicts: []*storage.Document{
				{Id: "test/1", Fullpath: "test/1", Collection: "test", Version: 1},
			},
			expectError: false,
		},
		{
			name: "Get Error",
			req: storage.ReplicationPushRequest{
				Collection: "test",
				Changes: []storage.ReplicationPushChange{
					{Doc: &storage.Document{Id: "test/1", Fullpath: "test/1"}},
				},
			},
			mockSetup: func(m *MockStorageBackend) {
				m.On("Get", mock.Anything, "default", "test/1").Return(nil, assert.AnError)
			},
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockStorage := new(MockStorageBackend)
			if tc.mockSetup != nil {
				tc.mockSetup(mockStorage)
			}
			engine := NewEngine(mockStorage, "http://mock-csp")
			ctx := context.Background()

			resp, err := engine.Push(ctx, "default", tc.req)

			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, len(tc.expectedConflicts), len(resp.Conflicts))
			}
			mockStorage.AssertExpectations(t)
		})
	}
}

func ptr(i int64) *int64 {
	return &i
}
