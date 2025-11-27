package nfc

import (
	"errors"
	"strings"
	"time"
)

// NFCData represents the data read from an NFC tag including any potential errors.
type NFCData struct {
	Card *Card // The detected card, nil if no card is present
	Err  error // Error that occurred during detection/reading
}

// DeviceStatus represents the status of the NFC device.
// This type might be used by the main application to display status.
type DeviceStatus struct {
	Connected   bool
	Message     string
	CardPresent bool
}

// Constants for NFC operations
const (
	MaxRetries          = 5
	BaseDelay           = 500 * time.Millisecond
	MaxReconnectTries   = 10
	ReconnectDelay      = time.Second * 2
	DeviceCheckInterval = time.Second * 2 // Interval to check for new devices
	DeviceEnumRetries   = 3               // Number of retries for device enumeration
)

// TagType represents the type of NFC tag as a string.
type TagType string

// Constants for common tag types
const (
	TagTypeMifareClassic TagType = "MIFARE_Classic"
	TagTypeType4         TagType = "Type4"
	TagTypeUnknown       TagType = "Unknown"
)

// Sentinel errors for device operations
var (
	// ErrTimeout indicates a timeout occurred during device communication
	ErrTimeout = errors.New("device operation timed out")

	// ErrDeviceClosed indicates the device connection was closed
	ErrDeviceClosed = errors.New("device closed")

	// ErrIO indicates an input/output error with the device
	ErrIO = errors.New("device I/O error")

	// ErrDeviceConfig indicates a device configuration error
	ErrDeviceConfig = errors.New("device configuration error")

	// ErrCooldownRequired indicates the device needs a cooldown period
	ErrCooldownRequired = errors.New("device cooldown required")

	// ErrACR122Specific indicates an ACR122-specific error requiring cooldown
	ErrACR122Specific = errors.New("ACR122 device error")
)

// Error checking helper functions
// These functions check for both typed errors and legacy string-based errors
// for backward compatibility during the transition period.

func IsTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	// Check for typed error first
	if errors.Is(err, ErrTimeout) {
		return true
	}
	// Fallback to string matching for legacy errors
	errStr := err.Error()
	return strings.Contains(errStr, "Operation timed out") ||
		strings.Contains(errStr, "operation timed out") ||
		strings.Contains(errStr, "Unable to write to USB") ||
		strings.Contains(errStr, "timeout")
}

func IsDeviceClosedError(err error) bool {
	if err == nil {
		return false
	}
	// Check for typed error first
	if errors.Is(err, ErrDeviceClosed) {
		return true
	}
	// Fallback to string matching for legacy errors
	return strings.Contains(err.Error(), "device closed")
}

func IsIOError(err error) bool {
	if err == nil {
		return false
	}
	// Check for typed error first
	if errors.Is(err, ErrIO) {
		return true
	}
	// Fallback to string matching for legacy errors
	errStr := err.Error()
	return strings.Contains(errStr, "input / output error") ||
		strings.Contains(errStr, "Input/output error") ||
		strings.Contains(errStr, "i/o error") ||
		strings.Contains(errStr, "broken pipe") ||
		strings.Contains(errStr, "Operation not permitted")
}

func IsDeviceConfigError(err error) bool {
	if err == nil {
		return false
	}
	// Check for typed error first
	if errors.Is(err, ErrDeviceConfig) {
		return true
	}
	// Fallback to string matching for legacy errors
	errStr := err.Error()
	return strings.Contains(errStr, "device not configured") ||
		strings.Contains(errStr, "Device not configured") ||
		strings.Contains(errStr, "Unable to write to USB") ||
		strings.Contains(errStr, "RDR_to_PC_DataBlock")
}

func IsACR122Error(err error) bool {
	if err == nil {
		return false
	}
	// Check for typed error first
	if errors.Is(err, ErrACR122Specific) {
		return true
	}
	// Fallback to string matching for ACR122-specific patterns
	errStr := err.Error()
	return strings.Contains(errStr, "Operation not permitted") ||
		strings.Contains(errStr, "broken pipe") ||
		strings.Contains(errStr, "RDR_to_PC_DataBlock")
}
