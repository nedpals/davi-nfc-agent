package phonenfc

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nedpals/davi-nfc-agent/nfc"
	"github.com/nedpals/davi-nfc-agent/server"
)

// Manager implements the nfc.Manager interface for managing smartphone connections.
type Manager struct {
	devices           map[string]*Device // deviceID -> device
	mu                sync.RWMutex       // Protects devices map
	cleanupTicker     *time.Ticker       // Periodic cleanup of inactive devices
	stopCleanup       chan struct{}      // Stop cleanup goroutine
	inactivityTimeout time.Duration      // Device timeout duration
}

// NewManager creates a new smartphone manager.
func NewManager(inactivityTimeout time.Duration) *Manager {
	if inactivityTimeout == 0 {
		inactivityTimeout = DeviceTimeout
	}

	m := &Manager{
		devices:           make(map[string]*Device),
		inactivityTimeout: inactivityTimeout,
		stopCleanup:       make(chan struct{}),
	}

	// Start cleanup routine
	m.startCleanupRoutine()

	return m
}

// OpenDevice opens connection to a registered smartphone device by ID.
// Format: "smartphone:{deviceID}" or just "{deviceID}"
func (m *Manager) OpenDevice(deviceStr string) (nfc.Device, error) {
	// Parse device string
	deviceID := deviceStr
	if strings.HasPrefix(deviceStr, "smartphone:") {
		deviceID = strings.TrimPrefix(deviceStr, "smartphone:")
	}

	m.mu.RLock()
	device, exists := m.devices[deviceID]
	m.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("smartphone device not found: %s", deviceID)
	}

	if !device.IsActive() {
		return nil, fmt.Errorf("smartphone device is inactive: %s", deviceID)
	}

	return device, nil
}

// ListDevices returns list of connected smartphone device connection strings.
func (m *Manager) ListDevices() ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	devices := make([]string, 0, len(m.devices))
	for deviceID, device := range m.devices {
		if device.IsActive() {
			devices = append(devices, fmt.Sprintf("smartphone:%s", deviceID))
		}
	}

	return devices, nil
}

// RegisterDevice creates and registers a new smartphone device.
func (m *Manager) RegisterDevice(req DeviceRegistrationRequest) (*Device, error) {
	// Validate request
	if req.DeviceName == "" {
		return nil, fmt.Errorf("device name is required")
	}
	if req.Platform != "ios" && req.Platform != "android" {
		return nil, fmt.Errorf("invalid platform: %s (must be 'ios' or 'android')", req.Platform)
	}

	// Generate unique device ID
	deviceID := uuid.New().String()

	// Create device
	device := NewDevice(deviceID, req)

	// Register device
	m.mu.Lock()
	m.devices[deviceID] = device
	m.mu.Unlock()

	log.Printf("[smartphone] Device registered: %s (%s, %s)", device.String(), req.Platform, req.AppVersion)

	return device, nil
}

// UnregisterDevice removes a smartphone device.
func (m *Manager) UnregisterDevice(deviceID string) error {
	m.mu.Lock()
	device, exists := m.devices[deviceID]
	if exists {
		delete(m.devices, deviceID)
	}
	m.mu.Unlock()

	if !exists {
		return fmt.Errorf("device not found: %s", deviceID)
	}

	// Close the device
	if err := device.Close(); err != nil {
		log.Printf("[smartphone] Error closing device %s: %v", deviceID, err)
	}

	log.Printf("[smartphone] Device unregistered: %s", device.String())

	return nil
}

// GetDevice retrieves a device by ID.
func (m *Manager) GetDevice(deviceID string) (*Device, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	device, exists := m.devices[deviceID]
	return device, exists
}

// SendTagData sends scanned tags to a device's channel.
func (m *Manager) SendTagData(deviceID string, tagData TagData) error {
	m.mu.RLock()
	device, exists := m.devices[deviceID]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("device not found: %s", deviceID)
	}

	// Convert tag data to internal format
	tag, err := ConvertTagData(tagData)
	if err != nil {
		return fmt.Errorf("failed to convert tag data: %w", err)
	}

	// Send to device's tag channel
	tags := []nfc.Tag{tag}
	if err := device.SendTags(tags); err != nil {
		return fmt.Errorf("failed to send tags to device: %w", err)
	}

	// Update heartbeat
	device.UpdateLastSeen()

	return nil
}

// UpdateHeartbeat updates device last-seen timestamp.
func (m *Manager) UpdateHeartbeat(deviceID string) error {
	m.mu.RLock()
	device, exists := m.devices[deviceID]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("device not found: %s", deviceID)
	}

	device.UpdateLastSeen()
	return nil
}

// Close cleanup and stop background tasks.
func (m *Manager) Close() {
	// Stop cleanup routine
	if m.cleanupTicker != nil {
		m.cleanupTicker.Stop()
	}
	close(m.stopCleanup)

	// Close all devices
	m.mu.Lock()
	for deviceID, device := range m.devices {
		if err := device.Close(); err != nil {
			log.Printf("[smartphone] Error closing device %s: %v", deviceID, err)
		}
	}
	m.devices = make(map[string]*Device)
	m.mu.Unlock()

	log.Printf("[smartphone] Manager closed")
}

// startCleanupRoutine starts a background goroutine to cleanup inactive devices.
func (m *Manager) startCleanupRoutine() {
	m.cleanupTicker = time.NewTicker(CleanupInterval)

	go func() {
		for {
			select {
			case <-m.cleanupTicker.C:
				m.cleanupInactiveDevices()
			case <-m.stopCleanup:
				return
			}
		}
	}()
}

// cleanupInactiveDevices removes devices that exceeded inactivity timeout.
func (m *Manager) cleanupInactiveDevices() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for deviceID, device := range m.devices {
		timeSinceLastSeen := now.Sub(device.LastSeen())
		if timeSinceLastSeen > m.inactivityTimeout {
			log.Printf("[smartphone] Cleaning up inactive device: %s (last seen %v ago)", device.String(), timeSinceLastSeen)

			// Close and remove device
			if err := device.Close(); err != nil {
				log.Printf("[smartphone] Error closing device %s: %v", deviceID, err)
			}
			delete(m.devices, deviceID)
		}
	}
}

// GetDeviceCount returns the number of registered devices.
func (m *Manager) GetDeviceCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.devices)
}

// GetActiveDeviceCount returns the number of active devices.
func (m *Manager) GetActiveDeviceCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, device := range m.devices {
		if device.IsActive() {
			count++
		}
	}
	return count
}

// Register implements server.ServerHandler interface.
// Delegates to internal Handler for WebSocket registration.
func (m *Manager) Register(s server.HandlerServer) {
	handler := NewHandler(m)
	handler.Register(s)
}
