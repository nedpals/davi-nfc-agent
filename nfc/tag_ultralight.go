package nfc

import (
	"fmt"
	"log"

	"github.com/clausecker/freefare"
)

// ultralightAdapter implements UltralightTag for MIFARE Ultralight tags.
type ultralightAdapter struct {
	tag freefare.UltralightTag
}

// Ensure ultralightAdapter implements UltralightTag
var _ UltralightTag = (*ultralightAdapter)(nil)

// newUltralightAdapter creates a new adapter for a MIFARE Ultralight tag.
func newUltralightAdapter(tag freefare.UltralightTag) *ultralightAdapter {
	return &ultralightAdapter{tag: tag}
}

func (u *ultralightAdapter) UID() string {
	return u.tag.UID()
}

func (u *ultralightAdapter) Type() string {
	switch u.tag.Type() {
	case freefare.Ultralight:
		return "MIFARE Ultralight"
	case freefare.UltralightC:
		return "MIFARE Ultralight C"
	default:
		return fmt.Sprintf("MIFARE Ultralight (type %d)", u.tag.Type())
	}
}

func (u *ultralightAdapter) NumericType() int {
	return int(u.tag.Type())
}

func (u *ultralightAdapter) GetFreefareTag() freefare.Tag {
	return u.tag
}

func (u *ultralightAdapter) Connect() error {
	return u.tag.Connect()
}

func (u *ultralightAdapter) Disconnect() error {
	return u.tag.Disconnect()
}

func (u *ultralightAdapter) Transceive(data []byte) ([]byte, error) {
	return nil, fmt.Errorf("Transceive not directly supported for ultralightAdapter; use ReadPage/WritePage")
}

// ReadPage reads a 4-byte page from the Ultralight tag.
func (u *ultralightAdapter) ReadPage(page byte) ([4]byte, error) {
	if err := u.tag.Connect(); err != nil {
		return [4]byte{}, fmt.Errorf("ultralightAdapter.ReadPage connect error: %w", err)
	}
	defer u.tag.Disconnect()

	data, err := u.tag.ReadPage(page)
	if err != nil {
		return [4]byte{}, fmt.Errorf("ultralightAdapter.ReadPage error: %w", err)
	}
	return data, nil
}

// WritePage writes a 4-byte page to the Ultralight tag.
func (u *ultralightAdapter) WritePage(page byte, data [4]byte) error {
	if err := u.tag.Connect(); err != nil {
		return fmt.Errorf("ultralightAdapter.WritePage connect error: %w", err)
	}
	defer u.tag.Disconnect()

	if err := u.tag.WritePage(page, data); err != nil {
		return fmt.Errorf("ultralightAdapter.WritePage error: %w", err)
	}
	return nil
}

// ReadData reads NDEF data from the Ultralight tag.
func (u *ultralightAdapter) ReadData() ([]byte, error) {
	if err := u.tag.Connect(); err != nil {
		return nil, fmt.Errorf("ultralightAdapter.ReadData connect error: %w", err)
	}
	defer u.tag.Disconnect()

	// Ultralight NDEF starts at page 4 (pages 0-3 are header/config)
	// Page 4 contains the NDEF TLV
	startPage := byte(4)
	maxPages := byte(16) // Ultralight has 16 pages (64 bytes total)

	// For Ultralight C, there are more pages
	if u.tag.Type() == freefare.UltralightC {
		maxPages = 48 // Ultralight C has 48 pages (192 bytes)
	}

	var allData []byte
	for page := startPage; page < maxPages; page++ {
		pageData, err := u.tag.ReadPage(page)
		if err != nil {
			log.Printf("ultralightAdapter.ReadData: error reading page %d: %v", page, err)
			break
		}
		allData = append(allData, pageData[:]...)
	}

	if len(allData) == 0 {
		return nil, nil
	}

	// Parse TLV structure (same as Classic)
	offset := 0
	for offset < len(allData) {
		if offset+1 > len(allData) {
			return nil, fmt.Errorf("TLV structure error at offset %d (type missing)", offset)
		}
		tlvType := allData[offset]

		if tlvType == 0x00 {
			offset++
			continue
		}
		if tlvType == 0xFE {
			break
		}

		lenFieldStart := offset + 1
		if lenFieldStart >= len(allData) {
			return nil, fmt.Errorf("TLV type 0x%X at offset %d: length field missing", tlvType, offset)
		}

		var msgLength int
		var lengthFieldSize int

		if allData[lenFieldStart] == 0xFF {
			if lenFieldStart+2 >= len(allData) {
				return nil, fmt.Errorf("TLV type 0x%X at offset %d: long format length bytes missing", tlvType, offset)
			}
			msgLength = int(allData[lenFieldStart+1])<<8 | int(allData[lenFieldStart+2])
			lengthFieldSize = 3
		} else {
			msgLength = int(allData[lenFieldStart])
			lengthFieldSize = 1
		}

		valueStart := lenFieldStart + lengthFieldSize
		if valueStart+msgLength > len(allData) {
			return nil, fmt.Errorf("TLV type 0x%X at offset %d: value (len %d) exceeds buffer bounds", tlvType, offset, msgLength)
		}

		message := allData[valueStart : valueStart+msgLength]

		if tlvType == 0x03 {
			return message, nil
		}
		offset = valueStart + msgLength
	}

	log.Println("ultralightAdapter.ReadData: No NDEF Message TLV (type 0x03) found.")
	return nil, nil
}

// WriteData writes NDEF data to the Ultralight tag.
func (u *ultralightAdapter) WriteData(data []byte) error {
	if err := u.tag.Connect(); err != nil {
		return fmt.Errorf("ultralightAdapter.WriteData connect error: %w", err)
	}
	defer u.tag.Disconnect()

	// Build TLV structure
	var tlvPayload []byte
	ndefMsgLen := len(data)

	tlvPayload = append(tlvPayload, 0x03) // NDEF Message TLV
	if ndefMsgLen < 255 {
		tlvPayload = append(tlvPayload, byte(ndefMsgLen))
	} else {
		tlvPayload = append(tlvPayload, 0xFF)
		tlvPayload = append(tlvPayload, byte(ndefMsgLen>>8))
		tlvPayload = append(tlvPayload, byte(ndefMsgLen&0xFF))
	}
	tlvPayload = append(tlvPayload, data...)
	tlvPayload = append(tlvPayload, 0xFE) // Terminator TLV

	// Ultralight NDEF starts at page 4
	startPage := byte(4)
	maxPages := byte(16)

	if u.tag.Type() == freefare.UltralightC {
		maxPages = 48
	}

	// Calculate how many pages we need
	pagesNeeded := (len(tlvPayload) + 3) / 4 // Round up to nearest page

	if startPage+byte(pagesNeeded) > maxPages {
		return fmt.Errorf("ultralightAdapter.WriteData: NDEF message too large (%d bytes, needs %d pages, only %d available)", len(data), pagesNeeded, maxPages-startPage)
	}

	// Write data page by page
	offset := 0
	for page := startPage; offset < len(tlvPayload); page++ {
		var pageData [4]byte
		for i := 0; i < 4 && offset < len(tlvPayload); i++ {
			pageData[i] = tlvPayload[offset]
			offset++
		}

		if err := u.tag.WritePage(page, pageData); err != nil {
			return fmt.Errorf("ultralightAdapter.WriteData: error writing page %d: %w", page, err)
		}
		log.Printf("ultralightAdapter.WriteData: Wrote page %d", page)
	}

	log.Printf("ultralightAdapter.WriteData: Successfully wrote %d bytes", len(data))
	return nil
}

// IsWritable checks if the Ultralight tag is writable.
func (u *ultralightAdapter) IsWritable() (bool, error) {
	if err := u.tag.Connect(); err != nil {
		return false, fmt.Errorf("ultralightAdapter.IsWritable connect error: %w", err)
	}
	defer u.tag.Disconnect()

	// Try to read page 4 to check if tag is accessible
	_, err := u.tag.ReadPage(4)
	if err != nil {
		return false, nil
	}

	// Ultralight tags are generally writable unless locked
	// We could check the lock bytes (page 2, bytes 2-3) but for now assume writable
	return true, nil
}

// CanMakeReadOnly checks if the Ultralight tag can be made read-only.
func (u *ultralightAdapter) CanMakeReadOnly() (bool, error) {
	// Ultralight tags have lock bits that can permanently lock pages
	// Check if they're not already locked
	if err := u.tag.Connect(); err != nil {
		return false, fmt.Errorf("ultralightAdapter.CanMakeReadOnly connect error: %w", err)
	}
	defer u.tag.Disconnect()

	// Read lock bytes from page 2
	lockPage, err := u.tag.ReadPage(2)
	if err != nil {
		return false, fmt.Errorf("ultralightAdapter.CanMakeReadOnly: error reading lock bytes: %w", err)
	}

	// Check if lock bits are already set
	// Bytes 2-3 of page 2 contain lock bits
	if lockPage[2] == 0xFF && lockPage[3] == 0xFF {
		return false, nil // Already locked
	}

	return true, nil
}

// MakeReadOnly makes the Ultralight tag read-only by setting lock bits.
func (u *ultralightAdapter) MakeReadOnly() error {
	if err := u.tag.Connect(); err != nil {
		return fmt.Errorf("ultralightAdapter.MakeReadOnly connect error: %w", err)
	}
	defer u.tag.Disconnect()

	// Set lock bits in page 2
	// WARNING: This is permanent and cannot be undone!
	lockData := [4]byte{0x00, 0x00, 0xFF, 0xFF}

	// Read current page 2 data first to preserve other bytes
	currentPage, err := u.tag.ReadPage(2)
	if err != nil {
		return fmt.Errorf("ultralightAdapter.MakeReadOnly: error reading page 2: %w", err)
	}

	// Preserve first 2 bytes, set lock bits in bytes 2-3
	lockData[0] = currentPage[0]
	lockData[1] = currentPage[1]

	if err := u.tag.WritePage(2, lockData); err != nil {
		return fmt.Errorf("ultralightAdapter.MakeReadOnly: error writing lock bits: %w", err)
	}

	log.Println("ultralightAdapter.MakeReadOnly: Tag locked to read-only mode")
	return nil
}
