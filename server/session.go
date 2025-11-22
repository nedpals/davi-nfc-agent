package server

import (
	"crypto/rand"
	"fmt"
	"log"
	"sync"
	"time"
)

// SessionManager handles session token lifecycle and validation
type SessionManager struct {
	token     string
	origin    string // Bound origin for the session
	ip        string // Bound IP address for the session
	apiSecret string // Optional API secret for handshake
	timeout   time.Duration
	timer     *time.Timer
	mu        sync.RWMutex
}

// NewSessionManager creates a new session manager
func NewSessionManager(apiSecret string, timeout time.Duration) *SessionManager {
	return &SessionManager{
		apiSecret: apiSecret,
		timeout:   timeout,
	}
}

// generateSessionToken generates a cryptographically secure random session token
func generateSessionToken() string {
	// Generate a random 32-byte token and encode as hex
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		log.Fatalf("Failed to generate session token: %v", err)
	}
	return fmt.Sprintf("%x", b)
}

// Acquire attempts to acquire the session token
// Returns the token if successful, or empty string if already claimed or invalid secret
// origin and remoteAddr are used for optional session binding
func (m *SessionManager) Acquire(secret string, origin string, remoteAddr string) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if API secret is required and validate
	if m.apiSecret != "" && secret != m.apiSecret {
		return ""
	}

	// If no active session, create one
	if m.token == "" {
		m.token = generateSessionToken()
		m.origin = origin
		m.ip = remoteAddr

		// Reset the session timeout timer
		if m.timer != nil {
			m.timer.Stop()
		}
		m.timer = time.AfterFunc(m.timeout, func() {
			m.Release()
			log.Println("Session timeout - token released")
		})

		log.Printf("Session acquired: %s (origin: %s, ip: %s)", m.token[:8]+"...", origin, remoteAddr)
		return m.token
	}

	// Session already claimed
	return ""
}

// Validate checks if the provided token matches the current session
// and optionally validates origin and IP binding
func (m *SessionManager) Validate(token string, origin string, remoteAddr string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Check token match
	if m.token == "" || m.token != token {
		return false
	}

	// Validate origin binding if it was set during acquisition
	if m.origin != "" && origin != m.origin {
		log.Printf("Session validation failed: origin mismatch (expected: %s, got: %s)", m.origin, origin)
		return false
	}

	// Validate IP binding if it was set during acquisition
	if m.ip != "" && remoteAddr != m.ip {
		log.Printf("Session validation failed: IP mismatch (expected: %s, got: %s)", m.ip, remoteAddr)
		return false
	}

	return true
}

// Release releases the current session token
func (m *SessionManager) Release() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.token != "" {
		log.Printf("Session released: %s", m.token[:8]+"...")
		m.token = ""
		m.origin = ""
		m.ip = ""

		if m.timer != nil {
			m.timer.Stop()
			m.timer = nil
		}
	}
}

// RefreshTimeout resets the session timeout timer
func (m *SessionManager) RefreshTimeout() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.timer != nil {
		m.timer.Reset(m.timeout)
	}
}
