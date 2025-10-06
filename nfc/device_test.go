package nfc

import (
	"fmt"
	"testing"
)

func TestMockDevice_BasicOperations(t *testing.T) {
	device := NewMockDevice()

	// Test String()
	if name := device.String(); name != "Mock NFC Reader" {
		t.Errorf("Expected device name 'Mock NFC Reader', got '%s'", name)
	}

	// Test Connection()
	if conn := device.Connection(); conn != "mock:usb:001" {
		t.Errorf("Expected connection 'mock:usb:001', got '%s'", conn)
	}

	// Test InitiatorInit()
	if err := device.InitiatorInit(); err != nil {
		t.Errorf("InitiatorInit() failed: %v", err)
	}

	// Test Close()
	if err := device.Close(); err != nil {
		t.Errorf("Close() failed: %v", err)
	}

	// Verify device is closed
	if device.IsOpen {
		t.Error("Device should be closed")
	}
}

func TestMockDevice_InitError(t *testing.T) {
	device := NewMockDevice()
	expectedErr := fmt.Errorf("init failed")
	device.InitError = expectedErr

	if err := device.InitiatorInit(); err != expectedErr {
		t.Errorf("Expected error '%v', got '%v'", expectedErr, err)
	}
}

func TestMockDevice_CloseError(t *testing.T) {
	device := NewMockDevice()
	expectedErr := fmt.Errorf("close failed")
	device.CloseError = expectedErr

	if err := device.Close(); err != expectedErr {
		t.Errorf("Expected error '%v', got '%v'", expectedErr, err)
	}
}

func TestMockDevice_Transceive(t *testing.T) {
	device := NewMockDevice()
	device.TransceiveResponse = []byte{0x90, 0x00}

	txData := []byte{0x00, 0xA4, 0x04, 0x00}
	rxData, err := device.Transceive(txData)
	if err != nil {
		t.Errorf("Transceive() failed: %v", err)
	}

	if len(rxData) != 2 || rxData[0] != 0x90 || rxData[1] != 0x00 {
		t.Errorf("Expected response [0x90 0x00], got %v", rxData)
	}
}

func TestMockDevice_TransceiveCustomFunc(t *testing.T) {
	device := NewMockDevice()
	device.TransceiveFunc = func(tx []byte) ([]byte, error) {
		// Echo back the transmitted data
		return tx, nil
	}

	txData := []byte{0x01, 0x02, 0x03}
	rxData, err := device.Transceive(txData)
	if err != nil {
		t.Errorf("Transceive() failed: %v", err)
	}

	if len(rxData) != len(txData) {
		t.Errorf("Expected response length %d, got %d", len(txData), len(rxData))
	}

	for i, b := range rxData {
		if b != txData[i] {
			t.Errorf("Expected byte %d at position %d, got %d", txData[i], i, b)
		}
	}
}

func TestMockDevice_CallLog(t *testing.T) {
	device := NewMockDevice()
	device.ClearCallLog()

	_ = device.String()
	_ = device.Connection()
	_ = device.InitiatorInit()
	_, _ = device.Transceive([]byte{0x01})
	_ = device.Close()

	callLog := device.GetCallLog()
	expectedCalls := []string{
		"String",
		"Connection",
		"InitiatorInit",
		"Transceive(1 bytes)",
		"Close",
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
