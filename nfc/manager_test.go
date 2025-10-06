package nfc

import (
	"fmt"
	"testing"
)

func TestMockManager_ListDevices(t *testing.T) {
	manager := NewMockManager()
	manager.DevicesList = []string{"mock:usb:001", "mock:usb:002", "mock:usb:003"}

	devices, err := manager.ListDevices()
	if err != nil {
		t.Errorf("ListDevices() failed: %v", err)
	}

	if len(devices) != 3 {
		t.Errorf("Expected 3 devices, got %d", len(devices))
	}

	expectedDevices := []string{"mock:usb:001", "mock:usb:002", "mock:usb:003"}
	for i, device := range devices {
		if device != expectedDevices[i] {
			t.Errorf("Expected device %d to be '%s', got '%s'", i, expectedDevices[i], device)
		}
	}
}

func TestMockManager_ListDevicesError(t *testing.T) {
	manager := NewMockManager()
	expectedErr := fmt.Errorf("no devices found")
	manager.ListDevicesError = expectedErr

	_, err := manager.ListDevices()
	if err != expectedErr {
		t.Errorf("Expected error '%v', got '%v'", expectedErr, err)
	}
}

func TestMockManager_OpenDevice(t *testing.T) {
	manager := NewMockManager()

	device, err := manager.OpenDevice("mock:usb:001")
	if err != nil {
		t.Errorf("OpenDevice() failed: %v", err)
	}

	if device == nil {
		t.Error("Expected device to be non-nil")
	}

	// Verify the device has the correct connection string
	if conn := device.Connection(); conn != "mock:usb:001" {
		t.Errorf("Expected connection 'mock:usb:001', got '%s'", conn)
	}
}

func TestMockManager_OpenDeviceError(t *testing.T) {
	manager := NewMockManager()
	expectedErr := fmt.Errorf("device busy")
	manager.OpenDeviceError = expectedErr

	_, err := manager.OpenDevice("mock:usb:001")
	if err != expectedErr {
		t.Errorf("Expected error '%v', got '%v'", expectedErr, err)
	}
}

func TestMockManager_GetTags(t *testing.T) {
	manager := NewMockManager()
	device := NewMockDevice()

	// Add some mock tags
	tag1 := NewMockTag("04A1B2C3")
	tag2 := NewMockTag("04D5E6F7")
	manager.SetTags([]Tag{tag1, tag2})

	tags, err := manager.GetTags(device)
	if err != nil {
		t.Errorf("GetTags() failed: %v", err)
	}

	if len(tags) != 2 {
		t.Errorf("Expected 2 tags, got %d", len(tags))
	}
}

func TestMockManager_GetTagsError(t *testing.T) {
	manager := NewMockManager()
	device := NewMockDevice()
	expectedErr := fmt.Errorf("no tags detected")
	manager.GetTagsError = expectedErr

	_, err := manager.GetTags(device)
	if err != expectedErr {
		t.Errorf("Expected error '%v', got '%v'", expectedErr, err)
	}
}

func TestMockManager_AddAndClearTags(t *testing.T) {
	manager := NewMockManager()
	device := NewMockDevice()

	// Initially should have no tags
	tags, _ := manager.GetTags(device)
	if len(tags) != 0 {
		t.Errorf("Expected 0 tags initially, got %d", len(tags))
	}

	// Add a tag
	tag := NewMockTag("04A1B2C3")
	manager.AddTag(tag)

	tags, _ = manager.GetTags(device)
	if len(tags) != 1 {
		t.Errorf("Expected 1 tag after adding, got %d", len(tags))
	}

	// Clear tags
	manager.ClearTags()
	tags, _ = manager.GetTags(device)
	if len(tags) != 0 {
		t.Errorf("Expected 0 tags after clearing, got %d", len(tags))
	}
}

func TestMockManager_CallLog(t *testing.T) {
	manager := NewMockManager()
	device := NewMockDevice()
	manager.ClearCallLog()

	manager.ListDevices()
	manager.OpenDevice("mock:usb:001")
	manager.GetTags(device)

	callLog := manager.GetCallLog()
	expectedCalls := []string{
		"ListDevices",
		"OpenDevice(mock:usb:001)",
		"GetTags",
	}

	if len(callLog) != len(expectedCalls) {
		t.Errorf("Expected %d calls, got %d", len(expectedCalls), len(callLog))
	}

	for i, call := range expectedCalls {
		if i >= len(callLog) {
			break
		}
		if callLog[i] != call {
			t.Errorf("Expected call %d to be '%s', got '%s'", i, call, callLog[i])
		}
	}
}
