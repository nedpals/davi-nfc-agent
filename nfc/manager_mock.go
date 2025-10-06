package nfc

import (
	"fmt"
	"sync"
)

// MockManager is a test implementation of Manager that simulates NFC device management.
//
// MockManager allows testing device discovery and tag detection without physical
// hardware by providing configurable mock responses.
//
// Example:
//
//	manager := &MockManager{
//	    DevicesList: []string{"mock:usb:001", "mock:usb:002"},
//	    MockDevice: NewMockDevice(),
//	}
//	devices, _ := manager.ListDevices()
type MockManager struct {
	// DevicesList is the list of device strings returned by ListDevices()
	DevicesList []string

	// ListDevicesError, if set, will be returned by ListDevices()
	ListDevicesError error

	// MockDevice is the device returned by OpenDevice()
	// If nil, a new MockDevice will be created
	MockDevice *MockDevice

	// OpenDeviceError, if set, will be returned by OpenDevice()
	OpenDeviceError error

	// Tags is the list of tags returned by GetTags()
	Tags []Tag

	// GetTagsError, if set, will be returned by GetTags()
	GetTagsError error

	// CallLog tracks all method calls for verification in tests
	CallLog []string

	mu sync.Mutex
}

// NewMockManager creates a new MockManager with default values.
func NewMockManager() *MockManager {
	return &MockManager{
		DevicesList: []string{"mock:usb:001"},
		MockDevice:  NewMockDevice(),
		Tags:        make([]Tag, 0),
		CallLog:     make([]string, 0),
	}
}

// OpenDevice simulates opening an NFC device.
func (m *MockManager) OpenDevice(deviceStr string) (Device, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.CallLog = append(m.CallLog, fmt.Sprintf("OpenDevice(%s)", deviceStr))

	if m.OpenDeviceError != nil {
		return nil, m.OpenDeviceError
	}

	if m.MockDevice == nil {
		m.MockDevice = NewMockDevice()
	}

	m.MockDevice.DeviceConnection = deviceStr
	return m.MockDevice, nil
}

// ListDevices simulates listing available NFC devices.
func (m *MockManager) ListDevices() ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.CallLog = append(m.CallLog, "ListDevices")

	if m.ListDevicesError != nil {
		return nil, m.ListDevicesError
	}

	// Return a copy to prevent external modification
	devicesCopy := make([]string, len(m.DevicesList))
	copy(devicesCopy, m.DevicesList)
	return devicesCopy, nil
}

// GetTags simulates detecting tags on the device.
func (m *MockManager) GetTags(dev Device) ([]Tag, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.CallLog = append(m.CallLog, "GetTags")

	if m.GetTagsError != nil {
		return nil, m.GetTagsError
	}

	// Return a copy to prevent external modification
	tagsCopy := make([]Tag, len(m.Tags))
	copy(tagsCopy, m.Tags)
	return tagsCopy, nil
}

// SetTags sets the tags that will be returned by GetTags().
func (m *MockManager) SetTags(tags []Tag) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Tags = tags
}

// AddTag adds a tag to the list returned by GetTags().
func (m *MockManager) AddTag(tag Tag) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Tags = append(m.Tags, tag)
}

// ClearTags removes all tags from the list returned by GetTags().
func (m *MockManager) ClearTags() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Tags = make([]Tag, 0)
}

// GetCallLog returns a copy of the call log for verification.
func (m *MockManager) GetCallLog() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	logCopy := make([]string, len(m.CallLog))
	copy(logCopy, m.CallLog)
	return logCopy
}

// ClearCallLog clears the call log.
func (m *MockManager) ClearCallLog() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.CallLog = make([]string, 0)
}
