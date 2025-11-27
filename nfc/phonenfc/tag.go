package phonenfc

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
	return fmt.Errorf("write operations not supported on smartphone tags directly")
}

// Transceive is not supported for smartphone tags.
func (t *Tag) Transceive(data []byte) ([]byte, error) {
	return nil, fmt.Errorf("transceive not supported on smartphone tags")
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
	return fmt.Errorf("make read-only not supported on smartphone tags")
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
