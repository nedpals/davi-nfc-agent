package nfc

import (
	"fmt"
	"sync"
)

// MockDevice is a test implementation of Device that simulates NFC hardware.
//
// MockDevice allows testing NFC functionality without physical hardware by
// simulating device behavior, connection states, and data transmission.
//
// Example:
//
//	mock := &MockDevice{
//	    DeviceName: "Mock NFC Reader",
//	    DeviceConnection: "mock:usb:001",
//	}
//	err := mock.InitiatorInit()
type MockDevice struct {
	// DeviceName is the simulated device name returned by String()
	DeviceName string

	// DeviceConnection is the simulated connection string returned by Connection()
	DeviceConnection string

	// IsOpen tracks whether the device is currently open
	IsOpen bool

	// InitError, if set, will be returned by InitiatorInit()
	InitError error

	// CloseError, if set, will be returned by Close()
	CloseError error

	// TransceiveFunc allows custom transceive behavior for testing
	// If nil, returns TransceiveResponse or TransceiveError
	TransceiveFunc func([]byte) ([]byte, error)

	// TransceiveResponse is the default response for Transceive calls
	TransceiveResponse []byte

	// TransceiveError, if set, will be returned by Transceive()
	TransceiveError error

	// CallLog tracks all method calls for verification in tests
	CallLog []string

	mu sync.Mutex
}

// NewMockDevice creates a new MockDevice with default values.
func NewMockDevice() *MockDevice {
	return &MockDevice{
		DeviceName:       "Mock NFC Reader",
		DeviceConnection: "mock:usb:001",
		IsOpen:           true,
		CallLog:          make([]string, 0),
	}
}

// Close simulates closing the device.
func (m *MockDevice) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.CallLog = append(m.CallLog, "Close")

	if !m.IsOpen {
		return fmt.Errorf("device already closed")
	}

	m.IsOpen = false
	return m.CloseError
}

// InitiatorInit simulates device initialization.
func (m *MockDevice) InitiatorInit() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.CallLog = append(m.CallLog, "InitiatorInit")

	if !m.IsOpen {
		return fmt.Errorf("device not open")
	}

	return m.InitError
}

// String returns the simulated device name.
func (m *MockDevice) String() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.CallLog = append(m.CallLog, "String")
	return m.DeviceName
}

// Connection returns the simulated connection string.
func (m *MockDevice) Connection() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.CallLog = append(m.CallLog, "Connection")
	return m.DeviceConnection
}

// Transceive simulates data transmission with the device.
func (m *MockDevice) Transceive(txData []byte) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.CallLog = append(m.CallLog, fmt.Sprintf("Transceive(%d bytes)", len(txData)))

	if !m.IsOpen {
		return nil, fmt.Errorf("device not open")
	}

	if m.TransceiveFunc != nil {
		return m.TransceiveFunc(txData)
	}

	if m.TransceiveError != nil {
		return nil, m.TransceiveError
	}

	return m.TransceiveResponse, nil
}

// GetCallLog returns a copy of the call log for verification.
func (m *MockDevice) GetCallLog() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	logCopy := make([]string, len(m.CallLog))
	copy(logCopy, m.CallLog)
	return logCopy
}

// ClearCallLog clears the call log.
func (m *MockDevice) ClearCallLog() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.CallLog = make([]string, 0)
}
