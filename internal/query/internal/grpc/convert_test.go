package grpc

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	pb "github.com/syntrixbase/syntrix/api/gen/query/v1"
	"github.com/syntrixbase/syntrix/internal/storage"
	"github.com/syntrixbase/syntrix/pkg/model"
)

func TestStoredDocToProto(t *testing.T) {
	t.Run("nil input", func(t *testing.T) {
		result := storedDocToProto(nil)
		assert.Nil(t, result)
	})

	t.Run("full document", func(t *testing.T) {
		doc := &storage.StoredDoc{
			Id:             "tenant1:abc123",
			TenantID:       "tenant1",
			Fullpath:       "users/user1",
			Collection:     "users",
			CollectionHash: "abc",
			Parent:         "",
			UpdatedAt:      1704067200000,
			CreatedAt:      1704067100000,
			Version:        5,
			Data:           map[string]interface{}{"name": "Alice", "age": float64(30)},
			Deleted:        false,
		}

		result := storedDocToProto(doc)

		assert.Equal(t, "tenant1:abc123", result.Id)
		assert.Equal(t, "tenant1", result.TenantId)
		assert.Equal(t, "users/user1", result.Fullpath)
		assert.Equal(t, "users", result.Collection)
		assert.Equal(t, int64(1704067200000), result.UpdatedAt)
		assert.Equal(t, int64(5), result.Version)
		assert.False(t, result.Deleted)

		// Check data is properly serialized
		var data map[string]interface{}
		err := json.Unmarshal(result.Data, &data)
		assert.NoError(t, err)
		assert.Equal(t, "Alice", data["name"])
		assert.Equal(t, float64(30), data["age"])
	})
}

func TestProtoToStoredDoc(t *testing.T) {
	t.Run("nil input", func(t *testing.T) {
		result := protoToStoredDoc(nil)
		assert.Nil(t, result)
	})

	t.Run("full document", func(t *testing.T) {
		data, _ := json.Marshal(map[string]interface{}{"name": "Bob"})
		proto := &pb.Document{
			Id:         "tenant1:xyz",
			TenantId:   "tenant1",
			Fullpath:   "users/user2",
			Collection: "users",
			UpdatedAt:  1704067300000,
			CreatedAt:  1704067200000,
			Version:    3,
			Data:       data,
			Deleted:    true,
		}

		result := protoToStoredDoc(proto)

		assert.Equal(t, "tenant1:xyz", result.Id)
		assert.Equal(t, "tenant1", result.TenantID)
		assert.Equal(t, "users/user2", result.Fullpath)
		assert.Equal(t, "users", result.Collection)
		assert.Equal(t, int64(3), result.Version)
		assert.True(t, result.Deleted)
		assert.Equal(t, "Bob", result.Data["name"])
	})
}

func TestModelDocToProto(t *testing.T) {
	t.Run("nil input", func(t *testing.T) {
		result := modelDocToProto(nil)
		assert.Nil(t, result)
	})

	t.Run("full document", func(t *testing.T) {
		doc := model.Document{
			"id":         "user1",
			"collection": "users",
			"version":    float64(5),
			"updatedAt":  float64(1704067200000),
			"createdAt":  float64(1704067100000),
			"deleted":    false,
			"name":       "Alice",
			"email":      "alice@example.com",
		}

		result := modelDocToProto(doc)

		assert.Equal(t, "user1", result.Id)
		assert.Equal(t, "users", result.Collection)
		assert.Equal(t, int64(5), result.Version)

		// Check user data is properly serialized (excludes system fields)
		var data map[string]interface{}
		err := json.Unmarshal(result.Data, &data)
		assert.NoError(t, err)
		assert.Equal(t, "Alice", data["name"])
		assert.Equal(t, "alice@example.com", data["email"])
		// System fields should not be in data
		assert.Nil(t, data["id"])
		assert.Nil(t, data["collection"])
		assert.Nil(t, data["version"])
	})
}

func TestProtoToModelDoc(t *testing.T) {
	t.Run("nil input", func(t *testing.T) {
		result := protoToModelDoc(nil)
		assert.Nil(t, result)
	})

	t.Run("full document", func(t *testing.T) {
		data, _ := json.Marshal(map[string]interface{}{"name": "Bob", "age": 25})
		proto := &pb.Document{
			Id:         "user2",
			Collection: "users",
			Version:    3,
			UpdatedAt:  1704067300000,
			CreatedAt:  1704067200000,
			Data:       data,
		}

		result := protoToModelDoc(proto)

		assert.Equal(t, "user2", result["id"])
		assert.Equal(t, "users", result["collection"])
		assert.Equal(t, int64(3), result["version"])
		assert.Equal(t, "Bob", result["name"])
		assert.Equal(t, float64(25), result["age"])
	})
}

func TestFilterConversions(t *testing.T) {
	t.Run("filterToProto", func(t *testing.T) {
		filter := model.Filter{
			Field: "status",
			Op:    model.OpEq,
			Value: "active",
		}

		result := filterToProto(filter)

		assert.Equal(t, "status", result.Field)
		assert.Equal(t, "==", result.Op)

		var value string
		json.Unmarshal(result.Value, &value)
		assert.Equal(t, "active", value)
	})

	t.Run("protoToFilter", func(t *testing.T) {
		value, _ := json.Marshal("pending")
		proto := &pb.Filter{
			Field: "status",
			Op:    "!=",
			Value: value,
		}

		result := protoToFilter(proto)

		assert.Equal(t, "status", result.Field)
		assert.Equal(t, model.OpNe, result.Op)
		assert.Equal(t, "pending", result.Value)
	})

	t.Run("protoToFilter nil", func(t *testing.T) {
		result := protoToFilter(nil)
		assert.Empty(t, result.Field)
	})

	t.Run("filtersToProto", func(t *testing.T) {
		filters := model.Filters{
			{Field: "a", Op: model.OpEq, Value: 1},
			{Field: "b", Op: model.OpGt, Value: 10},
		}

		result := filtersToProto(filters)

		assert.Len(t, result, 2)
		assert.Equal(t, "a", result[0].Field)
		assert.Equal(t, "b", result[1].Field)
	})

	t.Run("protoToFilters", func(t *testing.T) {
		v1, _ := json.Marshal(1)
		v2, _ := json.Marshal(10)
		protos := []*pb.Filter{
			{Field: "a", Op: "==", Value: v1},
			{Field: "b", Op: ">", Value: v2},
		}

		result := protoToFilters(protos)

		assert.Len(t, result, 2)
		assert.Equal(t, "a", result[0].Field)
		assert.Equal(t, model.OpEq, result[0].Op)
	})
}

func TestOrderConversions(t *testing.T) {
	t.Run("orderToProto", func(t *testing.T) {
		order := model.Order{
			Field:     "createdAt",
			Direction: "desc",
		}

		result := orderToProto(order)

		assert.Equal(t, "createdAt", result.Field)
		assert.Equal(t, "desc", result.Direction)
	})

	t.Run("protoToOrder", func(t *testing.T) {
		proto := &pb.OrderBy{
			Field:     "name",
			Direction: "asc",
		}

		result := protoToOrder(proto)

		assert.Equal(t, "name", result.Field)
		assert.Equal(t, "asc", result.Direction)
	})

	t.Run("protoToOrder nil", func(t *testing.T) {
		result := protoToOrder(nil)
		assert.Empty(t, result.Field)
	})
}

func TestQueryConversions(t *testing.T) {
	t.Run("queryToProto", func(t *testing.T) {
		query := model.Query{
			Collection: "users",
			Filters: model.Filters{
				{Field: "active", Op: model.OpEq, Value: true},
			},
			OrderBy: []model.Order{
				{Field: "name", Direction: "asc"},
			},
			Limit:       10,
			StartAfter:  "cursor123",
			ShowDeleted: true,
		}

		result := queryToProto(query)

		assert.Equal(t, "users", result.Collection)
		assert.Len(t, result.Filters, 1)
		assert.Len(t, result.OrderBy, 1)
		assert.Equal(t, int32(10), result.Limit)
		assert.Equal(t, "cursor123", result.StartAfter)
		assert.True(t, result.ShowDeleted)
	})

	t.Run("protoToQuery", func(t *testing.T) {
		value, _ := json.Marshal(true)
		proto := &pb.Query{
			Collection: "posts",
			Filters: []*pb.Filter{
				{Field: "published", Op: "==", Value: value},
			},
			OrderBy: []*pb.OrderBy{
				{Field: "createdAt", Direction: "desc"},
			},
			Limit:       20,
			StartAfter:  "abc",
			ShowDeleted: false,
		}

		result := protoToQuery(proto)

		assert.Equal(t, "posts", result.Collection)
		assert.Len(t, result.Filters, 1)
		assert.Len(t, result.OrderBy, 1)
		assert.Equal(t, 20, result.Limit)
		assert.Equal(t, "abc", result.StartAfter)
		assert.False(t, result.ShowDeleted)
	})

	t.Run("protoToQuery nil", func(t *testing.T) {
		result := protoToQuery(nil)
		assert.Empty(t, result.Collection)
	})
}

func TestPullRequestConversions(t *testing.T) {
	t.Run("protoToPullRequest", func(t *testing.T) {
		proto := &pb.PullRequest{
			Tenant:     "tenant1",
			Collection: "users",
			Checkpoint: 12345,
			Limit:      100,
		}

		result := protoToPullRequest(proto)

		assert.Equal(t, "users", result.Collection)
		assert.Equal(t, int64(12345), result.Checkpoint)
		assert.Equal(t, 100, result.Limit)
	})

	t.Run("protoToPullRequest nil", func(t *testing.T) {
		result := protoToPullRequest(nil)
		assert.Empty(t, result.Collection)
	})

	t.Run("pullResponseToProto", func(t *testing.T) {
		resp := &storage.ReplicationPullResponse{
			Documents: []*storage.StoredDoc{
				{Id: "doc1", Collection: "users"},
				{Id: "doc2", Collection: "users"},
			},
			Checkpoint: 67890,
		}

		result := pullResponseToProto(resp)

		assert.Len(t, result.Documents, 2)
		assert.Equal(t, int64(67890), result.Checkpoint)
	})

	t.Run("pullResponseToProto nil", func(t *testing.T) {
		result := pullResponseToProto(nil)
		assert.Nil(t, result)
	})
}

func TestPushRequestConversions(t *testing.T) {
	t.Run("protoToPushRequest", func(t *testing.T) {
		data, _ := json.Marshal(map[string]interface{}{"name": "test"})
		baseVersion := int64(5)
		proto := &pb.PushRequest{
			Tenant:     "tenant1",
			Collection: "users",
			Changes: []*pb.PushChange{
				{
					Document:    &pb.Document{Id: "doc1", Data: data},
					BaseVersion: baseVersion,
				},
			},
		}

		result := protoToPushRequest(proto)

		assert.Equal(t, "users", result.Collection)
		assert.Len(t, result.Changes, 1)
		assert.Equal(t, "doc1", result.Changes[0].Doc.Id)
		assert.NotNil(t, result.Changes[0].BaseVersion)
		assert.Equal(t, int64(5), *result.Changes[0].BaseVersion)
	})

	t.Run("protoToPushRequest nil", func(t *testing.T) {
		result := protoToPushRequest(nil)
		assert.Empty(t, result.Collection)
	})

	t.Run("protoToPushChange with negative baseVersion", func(t *testing.T) {
		change := &pb.PushChange{
			Document:    &pb.Document{Id: "doc1"},
			BaseVersion: -1,
		}

		result := protoToPushChange(change)

		assert.Nil(t, result.BaseVersion) // -1 means no conflict detection
	})

	t.Run("pushResponseToProto", func(t *testing.T) {
		resp := &storage.ReplicationPushResponse{
			Conflicts: []*storage.StoredDoc{
				{Id: "conflict1"},
			},
		}

		result := pushResponseToProto(resp)

		assert.Len(t, result.Conflicts, 1)
	})

	t.Run("pushResponseToProto nil", func(t *testing.T) {
		result := pushResponseToProto(nil)
		assert.Nil(t, result)
	})
}
