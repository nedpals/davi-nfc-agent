package nfc

import "strings"

// TagCapabilities describes what operations a tag supports.
type TagCapabilities struct {
	// Core capabilities
	CanRead       bool `json:"canRead"`
	CanWrite      bool `json:"canWrite"`
	CanTransceive bool `json:"canTransceive"`

	// Locking capabilities
	CanLock    bool `json:"canLock"`
	IsReadOnly bool `json:"isReadOnly,omitempty"`

	// Memory info
	MemorySize  int `json:"memorySize,omitempty"`  // Total memory in bytes
	MaxNDEFSize int `json:"maxNdefSize,omitempty"` // Max NDEF message size

	// Technology info
	Technology string `json:"technology,omitempty"` // "ISO14443A", "ISO14443B", etc.
	TagFamily  string `json:"tagFamily,omitempty"`  // "MIFARE Classic", "DESFire", "NTAG", etc.

	// Optional features
	SupportsNDEF           bool `json:"supportsNdef"`
	SupportsCrypto         bool `json:"supportsCrypto,omitempty"`
	SupportsAuthentication bool `json:"supportsAuthentication,omitempty"`
}

// DeviceCapabilities describes what operations a device supports.
type DeviceCapabilities struct {
	// Communication capabilities
	CanTransceive bool `json:"canTransceive"`
	CanPoll       bool `json:"canPoll"`

	// Supported tag types
	SupportedTagTypes []string `json:"supportedTagTypes,omitempty"`

	// Hardware info
	DeviceType  string `json:"deviceType"`            // "libnfc", "smartphone", etc.
	MaxBaudRate int    `json:"maxBaudRate,omitempty"` // Max baud rate in bps

	// Event capabilities
	SupportsEvents bool `json:"supportsEvents"` // Tag arrival/removal events
}

// TagCapabilityProvider is an optional interface for tags to report their capabilities.
type TagCapabilityProvider interface {
	Capabilities() TagCapabilities
}

// DeviceCapabilityProvider is an optional interface for devices to report capabilities.
type DeviceCapabilityProvider interface {
	Capabilities() DeviceCapabilities
}

// GetTagCapabilities returns capabilities for any Tag.
// If the tag implements TagCapabilityProvider, it uses that.
// Otherwise, it infers capabilities from the tag type string.
func GetTagCapabilities(tag Tag) TagCapabilities {
	if provider, ok := tag.(TagCapabilityProvider); ok {
		return provider.Capabilities()
	}
	return InferTagCapabilities(tag.Type())
}

// GetDeviceCapabilities returns capabilities for any Device.
// If the device implements DeviceCapabilityProvider, it uses that.
// Otherwise, returns conservative defaults.
func GetDeviceCapabilities(device Device) DeviceCapabilities {
	if provider, ok := device.(DeviceCapabilityProvider); ok {
		return provider.Capabilities()
	}
	// Conservative defaults for unknown devices
	return DeviceCapabilities{
		CanTransceive: true,
		CanPoll:       true,
		DeviceType:    "unknown",
	}
}

// InferTagCapabilities infers capabilities from a tag type string.
// This is used as a fallback when the tag doesn't implement TagCapabilityProvider.
func InferTagCapabilities(tagType string) TagCapabilities {
	caps := TagCapabilities{
		CanRead:      true, // All tags can read
		SupportsNDEF: true, // Assume NDEF support
	}

	tagTypeLower := strings.ToLower(tagType)

	switch {
	case strings.Contains(tagTypeLower, "mifare classic") || strings.Contains(tagTypeLower, "classic"):
		caps.CanWrite = true
		caps.CanTransceive = false
		caps.CanLock = true
		caps.TagFamily = "MIFARE Classic"
		caps.Technology = "ISO14443A"
		caps.SupportsCrypto = true
		caps.SupportsAuthentication = true
		if strings.Contains(tagTypeLower, "1k") {
			caps.MemorySize = 1024
			caps.MaxNDEFSize = 716 // Approximate usable NDEF space
		} else if strings.Contains(tagTypeLower, "4k") {
			caps.MemorySize = 4096
			caps.MaxNDEFSize = 3356
		}

	case strings.Contains(tagTypeLower, "desfire"):
		caps.CanWrite = true
		caps.CanTransceive = false
		caps.CanLock = false // Not implemented
		caps.TagFamily = "DESFire"
		caps.Technology = "ISO14443A"
		caps.SupportsCrypto = true
		caps.SupportsAuthentication = true
		caps.MemorySize = 8192 // Varies by model

	case strings.Contains(tagTypeLower, "ultralight"):
		caps.CanWrite = true
		caps.CanTransceive = false
		caps.CanLock = true
		caps.TagFamily = "MIFARE Ultralight"
		caps.Technology = "ISO14443A"
		if strings.Contains(tagTypeLower, "c") {
			caps.MemorySize = 192
			caps.MaxNDEFSize = 137
			caps.SupportsCrypto = true
		} else {
			caps.MemorySize = 64
			caps.MaxNDEFSize = 46
		}

	case strings.Contains(tagTypeLower, "ntag2") || tagTypeLower == "ntag" || strings.HasPrefix(tagTypeLower, "ntag "):
		caps.CanWrite = true
		caps.CanTransceive = false
		caps.CanLock = true
		caps.TagFamily = "NTAG"
		caps.Technology = "ISO14443A"
		if strings.Contains(tagTypeLower, "213") {
			caps.MemorySize = 180
			caps.MaxNDEFSize = 144
		} else if strings.Contains(tagTypeLower, "215") {
			caps.MemorySize = 540
			caps.MaxNDEFSize = 504
		} else if strings.Contains(tagTypeLower, "216") {
			caps.MemorySize = 924
			caps.MaxNDEFSize = 888
		}

	case strings.Contains(tagTypeLower, "type4") || strings.Contains(tagTypeLower, "iso14443"):
		caps.CanWrite = true
		caps.CanTransceive = true
		caps.CanLock = true
		caps.TagFamily = "Type 4"
		caps.Technology = "ISO14443A"

	default:
		// Conservative defaults for unknown types
		caps.CanWrite = false
		caps.CanTransceive = false
		caps.CanLock = false
		caps.TagFamily = "Unknown"
	}

	return caps
}

// CanTagRead checks if a tag supports read operations.
func CanTagRead(tag Tag) bool {
	return GetTagCapabilities(tag).CanRead
}

// CanTagWrite checks if a tag supports write operations.
func CanTagWrite(tag Tag) bool {
	return GetTagCapabilities(tag).CanWrite
}

// CanTagTransceive checks if a tag supports raw transceive operations.
func CanTagTransceive(tag Tag) bool {
	return GetTagCapabilities(tag).CanTransceive
}

// CanTagLock checks if a tag can be made read-only.
func CanTagLock(tag Tag) bool {
	return GetTagCapabilities(tag).CanLock
}
