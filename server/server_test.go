package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestHandshakeEndpoint tests the session handshake flow
func TestHandshakeEndpoint(t *testing.T) {
	// Create a new session manager for testing
	testSessionManager := NewSessionManager("", 60*time.Second)

	req := httptest.NewRequest(http.MethodPost, "/handshake", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://localhost:3000")
	req.RemoteAddr = "127.0.0.1:12345"

	w := httptest.NewRecorder()
	handler := func(w http.ResponseWriter, r *http.Request) {
		var requestBody struct {
			Secret string `json:"secret,omitempty"`
		}
		json.NewDecoder(r.Body).Decode(&requestBody)

		origin := r.Header.Get("Origin")
		remoteAddr := r.RemoteAddr

		token := testSessionManager.Acquire(requestBody.Secret, origin, remoteAddr)
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
	}

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

	// Verify token is valid
	if !testSessionManager.Validate(response["token"], "http://localhost:3000", "127.0.0.1:12345") {
		t.Error("Expected token to be valid with correct origin and IP")
	}
}

// TestHandshakeWithAPISecret tests handshake with API secret validation
func TestHandshakeWithAPISecret(t *testing.T) {
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
			// Create a new session manager with API secret for each test
			testSessionManager := NewSessionManager("test-secret", 60*time.Second)

			body := `{"secret":"` + tt.secret + `"}`
			req := httptest.NewRequest(http.MethodPost, "/handshake", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.RemoteAddr = "127.0.0.1:12345"

			w := httptest.NewRecorder()
			handler := func(w http.ResponseWriter, r *http.Request) {
				var requestBody struct {
					Secret string `json:"secret,omitempty"`
				}
				json.NewDecoder(r.Body).Decode(&requestBody)

				origin := r.Header.Get("Origin")
				remoteAddr := r.RemoteAddr

				token := testSessionManager.Acquire(requestBody.Secret, origin, remoteAddr)
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
			}

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
}
