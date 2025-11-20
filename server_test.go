package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/nedpals/davi-nfc-agent/nfc"
)

// TestHandshakeEndpoint tests the session handshake flow
func TestHandshakeEndpoint(t *testing.T) {
	// Reset session state before test
	sessionToken = ""
	sessionOrigin = ""
	sessionIP = ""
	apiSecretFlag = ""

	req := httptest.NewRequest(http.MethodPost, "/handshake", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://localhost:3000")
	req.RemoteAddr = "127.0.0.1:12345"

	w := httptest.NewRecorder()
	handler := enableCORS(func(w http.ResponseWriter, r *http.Request) {
		var requestBody struct {
			Secret string `json:"secret,omitempty"`
		}
		json.NewDecoder(r.Body).Decode(&requestBody)

		origin := r.Header.Get("Origin")
		remoteAddr := r.RemoteAddr

		token := acquireSession(requestBody.Secret, origin, remoteAddr)
		if token == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]any{
				"error": "Session already claimed or invalid secret",
			})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"token": token,
		})
	})

	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]string
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response["token"] == "" {
		t.Error("Expected token in response")
	}

	// Verify session binding
	if sessionOrigin != "http://localhost:3000" {
		t.Errorf("Expected origin binding to 'http://localhost:3000', got '%s'", sessionOrigin)
	}

	if sessionIP != "127.0.0.1:12345" {
		t.Errorf("Expected IP binding to '127.0.0.1:12345', got '%s'", sessionIP)
	}
}

// TestHandshakeWithAPISecret tests handshake with API secret validation
func TestHandshakeWithAPISecret(t *testing.T) {
	// Reset session state and set API secret
	sessionToken = ""
	sessionOrigin = ""
	sessionIP = ""
	apiSecretFlag = "test-secret"

	tests := []struct {
		name           string
		secret         string
		expectedStatus int
		expectToken    bool
	}{
		{"Valid secret", "test-secret", http.StatusOK, true},
		{"Invalid secret", "wrong-secret", http.StatusConflict, false},
		{"No secret", "", http.StatusConflict, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset session before each test
			sessionMux.Lock()
			sessionToken = ""
			sessionOrigin = ""
			sessionIP = ""
			if sessionTimer != nil {
				sessionTimer.Stop()
				sessionTimer = nil
			}
			sessionMux.Unlock()

			body := `{"secret":"` + tt.secret + `"}`
			req := httptest.NewRequest(http.MethodPost, "/handshake", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.RemoteAddr = "127.0.0.1:12345"

			w := httptest.NewRecorder()
			handler := enableCORS(func(w http.ResponseWriter, r *http.Request) {
				var requestBody struct {
					Secret string `json:"secret,omitempty"`
				}
				json.NewDecoder(r.Body).Decode(&requestBody)

				origin := r.Header.Get("Origin")
				remoteAddr := r.RemoteAddr

				token := acquireSession(requestBody.Secret, origin, remoteAddr)
				if token == "" {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusConflict)
					json.NewEncoder(w).Encode(map[string]any{
						"error": "Session already claimed or invalid secret",
					})
					return
				}

				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{
					"token": token,
				})
			})

			handler(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			var response map[string]any
			json.NewDecoder(w.Body).Decode(&response)

			if tt.expectToken {
				token, hasToken := response["token"].(string)
				if !hasToken || token == "" {
					t.Error("Expected token in response")
				}
			} else {
				if _, hasToken := response["token"]; hasToken {
					t.Error("Expected no token in response, but got one")
				}
			}
		})
	}

	// Cleanup
	apiSecretFlag = ""
}

// TestSessionValidation tests session validation with origin and IP binding
func TestSessionValidation(t *testing.T) {
	// Setup session
	sessionToken = "test-token-123"
	sessionOrigin = "http://localhost:3000"
	sessionIP = "127.0.0.1:12345"

	tests := []struct {
		name       string
		token      string
		origin     string
		remoteAddr string
		expected   bool
	}{
		{"Valid session", "test-token-123", "http://localhost:3000", "127.0.0.1:12345", true},
		{"Invalid token", "wrong-token", "http://localhost:3000", "127.0.0.1:12345", false},
		{"Wrong origin", "test-token-123", "http://evil.com", "127.0.0.1:12345", false},
		{"Wrong IP", "test-token-123", "http://localhost:3000", "192.168.1.1:54321", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validateSession(tt.token, tt.origin, tt.remoteAddr)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestBuildNDEFMessageWithOptions tests the NDEF message builder
func TestBuildNDEFMessageWithOptions(t *testing.T) {
	tests := []struct {
		name        string
		request     WriteRequest
		expectError bool
		checkOpts   func(*testing.T, nfc.WriteOptions)
	}{
		{
			name: "Simple text record",
			request: WriteRequest{
				Text: "Hello, World!",
			},
			expectError: false,
			checkOpts: func(t *testing.T, opts nfc.WriteOptions) {
				if !opts.Overwrite {
					t.Error("Expected Overwrite to be true for simple write")
				}
				if opts.Index != -1 {
					t.Error("Expected Index to be -1 for simple write")
				}
			},
		},
		{
			name: "Append mode",
			request: WriteRequest{
				Text:   "Additional text",
				Append: true,
			},
			expectError: false,
			checkOpts: func(t *testing.T, opts nfc.WriteOptions) {
				if opts.Overwrite {
					t.Error("Expected Overwrite to be false for append")
				}
				if opts.Index != -1 {
					t.Error("Expected Index to be -1 for append")
				}
			},
		},
		{
			name: "Replace mode",
			request: WriteRequest{
				Text:    "New content",
				Replace: true,
			},
			expectError: false,
			checkOpts: func(t *testing.T, opts nfc.WriteOptions) {
				if !opts.Overwrite {
					t.Error("Expected Overwrite to be true for replace")
				}
				if opts.Index != -1 {
					t.Error("Expected Index to be -1 for replace")
				}
			},
		},
		{
			name: "Update specific record",
			request: WriteRequest{
				Text:        "Updated text",
				RecordIndex: intPtr(0),
			},
			expectError: false,
			checkOpts: func(t *testing.T, opts nfc.WriteOptions) {
				if opts.Overwrite {
					t.Error("Expected Overwrite to be false for record update")
				}
				if opts.Index != 0 {
					t.Errorf("Expected Index to be 0, got %d", opts.Index)
				}
			},
		},
		{
			name: "URI record",
			request: WriteRequest{
				Text:       "https://example.com",
				RecordType: "uri",
			},
			expectError: false,
			checkOpts: func(t *testing.T, opts nfc.WriteOptions) {
				if !opts.Overwrite {
					t.Error("Expected Overwrite to be true for simple URI write")
				}
			},
		},
		{
			name: "Unsupported record type",
			request: WriteRequest{
				Text:       "test",
				RecordType: "unknown",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, opts, err := buildNDEFMessageWithOptions(tt.request)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if msg == nil {
				t.Fatal("Expected NDEF message, got nil")
			}

			if tt.checkOpts != nil {
				tt.checkOpts(t, opts)
			}
		})
	}
}

// TestHandleWriteRequest tests the write request handler with a mock reader
func TestHandleWriteRequest(t *testing.T) {
	// Create a mock reader
	mockReader := &mockNFCReader{
		writeFunc: func(msg *nfc.NDEFMessage, opts nfc.WriteOptions) error {
			return nil
		},
	}

	// Create context with reader
	ctx := context.WithValue(context.Background(), readerContextKey, mockReader)

	// Create test WebSocket server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("Failed to upgrade: %v", err)
			return
		}
		defer conn.Close()

		// Wait for write request
		var wsMessage WebSocketMessage
		if err := conn.ReadJSON(&wsMessage); err != nil {
			t.Fatalf("Failed to read message: %v", err)
			return
		}

		handleWriteRequest(conn, wsMessage, ctx)
	}))
	defer server.Close()

	// Connect WebSocket client
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Send write request
	writeReq := WebSocketMessage{
		Type: "writeRequest",
		Payload: map[string]any{
			"text": "Test message",
		},
	}

	if err := conn.WriteJSON(writeReq); err != nil {
		t.Fatalf("Failed to send write request: %v", err)
	}

	// Read response
	var response WebSocketMessage
	if err := conn.ReadJSON(&response); err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	if response.Type != "writeResponse" {
		t.Errorf("Expected writeResponse, got %s", response.Type)
	}

	payload, ok := response.Payload.(map[string]any)
	if !ok {
		t.Fatal("Payload is not a map")
	}

	if !payload["success"].(bool) {
		t.Error("Expected success to be true")
	}
}

// Helper function to create int pointer
func intPtr(i int) *int {
	return &i
}

// Mock NFC Reader for testing
type mockNFCReader struct {
	writeFunc func(msg *nfc.NDEFMessage, opts nfc.WriteOptions) error
}

func (m *mockNFCReader) WriteMessageWithOptions(msg *nfc.NDEFMessage, opts nfc.WriteOptions) error {
	if m.writeFunc != nil {
		return m.writeFunc(msg, opts)
	}
	return nil
}

func (m *mockNFCReader) Start()                     {}
func (m *mockNFCReader) Stop()                      {}
func (m *mockNFCReader) Close()                     {}
func (m *mockNFCReader) Data() chan nfc.NFCData    { return make(chan nfc.NFCData) }
func (m *mockNFCReader) StatusUpdates() chan nfc.DeviceStatus {
	return make(chan nfc.DeviceStatus)
}
func (m *mockNFCReader) GetDeviceStatus() nfc.DeviceStatus {
	return nfc.DeviceStatus{Connected: true, Message: "Mock device", CardPresent: false}
}
func (m *mockNFCReader) GetLastScannedData() string { return "" }
func (m *mockNFCReader) SetMode(mode nfc.ReaderMode) {}
func (m *mockNFCReader) LogDeviceInfo()              {}
func (m *mockNFCReader) WriteCardData(text string, options map[string]any) error {
	return nil
}

// TestSessionTimeout tests session timeout functionality
func TestSessionTimeout(t *testing.T) {
	// Reset session
	sessionToken = ""
	sessionOrigin = ""
	sessionIP = ""
	apiSecretFlag = ""

	// Acquire session with short timeout
	originalTimeout := sessionTimeout
	sessionTimeout = 100 * time.Millisecond
	defer func() { sessionTimeout = originalTimeout }()

	token := acquireSession("", "http://localhost:3000", "127.0.0.1:12345")
	if token == "" {
		t.Fatal("Failed to acquire session")
	}

	// Wait for timeout
	time.Sleep(150 * time.Millisecond)

	// Session should be released
	sessionMux.RLock()
	released := sessionToken == ""
	sessionMux.RUnlock()

	if !released {
		t.Error("Session should have timed out")
	}
}
