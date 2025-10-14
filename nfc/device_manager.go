package nfc

import (
	"fmt"
	"log"
	"math"
	"strings"
	"sync"
	"time"
)

// DeviceManager handles device lifecycle, connection management, and reconnection logic.
// It maintains a connection to a single NFC device and handles recovery from errors.
type DeviceManager struct {
	manager    Manager
	device     Device
	devicePath string
	hasDevice  bool

	// Reconnection state
	retryCount    int
	inCooldown    bool
	cooldownTimer *time.Timer

	// Status tracking
	mu sync.RWMutex
}

// NewDeviceManager creates a new DeviceManager for managing an NFC device connection.
func NewDeviceManager(manager Manager, devicePath string) *DeviceManager {
	timer := time.NewTimer(0)
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}

	return &DeviceManager{
		manager:       manager,
		devicePath:    devicePath,
		hasDevice:     false,
		cooldownTimer: timer,
	}
}

// Device returns the current active device, or nil if not connected.
func (dm *DeviceManager) Device() Device {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.device
}

// HasDevice returns true if a device is currently connected.
func (dm *DeviceManager) HasDevice() bool {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.hasDevice
}

// InCooldown returns true if the device manager is in a cooldown period.
func (dm *DeviceManager) InCooldown() bool {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.inCooldown
}

// DevicePath returns the path of the device being managed.
func (dm *DeviceManager) DevicePath() string {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.devicePath
}

// TryConnect attempts to connect to the device. If the device is already connected
// and responsive, it returns nil. Otherwise, it attempts to open and initialize the device.
func (dm *DeviceManager) TryConnect() error {
	dm.mu.Lock()
	hasDev := dm.hasDevice
	currentDevice := dm.device
	dm.mu.Unlock()

	var initErr error
	if hasDev && currentDevice != nil {
		// Quick check if device is responsive
		initErr = currentDevice.InitiatorInit()
		if initErr == nil {
			log.Println("Device already connected and responsive.")
			return nil
		}
		log.Printf("Device was marked connected, but Init failed: %v. Attempting full reconnect.", initErr)
		dm.mu.Lock()
		currentDevice.Close() // Ignore error
		dm.device = nil
		dm.hasDevice = false
		dm.mu.Unlock()
	}

	devicePathToConnect := dm.devicePath
	if devicePathToConnect == "" {
		devices, errList := dm.manager.ListDevices()
		if errList != nil {
			return fmt.Errorf("error listing NFC devices: %w", errList)
		}
		if len(devices) == 0 {
			return fmt.Errorf("no NFC devices found by manager")
		}
		devicePathToConnect = devices[0]
		log.Printf("No specific device path, trying first available: %s", devicePathToConnect)
	}

	log.Printf("Attempting to connect to device: %s", devicePathToConnect)
	newDevice, errOpen := dm.manager.OpenDevice(devicePathToConnect)
	if errOpen != nil {
		return fmt.Errorf("failed to open device %s: %w", devicePathToConnect, errOpen)
	}

	if errInit := newDevice.InitiatorInit(); errInit != nil {
		newDevice.Close()
		return fmt.Errorf("failed to initialize device %s: %w", devicePathToConnect, errInit)
	}

	dm.mu.Lock()
	dm.device = newDevice
	dm.hasDevice = true
	dm.devicePath = devicePathToConnect
	dm.mu.Unlock()

	log.Printf("Successfully connected to device: %s", newDevice.String())
	return nil
}

// Reconnect attempts to reconnect to the device with exponential backoff.
func (dm *DeviceManager) Reconnect(stopChan <-chan struct{}) error {
	return dm.reconnectDevice(false, stopChan)
}

// ForceReconnect attempts to force reconnect with device reset wait time.
func (dm *DeviceManager) ForceReconnect(stopChan <-chan struct{}) error {
	return dm.reconnectDevice(true, stopChan)
}

// reconnectDevice attempts to reconnect to the NFC device with configurable retry logic.
func (dm *DeviceManager) reconnectDevice(forceMode bool, stopChan <-chan struct{}) error {
	logPrefix := "Reconnect"
	maxAttempts := MaxReconnectTries
	if forceMode {
		logPrefix = "Force reconnect"
		maxAttempts = 3
	}

	log.Printf("%s: Attempting to reconnect device (path hint: %s)...", logPrefix, dm.devicePath)

	// Close existing device
	dm.mu.Lock()
	if dm.hasDevice && dm.device != nil {
		log.Printf("%s: Closing existing device connection.", logPrefix)
		dm.device.Close()
		dm.device = nil
		dm.hasDevice = false
	}
	dm.mu.Unlock()

	// For force mode, wait for device reset
	if forceMode {
		log.Println("Waiting for device to reset after close...")
		select {
		case <-time.After(DeviceResetWaitTime):
		case <-stopChan:
			log.Printf("%s: Stop signal received during wait, aborting.", logPrefix)
			return fmt.Errorf("reconnection aborted by stop signal")
		}
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		connectErr := dm.TryConnect()
		if connectErr == nil {
			log.Printf("%s: Attempt %d successful.", logPrefix, attempt)
			return nil
		}

		lastErr = connectErr
		log.Printf("%s: Attempt %d failed: %v", logPrefix, attempt, connectErr)

		// Calculate backoff delay
		var backoffDelay time.Duration
		if forceMode {
			backoffDelay = time.Second * time.Duration(attempt)
		} else {
			backoffDelay = ReconnectDelay * time.Duration(attempt)
		}

		select {
		case <-stopChan:
			log.Printf("%s: Stop signal received, aborting reconnection.", logPrefix)
			return fmt.Errorf("reconnection aborted by stop signal")
		case <-time.After(backoffDelay):
		}
	}

	errMsg := fmt.Sprintf("%s failed after %d attempts: %v", logPrefix, maxAttempts, lastErr)
	log.Println(errMsg)
	return fmt.Errorf(errMsg)
}

// Close closes the current device connection.
func (dm *DeviceManager) Close() {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if dm.hasDevice && dm.device != nil {
		log.Println("Closing device in DeviceManager.")
		if err := dm.device.Close(); err != nil {
			log.Printf("Error closing device: %v", err)
		}
		dm.device = nil
		dm.hasDevice = false
	}
}

// HandleError processes device errors and determines the appropriate recovery action.
// Returns the retry count and whether a cooldown was initiated.
func (dm *DeviceManager) HandleError(err error, retryCount int, stopChan <-chan struct{}) (newRetryCount int, needsCooldown bool) {
	log.Printf("Device error: %v", err)
	originalErrorString := err.Error()

	// Handle IO/Config errors
	if IsIOError(err) || IsDeviceConfigError(err) {
		log.Printf("Device error detected (IO/Config): %v. Closing device.", err)
		dm.mu.Lock()
		if dm.hasDevice && dm.device != nil {
			dm.device.Close()
		}
		dm.device = nil
		dm.hasDevice = false
		dm.mu.Unlock()

		// Check for ACR122-specific errors that need cooldown
		if strings.Contains(originalErrorString, "Operation not permitted") ||
			strings.Contains(originalErrorString, "broken pipe") ||
			strings.Contains(originalErrorString, "RDR_to_PC_DataBlock") {
			dm.mu.Lock()
			if !dm.inCooldown {
				dm.inCooldown = true
				log.Printf("ACR122-like error. Entering cooldown for %v", DeviceErrorCooldownPeriod)
				dm.cooldownTimer.Reset(DeviceErrorCooldownPeriod)
			}
			dm.mu.Unlock()
			return retryCount, true
		}

		log.Println("Attempting force reconnect after IO/Config error...")
		time.Sleep(PostErrorPauseTime)
		if errReconnect := dm.ForceReconnect(stopChan); errReconnect != nil {
			log.Printf("Force reconnection failed after IO/Config error: %v", errReconnect)
		}
		return retryCount, false
	}

	// Handle Timeout/Closed errors with retry logic
	if IsTimeoutError(err) || IsDeviceClosedError(err) {
		log.Printf("Device error (Timeout/Closed): %v", err)
		delay := time.Duration(math.Pow(2, float64(retryCount))) * BaseDelay
		if retryCount < MaxRetries {
			retryCount++
			log.Printf("Retrying connection (attempt %d/%d) in %v...", retryCount, MaxRetries, delay)
			select {
			case <-time.After(delay):
			case <-stopChan:
				return retryCount, false
			}
			if errReconnect := dm.Reconnect(stopChan); errReconnect != nil {
				log.Printf("Device reconnection failed: %v", errReconnect)
			} else {
				log.Println("Reconnected successfully.")
				retryCount = 0
			}
		} else {
			log.Printf("Max retries reached for Timeout/Closed error: %v. Closing device.", err)
			dm.mu.Lock()
			if dm.hasDevice && dm.device != nil {
				dm.device.Close()
			}
			dm.device = nil
			dm.hasDevice = false
			if !dm.inCooldown {
				dm.inCooldown = true
				dm.cooldownTimer.Reset(MaxRetriesCooldownPeriod)
				log.Println("Entering long cooldown after max retries for Timeout/Closed error.")
			}
			dm.mu.Unlock()
			return retryCount, true
		}
		return retryCount, false
	}

	// Unhandled error - caller should handle
	return retryCount, false
}

// EndCooldown ends the current cooldown period and attempts to reconnect.
func (dm *DeviceManager) EndCooldown(stopChan <-chan struct{}) {
	log.Println("Device cooldown period ended.")
	dm.mu.Lock()
	dm.inCooldown = false
	dm.mu.Unlock()
	if err := dm.ForceReconnect(stopChan); err != nil {
		log.Printf("Reconnection after cooldown failed: %v.", err)
	}
}

// CooldownChannel returns the cooldown timer channel for select statements.
func (dm *DeviceManager) CooldownChannel() <-chan time.Time {
	return dm.cooldownTimer.C
}

// ResetRetryCount resets the retry counter (call after successful operation).
func (dm *DeviceManager) ResetRetryCount(retryCount *int) {
	*retryCount = 0
}
