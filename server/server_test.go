package server

import (
	"testing"
)

// TestServerConfig tests the server configuration
func TestServerConfig(t *testing.T) {
	config := Config{
		Reader:           nil, // Mock reader would go here
		Port:             18080,
		APISecret:        "test-secret",
		AllowedCardTypes: map[string]bool{"MIFARE Classic 1K": true},
	}

	server := New(config)

	if server == nil {
		t.Fatal("Expected server instance, got nil")
	}

	if server.config.Port != 18080 {
		t.Errorf("Expected port 18080, got %d", server.config.Port)
	}

	if server.config.APISecret != "test-secret" {
		t.Errorf("Expected API secret 'test-secret', got '%s'", server.config.APISecret)
	}

	if !server.config.AllowedCardTypes["MIFARE Classic 1K"] {
		t.Error("Expected MIFARE Classic 1K to be allowed")
	}
}

// TestSessionLocking tests the session locking mechanism
func TestSessionLocking(t *testing.T) {
	server := New(Config{
		Port:      18080,
		APISecret: "",
	})

	// Initially no session should be active
	if server.sessionActive {
		t.Error("Expected sessionActive to be false initially")
	}

	// Simulate acquiring session
	server.sessionMux.Lock()
	if server.sessionActive {
		t.Error("Session should not be active yet")
	}
	server.sessionActive = true
	server.sessionMux.Unlock()

	// Check session is active
	server.sessionMux.Lock()
	isActive := server.sessionActive
	server.sessionMux.Unlock()

	if !isActive {
		t.Error("Expected session to be active")
	}

	// Release session
	server.sessionMux.Lock()
	server.sessionActive = false
	server.sessionMux.Unlock()

	// Check session is released
	server.sessionMux.Lock()
	isActive = server.sessionActive
	server.sessionMux.Unlock()

	if isActive {
		t.Error("Expected session to be released")
	}
}
