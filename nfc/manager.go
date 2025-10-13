package nfc

// Manager handles NFC device discovery.
//
// Manager provides methods to list available NFC readers and open connections
// to devices.
//
// Example:
//
//	manager := nfc.NewManager()
//	devices, _ := manager.ListDevices()
//	device, _ := manager.OpenDevice(devices[0])
//	tags, _ := device.GetTags()
type Manager interface {
	OpenDevice(deviceStr string) (Device, error)
	ListDevices() ([]string, error)
}

// NewManager creates a new Manager using the default libnfc/freefare implementation.
//
// Example:
//
//	manager := nfc.NewManager()
func NewManager() Manager {
	return &defaultManager{}
}
