package nfc

import (
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

// Error checking helper functions

func IsTimeoutError(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "Operation timed out") ||
		strings.Contains(err.Error(), "operation timed out") ||
		strings.Contains(err.Error(), "Unable to write to USB")) || strings.Contains(err.Error(), "timeout")
}

func IsDeviceClosedError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "device closed")
}

func IsIOError(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "input / output error") ||
		strings.Contains(err.Error(), "Input/output error") ||
		strings.Contains(err.Error(), "i/o error") ||
		strings.Contains(err.Error(), "broken pipe") ||
		strings.Contains(err.Error(), "Operation not permitted"))
}

func IsDeviceConfigError(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "device not configured") ||
		strings.Contains(err.Error(), "Device not configured") ||
		strings.Contains(err.Error(), "Unable to write to USB") ||
		strings.Contains(err.Error(), "RDR_to_PC_DataBlock"))
}
