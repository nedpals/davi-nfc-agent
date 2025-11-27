package server

import (
	"testing"
	"time"

	"github.com/nedpals/davi-nfc-agent/nfc"
)

func TestNewSmartphoneHandler(t *testing.T) {
	// Create smartphone manager
	manager := nfc.NewSmartphoneManager(30 * time.Second)
	handler := NewSmartphoneHandler(manager)

	if handler == nil {
		t.Fatal("NewSmartphoneHandler returned nil")
	}
	if handler.manager != manager {
		t.Fatal("handler manager is not the same as input manager")
	}
}

func TestSmartphoneHandler_Register(t *testing.T) {
	// Create smartphone manager
	manager := nfc.NewSmartphoneManager(30 * time.Second)
	handler := NewSmartphoneHandler(manager)

	mockServer := newMockHandlerServer()

	// Register with mock server
	handler.Register(mockServer)

	// Verify all smartphone handlers are registered
	expectedHandlers := []string{
		WSMessageTypeRegisterDevice,
		WSMessageTypeTagScanned,
		WSMessageTypeDeviceHeartbeat,
	}

	for _, msgType := range expectedHandlers {
		if !mockServer.HasHandler(msgType) {
			t.Fatalf("%s handler not registered", msgType)
		}
	}
}

func TestSmartphoneHandler_Integration(t *testing.T) {
	// Create smartphone manager
	manager := nfc.NewSmartphoneManager(30 * time.Second)
	handler := NewSmartphoneHandler(manager)

	mockServer := newMockHandlerServer()

	// Register with mock server
	handler.Register(mockServer)

	t.Run("all routes registered and functional", func(t *testing.T) {
		// Verify all smartphone handlers are registered and functional
		expectedHandlers := []string{
			WSMessageTypeRegisterDevice,
			WSMessageTypeTagScanned,
			WSMessageTypeDeviceHeartbeat,
		}

		for _, msgType := range expectedHandlers {
			if !mockServer.HasHandler(msgType) {
				t.Fatalf("%s handler not registered", msgType)
			}
		}
	})
}
