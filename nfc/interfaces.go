package nfc

import (
	"github.com/clausecker/freefare"
)

// DeviceInterface defines the operations needed from an NFC device.
type DeviceInterface interface {
	Close() error
	InitiatorInit() error
	String() string
	Connection() string
	// Add methods for polling for targets, transceiving data, etc.
	// For example:
	// SelectPassiveTarget(target nfc.Target) (nfc.Device, error)
	// Transceive(txData []byte) ([]byte, error)
}

// TagInterface defines the common methods for different tag types.
type TagInterface interface {
	UID() string
	Type() string                // Returns a string representation of the tag type, e.g., "MIFARE Classic 1K"
	NumericType() int            // Returns an integer representation of the tag type, suitable for comparison with freefare constants
	ReadData() ([]byte, error)   // Reads high-level data (e.g., NDEF message)
	WriteData(data []byte) error // Writes high-level data (e.g., NDEF message)

	// Changed keyType to int to resolve potential type recognition issues
	Read(sector, block uint8, key []byte, keyType int) ([]byte, error)
	Write(sector, block uint8, data []byte, key []byte, keyType int) error
	// Add other common methods here
}

// FreefareTagProvider is an interface for tags that can provide a raw freefare.Tag object.
type FreefareTagProvider interface {
	TagInterface // Embeds the common TagInterface (which now includes NumericType)
	GetFreefareTag() freefare.Tag
}

// ManagerInterface abstracts NFC device listing, opening, and tag retrieval.
type ManagerInterface interface {
	OpenDevice(deviceStr string) (DeviceInterface, error)
	ListDevices() ([]string, error)
	// GetTags should ideally return []TagInterface for broader compatibility.
	// For now, it returns the more specific FreefareTagProvider for MIFARE Classic.
	// This can be evolved to detect tag types and return appropriate wrappers.
	GetTags(dev DeviceInterface) ([]TagInterface, error)
}
