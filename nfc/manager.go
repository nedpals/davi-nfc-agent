package nfc

// Manager handles NFC device discovery and tag detection.
//
// Manager provides methods to list available NFC readers, open connections
// to devices, and detect tags on those devices.
//
// Example:
//
//	manager := nfc.NewManager()
//	devices, _ := manager.ListDevices()
//	device, _ := manager.OpenDevice(devices[0])
//	tags, _ := manager.GetTags(device)
type Manager interface {
	OpenDevice(deviceStr string) (Device, error)
	ListDevices() ([]string, error)
	GetTags(dev Device) ([]Tag, error)
}

// NewManager creates a new Manager using the default libnfc/freefare implementation.
//
// Example:
//
//	manager := nfc.NewManager()
func NewManager() Manager {
	return &defaultManager{}
}
