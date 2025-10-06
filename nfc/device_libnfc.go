package nfc

import (
	"fmt"

	"github.com/clausecker/nfc/v2"
)

// libnfcDevice implements Device using an actual nfc.Device from libnfc.
type libnfcDevice struct {
	device nfc.Device
}

// NewDevice creates a new Device from an nfc.Device.
func NewDevice(dev nfc.Device) Device {
	return &libnfcDevice{device: dev}
}

func (d *libnfcDevice) Close() error {
	return d.device.Close()
}

func (d *libnfcDevice) InitiatorInit() error {
	return d.device.InitiatorInit()
}

func (d *libnfcDevice) String() string {
	return d.device.String()
}

func (d *libnfcDevice) Connection() string {
	return d.device.Connection()
}

// Transceive implements the Device Transceive method for raw data exchange.
func (d *libnfcDevice) Transceive(txData []byte) ([]byte, error) {
	var rxData [262]byte // Max buffer size for NFC
	count, err := d.device.InitiatorTransceiveBytes(txData, rxData[:], 0)
	if err != nil {
		return nil, fmt.Errorf("libnfcDevice.Transceive: %w", err)
	}
	return rxData[:count], nil
}
