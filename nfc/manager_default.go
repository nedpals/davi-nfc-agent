package nfc

import (
	"fmt"
	"time"

	"github.com/clausecker/nfc/v2"
)

// defaultManager implements Manager using libnfc and freefare libraries.
type defaultManager struct{}

func (m *defaultManager) OpenDevice(deviceStr string) (Device, error) {
	dev, err := nfc.Open(deviceStr)
	if err != nil {
		return nil, err
	}

	// Initialize the device before returning - this ensures
	// the device is ready for use immediately
	if err := dev.InitiatorInit(); err != nil {
		dev.Close()
		return nil, fmt.Errorf("failed to initialize device: %w", err)
	}

	return NewDevice(dev), nil
}

func (m *defaultManager) ListDevices() ([]string, error) {
	var devices []string
	var err error
	for i := 0; i < DeviceEnumRetries; i++ {
		devices, err = nfc.ListDevices()
		if err == nil {
			return devices, nil
		}
		time.Sleep(time.Millisecond * 100)
	}
	return nil, fmt.Errorf("failed to list NFC devices after %d retries: %w", DeviceEnumRetries, err)
}
