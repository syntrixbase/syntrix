package integration

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/codetrek/syntrix/internal/config"
	"github.com/codetrek/syntrix/internal/services"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type ServiceEnv struct {
	APIURL      string
	QueryURL    string
	RealtimeURL string
	CSPURL      string
	Manager     *services.Manager
	Cancel      context.CancelFunc
	MongoURI    string
	DBName      string
	testPrefix  string // Unique prefix for this test to isolate users and collections
}

func setupServiceEnv(t *testing.T, rulesContent string, configModifiers ...func(*config.Config)) *ServiceEnv {
	return setupServiceEnvWithOptions(t, rulesContent, configModifiers, nil)
}

func setupServiceEnvWithOptions(t *testing.T, rulesContent string, configModifiers []func(*config.Config), optsModifiers []func(*services.Options)) *ServiceEnv {
	// Use the global test environment instead of creating a new one
	// This avoids route conflicts when multiple tests run in parallel
	env := GetGlobalEnv()
	if env == nil {
		t.Fatal("Global test environment not initialized. Make sure TestMain is running.")
	}

	// Create a unique collection prefix for this test to isolate data
	safeName := strings.ReplaceAll(t.Name(), "/", "_")
	safeName = strings.ReplaceAll(safeName, "\\", "_")
	if len(safeName) > 20 {
		safeName = safeName[len(safeName)-20:]
	}
	collectionPrefix := fmt.Sprintf("%s_%d", safeName, time.Now().UnixNano()%100000)

	// Return a ServiceEnv that wraps the global environment
	// The Cancel function just cleans up the test's data, not the global service
	return &ServiceEnv{
		APIURL:      env.APIURL,
		QueryURL:    env.QueryURL,
		RealtimeURL: env.APIURL, // Realtime is on the same port as API
		CSPURL:      env.CSPURL,
		Manager:     env.Manager,
		MongoURI:    env.MongoURI,
		DBName:      env.DBName,
		testPrefix:  collectionPrefix, // Use this prefix to isolate users and collections
		Cancel: func() {
			// Cleanup collections with our prefix
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cleanupCancel()

			client, err := mongo.Connect(cleanupCtx, options.Client().ApplyURI(env.MongoURI))
			if err == nil {
				defer client.Disconnect(context.Background())
				db := client.Database(env.DBName)

				// List and drop collections with our prefix
				collections, err := db.ListCollectionNames(cleanupCtx, map[string]interface{}{})
				if err == nil {
					for _, coll := range collections {
						if strings.HasPrefix(coll, collectionPrefix) {
							_ = db.Collection(coll).Drop(cleanupCtx)
						}
					}
				}
			}
		},
	}
}

func (e *ServiceEnv) GenerateSystemToken(t *testing.T) string {
	authService := e.Manager.AuthService()
	token, err := authService.GenerateSystemToken("system-worker")
	require.NoError(t, err)
	return token
}

func (e *ServiceEnv) GetToken(t *testing.T, uid string, role string) string {
	return e.GetTokenForTenant(t, "default", uid, role)
}

func (e *ServiceEnv) GetTokenForTenant(t *testing.T, tenant, uid, role string) string {
	// Prefix the username with testPrefix to ensure uniqueness across parallel tests
	prefixedUID := uid
	if e.testPrefix != "" {
		prefixedUID = fmt.Sprintf("%s_%s", e.testPrefix, uid)
	}

	// 1. Try SignUp
	signupBody := map[string]string{
		"tenant":   tenant,
		"username": prefixedUID,
		"password": "password123456",
	}
	bodyBytes, _ := json.Marshal(signupBody)
	resp, err := http.Post(e.APIURL+"/auth/v1/signup", "application/json", bytes.NewBuffer(bodyBytes))
	require.NoError(t, err)
	defer resp.Body.Close()

	var token string
	if resp.StatusCode == http.StatusOK {
		var res map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&res)
		require.NoError(t, err)
		token = res["access_token"].(string)
	} else {
		// Log the signup error
		body, _ := io.ReadAll(resp.Body)
		t.Logf("SignUp failed: status=%d body=%s", resp.StatusCode, string(body))

		// Assume user exists, try Login
		loginBody := map[string]string{
			"tenant":   tenant,
			"username": prefixedUID,
			"password": "password123456",
		}
		bodyBytes, _ := json.Marshal(loginBody)
		respLogin, err := http.Post(e.APIURL+"/auth/v1/login", "application/json", bytes.NewBuffer(bodyBytes))
		require.NoError(t, err)
		defer respLogin.Body.Close()

		if respLogin.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(respLogin.Body)
			t.Logf("Login failed: status=%d body=%s", respLogin.StatusCode, string(body))
		}
		require.Equal(t, http.StatusOK, respLogin.StatusCode)

		var res map[string]interface{}
		err = json.NewDecoder(respLogin.Body).Decode(&res)
		require.NoError(t, err)
		token = res["access_token"].(string)
	}

	// 2. Check Role
	if role == "user" {
		return token
	}

	// 3. Update Role if needed
	claims, err := parseTokenClaims(token)
	require.NoError(t, err)

	// Check if role is already present
	if roles, ok := claims["roles"].([]interface{}); ok {
		for _, r := range roles {
			if r.(string) == role {
				return token
			}
		}
	}

	userID := claims["oid"].(string)
	sysToken := e.GenerateSystemToken(t)

	updateBody := map[string]interface{}{
		"roles": []string{role},
	}
	updateBytes, _ := json.Marshal(updateBody)
	req, err := http.NewRequest("PATCH", e.APIURL+"/admin/users/"+userID, bytes.NewBuffer(updateBytes))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+sysToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	respUpdate, err := client.Do(req)
	require.NoError(t, err)
	defer respUpdate.Body.Close()
	require.Equal(t, http.StatusOK, respUpdate.StatusCode)

	// 4. Re-Login to get new token
	loginBody := map[string]string{
		"tenant":   tenant,
		"username": prefixedUID,
		"password": "password123456",
	}
	bodyBytes, _ = json.Marshal(loginBody)
	respLogin, err := http.Post(e.APIURL+"/auth/v1/login", "application/json", bytes.NewBuffer(bodyBytes))
	require.NoError(t, err)
	defer respLogin.Body.Close()
	require.Equal(t, http.StatusOK, respLogin.StatusCode)

	var res map[string]interface{}
	err = json.NewDecoder(respLogin.Body).Decode(&res)
	require.NoError(t, err)
	return res["access_token"].(string)

}

func parseTokenClaims(token string) (map[string]interface{}, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid token format")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}
	var claims map[string]interface{}
	err = json.Unmarshal(payload, &claims)
	return claims, err
}

func (e *ServiceEnv) MakeRequest(t *testing.T, method, path string, body interface{}, token string) *http.Response {
	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		require.NoError(t, err)
		bodyReader = bytes.NewBuffer(jsonBody)
	}

	req, err := http.NewRequest(method, e.APIURL+path, bodyReader)
	require.NoError(t, err)

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	require.NoError(t, err)
	return resp
}

func (e *ServiceEnv) CreateDocument(t *testing.T, collection string, data map[string]interface{}, token string) map[string]interface{} {
	resp := e.MakeRequest(t, "POST", "/api/v1/"+collection, data, token)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var doc map[string]interface{}
	err := json.NewDecoder(resp.Body).Decode(&doc)
	require.NoError(t, err)
	resp.Body.Close()
	return doc
}

func (e *ServiceEnv) GetDocument(t *testing.T, collection, id, token string) map[string]interface{} {
	resp := e.MakeRequest(t, "GET", fmt.Sprintf("/api/v1/%s/%s", collection, id), nil, token)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var doc map[string]interface{}
	err := json.NewDecoder(resp.Body).Decode(&doc)
	require.NoError(t, err)
	resp.Body.Close()
	return doc
}

func (e *ServiceEnv) PatchDocument(t *testing.T, collection, id string, data map[string]interface{}, token string) map[string]interface{} {
	resp := e.MakeRequest(t, "PATCH", fmt.Sprintf("/api/v1/%s/%s", collection, id), data, token)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var doc map[string]interface{}
	err := json.NewDecoder(resp.Body).Decode(&doc)
	require.NoError(t, err)
	resp.Body.Close()
	return doc
}

func (e *ServiceEnv) PutDocument(t *testing.T, collection, id string, data map[string]interface{}, token string) map[string]interface{} {
	resp := e.MakeRequest(t, "PUT", fmt.Sprintf("/api/v1/%s/%s", collection, id), data, token)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var doc map[string]interface{}
	err := json.NewDecoder(resp.Body).Decode(&doc)
	require.NoError(t, err)
	resp.Body.Close()
	return doc
}

func waitForHealth(t *testing.T, url string) {
	timeout := 10 * time.Second
	deadline := time.Now().Add(timeout)
	client := &http.Client{
		Timeout: 1 * time.Second,
	}
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("Timeout waiting for service at %s to be healthy", url)
}

func getFreePort(t *testing.T) int {
	listener, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	defer listener.Close()

	addr := listener.Addr().(*net.TCPAddr)
	return addr.Port
}

// --- Realtime/WebSocket Helpers ---

const (
	TypeAuth           = "auth"
	TypeAuthAck        = "auth_ack"
	TypeSubscribe      = "subscribe"
	TypeSubscribeAck   = "subscribe_ack"
	TypeUnsubscribe    = "unsubscribe"
	TypeUnsubscribeAck = "unsubscribe_ack"
	TypeEvent          = "event"
	TypeSnapshot       = "snapshot"
	TypeError          = "error"

	EventCreate = "create"
)

type BaseMessage struct {
	ID      string          `json:"id,omitempty"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type AuthPayload struct {
	Token string `json:"token"`
}

type SubscribePayload struct {
	Query        Query `json:"query"`
	IncludeData  bool  `json:"includeData"`
	SendSnapshot bool  `json:"sendSnapshot"`
}

type Query struct {
	Collection string   `json:"collection"`
	Filters    []Filter `json:"filters,omitempty"`
}

type Filter struct {
	Field string      `json:"field"`
	Op    string      `json:"op"`
	Value interface{} `json:"value"`
}

type UnsubscribePayload struct {
	ID string `json:"id"`
}

type EventPayload struct {
	SubID string      `json:"subId"`
	Delta PublicEvent `json:"delta"`
}

type PublicEvent struct {
	Type     string                 `json:"type"` // "create", "update", "delete"
	Document map[string]interface{} `json:"document,omitempty"`
	Path     string                 `json:"path"`
}

type SnapshotPayload struct {
	Documents []map[string]interface{} `json:"documents"`
}

func (e *ServiceEnv) ConnectWebSocket(t *testing.T) *websocket.Conn {
	wsURL := "ws" + strings.TrimPrefix(e.RealtimeURL, "http") + "/realtime/ws"
	dialer := websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
	}
	ws, _, err := dialer.Dial(wsURL, nil)
	require.NoError(t, err, "Failed to connect to websocket")
	return ws
}

func (e *ServiceEnv) AuthenticateWebSocket(t *testing.T, ws *websocket.Conn, token string) {
	authMsg := BaseMessage{
		ID:   "auth-" + t.Name(),
		Type: TypeAuth,
		Payload: mustMarshal(AuthPayload{
			Token: token,
		}),
	}
	err := ws.WriteJSON(authMsg)
	require.NoError(t, err, "Failed to send auth message")

	// Wait for auth_ack with timeout
	ws.SetReadDeadline(time.Now().Add(5 * time.Second))
	msg := e.ReadWebSocketMessage(t, ws)
	require.Equal(t, TypeAuthAck, msg.Type, "Expected auth_ack")
}

func (e *ServiceEnv) ReadWebSocketMessage(t *testing.T, ws *websocket.Conn) BaseMessage {
	var msg BaseMessage
	err := ws.ReadJSON(&msg)
	require.NoError(t, err, "Failed to read websocket message")
	return msg
}

func mustMarshal(v interface{}) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func TestEnvHelper(t *testing.T) {
	t.Parallel()

	// Test setupServiceEnv
	env := setupServiceEnv(t, "")
	defer env.Cancel()

	// Test GenerateSystemToken
	t.Run("GenerateSystemToken", func(t *testing.T) {
		token := env.GenerateSystemToken(t)
		assert.NotEmpty(t, token)
	})

	// Test GetToken
	t.Run("UserManagement", func(t *testing.T) {
		username := "testuser_helper"
		role := "user"

		token := env.GetToken(t, username, role)
		assert.NotEmpty(t, token)

		// Call again to trigger Login path
		token2 := env.GetToken(t, username, role)
		assert.NotEmpty(t, token2)
	})

	t.Run("UserManagement_AdminRole", func(t *testing.T) {
		username := "testadmin_helper"
		role := "admin"

		// First call: SignUp -> Update Role
		token := env.GetToken(t, username, role)
		assert.NotEmpty(t, token)

		// Second call: Login -> Check Role (already present)
		token2 := env.GetToken(t, username, role)
		assert.NotEmpty(t, token2)
	})

	// Test MakeRequest
	t.Run("MakeRequest", func(t *testing.T) {
		resp := env.MakeRequest(t, http.MethodGet, "/health", nil, "")
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})
}

func TestMustMarshal(t *testing.T) {
	// Case 1: Valid input
	input := map[string]string{"key": "value"}
	output := mustMarshal(input)
	var result map[string]string
	err := json.Unmarshal(output, &result)
	require.NoError(t, err)
	assert.Equal(t, input, result)

	// Case 2: Invalid input (channel cannot be marshaled)
	assert.Panics(t, func() {
		mustMarshal(make(chan int))
	})
}

func TestParseTokenClaims(t *testing.T) {
	// Case 1: Valid token
	claims := map[string]interface{}{"sub": "123", "name": "test"}
	claimsBytes, _ := json.Marshal(claims)
	payload := base64.RawURLEncoding.EncodeToString(claimsBytes)
	token := "header." + payload + ".signature"

	parsed, err := parseTokenClaims(token)
	require.NoError(t, err)
	assert.Equal(t, "123", parsed["sub"])
	assert.Equal(t, "test", parsed["name"])

	// Case 2: Invalid format (not enough parts)
	_, err = parseTokenClaims("invalid.token")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid token format")

	// Case 3: Invalid base64
	_, err = parseTokenClaims("header.invalid-base64.signature")
	assert.Error(t, err)

	// Case 4: Invalid JSON in payload
	badPayload := base64.RawURLEncoding.EncodeToString([]byte("{invalid-json"))
	_, err = parseTokenClaims("header." + badPayload + ".signature")
	assert.Error(t, err)
}
