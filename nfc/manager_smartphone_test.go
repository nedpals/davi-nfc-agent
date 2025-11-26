package nfc

import (
	"testing"
	"time"
)

func TestNewSmartphoneManager(t *testing.T) {
	sm := NewSmartphoneManager(30 * time.Second)
	defer sm.Close()

	if sm == nil {
		t.Fatal("NewSmartphoneManager() returned nil")
	}

	if sm.GetDeviceCount() != 0 {
		t.Errorf("New manager should have 0 devices, got %d", sm.GetDeviceCount())
	}
}

func TestSmartphoneManagerRegisterDevice(t *testing.T) {
	sm := NewSmartphoneManager(30 * time.Second)
	defer sm.Close()

	req := DeviceRegistrationRequest{
		DeviceName: "Test iPhone",
		Platform:   "ios",
		AppVersion: "1.0.0",
		Capabilities: DeviceCapabilities{
			CanRead:  true,
			CanWrite: true,
			NFCType:  "isodep",
		},
	}

	device, err := sm.RegisterDevice(req)
	if err != nil {
		t.Fatalf("RegisterDevice() failed: %v", err)
	}

	if device == nil {
		t.Fatal("RegisterDevice() returned nil device")
	}

	if device.DeviceID() == "" {
		t.Error("Device should have non-empty ID")
	}

	if sm.GetDeviceCount() != 1 {
		t.Errorf("Manager should have 1 device, got %d", sm.GetDeviceCount())
	}
}

func TestSmartphoneManagerRegisterDeviceValidation(t *testing.T) {
	sm := NewSmartphoneManager(30 * time.Second)
	defer sm.Close()

	tests := []struct {
		name    string
		req     DeviceRegistrationRequest
		wantErr bool
	}{
		{
			name: "valid iOS device",
			req: DeviceRegistrationRequest{
				DeviceName: "iPhone",
				Platform:   "ios",
				AppVersion: "1.0.0",
			},
			wantErr: false,
		},
		{
			name: "valid Android device",
			req: DeviceRegistrationRequest{
				DeviceName: "Pixel",
				Platform:   "android",
				AppVersion: "1.0.0",
			},
			wantErr: false,
		},
		{
			name: "missing device name",
			req: DeviceRegistrationRequest{
				Platform:   "ios",
				AppVersion: "1.0.0",
			},
			wantErr: true,
		},
		{
			name: "invalid platform",
			req: DeviceRegistrationRequest{
				DeviceName: "Device",
				Platform:   "windows",
				AppVersion: "1.0.0",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := sm.RegisterDevice(tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("RegisterDevice() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSmartphoneManagerUnregisterDevice(t *testing.T) {
	sm := NewSmartphoneManager(30 * time.Second)
	defer sm.Close()

	req := DeviceRegistrationRequest{
		DeviceName: "Test Device",
		Platform:   "ios",
		AppVersion: "1.0.0",
	}

	device, err := sm.RegisterDevice(req)
	if err != nil {
		t.Fatalf("RegisterDevice() failed: %v", err)
	}

	deviceID := device.DeviceID()

	// Unregister device
	err = sm.UnregisterDevice(deviceID)
	if err != nil {
		t.Errorf("UnregisterDevice() failed: %v", err)
	}

	if sm.GetDeviceCount() != 0 {
		t.Errorf("Manager should have 0 devices after unregister, got %d", sm.GetDeviceCount())
	}

	// Try to unregister again (should fail)
	err = sm.UnregisterDevice(deviceID)
	if err == nil {
		t.Error("UnregisterDevice() should fail for non-existent device")
	}
}

func TestSmartphoneManagerOpenDevice(t *testing.T) {
	sm := NewSmartphoneManager(30 * time.Second)
	defer sm.Close()

	req := DeviceRegistrationRequest{
		DeviceName: "Test Device",
		Platform:   "android",
		AppVersion: "1.0.0",
	}

	registeredDevice, err := sm.RegisterDevice(req)
	if err != nil {
		t.Fatalf("RegisterDevice() failed: %v", err)
	}

	deviceID := registeredDevice.DeviceID()

	// Test opening with full connection string
	device, err := sm.OpenDevice("smartphone:" + deviceID)
	if err != nil {
		t.Errorf("OpenDevice() with prefix failed: %v", err)
	}
	if device == nil {
		t.Error("OpenDevice() returned nil device")
	}

	// Test opening with just device ID
	device, err = sm.OpenDevice(deviceID)
	if err != nil {
		t.Errorf("OpenDevice() without prefix failed: %v", err)
	}
	if device == nil {
		t.Error("OpenDevice() returned nil device")
	}

	// Test opening non-existent device
	_, err = sm.OpenDevice("non-existent-id")
	if err == nil {
		t.Error("OpenDevice() should fail for non-existent device")
	}
}

func TestSmartphoneManagerListDevices(t *testing.T) {
	sm := NewSmartphoneManager(30 * time.Second)
	defer sm.Close()

	// Initially empty
	devices, err := sm.ListDevices()
	if err != nil {
		t.Errorf("ListDevices() failed: %v", err)
	}
	if len(devices) != 0 {
		t.Errorf("ListDevices() should return empty list, got %d devices", len(devices))
	}

	// Register multiple devices
	for i := 0; i < 3; i++ {
		req := DeviceRegistrationRequest{
			DeviceName: "Device",
			Platform:   "ios",
			AppVersion: "1.0.0",
		}
		_, err := sm.RegisterDevice(req)
		if err != nil {
			t.Fatalf("RegisterDevice() failed: %v", err)
		}
	}

	devices, err = sm.ListDevices()
	if err != nil {
		t.Errorf("ListDevices() failed: %v", err)
	}
	if len(devices) != 3 {
		t.Errorf("ListDevices() should return 3 devices, got %d", len(devices))
	}

	// Check format
	for _, device := range devices {
		if len(device) < len("smartphone:") {
			t.Errorf("Device string should have 'smartphone:' prefix, got: %s", device)
		}
	}
}

func TestSmartphoneManagerSendTagData(t *testing.T) {
	sm := NewSmartphoneManager(30 * time.Second)
	defer sm.Close()

	req := DeviceRegistrationRequest{
		DeviceName: "Test Device",
		Platform:   "ios",
		AppVersion: "1.0.0",
	}

	device, err := sm.RegisterDevice(req)
	if err != nil {
		t.Fatalf("RegisterDevice() failed: %v", err)
	}

	tagData := SmartphoneTagData{
		DeviceID:   device.DeviceID(),
		UID:        "04:AB:CD:EF",
		Technology: "ISO14443A",
		Type:       "Type4",
		ScannedAt:  time.Now(),
	}

	// Send tag data
	err = sm.SendTagData(device.DeviceID(), tagData)
	if err != nil {
		t.Errorf("SendTagData() failed: %v", err)
	}

	// Try sending to non-existent device
	tagData.DeviceID = "non-existent"
	err = sm.SendTagData("non-existent", tagData)
	if err == nil {
		t.Error("SendTagData() should fail for non-existent device")
	}
}

func TestSmartphoneManagerUpdateHeartbeat(t *testing.T) {
	sm := NewSmartphoneManager(30 * time.Second)
	defer sm.Close()

	req := DeviceRegistrationRequest{
		DeviceName: "Test Device",
		Platform:   "android",
		AppVersion: "1.0.0",
	}

	device, err := sm.RegisterDevice(req)
	if err != nil {
		t.Fatalf("RegisterDevice() failed: %v", err)
	}

	deviceID := device.DeviceID()
	firstSeen := device.LastSeen()

	time.Sleep(10 * time.Millisecond)

	err = sm.UpdateHeartbeat(deviceID)
	if err != nil {
		t.Errorf("UpdateHeartbeat() failed: %v", err)
	}

	// Get device and check last seen
	updatedDevice, exists := sm.GetDevice(deviceID)
	if !exists {
		t.Fatal("Device should exist after heartbeat")
	}

	if !updatedDevice.LastSeen().After(firstSeen) {
		t.Error("LastSeen should be updated after heartbeat")
	}

	// Try updating non-existent device
	err = sm.UpdateHeartbeat("non-existent")
	if err == nil {
		t.Error("UpdateHeartbeat() should fail for non-existent device")
	}
}

func TestSmartphoneManagerCleanupInactiveDevices(t *testing.T) {
	// Use short timeout for testing
	sm := NewSmartphoneManager(100 * time.Millisecond)
	defer sm.Close()

	req := DeviceRegistrationRequest{
		DeviceName: "Test Device",
		Platform:   "ios",
		AppVersion: "1.0.0",
	}

	device, err := sm.RegisterDevice(req)
	if err != nil {
		t.Fatalf("RegisterDevice() failed: %v", err)
	}

	deviceID := device.DeviceID()

	// Device should exist initially
	if sm.GetDeviceCount() != 1 {
		t.Error("Device should exist initially")
	}

	// Wait for cleanup to run (timeout + cleanup interval)
	time.Sleep(200 * time.Millisecond)

	// Manually trigger cleanup
	sm.cleanupInactiveDevices()

	// Device should be removed
	_, exists := sm.GetDevice(deviceID)
	if exists {
		t.Error("Device should be removed after timeout")
	}
}

func TestSmartphoneManagerGetDevice(t *testing.T) {
	sm := NewSmartphoneManager(30 * time.Second)
	defer sm.Close()

	req := DeviceRegistrationRequest{
		DeviceName: "Test Device",
		Platform:   "android",
		AppVersion: "1.0.0",
	}

	registeredDevice, err := sm.RegisterDevice(req)
	if err != nil {
		t.Fatalf("RegisterDevice() failed: %v", err)
	}

	deviceID := registeredDevice.DeviceID()

	// Get existing device
	device, exists := sm.GetDevice(deviceID)
	if !exists {
		t.Error("GetDevice() should return true for existing device")
	}
	if device == nil {
		t.Error("GetDevice() returned nil device")
	}

	// Get non-existent device
	_, exists = sm.GetDevice("non-existent")
	if exists {
		t.Error("GetDevice() should return false for non-existent device")
	}
}

func TestSmartphoneManagerClose(t *testing.T) {
	sm := NewSmartphoneManager(30 * time.Second)

	// Register some devices
	for i := 0; i < 3; i++ {
		req := DeviceRegistrationRequest{
			DeviceName: "Device",
			Platform:   "ios",
			AppVersion: "1.0.0",
		}
		_, err := sm.RegisterDevice(req)
		if err != nil {
			t.Fatalf("RegisterDevice() failed: %v", err)
		}
	}

	if sm.GetDeviceCount() != 3 {
		t.Errorf("Should have 3 devices before close, got %d", sm.GetDeviceCount())
	}

	// Close manager
	sm.Close()

	// All devices should be removed
	if sm.GetDeviceCount() != 0 {
		t.Errorf("Should have 0 devices after close, got %d", sm.GetDeviceCount())
	}
}
