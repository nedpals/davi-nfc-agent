package server

import (
	"testing"
	"time"
)

// TestAcquire tests basic session acquisition
func TestAcquire(t *testing.T) {
	manager := NewSessionManager("", 60*time.Second)

	// First acquisition should succeed
	token := manager.Acquire("", "http://localhost:3000", "127.0.0.1:12345")
	if token == "" {
		t.Error("Expected token on first acquisition")
	}

	// Second acquisition should fail (session already claimed)
	token2 := manager.Acquire("", "http://localhost:3001", "127.0.0.1:12346")
	if token2 != "" {
		t.Error("Expected empty token on second acquisition (session already claimed)")
	}

	// Release and try again
	manager.Release()
	token3 := manager.Acquire("", "http://localhost:3002", "127.0.0.1:12347")
	if token3 == "" {
		t.Error("Expected token after release")
	}
}

// TestAcquireWithAPISecret tests session acquisition with API secret validation
func TestAcquireWithAPISecret(t *testing.T) {
	secret := "test-secret"
	manager := NewSessionManager(secret, 60*time.Second)

	tests := []struct {
		name        string
		secret      string
		expectToken bool
	}{
		{"Valid secret", "test-secret", true},
		{"Invalid secret", "wrong-secret", false},
		{"No secret", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Release any existing session
			manager.Release()

			token := manager.Acquire(tt.secret, "http://localhost:3000", "127.0.0.1:12345")
			if tt.expectToken && token == "" {
				t.Error("Expected token with valid secret")
			}
			if !tt.expectToken && token != "" {
				t.Error("Expected empty token with invalid secret")
			}
		})
	}
}

// TestValidate tests session validation with origin and IP binding
func TestValidate(t *testing.T) {
	manager := NewSessionManager("", 60*time.Second)

	origin := "http://localhost:3000"
	ip := "127.0.0.1:12345"

	token := manager.Acquire("", origin, ip)
	if token == "" {
		t.Fatal("Failed to acquire session")
	}

	tests := []struct {
		name     string
		token    string
		origin   string
		ip       string
		expected bool
	}{
		{"Valid token and binding", token, origin, ip, true},
		{"Invalid token", "wrong-token", origin, ip, false},
		{"Wrong origin", token, "http://evil.com", ip, false},
		{"Wrong IP", token, origin, "192.168.1.1:8080", false},
		{"Empty token", "", origin, ip, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := manager.Validate(tt.token, tt.origin, tt.ip)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestRelease tests session release
func TestRelease(t *testing.T) {
	manager := NewSessionManager("", 60*time.Second)

	token := manager.Acquire("", "http://localhost:3000", "127.0.0.1:12345")
	if token == "" {
		t.Fatal("Failed to acquire session")
	}

	// Validate before release
	if !manager.Validate(token, "http://localhost:3000", "127.0.0.1:12345") {
		t.Error("Session should be valid before release")
	}

	// Release
	manager.Release()

	// Validate after release
	if manager.Validate(token, "http://localhost:3000", "127.0.0.1:12345") {
		t.Error("Session should be invalid after release")
	}

	// Should be able to acquire new session
	token2 := manager.Acquire("", "http://localhost:3001", "127.0.0.1:12346")
	if token2 == "" {
		t.Error("Should be able to acquire new session after release")
	}
}

// TestRefreshTimeout tests session timeout refresh
func TestRefreshTimeout(t *testing.T) {
	manager := NewSessionManager("", 100*time.Millisecond) // Short timeout for testing

	token := manager.Acquire("", "http://localhost:3000", "127.0.0.1:12345")
	if token == "" {
		t.Fatal("Failed to acquire session")
	}

	// Wait half the timeout period
	time.Sleep(50 * time.Millisecond)

	// Refresh the timeout
	manager.RefreshTimeout()

	// Wait another half period (should not timeout yet)
	time.Sleep(50 * time.Millisecond)

	// Session should still be valid
	if !manager.Validate(token, "http://localhost:3000", "127.0.0.1:12345") {
		t.Error("Session should still be valid after refresh")
	}

	// Wait for full timeout
	time.Sleep(100 * time.Millisecond)

	// Session should now be invalid
	if manager.Validate(token, "http://localhost:3000", "127.0.0.1:12345") {
		t.Error("Session should be invalid after timeout")
	}
}

// TestSessionTimeout tests automatic session timeout
func TestSessionTimeout(t *testing.T) {
	manager := NewSessionManager("", 100*time.Millisecond) // Short timeout for testing

	token := manager.Acquire("", "http://localhost:3000", "127.0.0.1:12345")
	if token == "" {
		t.Fatal("Failed to acquire session")
	}

	// Session should be valid immediately
	if !manager.Validate(token, "http://localhost:3000", "127.0.0.1:12345") {
		t.Error("Session should be valid immediately after acquisition")
	}

	// Wait for timeout
	time.Sleep(150 * time.Millisecond)

	// Session should be invalid after timeout
	if manager.Validate(token, "http://localhost:3000", "127.0.0.1:12345") {
		t.Error("Session should be invalid after timeout")
	}

	// Should be able to acquire new session after timeout
	token2 := manager.Acquire("", "http://localhost:3001", "127.0.0.1:12346")
	if token2 == "" {
		t.Error("Should be able to acquire new session after timeout")
	}
}
