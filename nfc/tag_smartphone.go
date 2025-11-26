package nfc

import (
	"fmt"
	"sync"
	"time"
)

// SmartphoneTag wraps mobile app NFC data in our Tag interface.
type SmartphoneTag struct {
	uid          string
	tagType      string
	technology   string
	ndefData     []byte       // Encoded NDEF message
	ndefMsg      *NDEFMessage // Parsed NDEF message
	rawData      []byte       // Raw tag data from mobile app
	scannedAt    time.Time
	sourceDevice string // Device ID that scanned this tag
	mu           sync.RWMutex
}

// UID returns the tag's unique identifier.
func (st *SmartphoneTag) UID() string {
	return st.uid
}

// Type returns the tag type as a string.
func (st *SmartphoneTag) Type() string {
	return st.tagType
}

// NumericType returns a numeric representation of the tag type.
// For smartphone tags, we return 0 as they don't have freefare numeric types.
func (st *SmartphoneTag) NumericType() int {
	return 0
}

// ReadData returns the tag data (NDEF or raw).
func (st *SmartphoneTag) ReadData() ([]byte, error) {
	st.mu.RLock()
	defer st.mu.RUnlock()

	if st.ndefData != nil {
		return st.ndefData, nil
	}

	return st.rawData, nil
}

// WriteData is not supported for smartphone tags.
// Write requests must go through server -> device WebSocket.
func (st *SmartphoneTag) WriteData(data []byte) error {
	return fmt.Errorf("write operations not supported on smartphone tags directly")
}

// Transceive is not supported for smartphone tags.
func (st *SmartphoneTag) Transceive(data []byte) ([]byte, error) {
	return nil, fmt.Errorf("transceive not supported on smartphone tags")
}

// Connect is a no-op for smartphone tags (already "connected" via WebSocket).
func (st *SmartphoneTag) Connect() error {
	return nil
}

// Disconnect is a no-op for smartphone tags.
func (st *SmartphoneTag) Disconnect() error {
	return nil
}

// IsWritable returns false for smartphone tags as they don't support direct writes.
func (st *SmartphoneTag) IsWritable() (bool, error) {
	return false, nil
}

// CanMakeReadOnly returns false for smartphone tags.
func (st *SmartphoneTag) CanMakeReadOnly() (bool, error) {
	return false, nil
}

// MakeReadOnly is not supported for smartphone tags.
func (st *SmartphoneTag) MakeReadOnly() error {
	return fmt.Errorf("make read-only not supported on smartphone tags")
}

// GetNDEFMessage returns the parsed NDEF message if available.
func (st *SmartphoneTag) GetNDEFMessage() (*NDEFMessage, error) {
	st.mu.RLock()
	defer st.mu.RUnlock()

	if st.ndefMsg != nil {
		return st.ndefMsg, nil
	}

	return nil, fmt.Errorf("no NDEF message available")
}

// ScannedAt returns the timestamp when this tag was scanned.
func (st *SmartphoneTag) ScannedAt() time.Time {
	return st.scannedAt
}

// SourceDevice returns the device ID that scanned this tag.
func (st *SmartphoneTag) SourceDevice() string {
	return st.sourceDevice
}
