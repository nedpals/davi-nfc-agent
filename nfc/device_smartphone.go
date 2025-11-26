package nfc

import (
	"fmt"
	"sync"
	"time"
)

// SmartphoneDevice implements the Device interface for smartphone NFC scanning.
type SmartphoneDevice struct {
	deviceID     string                 // Unique ID for this smartphone (UUID)
	connection   string                 // Connection info (e.g., "smartphone:uuid")
	deviceName   string                 // Human-readable name (e.g., "iPhone 12 Pro")
	platform     string                 // "ios" or "android"
	appVersion   string                 // Mobile app version
	isActive     bool                   // Whether device is connected
	tagChannel   chan []Tag             // Channel to receive tags from smartphone
	closeChannel chan struct{}          // Signal to close device
	mu           sync.RWMutex           // Protects device state
	lastSeen     time.Time              // Last activity timestamp (for health monitoring)
	capabilities DeviceCapabilities     // Read/write capabilities
	metadata     map[string]string      // Additional device info
}

// NewSmartphoneDevice creates a new smartphone device instance.
func NewSmartphoneDevice(deviceID string, req DeviceRegistrationRequest) *SmartphoneDevice {
	return &SmartphoneDevice{
		deviceID:     deviceID,
		connection:   fmt.Sprintf("smartphone:%s", deviceID),
		deviceName:   req.DeviceName,
		platform:     req.Platform,
		appVersion:   req.AppVersion,
		isActive:     true,
		tagChannel:   make(chan []Tag, SmartphoneTagChannelBuffer),
		closeChannel: make(chan struct{}),
		lastSeen:     time.Now(),
		capabilities: req.Capabilities,
		metadata:     req.Metadata,
	}
}

// Close closes the device connection.
func (sd *SmartphoneDevice) Close() error {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	if !sd.isActive {
		return nil
	}

	sd.isActive = false
	close(sd.closeChannel)
	close(sd.tagChannel)

	return nil
}

// InitiatorInit initializes the device (validates connection health).
func (sd *SmartphoneDevice) InitiatorInit() error {
	sd.mu.RLock()
	defer sd.mu.RUnlock()

	if !sd.isActive {
		return fmt.Errorf("device is not active")
	}

	// Check if device has timed out
	timeSinceLastSeen := time.Since(sd.lastSeen)
	if timeSinceLastSeen > SmartphoneDeviceTimeout {
		return fmt.Errorf("device timeout: last seen %v ago", timeSinceLastSeen)
	}

	return nil
}

// String returns a human-readable device name.
func (sd *SmartphoneDevice) String() string {
	return fmt.Sprintf("%s [%s]", sd.deviceName, sd.connection)
}

// Connection returns the device connection string.
func (sd *SmartphoneDevice) Connection() string {
	return sd.connection
}

// Transceive is not directly applicable for smartphones.
func (sd *SmartphoneDevice) Transceive(txData []byte) ([]byte, error) {
	return nil, fmt.Errorf("transceive not supported on smartphone devices")
}

// GetTags returns tags from the tag channel with timeout.
// Returns empty slice (not error) on timeout to maintain compatibility with polling loop.
func (sd *SmartphoneDevice) GetTags() ([]Tag, error) {
	sd.mu.RLock()
	if !sd.isActive {
		sd.mu.RUnlock()
		return nil, fmt.Errorf("device is closed")
	}
	sd.mu.RUnlock()

	// Wait for tags with timeout
	select {
	case tags, ok := <-sd.tagChannel:
		if !ok {
			// Channel closed
			return nil, fmt.Errorf("device is closed")
		}
		return tags, nil
	case <-time.After(SmartphoneGetTagsTimeout):
		// Timeout - return empty slice (not error) for compatibility
		return []Tag{}, nil
	case <-sd.closeChannel:
		return nil, fmt.Errorf("device is closed")
	}
}

// SendTags sends tags to the device's tag channel (called by manager).
func (sd *SmartphoneDevice) SendTags(tags []Tag) error {
	sd.mu.RLock()
	defer sd.mu.RUnlock()

	if !sd.isActive {
		return fmt.Errorf("device is not active")
	}

	// Non-blocking send
	select {
	case sd.tagChannel <- tags:
		return nil
	default:
		// Channel full - skip this update
		return fmt.Errorf("tag channel full, skipping update")
	}
}

// UpdateLastSeen updates the device's last activity timestamp.
func (sd *SmartphoneDevice) UpdateLastSeen() {
	sd.mu.Lock()
	defer sd.mu.Unlock()
	sd.lastSeen = time.Now()
}

// IsActive returns whether the device is currently active.
func (sd *SmartphoneDevice) IsActive() bool {
	sd.mu.RLock()
	defer sd.mu.RUnlock()
	return sd.isActive
}

// LastSeen returns the last activity timestamp.
func (sd *SmartphoneDevice) LastSeen() time.Time {
	sd.mu.RLock()
	defer sd.mu.RUnlock()
	return sd.lastSeen
}

// DeviceID returns the device's unique identifier.
func (sd *SmartphoneDevice) DeviceID() string {
	return sd.deviceID
}

// Platform returns the device platform ("ios" or "android").
func (sd *SmartphoneDevice) Platform() string {
	return sd.platform
}

// AppVersion returns the mobile app version.
func (sd *SmartphoneDevice) AppVersion() string {
	return sd.appVersion
}

// Capabilities returns the device capabilities.
func (sd *SmartphoneDevice) Capabilities() DeviceCapabilities {
	sd.mu.RLock()
	defer sd.mu.RUnlock()
	return sd.capabilities
}

// Metadata returns additional device metadata.
func (sd *SmartphoneDevice) Metadata() map[string]string {
	sd.mu.RLock()
	defer sd.mu.RUnlock()
	// Return a copy to prevent external modification
	metadataCopy := make(map[string]string)
	for k, v := range sd.metadata {
		metadataCopy[k] = v
	}
	return metadataCopy
}
