package server

import (
	"sync"
	"time"

	"github.com/nedpals/davi-nfc-agent/nfc"
)

// HTTPInputTag implements nfc.Tag for HTTP-injected tag data.
// This is read-only; write operations are not supported.
type HTTPInputTag struct {
	uid        string
	tagType    string
	technology string
	ndefData   []byte           // Encoded NDEF message
	ndefMsg    *nfc.NDEFMessage // Parsed NDEF message
	scannedAt  time.Time
	source     string // Source identifier (e.g., "http-api", "manual-tool")
	mu         sync.RWMutex
}

// UID returns the tag's unique identifier.
func (t *HTTPInputTag) UID() string {
	return t.uid
}

// Type returns the tag type as a string.
func (t *HTTPInputTag) Type() string {
	return t.tagType
}

// NumericType returns a numeric representation of the tag type.
// For HTTP input tags, we return 0 as they don't have freefare numeric types.
func (t *HTTPInputTag) NumericType() int {
	return 0
}

// Capabilities returns the capabilities of this HTTP input tag.
// HTTP input tags are read-only.
func (t *HTTPInputTag) Capabilities() nfc.TagCapabilities {
	return nfc.TagCapabilities{
		CanRead:       true,
		CanWrite:      false,
		CanTransceive: false,
		CanLock:       false,
		TagFamily:     t.tagType,
		Technology:    t.technology,
		SupportsNDEF:  t.ndefMsg != nil || t.ndefData != nil,
	}
}

// ReadData returns the NDEF data if available.
func (t *HTTPInputTag) ReadData() ([]byte, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.ndefData != nil {
		return t.ndefData, nil
	}
	return nil, nil
}

// WriteData is not supported for HTTP input tags.
func (t *HTTPInputTag) WriteData(data []byte) error {
	return nfc.NewNotSupportedError("WriteData")
}

// Transceive is not supported for HTTP input tags.
func (t *HTTPInputTag) Transceive(data []byte) ([]byte, error) {
	return nil, nfc.NewNotSupportedError("Transceive")
}

// Connect is a no-op for HTTP input tags.
func (t *HTTPInputTag) Connect() error {
	return nil
}

// Disconnect is a no-op for HTTP input tags.
func (t *HTTPInputTag) Disconnect() error {
	return nil
}

// IsWritable returns false for HTTP input tags.
func (t *HTTPInputTag) IsWritable() (bool, error) {
	return false, nil
}

// CanMakeReadOnly returns false for HTTP input tags.
func (t *HTTPInputTag) CanMakeReadOnly() (bool, error) {
	return false, nil
}

// MakeReadOnly is not supported for HTTP input tags.
func (t *HTTPInputTag) MakeReadOnly() error {
	return nfc.NewNotSupportedError("MakeReadOnly")
}

// GetNDEFMessage returns the parsed NDEF message if available.
func (t *HTTPInputTag) GetNDEFMessage() *nfc.NDEFMessage {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.ndefMsg
}

// ScannedAt returns the timestamp when this tag was scanned.
func (t *HTTPInputTag) ScannedAt() time.Time {
	return t.scannedAt
}

// Source returns the source identifier for this tag.
func (t *HTTPInputTag) Source() string {
	return t.source
}
