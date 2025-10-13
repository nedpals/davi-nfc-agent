package nfc

// Device represents an NFC reader/writer hardware device.
//
// A Device is obtained from a Manager and provides low-level access
// to NFC communication capabilities.
//
// Example:
//
//	manager := nfc.NewManager()
//	device, err := manager.OpenDevice("")
//	defer device.Close()
type Device interface {
	Close() error
	InitiatorInit() error
	String() string
	Connection() string
	Transceive(txData []byte) ([]byte, error)
	GetTags() ([]Tag, error)
}
