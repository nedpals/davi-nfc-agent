package nfc

import "github.com/clausecker/freefare"

// TagIdentifier provides basic tag identification.
// All tags implement this interface.
type TagIdentifier interface {
	// UID returns the unique identifier of the tag.
	UID() string
	// Type returns a human-readable string describing the tag type.
	Type() string
	// NumericType returns a numeric type identifier (implementation-specific).
	NumericType() int
}

// TagConnection manages the connection lifecycle to a tag.
// Tags that require explicit connection management implement this interface.
type TagConnection interface {
	// Connect establishes a connection to the tag.
	Connect() error
	// Disconnect closes the connection to the tag.
	Disconnect() error
}

// TagReader provides read capability for NDEF data.
// Tags that support reading implement this interface.
type TagReader interface {
	// ReadData reads NDEF data from the tag.
	ReadData() ([]byte, error)
}

// TagWriter provides write capability for NDEF data.
// Tags that support writing implement this interface.
// Use GetTagCapabilities(tag).CanWrite to check if a tag supports this.
type TagWriter interface {
	// WriteData writes NDEF data to the tag.
	WriteData(data []byte) error
}

// TagTransceiver provides raw data exchange with the tag.
// Only some tag types (e.g., Type 4) support this.
// Use GetTagCapabilities(tag).CanTransceive to check if a tag supports this.
type TagTransceiver interface {
	// Transceive sends raw data to the tag and returns the response.
	Transceive(data []byte) ([]byte, error)
}

// TagLocker provides read-only locking capability.
// Tags that can be made permanently read-only implement this interface.
// Use GetTagCapabilities(tag).CanLock to check if a tag supports this.
type TagLocker interface {
	// IsWritable checks if the tag can be written to.
	IsWritable() (bool, error)
	// CanMakeReadOnly checks if the tag supports being made read-only.
	CanMakeReadOnly() (bool, error)
	// MakeReadOnly permanently locks the tag to prevent further writes.
	MakeReadOnly() error
}

// Tag represents an NFC tag at the hardware protocol level.
//
// Tag provides a unified interface for reading and writing NDEF data
// regardless of the underlying tag technology (MIFARE Classic, ISO14443-4, etc.).
//
// Not all tags support all operations. Use GetTagCapabilities(tag) to check
// what operations a specific tag supports before calling methods that may
// return "not supported" errors.
//
// For most use cases, prefer using Card which provides a higher-level,
// io.Reader/Writer compatible API.
//
// Example:
//
//	tags, _ := manager.GetTags(device)
//	for _, tag := range tags {
//	    caps := nfc.GetTagCapabilities(tag)
//	    if caps.CanRead {
//	        data, _ := tag.ReadData()
//	    }
//	}
type Tag interface {
	TagIdentifier
	TagConnection
	TagReader
	TagWriter
	TagTransceiver
	TagLocker
}

// FreefareTagProvider provides access to the underlying freefare.Tag object.
// This is used for advanced operations that require direct access to the
// freefare library.
type FreefareTagProvider interface {
	GetFreefareTag() freefare.Tag
}

// TagWriteOptions defines options for tag write operations.
type TagWriteOptions struct {
	// ForceInitialize forces reinitialization of the tag even if it contains existing data.
	// WARNING: This will erase all existing data on the tag.
	// Only use this if you explicitly want to wipe and reinitialize the tag.
	ForceInitialize bool
}

// AdvancedWriter is an optional interface that tags can implement to support
// write operations with options. If a tag implements this interface, the reader
// will use WriteDataWithOptions instead of WriteData when options are provided.
type AdvancedWriter interface {
	WriteDataWithOptions(data []byte, opts TagWriteOptions) error
}
