package nfc

// pcscBaseTag provides common functionality for PC/SC tag implementations
type pcscBaseTag struct {
	device       *pcscDevice
	uid          string
	detectedType DetectedTagType
	connected    bool
}

func (t *pcscBaseTag) UID() string {
	return t.uid
}

func (t *pcscBaseTag) Connect() error {
	t.connected = true
	return nil
}

func (t *pcscBaseTag) Disconnect() error {
	t.connected = false
	return nil
}

// transceive sends an APDU and returns the response data.
// Card removal detection is handled at the device layer via Transceive().
func (t *pcscBaseTag) transceive(cmd []byte) ([]byte, error) {
	resp, err := t.device.Transceive(cmd)
	if err != nil {
		return nil, err // Device layer already wraps card removal errors
	}

	parsed, err := ParseAPDUResponse(resp)
	if err != nil {
		return nil, err
	}

	if !parsed.IsSuccess() && !parsed.HasMoreData() {
		return nil, parsed.Error()
	}

	return parsed.Data, nil
}

// transmitRaw sends an APDU and returns the raw response (with SW bytes).
// Card removal detection is handled at the device layer via Transceive().
func (t *pcscBaseTag) transmitRaw(cmd []byte) ([]byte, error) {
	return t.device.Transceive(cmd)
}
