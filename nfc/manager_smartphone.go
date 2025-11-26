package nfc

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// SmartphoneManager implements the Manager interface for managing smartphone connections.
type SmartphoneManager struct {
	devices           map[string]*SmartphoneDevice // deviceID -> device
	mu                sync.RWMutex                 // Protects devices map
	cleanupTicker     *time.Ticker                 // Periodic cleanup of inactive devices
	stopCleanup       chan struct{}                // Stop cleanup goroutine
	inactivityTimeout time.Duration                // Device timeout duration
}

// NewSmartphoneManager creates a new smartphone manager.
func NewSmartphoneManager(inactivityTimeout time.Duration) *SmartphoneManager {
	if inactivityTimeout == 0 {
		inactivityTimeout = SmartphoneDeviceTimeout
	}

	sm := &SmartphoneManager{
		devices:           make(map[string]*SmartphoneDevice),
		inactivityTimeout: inactivityTimeout,
		stopCleanup:       make(chan struct{}),
	}

	// Start cleanup routine
	sm.startCleanupRoutine()

	return sm
}

// OpenDevice opens connection to a registered smartphone device by ID.
// Format: "smartphone:{deviceID}" or just "{deviceID}"
func (sm *SmartphoneManager) OpenDevice(deviceStr string) (Device, error) {
	// Parse device string
	deviceID := deviceStr
	if strings.HasPrefix(deviceStr, "smartphone:") {
		deviceID = strings.TrimPrefix(deviceStr, "smartphone:")
	}

	sm.mu.RLock()
	device, exists := sm.devices[deviceID]
	sm.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("smartphone device not found: %s", deviceID)
	}

	if !device.IsActive() {
		return nil, fmt.Errorf("smartphone device is inactive: %s", deviceID)
	}

	return device, nil
}

// ListDevices returns list of connected smartphone device connection strings.
func (sm *SmartphoneManager) ListDevices() ([]string, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	devices := make([]string, 0, len(sm.devices))
	for deviceID, device := range sm.devices {
		if device.IsActive() {
			devices = append(devices, fmt.Sprintf("smartphone:%s", deviceID))
		}
	}

	return devices, nil
}

// RegisterDevice creates and registers a new smartphone device.
func (sm *SmartphoneManager) RegisterDevice(req DeviceRegistrationRequest) (*SmartphoneDevice, error) {
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
	device := NewSmartphoneDevice(deviceID, req)

	// Register device
	sm.mu.Lock()
	sm.devices[deviceID] = device
	sm.mu.Unlock()

	log.Printf("[smartphone] Device registered: %s (%s, %s)", device.String(), req.Platform, req.AppVersion)

	return device, nil
}

// UnregisterDevice removes a smartphone device.
func (sm *SmartphoneManager) UnregisterDevice(deviceID string) error {
	sm.mu.Lock()
	device, exists := sm.devices[deviceID]
	if exists {
		delete(sm.devices, deviceID)
	}
	sm.mu.Unlock()

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
func (sm *SmartphoneManager) GetDevice(deviceID string) (*SmartphoneDevice, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	device, exists := sm.devices[deviceID]
	return device, exists
}

// SendTagData sends scanned tags to a device's channel.
func (sm *SmartphoneManager) SendTagData(deviceID string, tagData SmartphoneTagData) error {
	sm.mu.RLock()
	device, exists := sm.devices[deviceID]
	sm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("device not found: %s", deviceID)
	}

	// Convert tag data to internal format
	tag, err := ConvertSmartphoneTagData(tagData)
	if err != nil {
		return fmt.Errorf("failed to convert tag data: %w", err)
	}

	// Send to device's tag channel
	tags := []Tag{tag}
	if err := device.SendTags(tags); err != nil {
		return fmt.Errorf("failed to send tags to device: %w", err)
	}

	// Update heartbeat
	device.UpdateLastSeen()

	return nil
}

// UpdateHeartbeat updates device last-seen timestamp.
func (sm *SmartphoneManager) UpdateHeartbeat(deviceID string) error {
	sm.mu.RLock()
	device, exists := sm.devices[deviceID]
	sm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("device not found: %s", deviceID)
	}

	device.UpdateLastSeen()
	return nil
}

// Close cleanup and stop background tasks.
func (sm *SmartphoneManager) Close() {
	// Stop cleanup routine
	if sm.cleanupTicker != nil {
		sm.cleanupTicker.Stop()
	}
	close(sm.stopCleanup)

	// Close all devices
	sm.mu.Lock()
	for deviceID, device := range sm.devices {
		if err := device.Close(); err != nil {
			log.Printf("[smartphone] Error closing device %s: %v", deviceID, err)
		}
	}
	sm.devices = make(map[string]*SmartphoneDevice)
	sm.mu.Unlock()

	log.Printf("[smartphone] SmartphoneManager closed")
}

// startCleanupRoutine starts a background goroutine to cleanup inactive devices.
func (sm *SmartphoneManager) startCleanupRoutine() {
	sm.cleanupTicker = time.NewTicker(SmartphoneCleanupInterval)

	go func() {
		for {
			select {
			case <-sm.cleanupTicker.C:
				sm.cleanupInactiveDevices()
			case <-sm.stopCleanup:
				return
			}
		}
	}()
}

// cleanupInactiveDevices removes devices that exceeded inactivity timeout.
func (sm *SmartphoneManager) cleanupInactiveDevices() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	now := time.Now()
	for deviceID, device := range sm.devices {
		timeSinceLastSeen := now.Sub(device.LastSeen())
		if timeSinceLastSeen > sm.inactivityTimeout {
			log.Printf("[smartphone] Cleaning up inactive device: %s (last seen %v ago)", device.String(), timeSinceLastSeen)
			
			// Close and remove device
			if err := device.Close(); err != nil {
				log.Printf("[smartphone] Error closing device %s: %v", deviceID, err)
			}
			delete(sm.devices, deviceID)
		}
	}
}

// GetDeviceCount returns the number of registered devices.
func (sm *SmartphoneManager) GetDeviceCount() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return len(sm.devices)
}

// GetActiveDeviceCount returns the number of active devices.
func (sm *SmartphoneManager) GetActiveDeviceCount() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	count := 0
	for _, device := range sm.devices {
		if device.IsActive() {
			count++
		}
	}
	return count
}
