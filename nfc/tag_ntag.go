package nfc

import (
	"fmt"
	"log"

	"github.com/clausecker/freefare"
)

// NTAG215 memory layout constants
const (
	ntag215TotalPages    = 135 // Total pages (540 bytes)
	ntag215UserStartPage = 4   // First user data page
	ntag215UserEndPage   = 129 // Last user data page (inclusive)
	ntag215UserPages     = 126 // Pages 4-129 (504 bytes user data)
	ntag215ConfigStart   = 130 // Configuration pages start
)

// NtagTag wraps an NTAG21x tag (NTAG213, NTAG215, NTAG216) with NFC operations.
//
// NtagTag provides page-based read/write operations for NTAG tags which are
// detected by freefare as Ultralight tags but have larger memory.
//
// Example:
//
//	tags, _ := device.GetTags()
//	for _, tag := range tags {
//	    if ntag, ok := tag.(*NtagTag); ok {
//	        data, _ := ntag.ReadPage(4)
//	        ntag.WritePage(5, [4]byte{0x01, 0x02, 0x03, 0x04})
//	    }
//	}
type NtagTag struct {
	tag freefare.UltralightTag
}

// NewNtagTag creates a new NTAG tag wrapper.
func NewNtagTag(tag freefare.UltralightTag) *NtagTag {
	return &NtagTag{tag: tag}
}

func (n *NtagTag) UID() string {
	return n.tag.UID()
}

func (n *NtagTag) Type() string {
	return CardTypeNtag215
}

func (n *NtagTag) NumericType() int {
	// Use a distinct numeric type for NTAG215
	// freefare uses 0=Ultralight, 1=UltralightC, so we use 100+ for NTAG variants
	return 100
}

func (n *NtagTag) GetFreefareTag() freefare.Tag {
	return n.tag
}

func (n *NtagTag) Connect() error {
	return n.tag.Connect()
}

func (n *NtagTag) Disconnect() error {
	return n.tag.Disconnect()
}

func (n *NtagTag) Transceive(data []byte) ([]byte, error) {
	return nil, fmt.Errorf("Transceive not directly supported for NtagTag; use ReadPage/WritePage")
}

// ReadPage reads a 4-byte page from the NTAG tag.
func (n *NtagTag) ReadPage(page byte) ([4]byte, error) {
	if err := n.tag.Connect(); err != nil {
		return [4]byte{}, fmt.Errorf("NtagTag.ReadPage connect error: %w", err)
	}
	defer n.tag.Disconnect()

	data, err := n.tag.ReadPage(page)
	if err != nil {
		return [4]byte{}, fmt.Errorf("NtagTag.ReadPage error: %w", err)
	}
	return data, nil
}

// WritePage writes a 4-byte page to the NTAG tag.
func (n *NtagTag) WritePage(page byte, data [4]byte) error {
	if err := n.tag.Connect(); err != nil {
		return fmt.Errorf("NtagTag.WritePage connect error: %w", err)
	}
	defer n.tag.Disconnect()

	if err := n.tag.WritePage(page, data); err != nil {
		return fmt.Errorf("NtagTag.WritePage error: %w", err)
	}
	return nil
}

// ReadData reads NDEF data from the NTAG tag.
func (n *NtagTag) ReadData() ([]byte, error) {
	if err := n.tag.Connect(); err != nil {
		return nil, fmt.Errorf("NtagTag.ReadData connect error: %w", err)
	}
	defer n.tag.Disconnect()

	// NTAG215 NDEF starts at page 4, user data ends at page 129
	var allData []byte
	for page := byte(ntag215UserStartPage); page <= ntag215UserEndPage; page++ {
		pageData, err := n.tag.ReadPage(page)
		if err != nil {
			log.Printf("NtagTag.ReadData: error reading page %d: %v", page, err)
			break
		}
		allData = append(allData, pageData[:]...)
	}

	if len(allData) == 0 {
		return nil, nil
	}

	// Parse TLV structure - skip null TLVs and find NDEF message (type 0x03)
	offset := 0
	for offset < len(allData) {
		tlvType := allData[offset]

		if tlvType == 0x00 {
			// Null TLV, skip
			offset++
			continue
		}
		if tlvType == 0xFE {
			// Terminator
			break
		}

		if offset+1 >= len(allData) {
			return nil, fmt.Errorf("TLV structure error: length field missing")
		}

		fls, fvs := freefare.TLVrecordLength(allData[offset:])
		if offset+1+fls+fvs > len(allData) {
			return nil, fmt.Errorf("TLV structure error: value exceeds buffer")
		}

		if tlvType == 0x03 {
			// NDEF Message TLV - use freefare's decoder
			data, _ := freefare.TLVdecode(allData[offset:])
			return data, nil
		}

		// Skip this TLV and move to next
		offset += 1 + fls + fvs
	}

	log.Println("NtagTag.ReadData: No NDEF Message TLV (type 0x03) found.")
	return nil, nil
}

// WriteData writes NDEF data to the NTAG tag.
func (n *NtagTag) WriteData(data []byte) error {
	if err := n.tag.Connect(); err != nil {
		return fmt.Errorf("NtagTag.WriteData connect error: %w", err)
	}
	defer n.tag.Disconnect()

	// Build TLV structure using freefare's encoder (type 0x03 = NDEF Message)
	tlvPayload := freefare.TLVencode(data, 0x03)
	if tlvPayload == nil {
		return fmt.Errorf("NtagTag.WriteData: NDEF message too large (max 65534 bytes)")
	}

	// Calculate how many pages we need
	pagesNeeded := (len(tlvPayload) + 3) / 4 // Round up to nearest page
	availablePages := ntag215UserPages

	if pagesNeeded > availablePages {
		return fmt.Errorf("NtagTag.WriteData: NDEF message too large (%d bytes, needs %d pages, only %d available)",
			len(data), pagesNeeded, availablePages)
	}

	// Write data page by page
	offset := 0
	for page := byte(ntag215UserStartPage); offset < len(tlvPayload); page++ {
		var pageData [4]byte
		for i := 0; i < 4 && offset < len(tlvPayload); i++ {
			pageData[i] = tlvPayload[offset]
			offset++
		}

		if err := n.tag.WritePage(page, pageData); err != nil {
			return fmt.Errorf("NtagTag.WriteData: error writing page %d: %w", page, err)
		}
		log.Printf("NtagTag.WriteData: Wrote page %d", page)
	}

	log.Printf("NtagTag.WriteData: Successfully wrote %d bytes", len(data))
	return nil
}

// IsWritable checks if the NTAG tag is writable.
func (n *NtagTag) IsWritable() (bool, error) {
	if err := n.tag.Connect(); err != nil {
		return false, fmt.Errorf("NtagTag.IsWritable connect error: %w", err)
	}
	defer n.tag.Disconnect()

	// Try to read page 4 to check if tag is accessible
	_, err := n.tag.ReadPage(ntag215UserStartPage)
	if err != nil {
		return false, nil
	}

	return true, nil
}

// CanMakeReadOnly checks if the NTAG tag can be made read-only.
func (n *NtagTag) CanMakeReadOnly() (bool, error) {
	if err := n.tag.Connect(); err != nil {
		return false, fmt.Errorf("NtagTag.CanMakeReadOnly connect error: %w", err)
	}
	defer n.tag.Disconnect()

	// Read lock bytes from page 2
	lockPage, err := n.tag.ReadPage(2)
	if err != nil {
		return false, fmt.Errorf("NtagTag.CanMakeReadOnly: error reading lock bytes: %w", err)
	}

	// Check if lock bits are already set (bytes 2-3 of page 2)
	if lockPage[2] == 0xFF && lockPage[3] == 0xFF {
		return false, nil // Already locked
	}

	return true, nil
}

// MakeReadOnly makes the NTAG tag read-only by setting lock bits.
func (n *NtagTag) MakeReadOnly() error {
	if err := n.tag.Connect(); err != nil {
		return fmt.Errorf("NtagTag.MakeReadOnly connect error: %w", err)
	}
	defer n.tag.Disconnect()

	// Read current page 2 data first to preserve other bytes
	currentPage, err := n.tag.ReadPage(2)
	if err != nil {
		return fmt.Errorf("NtagTag.MakeReadOnly: error reading page 2: %w", err)
	}

	// Set lock bits in bytes 2-3 (preserve first 2 bytes)
	// WARNING: This is permanent and cannot be undone!
	lockData := [4]byte{currentPage[0], currentPage[1], 0xFF, 0xFF}

	if err := n.tag.WritePage(2, lockData); err != nil {
		return fmt.Errorf("NtagTag.MakeReadOnly: error writing lock bits: %w", err)
	}

	log.Println("NtagTag.MakeReadOnly: Tag locked to read-only mode")
	return nil
}

