package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"syntrix/internal/realtime"
	"syntrix/internal/storage"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRealtime_FullFlow(t *testing.T) {
	env := setupMicroservices(t)
	defer env.Cancel()

	// Convert http URL to ws URL
	wsURL := "ws" + strings.TrimPrefix(env.RealtimeServer.URL, "http") + "/v1/realtime"

	// Connect to Websocket
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err, "Failed to connect to websocket")
	defer ws.Close()

	// 1. Authenticate
	authMsg := realtime.BaseMessage{
		ID:   "auth-1",
		Type: realtime.TypeAuth,
		Payload: mustMarshal(realtime.AuthPayload{
			Token: "dummy-token",
		}),
	}
	err = ws.WriteJSON(authMsg)
	require.NoError(t, err)

	// Read Auth Ack
	var ackMsg realtime.BaseMessage
	err = ws.ReadJSON(&ackMsg)
	require.NoError(t, err)
	assert.Equal(t, realtime.TypeAuthAck, ackMsg.Type)
	assert.Equal(t, "auth-1", ackMsg.ID)

	// 2. Subscribe
	collectionName := "realtime_test_col"
	subID := "sub-1"
	subMsg := realtime.BaseMessage{
		ID:   subID,
		Type: realtime.TypeSubscribe,
		Payload: mustMarshal(realtime.SubscribePayload{
			Query: storage.Query{
				Collection: collectionName,
			},
		}),
	}
	err = ws.WriteJSON(subMsg)
	require.NoError(t, err)

	// Give some time for subscription to register
	time.Sleep(100 * time.Millisecond)

	// 3. Trigger Event (Create Document via API Gateway)
	docData := map[string]interface{}{
		"msg": "hello realtime",
	}
	body, _ := json.Marshal(map[string]interface{}{"data": docData})
	resp, err := env.APIServer.Client().Post(fmt.Sprintf("%s/v1/%s", env.APIServer.URL, collectionName), "application/json", bytes.NewBuffer(body))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	// 4. Receive Event
	ws.SetReadDeadline(time.Now().Add(5 * time.Second))
	var eventMsg realtime.BaseMessage
	err = ws.ReadJSON(&eventMsg)
	require.NoError(t, err, "Should receive event message")

	assert.Equal(t, realtime.TypeEvent, eventMsg.Type)

	var eventPayload realtime.EventPayload
	err = json.Unmarshal(eventMsg.Payload, &eventPayload)
	require.NoError(t, err)

	assert.Equal(t, subID, eventPayload.SubID)
	assert.Equal(t, storage.EventCreate, eventPayload.Delta.Type)
	assert.Equal(t, collectionName, eventPayload.Delta.Document.Collection)
	assert.Equal(t, "hello realtime", eventPayload.Delta.Document.Data["msg"])

	// 5. Unsubscribe
	unsubMsg := realtime.BaseMessage{
		ID:   "unsub-1",
		Type: realtime.TypeUnsubscribe,
		Payload: mustMarshal(realtime.UnsubscribePayload{
			ID: subID,
		}),
	}
	err = ws.WriteJSON(unsubMsg)
	require.NoError(t, err)

	// Read Unsubscribe Ack
	var unsubAckMsg realtime.BaseMessage
	err = ws.ReadJSON(&unsubAckMsg)
	require.NoError(t, err)
	assert.Equal(t, realtime.TypeUnsubscribeAck, unsubAckMsg.Type)
	assert.Equal(t, "unsub-1", unsubAckMsg.ID)

	// Wait a bit
	time.Sleep(100 * time.Millisecond)

	// 6. Trigger another event
	docData2 := map[string]interface{}{
		"msg": "should not receive",
	}
	body2, _ := json.Marshal(map[string]interface{}{"data": docData2})
	resp, err = env.APIServer.Client().Post(fmt.Sprintf("%s/v1/%s", env.APIServer.URL, collectionName), "application/json", bytes.NewBuffer(body2))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	// 7. Verify NO event received
	ws.SetReadDeadline(time.Now().Add(1 * time.Second))
	err = ws.ReadJSON(&eventMsg)
	assert.Error(t, err, "Should timeout and not receive event")
}

func mustMarshal(v interface{}) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
