package nfc

import (
	"encoding/binary"
	"fmt"
	"log"

	"github.com/clausecker/nfc/v2"
)

// APDU Command constants
const (
	claISO7816      byte = 0x00
	insSelectFile   byte = 0xA4
	insReadBinary   byte = 0xB0
	insUpdateBinary byte = 0xD6
)

// P1 parameters for SELECT FILE
const (
	p1SelectByID     byte = 0x00 // Select by File ID (FID)
	p1SelectByDFName byte = 0x04 // Select by DF Name (AID)
)

// P2 parameters for SELECT FILE
const (
	p2SelectFirstOrOnly byte = 0x00
	p2SelectNoData      byte = 0x0C // No response data expected
)

// NDEF Application ID (AID)
var aidNDEF = []byte{0xD2, 0x76, 0x00, 0x00, 0x85, 0x01, 0x01}

// File IDs (FID)
var fidCC = []byte{0xE1, 0x03}   // Capability Container FID
var fidNDEF = []byte{0xE1, 0x04} // Default NDEF File FID (can be different, read from CC)

// Expected SW1SW2 success status
var sw1sw2Success = []byte{0x90, 0x00}

// Helper function to check APDU response status (SW1SW2)
func checkSW1SW2(response []byte, expected []byte) error {
	if len(response) < 2 {
		return fmt.Errorf("APDU response too short: %x", response)
	}
	sw1 := response[len(response)-2]
	sw2 := response[len(response)-1]
	if sw1 == expected[0] && sw2 == expected[1] {
		return nil
	}
	return fmt.Errorf("APDU error: SW1=%02X, SW2=%02X", sw1, sw2)
}

// ISO14443Tag wraps an ISO14443-4 Type A tag with NFC operations.
//
// ISO14443Tag represents NFC Forum Type 4 tags which use ISO7816-4 APDUs
// for communication. These tags are commonly used in contactless payment
// and high-security applications.
//
// Example:
//
//	tags, _ := device.GetTags()
//	for _, tag := range tags {
//	    if iso14443, ok := tag.(*ISO14443Tag); ok {
//	        data, _ := iso14443.ReadData()
//	    }
//	}
type ISO14443Tag struct {
	device      Device // The underlying NFC device, needed for Transceive
	uid         string
	tagType     TagType // Changed from string to TagType
	numericType int
	rawTarget   nfc.Target
	// Add other necessary fields, e.g., connection details, selected application AID
}

// NewISO14443Tag creates a new Type 4 tag wrapper.
// It requires the underlying nfc.Target (which should be an *nfc.ISO14443aTarget)
// and the Device for transceiving commands.
func NewISO14443Tag(target nfc.Target, device Device) *ISO14443Tag {
	uid := "unknown"
	if isoATarget, ok := target.(*nfc.ISO14443aTarget); ok {
		// Correctly access UID from ISO14443aTarget
		// The fields are UID (byte array) and UIDLen (length of UID in UID)
		uidBytes := isoATarget.UID[:isoATarget.UIDLen]
		uid = fmt.Sprintf("%x", uidBytes)
	} else if generalTarget, ok := target.(interface{ UID() string }); ok { // Fallback for other nfc.Target types if they have UID()
		uid = generalTarget.UID()
	}
	return &ISO14443Tag{
		rawTarget:   target,
		device:      device,
		uid:         uid,
		tagType:     TagTypeType4, // This should now work correctly
		numericType: 4,            // Standard numeric type for Type 4
	}
}

// UID returns the Unique Identifier of the tag.
func (i *ISO14443Tag) UID() string {
	return i.uid
}

// Type returns the type of the tag (e.g., "Type4").
func (i *ISO14443Tag) Type() string {
	return string(i.tagType) // Convert TagType to string for the interface method
}

// NumericType returns a numeric representation of the tag type.
func (i *ISO14443Tag) NumericType() int {
	return i.numericType
}

// Connect establishes a connection to the tag.
// For Type 4 tags, this might involve selecting the NDEF application.
// For now, it's a placeholder.
func (i *ISO14443Tag) Connect() error {
	log.Printf("Connecting to Type 4 tag: %s", i.UID())

	// Select NDEF Application (AID D2760000850101)
	// APDU: 00 A4 04 00 Lc <AID> Le (Lc is length of AID, Le is 00 for expected data)
	apduSelectApp := []byte{claISO7816, insSelectFile, p1SelectByDFName, p2SelectFirstOrOnly, byte(len(aidNDEF))}
	apduSelectApp = append(apduSelectApp, aidNDEF...)
	apduSelectApp = append(apduSelectApp, 0x00) // Le = 00, expect data in response (e.g., FCI)

	resp, err := i.Transceive(apduSelectApp)
	if err != nil {
		return fmt.Errorf("failed to select NDEF application during connect: %w", err)
	}
	if err := checkSW1SW2(resp, sw1sw2Success); err != nil {
		// Some tags might return 6A82 (File not found) if NDEF app is not present or not the "default" selected one.
		// Or 6283 (Selected file invalidated) if tag is already in a state where NDEF app cannot be selected.
		// For now, we treat any non-9000 as an error for NDEF app selection.
		return fmt.Errorf("error selecting NDEF application during connect: %w", err)
	}
	log.Println("NDEF application selected successfully during connect.")
	return nil
}

// Disconnect performs any necessary cleanup or session termination with the tag.
// For Type 4 tags, this is often a no-op at this level unless specific session
// management commands were used.
func (i *ISO14443Tag) Disconnect() error {
	log.Printf("Disconnecting from Type 4 tag: %s", i.UID())
	// For many Type 4 tags and NDEF operations, an explicit deselect of the application
	// is not strictly necessary, as the session often ends when the tag is removed
	// from the field or the device connection is closed.
	// However, if a specific application context needs to be cleared, one might
	// re-select the "master file" (usually with an APDU like 00 A4 00 00 00).
	// For now, we'll keep it as a no-op, as it's generally safe.
	return nil
}

// Transceive sends raw APDU commands to the Type 4 tag and receives the response.
func (i *ISO14443Tag) Transceive(txData []byte) ([]byte, error) {
	if i.device == nil {
		return nil, fmt.Errorf("no device available for transceive operation on tag %s", i.uid)
	}
	log.Printf("ISO14443Tag UID %s Transceive (APDU): %x", i.uid, txData)

	// The device.Transceive expects a PCSC compatible transceive.
	// For nfc-go, InitiatorTransceive on the device might be more direct if the target is already selected.
	// However, the Device is abstracted. We assume Device.Transceive handles it.
	// If `i.rawTarget` needs to be passed or used to select, that logic is within the `Device` impl.

	// For Type 4 tags, communication is often packet-based (APDU).
	// The `Device.Transceive` should be capable of handling this.
	// This might involve wrapping/unwrapping if the device interface is at a lower level.
	// For now, assume it directly sends the command to the currently active/selected tag.

	respData, err := i.device.Transceive(txData) // This should ideally work if the device is correctly set up and tag selected
	if err != nil {
		log.Printf("ISO14443Tag UID %s Transceive Error: %v", i.uid, err)
		return nil, err
	}
	log.Printf("ISO14443Tag UID %s Response (APDU): %x", i.uid, respData)
	return respData, nil
}

// ReadData for ISO14443Tag is intended to read NDEF data.
// Implementation will involve APDU commands for NDEF app selection, file selection, and reading.
func (i *ISO14443Tag) ReadData() ([]byte, error) {
	log.Printf("ReadData (NDEF) called on Type 4 tag %s", i.UID())

	// Connect to the tag and select NDEF application
	if err := i.Connect(); err != nil {
		return nil, fmt.Errorf("ReadData: failed to connect to tag: %w", err)
	}
	defer i.Disconnect()

	// 2. Select Capability Container (CC) File (FID E103)
	// APDU: 00 A4 00 0C 02 <FID_CC> (P1=00 select by FID, P2=0C no data expected in response to SELECT)
	// However, to read it next, we might want to select it with P2=00 to make it current.
	// Let's use P1=00 (Select by FID), P2=00 (First or Only)
	apduSelectCC := []byte{claISO7816, insSelectFile, p1SelectByID, p2SelectFirstOrOnly, byte(len(fidCC))}
	apduSelectCC = append(apduSelectCC, fidCC...)
	// No Le, as we are selecting. Some cards might require Le=00 if data is returned (e.g. FCI)
	// For simplicity, let's assume no Le needed or it's handled by Transceive if FCI is returned.
	// Or, more robustly, add Le=00 if FCI is expected.
	// Let's try without Le first. If it fails, we can add Le=00.

	resp, err := i.Transceive(apduSelectCC)
	if err != nil {
		return nil, fmt.Errorf("failed to select CC file: %w", err)
	}
	if err := checkSW1SW2(resp, sw1sw2Success); err != nil {
		return nil, fmt.Errorf("error selecting CC file: %w", err)
	}
	log.Println("CC file selected successfully.")

	// 3. Read CC File
	// APDU: 00 B0 <Offset_MSB> <Offset_LSB> <Le>
	// Read first 15 bytes of CC file (standard size for basic info)
	// Offset 0000, Le = 0F (15 bytes)
	apduReadCC := []byte{claISO7816, insReadBinary, 0x00, 0x00, 0x0F}
	ccData, err := i.Transceive(apduReadCC)
	if err != nil {
		return nil, fmt.Errorf("failed to read CC file: %w", err)
	}
	if err := checkSW1SW2(ccData, sw1sw2Success); err != nil {
		return nil, fmt.Errorf("error reading CC file: %w", err)
	}
	// ccData now contains the CC file content (excluding SW1SW2)
	ccBytes := ccData[:len(ccData)-2]
	log.Printf("CC file data: %x", ccBytes)

	// 4. Parse CC to find NDEF File ID and max NDEF size.
	if len(ccBytes) < 15 {
		return nil, fmt.Errorf("CC file too short (expected at least 15 bytes, got %d)", len(ccBytes))
	}
	// CCLEN (Capability Container Length) - Bytes 0-1
	// Mapping Version (must be >= 2.0 for NDEF app) - Byte 2
	// MLe (Max R-APDU data size) - Bytes 3-4
	// MLc (Max C-APDU data size) - Bytes 5-6
	// NDEF File Control TLV - Byte 7 onwards
	//   Tag: 04 (NDEF File Control TLV)
	//   Length: 06 (Length of value)
	//   Value:
	//     Bytes 0-1: NDEF File ID (e.g., E104)
	//     Bytes 2-3: Max NDEF file size (MaxFileSize)
	//     Byte 4: Read Access (00 = free access)
	//     Byte 5: Write Access (00 = free access, FF = no access)

	mappingVersion := ccBytes[2]
	if mappingVersion < 0x20 { // Version 2.0
		return nil, fmt.Errorf("CC mapping version %02X not supported (must be >= 2.0)", mappingVersion)
	}

	mLe := binary.BigEndian.Uint16(ccBytes[3:5]) // Max R-APDU data size
	if mLe == 0 {                                // If MLe is 0, it means no limit or not specified, use a sensible default.
		log.Println("MLe from CC is 0, using default 253 for chunk size")
		mLe = 253 // Default practical max for Le in a single ReadBinary command if not chunking extended length
	} else if mLe > 253 { // Cap MLe at a practical single APDU Le value if not handling extended length APDUs
		log.Printf("MLe from CC is %d, capping at 253 for non-extended APDU chunk size", mLe)
		mLe = 253
	}

	var ndefFileID []byte
	var maxNdefFileSize uint16
	// Find NDEF File Control TLV (Tag 0x04)
	foundNDEFControlTLV := false
	for i := 7; i < len(ccBytes)-1; { // -1 because TLV needs at least T and L
		tlvTag := ccBytes[i]
		tlvLen := ccBytes[i+1]
		if tlvTag == 0x04 && tlvLen >= 0x06 { // NDEF File Control TLV
			if i+2+int(tlvLen) > len(ccBytes) {
				return nil, fmt.Errorf("NDEF File Control TLV in CC is truncated")
			}
			ndefFileID = ccBytes[i+2 : i+2+2]                                   // Next 2 bytes are NDEF File ID
			maxNdefFileSize = binary.BigEndian.Uint16(ccBytes[i+2+2 : i+2+2+2]) // Next 2 bytes are Max NDEF File Size
			// byte 4 is read access, byte 5 is write access
			foundNDEFControlTLV = true
			log.Printf("Parsed CC: NDEF File ID = %x, Max NDEF Size = %d", ndefFileID, maxNdefFileSize)
			break
		}
		i += 2 + int(tlvLen) // Move to next TLV
	}
	if !foundNDEFControlTLV {
		log.Println("NDEF File Control TLV (0x04) not found in CC. Using default NDEF File ID E104.")
		ndefFileID = fidNDEF // Fallback to default if not found (though spec says it should be there)
		// maxNdefFileSize would be unknown, which is problematic.
		// For now, we'll proceed, but a robust implementation might error here or have a default max size.
		// Let's assume if TLV 0x04 is not there, it's not a T4T NDEF tag.
		return nil, fmt.Errorf("NDEF File Control TLV (Tag 0x04) not found in CC file")
	}

	// 5. Select NDEF File (using FID from CC)
	// APDU: 00 A4 00 0C 02 <NDEF_FID>
	apduSelectNDEF := []byte{claISO7816, insSelectFile, p1SelectByID, p2SelectFirstOrOnly, byte(len(ndefFileID))}
	apduSelectNDEF = append(apduSelectNDEF, ndefFileID...)
	// No Le, similar to CC select.

	resp, err = i.Transceive(apduSelectNDEF)
	if err != nil {
		return nil, fmt.Errorf("failed to select NDEF file (ID %x): %w", ndefFileID, err)
	}
	if err := checkSW1SW2(resp, sw1sw2Success); err != nil {
		return nil, fmt.Errorf("error selecting NDEF file (ID %x): %w", ndefFileID, err)
	}
	log.Printf("NDEF file (ID %x) selected successfully.", ndefFileID)

	// 6. Read NDEF Message Length (NLEN - first 2 bytes of NDEF File)
	// APDU: 00 B0 00 00 02 (Read 2 bytes from offset 0000)
	apduReadNLEN := []byte{claISO7816, insReadBinary, 0x00, 0x00, 0x02}
	nlenData, err := i.Transceive(apduReadNLEN)
	if err != nil {
		return nil, fmt.Errorf("failed to read NLEN: %w", err)
	}
	if err := checkSW1SW2(nlenData, sw1sw2Success); err != nil {
		return nil, fmt.Errorf("error reading NLEN: %w", err)
	}
	if len(nlenData) < 2+2 { // NLEN (2 bytes) + SW1SW2 (2 bytes)
		return nil, fmt.Errorf("NLEN response too short: %x", nlenData)
	}
	actualNdefLength := binary.BigEndian.Uint16(nlenData[0:2])
	log.Printf("Actual NDEF message length (NLEN): %d", actualNdefLength)

	if actualNdefLength == 0 {
		log.Println("NDEF message length is 0. No data to read.")
		return []byte{}, nil // Empty NDEF message
	}

	// Check if NLEN exceeds MaxNdefFileSize (from CC) -2 (for NLEN bytes themselves)
	if maxNdefFileSize > 0 && actualNdefLength > maxNdefFileSize-2 {
		return nil, fmt.Errorf("NLEN (%d) exceeds max NDEF file size (%d - 2)", actualNdefLength, maxNdefFileSize)
	}

	// 7. Read NDEF Message
	// APDU: 00 B0 <Offset_MSB> <Offset_LSB> <Length to read>
	// Offset is 0002 (after NLEN). Length is actualNdefLength.
	// Reading in chunks might be necessary if actualNdefLength > MLe (Max R-APDU data size from CC)
	// For now, assume it fits in one read. MLe is usually around 240-250 bytes.
	// A robust implementation would loop here.

	// We need to read 'actualNdefLength' bytes starting from offset 2.
	// The Le field in ReadBinary can be up to 255 (0xFF). If actualNdefLength > 255, we need to chunk.
	// Let's assume MLe from CC (bytes 3-4) is reliable and use it.
	// MLe = binary.BigEndian.Uint16(ccBytes[3:5])
	// For simplicity in this step, let's assume actualNdefLength <= 253 (so Le can be actualNdefLength)
	// and the response won't exceed typical buffer sizes.
	// A full implementation needs chunking based on MLe.

	/*
		if actualNdefLength > 253 { // Simplified check, real check against MLe
			// TODO: Implement chunked reading
			return nil, fmt.Errorf("NDEF message too long for single read (%d bytes), chunking not yet implemented", actualNdefLength)
		}

		apduReadNDEF := []byte{claISO7816, insReadBinary, 0x00, 0x02, byte(actualNdefLength)} // Offset 2, read NLEN bytes
		ndefMessageData, err := i.Transceive(apduReadNDEF)
		if err != nil {
			return nil, fmt.Errorf("failed to read NDEF message: %w", err)
		}
		if err := checkSW1SW2(ndefMessageData, sw1sw2Success); err != nil {
			return nil, fmt.Errorf("error reading NDEF message: %w", err)
		}

		// ndefMessageData contains the NDEF message (excluding SW1SW2)
		ndefMessageBytes := ndefMessageData[:len(ndefMessageData)-2]
		if len(ndefMessageBytes) != int(actualNdefLength) {
			return nil, fmt.Errorf("read NDEF message length mismatch (expected %d, got %d)", actualNdefLength, len(ndefMessageBytes))
		}
	*/

	// Chunked Read Implementation
	var allNdefBytes []byte
	var currentOffset uint16 = 2 // Start reading after NLEN
	bytesRemaining := actualNdefLength

	for bytesRemaining > 0 {
		bytesToReadThisChunk := bytesRemaining
		if bytesToReadThisChunk > mLe { // MLe already capped at 253 or actual MLe if smaller
			bytesToReadThisChunk = mLe
		}
		if bytesToReadThisChunk > 253 { // Defensive: Le field in non-extended APDU is 1 byte (max 255, but 00 means 256, FF means 255)
			bytesToReadThisChunk = 253 // Max practical value for Le if not using extended Le
		}
		if currentOffset+bytesToReadThisChunk > maxNdefFileSize && maxNdefFileSize > 0 { // Check against NDEF file capacity
			// This case should ideally be caught by NLEN > maxNdefFileSize-2 check earlier
			return nil, fmt.Errorf("read attempt exceeds max NDEF file size. Offset: %d, Read: %d, Max: %d", currentOffset, bytesToReadThisChunk, maxNdefFileSize)
		}

		offsetMSB := byte(currentOffset >> 8)
		offsetLSB := byte(currentOffset & 0xFF)
		leByte := byte(bytesToReadThisChunk) // Le is the number of bytes to read

		apduReadChunk := []byte{claISO7816, insReadBinary, offsetMSB, offsetLSB, leByte}
		log.Printf("Reading chunk: Offset %d (%02X%02X), Length %d (%02X)", currentOffset, offsetMSB, offsetLSB, bytesToReadThisChunk, leByte)

		chunkData, err := i.Transceive(apduReadChunk)
		if err != nil {
			return nil, fmt.Errorf("failed to read NDEF chunk at offset %d: %w", currentOffset, err)
		}
		if err := checkSW1SW2(chunkData, sw1sw2Success); err != nil {
			return nil, fmt.Errorf("error reading NDEF chunk at offset %d: %w", currentOffset, err)
		}

		payload := chunkData[:len(chunkData)-2]
		if len(payload) != int(bytesToReadThisChunk) && bytesToReadThisChunk != 0 { // if bytesToReadThisChunk is 0 (Le=00), card might return up to MLe bytes
			// If Le was 00 (meaning 256 for some interpretations, or max available for others)
			// and card returns less than requested but more than 0, it's still valid data for that chunk.
			// However, we explicitly set Le to bytesToReadThisChunk (<=253).
			// So, we expect exactly that many bytes.
			log.Printf("Warning: NDEF chunk read length mismatch (expected %d, got %d). Payload: %x", bytesToReadThisChunk, len(payload), payload)
			// Some cards might return less than Le if end of file is reached sooner than Le bytes.
			// This should be fine as long as we correctly track bytesRemaining.
		}
		allNdefBytes = append(allNdefBytes, payload...)
		bytesRemaining -= uint16(len(payload)) // Decrement by actual bytes read
		currentOffset += uint16(len(payload))  // Increment offset by actual bytes read

		if len(payload) == 0 && bytesRemaining > 0 {
			// Read 0 bytes but expected more, this indicates an issue or premature end of file.
			return nil, fmt.Errorf("read 0 bytes for NDEF chunk at offset %d but %d bytes still remaining", currentOffset-uint16(len(payload)), bytesRemaining)
		}
	}

	if len(allNdefBytes) != int(actualNdefLength) {
		return nil, fmt.Errorf("final NDEF message length mismatch (expected %d, got %d)", actualNdefLength, len(allNdefBytes))
	}

	log.Printf("Successfully read NDEF message (%d bytes) with chunking.", len(allNdefBytes))
	return allNdefBytes, nil
}

// WriteData for ISO14443Tag is intended to write NDEF data.
// Implementation will involve APDU commands similar to ReadData but using UpdateBinary.
func (i *ISO14443Tag) WriteData(data []byte) error {
	log.Printf("WriteData (NDEF) called on Type 4 tag %s with %d bytes", i.UID(), len(data))

	// 0. Preliminary checks
	ndefDataLen := len(data)
	if ndefDataLen > 0xFFFF-2 { // Max NLEN is 0xFFFF, NDEF file stores NLEN + NDEF_Data
		return fmt.Errorf("NDEF data too large (%d bytes), max supported is %d", ndefDataLen, 0xFFFF-2)
	}

	// Connect to the tag and select NDEF application
	if err := i.Connect(); err != nil {
		return fmt.Errorf("WriteData: failed to connect to tag: %w", err)
	}
	defer i.Disconnect()

	// 2. Select Capability Container (CC) File (FID E103) - Optional but good for checks
	apduSelectCC := []byte{claISO7816, insSelectFile, p1SelectByID, p2SelectFirstOrOnly, byte(len(fidCC))}
	apduSelectCC = append(apduSelectCC, fidCC...)

	resp, err := i.Transceive(apduSelectCC)
	if err != nil {
		return fmt.Errorf("WriteData: failed to select CC file: %w", err)
	}
	if err := checkSW1SW2(resp, sw1sw2Success); err != nil {
		return fmt.Errorf("WriteData: error selecting CC file: %w", err)
	}
	log.Println("WriteData: CC file selected.")

	// 3. Read CC File to check MaxNdefFileSize and WriteAccess
	apduReadCC := []byte{claISO7816, insReadBinary, 0x00, 0x00, 0x0F} // Read 15 bytes
	ccData, err := i.Transceive(apduReadCC)
	if err != nil {
		return fmt.Errorf("WriteData: failed to read CC file: %w", err)
	}
	if err := checkSW1SW2(ccData, sw1sw2Success); err != nil {
		return fmt.Errorf("WriteData: error reading CC file: %w", err)
	}
	ccBytes := ccData[:len(ccData)-2]

	if len(ccBytes) < 15 {
		return fmt.Errorf("WriteData: CC file too short (got %d bytes)", len(ccBytes))
	}
	// Parse CC for NDEF File Control TLV (Tag 0x04)
	var ndefFileID []byte
	var maxNdefFileSize uint16
	var writeAccess byte = 0xFF                  // Default to no access
	mLc := binary.BigEndian.Uint16(ccBytes[5:7]) // MLc (Max C-APDU data size)
	if mLc == 0 {                                // If MLc is 0, it means no limit or not specified, use a sensible default.
		log.Println("MLc from CC is 0, using default 253 for chunk size")
		mLc = 253 // Default practical max for Lc in a single UpdateBinary command if not chunking extended length
	} else if mLc > 253 { // Cap MLc at a practical single APDU Lc value if not handling extended length APDUs
		log.Printf("MLc from CC is %d, capping at 253 for non-extended APDU chunk size", mLc)
		mLc = 253
	}

	foundNDEFControlTLV := false
	for i := 7; i < len(ccBytes)-1; {
		tlvTag := ccBytes[i]
		tlvLen := ccBytes[i+1]
		if tlvTag == 0x04 && tlvLen >= 0x06 {
			if i+2+int(tlvLen) > len(ccBytes) {
				return fmt.Errorf("WriteData: NDEF File Control TLV in CC is truncated")
			}
			ndefFileID = ccBytes[i+2 : i+2+2]
			maxNdefFileSize = binary.BigEndian.Uint16(ccBytes[i+2+2 : i+2+2+2])
			// byte 4 is read access
			writeAccess = ccBytes[i+2+5] // byte 5 is write access
			foundNDEFControlTLV = true
			log.Printf("WriteData Parsed CC: NDEF File ID=%x, MaxSize=%d, WriteAccess=%02X", ndefFileID, maxNdefFileSize, writeAccess)
			break
		}
		i += 2 + int(tlvLen)
	}
	if !foundNDEFControlTLV {
		return fmt.Errorf("WriteData: NDEF File Control TLV (Tag 0x04) not found in CC file")
	}

	if writeAccess == 0xFF {
		return fmt.Errorf("WriteData: NDEF file write access denied by CC (WriteAccess: %02X)", writeAccess)
	}
	if writeAccess != 0x00 { // Other values are proprietary or RFU, 00 means granted
		log.Printf("Warning: NDEF file write access condition in CC is %02X (not 00). Assuming write is allowed.", writeAccess)
	}

	// Check if data length + 2 (for NLEN) exceeds max file size from CC
	if maxNdefFileSize > 0 && (uint16(ndefDataLen)+2) > maxNdefFileSize {
		return fmt.Errorf("WriteData: NDEF data length (%d) + NLEN (2) exceeds MaxNdefFileSize (%d) from CC", ndefDataLen, maxNdefFileSize)
	}

	// 4. Select NDEF File (using FID from CC)
	apduSelectNDEF := []byte{claISO7816, insSelectFile, p1SelectByID, p2SelectFirstOrOnly, byte(len(ndefFileID))}
	apduSelectNDEF = append(apduSelectNDEF, ndefFileID...)

	resp, err = i.Transceive(apduSelectNDEF)
	if err != nil {
		return fmt.Errorf("WriteData: failed to select NDEF file (ID %x): %w", ndefFileID, err)
	}
	if err := checkSW1SW2(resp, sw1sw2Success); err != nil {
		return fmt.Errorf("WriteData: error selecting NDEF file (ID %x): %w", ndefFileID, err)
	}
	log.Printf("WriteData: NDEF file (ID %x) selected.", ndefFileID)

	// 5. Write NDEF Message Length (NLEN) - first 2 bytes of NDEF file set to 0000 (to clear)
	// Then write actual NLEN, then write data.
	// Some Type 4A/B tags require NLEN to be 0 before writing new data.
	// APDU: 00 D6 <Offset_MSB> <Offset_LSB> <Lc> <Data>

	// Step 5a: Write NLEN = 0000 (2 bytes at offset 0000)
	apduWriteEmptyNLEN := []byte{claISO7816, insUpdateBinary, 0x00, 0x00, 0x02, 0x00, 0x00}
	resp, err = i.Transceive(apduWriteEmptyNLEN)
	if err != nil {
		return fmt.Errorf("WriteData: failed to write empty NLEN: %w", err)
	}
	if err := checkSW1SW2(resp, sw1sw2Success); err != nil {
		return fmt.Errorf("WriteData: error writing empty NLEN: %w", err)
	}
	log.Println("WriteData: NLEN field cleared (0x0000).")

	// Step 5b: Write actual NDEF data (if any)
	// This needs chunking if data > MLc (Max C-APDU data size from CC)
	// MLc = binary.BigEndian.Uint16(ccBytes[5:7])
	// For now, assume data fits in one UpdateBinary. Max Lc is 255.
	// A full implementation needs chunking based on MLc.

	if ndefDataLen > 0 {
		/*
			if ndefDataLen > 253 { // Simplified check, real check against mLc
				// TODO: Implement chunked writing for NDEF data
				return fmt.Errorf("WriteData: NDEF data too long for single UpdateBinary (%d bytes), chunking not implemented", ndefDataLen)
			}
			apduWriteNDEFData := []byte{claISO7816, insUpdateBinary, 0x00, 0x02, byte(ndefDataLen)} // Offset 0002 (after NLEN)
			apduWriteNDEFData = append(apduWriteNDEFData, data...)

			resp, err = i.Transceive(apduWriteNDEFData)
			if err != nil {
				return fmt.Errorf("WriteData: failed to write NDEF data: %w", err)
			}
			if err := checkSW1SW2(resp, sw1sw2Success); err != nil {
				return fmt.Errorf("WriteData: error writing NDEF data: %w", err)
			}
			log.Printf("WriteData: NDEF data (%d bytes) written successfully.", ndefDataLen)
		*/

		// Chunked Write Implementation for NDEF data
		var currentOffset uint16 = 2 // Start writing NDEF data after NLEN field
		bytesRemainingToWrite := ndefDataLen
		dataOffset := 0

		for bytesRemainingToWrite > 0 {
			bytesToWriteThisChunk := bytesRemainingToWrite
			if uint16(bytesToWriteThisChunk) > mLc { // Use mLc from CC (already capped at 253 or actual mLc)
				bytesToWriteThisChunk = int(mLc)
			}
			if bytesToWriteThisChunk > 253 { // Defensive: Lc field in non-extended APDU is 1 byte (max 255)
				bytesToWriteThisChunk = 253
			}

			offsetMSB := byte(currentOffset >> 8)
			offsetLSB := byte(currentOffset & 0xFF)
			lcByte := byte(bytesToWriteThisChunk)

			chunkPayload := data[dataOffset : dataOffset+bytesToWriteThisChunk]
			apduWriteChunk := []byte{claISO7816, insUpdateBinary, offsetMSB, offsetLSB, lcByte}
			apduWriteChunk = append(apduWriteChunk, chunkPayload...)

			log.Printf("Writing NDEF data chunk: Offset %d (%02X%02X), Length %d (%02X)", currentOffset, offsetMSB, offsetLSB, bytesToWriteThisChunk, lcByte)

			resp, err = i.Transceive(apduWriteChunk)
			if err != nil {
				return fmt.Errorf("WriteData: failed to write NDEF data chunk at offset %d: %w", currentOffset, err)
			}
			if err := checkSW1SW2(resp, sw1sw2Success); err != nil {
				return fmt.Errorf("WriteData: error writing NDEF data chunk at offset %d: %w", currentOffset, err)
			}

			bytesRemainingToWrite -= bytesToWriteThisChunk
			currentOffset += uint16(bytesToWriteThisChunk)
			dataOffset += bytesToWriteThisChunk
		}
		log.Printf("WriteData: NDEF data (%d bytes) written successfully with chunking.", ndefDataLen)
	}

	// Step 5c: Update NLEN field with actual length of data written
	nlenBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(nlenBytes, uint16(ndefDataLen))
	apduWriteActualNLEN := []byte{claISO7816, insUpdateBinary, 0x00, 0x00, 0x02} // Offset 0000, Lc=2
	apduWriteActualNLEN = append(apduWriteActualNLEN, nlenBytes...)

	resp, err = i.Transceive(apduWriteActualNLEN)
	if err != nil {
		return fmt.Errorf("WriteData: failed to write actual NLEN: %w", err)
	}
	if err := checkSW1SW2(resp, sw1sw2Success); err != nil {
		return fmt.Errorf("WriteData: error writing actual NLEN: %w", err)
	}
	log.Printf("WriteData: Actual NLEN (%d) written successfully.", ndefDataLen)

	return nil
}

func (i *ISO14443Tag) IsWritable() (bool, error) {
	log.Printf("IsWritable called on Type 4 tag %s", i.UID())

	// Connect to the tag and select NDEF application
	if err := i.Connect(); err != nil {
		return false, fmt.Errorf("IsWritable: failed to connect to tag: %w", err)
	}
	defer i.Disconnect()

	// 1. Select Capability Container (CC) File (FID E103)
	apduSelectCC := []byte{claISO7816, insSelectFile, p1SelectByID, p2SelectFirstOrOnly, byte(len(fidCC))}
	apduSelectCC = append(apduSelectCC, fidCC...)

	resp, err := i.Transceive(apduSelectCC)
	if err != nil {
		return false, fmt.Errorf("IsWritable: failed to select CC file: %w", err)
	}
	if err := checkSW1SW2(resp, sw1sw2Success); err != nil {
		return false, fmt.Errorf("IsWritable: error selecting CC file: %w", err)
	}

	// 2. Read CC File
	apduReadCC := []byte{claISO7816, insReadBinary, 0x00, 0x00, 0x0F} // Read 15 bytes
	ccData, err := i.Transceive(apduReadCC)
	if err != nil {
		return false, fmt.Errorf("IsWritable: failed to read CC file: %w", err)
	}
	if err := checkSW1SW2(ccData, sw1sw2Success); err != nil {
		return false, fmt.Errorf("IsWritable: error reading CC file: %w", err)
	}

	ccBytes := ccData[:len(ccData)-2]
	if len(ccBytes) < 15 {
		return false, fmt.Errorf("IsWritable: CC file too short (got %d bytes)", len(ccBytes))
	}

	// 3. Parse CC for NDEF File Control TLV (Tag 0x04) to check WriteAccess
	for i := 7; i < len(ccBytes)-1; {
		tlvTag := ccBytes[i]
		tlvLen := ccBytes[i+1]
		if tlvTag == 0x04 && tlvLen >= 0x06 {
			if i+2+int(tlvLen) > len(ccBytes) {
				return false, fmt.Errorf("IsWritable: NDEF File Control TLV in CC is truncated")
			}
			// byte 5 is write access (0x00 = granted, 0xFF = no access)
			writeAccess := ccBytes[i+2+5]
			log.Printf("IsWritable: WriteAccess byte = %02X", writeAccess)

			if writeAccess == 0xFF {
				return false, nil // No write access
			}
			return true, nil // Write access granted (0x00 or other non-0xFF values)
		}
		i += 2 + int(tlvLen)
	}

	return false, fmt.Errorf("IsWritable: NDEF File Control TLV (Tag 0x04) not found in CC file")
}

func (i *ISO14443Tag) CanMakeReadOnly() (bool, error) {
	log.Printf("CanMakeReadOnly called on Type 4 tag %s", i.UID())

	// Connect to the tag and select NDEF application
	if err := i.Connect(); err != nil {
		return false, fmt.Errorf("CanMakeReadOnly: failed to connect to tag: %w", err)
	}
	defer i.Disconnect()

	// To make a tag read-only, we need to check:
	// 1. Current write access is not already 0xFF (already read-only)
	// 2. We have permission to write to the Capability Container

	// 1. Select Capability Container (CC) File (FID E103)
	apduSelectCC := []byte{claISO7816, insSelectFile, p1SelectByID, p2SelectFirstOrOnly, byte(len(fidCC))}
	apduSelectCC = append(apduSelectCC, fidCC...)

	resp, err := i.Transceive(apduSelectCC)
	if err != nil {
		return false, fmt.Errorf("CanMakeReadOnly: failed to select CC file: %w", err)
	}
	if err := checkSW1SW2(resp, sw1sw2Success); err != nil {
		return false, fmt.Errorf("CanMakeReadOnly: error selecting CC file: %w", err)
	}

	// 2. Read CC File
	apduReadCC := []byte{claISO7816, insReadBinary, 0x00, 0x00, 0x0F} // Read 15 bytes
	ccData, err := i.Transceive(apduReadCC)
	if err != nil {
		return false, fmt.Errorf("CanMakeReadOnly: failed to read CC file: %w", err)
	}
	if err := checkSW1SW2(ccData, sw1sw2Success); err != nil {
		return false, fmt.Errorf("CanMakeReadOnly: error reading CC file: %w", err)
	}

	ccBytes := ccData[:len(ccData)-2]
	if len(ccBytes) < 15 {
		return false, fmt.Errorf("CanMakeReadOnly: CC file too short (got %d bytes)", len(ccBytes))
	}

	// 3. Parse CC for NDEF File Control TLV (Tag 0x04) to check current WriteAccess
	for idx := 7; idx < len(ccBytes)-1; {
		tlvTag := ccBytes[idx]
		tlvLen := ccBytes[idx+1]
		if tlvTag == 0x04 && tlvLen >= 0x06 {
			if idx+2+int(tlvLen) > len(ccBytes) {
				return false, fmt.Errorf("CanMakeReadOnly: NDEF File Control TLV in CC is truncated")
			}
			// byte 5 is write access (0x00 = granted, 0xFF = no access)
			writeAccess := ccBytes[idx+2+5]
			log.Printf("CanMakeReadOnly: WriteAccess byte = %02X", writeAccess)

			if writeAccess == 0xFF {
				// Already read-only, cannot make it more read-only
				return false, nil
			}

			// WriteAccess is not 0xFF, so we can potentially make it read-only
			// Try a test write to the same byte to verify we have write permissions
			// (We won't actually change it, just write the same value back)
			writeAccessOffset := idx + 2 + 5
			offsetMSB := byte(writeAccessOffset >> 8)
			offsetLSB := byte(writeAccessOffset & 0xFF)

			apduTestWrite := []byte{claISO7816, insUpdateBinary, offsetMSB, offsetLSB, 0x01, writeAccess}
			resp, err := i.Transceive(apduTestWrite)
			if err != nil {
				// Write failed, we don't have permission to modify CC
				log.Printf("CanMakeReadOnly: Test write failed: %v", err)
				return false, nil
			}
			if err := checkSW1SW2(resp, sw1sw2Success); err != nil {
				// Write failed, we don't have permission to modify CC
				log.Printf("CanMakeReadOnly: Test write returned error: %v", err)
				return false, nil
			}

			// We successfully wrote (same value back), so we can make it read-only
			return true, nil
		}
		idx += 2 + int(tlvLen)
	}

	return false, fmt.Errorf("CanMakeReadOnly: NDEF File Control TLV (Tag 0x04) not found in CC file")
}

func (i *ISO14443Tag) MakeReadOnly() error {
	log.Printf("MakeReadOnly called on Type 4 tag %s", i.UID())

	// Connect to the tag and select NDEF application
	if err := i.Connect(); err != nil {
		return fmt.Errorf("MakeReadOnly: failed to connect to tag: %w", err)
	}
	defer i.Disconnect()

	// To make the tag read-only, we need to update the WriteAccess byte in the
	// Capability Container's NDEF File Control TLV to 0xFF

	// 1. Select Capability Container (CC) File (FID E103)
	apduSelectCC := []byte{claISO7816, insSelectFile, p1SelectByID, p2SelectFirstOrOnly, byte(len(fidCC))}
	apduSelectCC = append(apduSelectCC, fidCC...)

	resp, err := i.Transceive(apduSelectCC)
	if err != nil {
		return fmt.Errorf("MakeReadOnly: failed to select CC file: %w", err)
	}
	if err := checkSW1SW2(resp, sw1sw2Success); err != nil {
		return fmt.Errorf("MakeReadOnly: error selecting CC file: %w", err)
	}

	// 2. Read CC File
	apduReadCC := []byte{claISO7816, insReadBinary, 0x00, 0x00, 0x0F} // Read 15 bytes
	ccData, err := i.Transceive(apduReadCC)
	if err != nil {
		return fmt.Errorf("MakeReadOnly: failed to read CC file: %w", err)
	}
	if err := checkSW1SW2(ccData, sw1sw2Success); err != nil {
		return fmt.Errorf("MakeReadOnly: error reading CC file: %w", err)
	}

	ccBytes := ccData[:len(ccData)-2]
	if len(ccBytes) < 15 {
		return fmt.Errorf("MakeReadOnly: CC file too short (got %d bytes)", len(ccBytes))
	}

	// 3. Find the WriteAccess byte location in NDEF File Control TLV
	var writeAccessOffset int = -1
	for i := 7; i < len(ccBytes)-1; {
		tlvTag := ccBytes[i]
		tlvLen := ccBytes[i+1]
		if tlvTag == 0x04 && tlvLen >= 0x06 {
			if i+2+int(tlvLen) > len(ccBytes) {
				return fmt.Errorf("MakeReadOnly: NDEF File Control TLV in CC is truncated")
			}
			// byte 5 is write access
			writeAccessOffset = i + 2 + 5
			break
		}
		i += 2 + int(tlvLen)
	}

	if writeAccessOffset == -1 {
		return fmt.Errorf("MakeReadOnly: NDEF File Control TLV (Tag 0x04) not found in CC file")
	}

	// 4. Update the WriteAccess byte to 0xFF (no write access)
	// We need to write just that one byte at the correct offset
	offsetMSB := byte(writeAccessOffset >> 8)
	offsetLSB := byte(writeAccessOffset & 0xFF)

	apduUpdateWriteAccess := []byte{claISO7816, insUpdateBinary, offsetMSB, offsetLSB, 0x01, 0xFF}
	resp, err = i.Transceive(apduUpdateWriteAccess)
	if err != nil {
		return fmt.Errorf("MakeReadOnly: failed to update WriteAccess byte: %w", err)
	}
	if err := checkSW1SW2(resp, sw1sw2Success); err != nil {
		return fmt.Errorf("MakeReadOnly: error updating WriteAccess byte: %w", err)
	}

	log.Printf("MakeReadOnly: Successfully set WriteAccess to 0xFF at offset %d. Tag is now read-only.", writeAccessOffset)
	return nil
}

// TODO: Implement chunked reading in ReadData based on MLe from CC
// TODO: Implement chunked writing in WriteData based on MLc from CC
// TODO: Implement helper functions for APDU construction and parsing SW1SW2 status codes.
// TODO: Implement ReadData and WriteData using Transceive.
// TODO: Integrate with an actual Type 4 library or use the device's Transceive for APDU exchange.
