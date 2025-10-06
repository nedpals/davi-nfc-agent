package nfc

import (
	"fmt"
	"sync"

	"github.com/clausecker/freefare"
)

// MockTag is a test implementation of Tag that simulates NFC tag behavior.
//
// MockTag allows testing tag operations without physical tags by providing
// configurable mock responses for read/write operations.
//
// Example:
//
//	tag := &MockTag{
//	    TagUID: "04A1B2C3",
//	    TagType: "MIFARE Classic 1K",
//	    Data: []byte{0x00, 0x01, 0x02},
//	}
//	data, _ := tag.ReadData()
type MockTag struct {
	// TagUID is the UID returned by UID()
	TagUID string

	// TagType is the type string returned by Type()
	TagType string

	// TagNumericType is the numeric type returned by NumericType()
	TagNumericType int

	// Data is the data returned by ReadData()
	Data []byte

	// ReadDataError, if set, will be returned by ReadData()
	ReadDataError error

	// WriteDataError, if set, will be returned by WriteData()
	WriteDataError error

	// TransceiveFunc allows custom transceive behavior
	// If nil, returns TransceiveResponse or TransceiveError
	TransceiveFunc func([]byte) ([]byte, error)

	// TransceiveResponse is the default response for Transceive calls
	TransceiveResponse []byte

	// TransceiveError, if set, will be returned by Transceive()
	TransceiveError error

	// ConnectError, if set, will be returned by Connect()
	ConnectError error

	// DisconnectError, if set, will be returned by Disconnect()
	DisconnectError error

	// IsConnected tracks whether the tag is currently connected
	IsConnected bool

	// IsReadOnly tracks whether the tag is in read-only mode
	IsReadOnly bool

	// IsWritableFunc allows custom IsWritable behavior
	// If nil, returns !IsReadOnly and IsWritableError
	IsWritableFunc func() (bool, error)

	// IsWritableError, if set, will be returned by IsWritable()
	IsWritableError error

	// MakeReadOnlyFunc allows custom MakeReadOnly behavior
	// If nil, sets IsReadOnly to true or returns MakeReadOnlyError
	MakeReadOnlyFunc func() error

	// MakeReadOnlyError, if set, will be returned by MakeReadOnly()
	MakeReadOnlyError error

	// CanMakeReadOnlyFunc allows custom CanMakeReadOnly behavior
	// If nil, returns !IsReadOnly and CanMakeReadOnlyError
	CanMakeReadOnlyFunc func() (bool, error)

	// CanMakeReadOnlyError, if set, will be returned by CanMakeReadOnly()
	CanMakeReadOnlyError error

	// CallLog tracks all method calls for verification in tests
	CallLog []string

	mu sync.Mutex
}

// NewMockTag creates a new MockTag with default values.
func NewMockTag(uid string) *MockTag {
	return &MockTag{
		TagUID:         uid,
		TagType:        "Mock Tag",
		TagNumericType: 0,
		Data:           []byte{},
		IsConnected:    false,
		IsReadOnly:     false,
		CallLog:        make([]string, 0),
	}
}

// UID returns the tag's UID.
func (m *MockTag) UID() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.CallLog = append(m.CallLog, "UID")
	return m.TagUID
}

// Type returns the tag's type string.
func (m *MockTag) Type() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.CallLog = append(m.CallLog, "Type")
	return m.TagType
}

// NumericType returns the tag's numeric type.
func (m *MockTag) NumericType() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.CallLog = append(m.CallLog, "NumericType")
	return m.TagNumericType
}

// ReadData simulates reading data from the tag.
func (m *MockTag) ReadData() ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.CallLog = append(m.CallLog, "ReadData")

	if !m.IsConnected {
		return nil, fmt.Errorf("tag not connected")
	}

	if m.ReadDataError != nil {
		return nil, m.ReadDataError
	}

	// Return a copy to prevent external modification
	dataCopy := make([]byte, len(m.Data))
	copy(dataCopy, m.Data)
	return dataCopy, nil
}

// WriteData simulates writing data to the tag.
func (m *MockTag) WriteData(data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.CallLog = append(m.CallLog, fmt.Sprintf("WriteData(%d bytes)", len(data)))

	if !m.IsConnected {
		return fmt.Errorf("tag not connected")
	}

	if m.IsReadOnly {
		return fmt.Errorf("tag is read-only")
	}

	if m.WriteDataError != nil {
		return m.WriteDataError
	}

	// Store a copy of the data
	m.Data = make([]byte, len(data))
	copy(m.Data, data)
	return nil
}

// Transceive simulates data exchange with the tag.
func (m *MockTag) Transceive(data []byte) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.CallLog = append(m.CallLog, fmt.Sprintf("Transceive(%d bytes)", len(data)))

	if !m.IsConnected {
		return nil, fmt.Errorf("tag not connected")
	}

	if m.TransceiveFunc != nil {
		return m.TransceiveFunc(data)
	}

	if m.TransceiveError != nil {
		return nil, m.TransceiveError
	}

	return m.TransceiveResponse, nil
}

// Connect simulates connecting to the tag.
func (m *MockTag) Connect() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.CallLog = append(m.CallLog, "Connect")

	if m.IsConnected {
		return fmt.Errorf("tag already connected")
	}

	if m.ConnectError != nil {
		return m.ConnectError
	}

	m.IsConnected = true
	return nil
}

// Disconnect simulates disconnecting from the tag.
func (m *MockTag) Disconnect() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.CallLog = append(m.CallLog, "Disconnect")

	if !m.IsConnected {
		return fmt.Errorf("tag not connected")
	}

	if m.DisconnectError != nil {
		return m.DisconnectError
	}

	m.IsConnected = false
	return nil
}

// GetCallLog returns a copy of the call log for verification.
func (m *MockTag) GetCallLog() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	logCopy := make([]string, len(m.CallLog))
	copy(logCopy, m.CallLog)
	return logCopy
}

// ClearCallLog clears the call log.
func (m *MockTag) ClearCallLog() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.CallLog = make([]string, 0)
}

// IsWritable simulates checking if the tag is writable.
func (m *MockTag) IsWritable() (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.CallLog = append(m.CallLog, "IsWritable")

	if !m.IsConnected {
		return false, fmt.Errorf("tag not connected")
	}

	if m.IsWritableFunc != nil {
		return m.IsWritableFunc()
	}

	if m.IsWritableError != nil {
		return false, m.IsWritableError
	}

	return !m.IsReadOnly, nil
}

// CanMakeReadOnly simulates checking if the tag can be made read-only.
func (m *MockTag) CanMakeReadOnly() (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.CallLog = append(m.CallLog, "CanMakeReadOnly")

	if !m.IsConnected {
		return false, fmt.Errorf("tag not connected")
	}

	if m.CanMakeReadOnlyFunc != nil {
		return m.CanMakeReadOnlyFunc()
	}

	if m.CanMakeReadOnlyError != nil {
		return false, m.CanMakeReadOnlyError
	}

	// By default, can make read-only if not already read-only
	return !m.IsReadOnly, nil
}

// MakeReadOnly simulates making the tag read-only.
func (m *MockTag) MakeReadOnly() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.CallLog = append(m.CallLog, "MakeReadOnly")

	if !m.IsConnected {
		return fmt.Errorf("tag not connected")
	}

	if m.MakeReadOnlyFunc != nil {
		return m.MakeReadOnlyFunc()
	}

	if m.MakeReadOnlyError != nil {
		return m.MakeReadOnlyError
	}

	m.IsReadOnly = true
	return nil
}

// MockClassicTag is a test implementation of ClassicTag for MIFARE Classic tags.
type MockClassicTag struct {
	*MockTag

	// BlockData stores data for each sector/block combination
	// Key format: "sector:block" (e.g., "1:0")
	BlockData map[string][]byte

	// ReadError, if set, will be returned by Read()
	ReadError error

	// WriteError, if set, will be returned by Write()
	WriteError error

	// FreefareTag is the mock freefare tag (can be nil)
	FreefareTag freefare.Tag

	mu sync.Mutex
}

// NewMockClassicTag creates a new MockClassicTag with default values.
func NewMockClassicTag(uid string) *MockClassicTag {
	return &MockClassicTag{
		MockTag:   NewMockTag(uid),
		BlockData: make(map[string][]byte),
	}
}

// Read simulates reading a block from the tag.
func (m *MockClassicTag) Read(sector, block uint8, key []byte, keyType int) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.CallLog = append(m.CallLog, fmt.Sprintf("Read(sector:%d, block:%d, keyType:0x%02X)", sector, block, keyType))

	if !m.IsConnected {
		return nil, fmt.Errorf("tag not connected")
	}

	if m.ReadError != nil {
		return nil, m.ReadError
	}

	blockKey := fmt.Sprintf("%d:%d", sector, block)
	data, exists := m.BlockData[blockKey]
	if !exists {
		// Return empty block if not set
		return make([]byte, 16), nil
	}

	// Return a copy
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)
	return dataCopy, nil
}

// Write simulates writing a block to the tag.
func (m *MockClassicTag) Write(sector, block uint8, data []byte, key []byte, keyType int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.CallLog = append(m.CallLog, fmt.Sprintf("Write(sector:%d, block:%d, %d bytes, keyType:0x%02X)", sector, block, len(data), keyType))

	if !m.IsConnected {
		return fmt.Errorf("tag not connected")
	}

	if m.WriteError != nil {
		return m.WriteError
	}

	blockKey := fmt.Sprintf("%d:%d", sector, block)
	// Store a copy of the data
	m.BlockData[blockKey] = make([]byte, len(data))
	copy(m.BlockData[blockKey], data)
	return nil
}

// GetFreefareTag returns the mock freefare tag.
func (m *MockClassicTag) GetFreefareTag() freefare.Tag {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.FreefareTag
}

// SetBlockData sets the data for a specific sector/block combination.
func (m *MockClassicTag) SetBlockData(sector, block uint8, data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()

	blockKey := fmt.Sprintf("%d:%d", sector, block)
	m.BlockData[blockKey] = make([]byte, len(data))
	copy(m.BlockData[blockKey], data)
}

// GetBlockData retrieves the data for a specific sector/block combination.
func (m *MockClassicTag) GetBlockData(sector, block uint8) ([]byte, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	blockKey := fmt.Sprintf("%d:%d", sector, block)
	data, exists := m.BlockData[blockKey]
	if !exists {
		return nil, false
	}

	// Return a copy
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)
	return dataCopy, true
}

// MockISO14443Tag is a test implementation of ISO14443Tag for Type 4 tags.
type MockISO14443Tag struct {
	*MockTag
}

// NewMockISO14443Tag creates a new MockISO14443Tag with default values.
func NewMockISO14443Tag(uid string) *MockISO14443Tag {
	return &MockISO14443Tag{
		MockTag: NewMockTag(uid),
	}
}
