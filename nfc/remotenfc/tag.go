package remotenfc

import (
	"fmt"
	"sync"
	"time"

	"github.com/nedpals/davi-nfc-agent/nfc"
)

// Tag wraps mobile app NFC data in the nfc.Tag interface.
type Tag struct {
	uid          string
	tagType      string
	technology   string
	ndefData     []byte           // Encoded NDEF message
	ndefMsg      *nfc.NDEFMessage // Parsed NDEF message
	rawData      []byte           // Raw tag data from mobile app
	scannedAt    time.Time
	sourceDevice string // Device ID that scanned this tag
	mu           sync.RWMutex
}

// UID returns the tag's unique identifier.
func (t *Tag) UID() string {
	return t.uid
}

// Type returns the tag type as a string.
func (t *Tag) Type() string {
	return t.tagType
}

// NumericType returns a numeric representation of the tag type.
// For smartphone tags, we return 0 as they don't have freefare numeric types.
func (t *Tag) NumericType() int {
	return 0
}

// Capabilities returns the capabilities of this smartphone tag.
// Smartphone tags are read-only as writes must go through the WebSocket protocol.
func (t *Tag) Capabilities() nfc.TagCapabilities {
	return nfc.TagCapabilities{
		CanRead:       true,
		CanWrite:      false, // Writes require WebSocket protocol
		CanTransceive: false,
		CanLock:       false,
		TagFamily:     t.tagType,
		Technology:    t.technology,
		SupportsNDEF:  t.ndefMsg != nil || t.ndefData != nil,
	}
}

// ReadData returns the tag data (NDEF or raw).
func (t *Tag) ReadData() ([]byte, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.ndefData != nil {
		return t.ndefData, nil
	}

	return t.rawData, nil
}

// WriteData is not supported for smartphone tags.
// Write requests must go through server -> device WebSocket.
func (t *Tag) WriteData(data []byte) error {
	return nfc.NewNotSupportedError("WriteData")
}

// Transceive is not supported for smartphone tags.
func (t *Tag) Transceive(data []byte) ([]byte, error) {
	return nil, nfc.NewNotSupportedError("Transceive")
}

// Connect is a no-op for smartphone tags (already "connected" via WebSocket).
func (t *Tag) Connect() error {
	return nil
}

// Disconnect is a no-op for smartphone tags.
func (t *Tag) Disconnect() error {
	return nil
}

// IsWritable returns false for smartphone tags as they don't support direct writes.
func (t *Tag) IsWritable() (bool, error) {
	return false, nil
}

// CanMakeReadOnly returns false for smartphone tags.
func (t *Tag) CanMakeReadOnly() (bool, error) {
	return false, nil
}

// MakeReadOnly is not supported for smartphone tags.
func (t *Tag) MakeReadOnly() error {
	return nfc.NewNotSupportedError("MakeReadOnly")
}

// GetNDEFMessage returns the parsed NDEF message if available.
func (t *Tag) GetNDEFMessage() (*nfc.NDEFMessage, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.ndefMsg != nil {
		return t.ndefMsg, nil
	}

	return nil, fmt.Errorf("no NDEF message available")
}

// ScannedAt returns the timestamp when this tag was scanned.
func (t *Tag) ScannedAt() time.Time {
	return t.scannedAt
}

// SourceDevice returns the device ID that scanned this tag.
func (t *Tag) SourceDevice() string {
	return t.sourceDevice
}
