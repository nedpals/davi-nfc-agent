package nfc

import (
	"encoding/binary"
	"fmt"
	"log"

	"github.com/clausecker/freefare"
)

// DESFireTag wraps a MIFARE DESFire tag with NFC operations.
//
// DESFireTag provides access to DESFire-specific features like applications,
// files, and authentication with various key types (DES, 3DES, AES).
//
// Example:
//
//	tags, _ := device.GetTags()
//	for _, tag := range tags {
//	    if desfire, ok := tag.(*DESFireTag); ok {
//	        version, _ := desfire.Version()
//	        apps, _ := desfire.ApplicationIds()
//	    }
//	}
type DESFireTag struct {
	tag freefare.DESFireTag
}

// NewDESFireTag creates a new DESFire tag wrapper.
func NewDESFireTag(tag freefare.DESFireTag) *DESFireTag {
	return &DESFireTag{tag: tag}
}

func (d *DESFireTag) UID() string {
	return d.tag.UID()
}

func (d *DESFireTag) Type() string {
	return fmt.Sprintf("MIFARE DESFire (type %d)", d.tag.Type())
}

func (d *DESFireTag) NumericType() int {
	return int(d.tag.Type())
}

func (d *DESFireTag) GetFreefareTag() freefare.Tag {
	return d.tag
}

// Capabilities returns the capabilities of this DESFire tag.
func (d *DESFireTag) Capabilities() TagCapabilities {
	return TagCapabilities{
		CanRead:                true,
		CanWrite:               true,
		CanTransceive:          false,
		CanLock:                false, // Not implemented
		TagFamily:              "DESFire",
		Technology:             "ISO14443A",
		MemorySize:             8192, // Varies by model
		SupportsNDEF:           true,
		SupportsCrypto:         true,
		SupportsAuthentication: true,
	}
}

func (d *DESFireTag) Connect() error {
	return d.tag.Connect()
}

func (d *DESFireTag) Disconnect() error {
	return d.tag.Disconnect()
}

func (d *DESFireTag) Transceive(data []byte) ([]byte, error) {
	return nil, NewNotSupportedError("Transceive")
}

// Version returns the DESFire version information as a byte slice.
func (d *DESFireTag) Version() ([]byte, error) {
	if err := d.tag.Connect(); err != nil {
		return nil, fmt.Errorf("DESFireTag.Version connect error: %w", err)
	}
	defer d.tag.Disconnect()

	versionInfo, err := d.tag.Version()
	if err != nil {
		return nil, fmt.Errorf("DESFireTag.Version error: %w", err)
	}

	// Pack version info into byte slice
	result := make([]byte, 21)
	result[0] = versionInfo.Hardware.VendorID
	result[1] = versionInfo.Hardware.Type
	result[2] = versionInfo.Hardware.Subtype
	result[3] = versionInfo.Hardware.VersionMajor
	result[4] = versionInfo.Hardware.VersionMinor
	result[5] = versionInfo.Hardware.StorageSize
	result[6] = versionInfo.Hardware.Protocol

	result[7] = versionInfo.Software.VendorID
	result[8] = versionInfo.Software.Type
	result[9] = versionInfo.Software.Subtype
	result[10] = versionInfo.Software.VersionMajor
	result[11] = versionInfo.Software.VersionMinor
	result[12] = versionInfo.Software.StorageSize
	result[13] = versionInfo.Software.Protocol

	copy(result[14:], versionInfo.UID[:])

	return result, nil
}

// ApplicationIds returns the list of application IDs on the DESFire card.
func (d *DESFireTag) ApplicationIds() ([][]byte, error) {
	if err := d.tag.Connect(); err != nil {
		return nil, fmt.Errorf("DESFireTag.ApplicationIds connect error: %w", err)
	}
	defer d.tag.Disconnect()

	aids, err := d.tag.ApplicationIds()
	if err != nil {
		return nil, fmt.Errorf("DESFireTag.ApplicationIds error: %w", err)
	}

	result := make([][]byte, len(aids))
	for i, aid := range aids {
		result[i] = aid[:]
	}
	return result, nil
}

// SelectApplication selects an application on the DESFire card.
func (d *DESFireTag) SelectApplication(aid []byte) error {
	if len(aid) != 3 {
		return fmt.Errorf("DESFireTag.SelectApplication: AID must be 3 bytes, got %d", len(aid))
	}

	if err := d.tag.Connect(); err != nil {
		return fmt.Errorf("DESFireTag.SelectApplication connect error: %w", err)
	}
	defer d.tag.Disconnect()

	var aidArray [3]byte
	copy(aidArray[:], aid)
	desFireAid := freefare.DESFireAid(aidArray)

	if err := d.tag.SelectApplication(desFireAid); err != nil {
		return fmt.Errorf("DESFireTag.SelectApplication error: %w", err)
	}
	return nil
}

// Authenticate authenticates with a key on the DESFire card.
func (d *DESFireTag) Authenticate(keyNo byte, key []byte) error {
	if err := d.tag.Connect(); err != nil {
		return fmt.Errorf("DESFireTag.Authenticate connect error: %w", err)
	}
	defer d.tag.Disconnect()

	// Create appropriate key type based on key length
	var desFireKey *freefare.DESFireKey
	switch len(key) {
	case 8: // DES
		var keyArray [8]byte
		copy(keyArray[:], key)
		desFireKey = freefare.NewDESFireDESKey(keyArray)
	case 16: // 3DES or AES
		var keyArray [16]byte
		copy(keyArray[:], key)
		// Default to 3DES for 16-byte keys
		desFireKey = freefare.NewDESFire3DESKey(keyArray)
	case 24: // 3K3DES
		var keyArray [24]byte
		copy(keyArray[:], key)
		desFireKey = freefare.NewDESFire3K3DESKey(keyArray)
	default:
		return fmt.Errorf("DESFireTag.Authenticate: invalid key length %d (must be 8, 16, or 24)", len(key))
	}

	if err := d.tag.Authenticate(keyNo, *desFireKey); err != nil {
		return fmt.Errorf("DESFireTag.Authenticate error: %w", err)
	}
	return nil
}

// FileIds returns the list of file IDs in the currently selected application.
func (d *DESFireTag) FileIds() ([]byte, error) {
	if err := d.tag.Connect(); err != nil {
		return nil, fmt.Errorf("DESFireTag.FileIds connect error: %w", err)
	}
	defer d.tag.Disconnect()

	fileIds, err := d.tag.FileIds()
	if err != nil {
		return nil, fmt.Errorf("DESFireTag.FileIds error: %w", err)
	}
	return fileIds, nil
}

// ReadFile reads data from a file on the DESFire card.
func (d *DESFireTag) ReadFile(fileNo byte, offset int64, length int) ([]byte, error) {
	if err := d.tag.Connect(); err != nil {
		return nil, fmt.Errorf("DESFireTag.ReadFile connect error: %w", err)
	}
	defer d.tag.Disconnect()

	buf := make([]byte, length)
	n, err := d.tag.ReadData(fileNo, offset, buf)
	if err != nil {
		return nil, fmt.Errorf("DESFireTag.ReadFile error: %w", err)
	}
	return buf[:n], nil
}

// WriteFile writes data to a file on the DESFire card.
func (d *DESFireTag) WriteFile(fileNo byte, offset int64, data []byte) error {
	if err := d.tag.Connect(); err != nil {
		return fmt.Errorf("DESFireTag.WriteFile connect error: %w", err)
	}
	defer d.tag.Disconnect()

	_, err := d.tag.WriteData(fileNo, offset, data)
	if err != nil {
		return fmt.Errorf("DESFireTag.WriteFile error: %w", err)
	}
	return nil
}

// ReadData reads NDEF data from the DESFire card.
func (d *DESFireTag) ReadData() ([]byte, error) {
	if err := d.tag.Connect(); err != nil {
		return nil, fmt.Errorf("DESFireTag.ReadData connect error: %w", err)
	}
	defer d.tag.Disconnect()

	// Standard NDEF application ID for DESFire
	ndefAid := freefare.DESFireAid{0x01, 0x00, 0x00}

	// Try to select NDEF application
	if err := d.tag.SelectApplication(ndefAid); err != nil {
		log.Printf("DESFireTag.ReadData: NDEF application not found or error: %v", err)
		return nil, nil
	}

	// Try to read standard NDEF file (usually file 1 or 2)
	// First try file 2 (capability container is usually file 1)
	fileIds, err := d.tag.FileIds()
	if err != nil || len(fileIds) == 0 {
		log.Printf("DESFireTag.ReadData: no files found in NDEF application")
		return nil, nil
	}

	// Look for NDEF file (typically file 2)
	var ndefFileNo byte = 2
	foundFile := false
	for _, fid := range fileIds {
		if fid == 2 || fid == 1 {
			ndefFileNo = fid
			foundFile = true
			break
		}
	}

	if !foundFile && len(fileIds) > 0 {
		ndefFileNo = fileIds[0]
	}

	// Read first 2 bytes to get NDEF message length
	lenBuf := make([]byte, 2)
	_, err = d.tag.ReadData(ndefFileNo, 0, lenBuf)
	if err != nil {
		log.Printf("DESFireTag.ReadData: error reading NDEF length: %v", err)
		return nil, nil
	}

	ndefLen := binary.BigEndian.Uint16(lenBuf)
	if ndefLen == 0 {
		log.Println("DESFireTag.ReadData: NDEF message is empty")
		return nil, nil
	}

	// Read the NDEF message
	buf := make([]byte, ndefLen)
	n, err := d.tag.ReadData(ndefFileNo, 2, buf)
	if err != nil {
		return nil, fmt.Errorf("DESFireTag.ReadData: error reading NDEF message: %w", err)
	}

	return buf[:n], nil
}

// WriteData writes NDEF data to the DESFire card.
func (d *DESFireTag) WriteData(data []byte) error {
	if err := d.tag.Connect(); err != nil {
		return fmt.Errorf("DESFireTag.WriteData connect error: %w", err)
	}
	defer d.tag.Disconnect()

	// Standard NDEF application ID for DESFire
	ndefAid := freefare.DESFireAid{0x01, 0x00, 0x00}

	// Try to select NDEF application
	if err := d.tag.SelectApplication(ndefAid); err != nil {
		return fmt.Errorf("DESFireTag.WriteData: NDEF application not found: %w", err)
	}

	// Write to standard NDEF file (usually file 2)
	var ndefFileNo byte = 2

	// Write NDEF message length (2 bytes, big-endian) followed by data
	lenBuf := make([]byte, 2)
	binary.BigEndian.PutUint16(lenBuf, uint16(len(data)))

	// Write length
	if _, err := d.tag.WriteData(ndefFileNo, 0, lenBuf); err != nil {
		return fmt.Errorf("DESFireTag.WriteData: error writing NDEF length: %w", err)
	}

	// Write NDEF message
	if len(data) > 0 {
		if _, err := d.tag.WriteData(ndefFileNo, 2, data); err != nil {
			return fmt.Errorf("DESFireTag.WriteData: error writing NDEF message: %w", err)
		}
	}

	log.Printf("DESFireTag.WriteData: successfully wrote %d bytes", len(data))
	return nil
}

// IsWritable checks if the DESFire tag has a writable NDEF application.
func (d *DESFireTag) IsWritable() (bool, error) {
	if err := d.tag.Connect(); err != nil {
		return false, fmt.Errorf("DESFireTag.IsWritable connect error: %w", err)
	}
	defer d.tag.Disconnect()

	// Standard NDEF application ID for DESFire
	ndefAid := freefare.DESFireAid{0x01, 0x00, 0x00}

	// Try to select NDEF application
	if err := d.tag.SelectApplication(ndefAid); err != nil {
		return false, nil // No NDEF application, not writable
	}

	// Try to authenticate with default key (all zeros)
	defaultKey := freefare.NewDESFire3DESKey([16]byte{})
	if err := d.tag.Authenticate(0, *defaultKey); err != nil {
		// Can't authenticate, assume not writable
		return false, nil
	}

	return true, nil
}

// CanMakeReadOnly checks if the DESFire tag can be made read-only.
func (d *DESFireTag) CanMakeReadOnly() (bool, error) {
	// DESFire read-only functionality depends on application-level permissions
	// and is more complex than Classic tags. For now, return false.
	return false, nil
}

// MakeReadOnly makes the DESFire tag read-only.
func (d *DESFireTag) MakeReadOnly() error {
	// DESFire read-only functionality would require changing application-level
	// access rights, which is complex and card-specific. Not implemented yet.
	return NewNotSupportedError("MakeReadOnly")
}
