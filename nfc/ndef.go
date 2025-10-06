package nfc

import (
	"encoding/binary"
	"fmt"
	"strings"
	"unicode/utf16"
)

// ParseNdefMessageForTextRecord parses an NDEF message and returns the text from the first Text Record.
// This is a convenience function that uses the record-based parsing internally.
func ParseNdefMessageForTextRecord(ndefMessage []byte) (string, error) {
	if len(ndefMessage) == 0 {
		return "", nil // No message, no text
	}

	// Use the record-based parser
	records, err := parseNDEFRecords(ndefMessage)
	if err != nil {
		return "", err
	}

	// Find first text record
	for _, record := range records {
		if text, ok := record.GetText(); ok {
			return text, nil
		}
	}

	return "", nil // No text record found
}

// ParseNdefMessageForURIRecord parses an NDEF message and returns the URI from the first URI Record.
// This is a convenience function that uses the record-based parsing internally.
func ParseNdefMessageForURIRecord(ndefMessage []byte) (string, error) {
	if len(ndefMessage) == 0 {
		return "", nil // No message, no URI
	}

	// Use the record-based parser
	records, err := parseNDEFRecords(ndefMessage)
	if err != nil {
		return "", err
	}

	// Find first URI record
	for _, record := range records {
		if uri, ok := record.GetURI(); ok {
			return uri, nil
		}
	}

	return "", nil // No URI record found
}

// EncodeNdefMessageWithTextRecord creates an NDEF message containing a single Text Record.
func EncodeNdefMessageWithTextRecord(text string, langCodeStr string) []byte {
	textRecordPayload := MakeTextRecordPayload(text, langCodeStr)

	payloadLen := len(textRecordPayload)
	isShortRecord := payloadLen <= 255

	header := byte(0x01) // TNF = Well Known
	header |= (1 << 7)   // MB = 1
	header |= (1 << 6)   // ME = 1
	if isShortRecord {
		header |= (1 << 4) // SR = 1
	}

	typeByte := byte('T')
	typeLenByte := byte(1)

	var ndefRecord []byte
	if isShortRecord {
		ndefRecord = make([]byte, 1 /*header*/ +1 /*typeLen*/ +1 /*payloadLen*/ +1 /*type*/ +payloadLen)
		ndefRecord[0] = header
		ndefRecord[1] = typeLenByte
		ndefRecord[2] = byte(payloadLen)
		ndefRecord[3] = typeByte
		copy(ndefRecord[4:], textRecordPayload)
	} else {
		ndefRecord = make([]byte, 1 /*header*/ +1 /*typeLen*/ +4 /*payloadLen*/ +1 /*type*/ +payloadLen)
		ndefRecord[0] = header
		ndefRecord[1] = typeLenByte
		binary.BigEndian.PutUint32(ndefRecord[2:6], uint32(payloadLen))
		ndefRecord[6] = typeByte
		copy(ndefRecord[7:], textRecordPayload)
	}
	return ndefRecord
}

// parseTextRecordPayload extracts text from an NDEF Text Record's payload.
func parseTextRecordPayload(payload []byte) (string, error) {
	if len(payload) < 1 {
		return "", fmt.Errorf("text record payload too short (status byte missing)")
	}
	status := payload[0]
	langLength := int(status & 0x3F)
	isUTF16 := (status & 0x80) != 0

	textDataStart := 1 + langLength
	if textDataStart > len(payload) {
		return "", fmt.Errorf("text record payload too short (language code or text missing)")
	}
	textBytes := payload[textDataStart:]

	if isUTF16 {
		if len(textBytes) == 0 {
			return "", nil
		}
		if len(textBytes)%2 != 0 {
			return "", fmt.Errorf("invalid UTF-16 text length: %d", len(textBytes))
		}
		return decodeUTF16Internal(textBytes), nil
	}
	return string(textBytes), nil
}

func decodeUTF16Internal(b []byte) string {
	if len(b)%2 != 0 || len(b) == 0 {
		return ""
	}
	u16s := make([]uint16, len(b)/2)
	for i := 0; i < len(b)/2; i++ {
		u16s[i] = binary.LittleEndian.Uint16(b[i*2 : (i*2)+2])
	}
	return strings.TrimSpace(string(utf16.Decode(u16s)))
}

// MakeTextRecordPayload creates an NDEF Text Record payload with the specified text and language code.
func MakeTextRecordPayload(text string, langCodeStr string) []byte {
	if langCodeStr == "" {
		langCodeStr = "en"
	}
	langCode := []byte(langCodeStr)
	if len(langCode) > 0x3F {
		langCode = langCode[:0x3F]
	}
	textBytes := []byte(text)
	statusByte := byte(len(langCode)) // Assumes UTF-8
	payload := make([]byte, 1+len(langCode)+len(textBytes))
	payload[0] = statusByte
	copy(payload[1:], langCode)
	copy(payload[1+len(langCode):], textBytes)
	return payload
}

// GetLengthFieldSize returns the size of the TLV length field.
func GetLengthFieldSize(length int) int {
	if length > 0xFF {
		return 3
	}
	return 1
}

// parseNDEFRecords parses raw NDEF message bytes into a slice of NDEFRecord structs.
// This is a more general version of ParseNdefMessageForTextRecord that returns all records.
func parseNDEFRecords(ndefMessage []byte) ([]NDEFRecord, error) {
	if len(ndefMessage) == 0 {
		return nil, fmt.Errorf("empty NDEF message")
	}

	var records []NDEFRecord
	offset := 0

	for offset < len(ndefMessage) {
		if offset+1 > len(ndefMessage) {
			return nil, fmt.Errorf("invalid NDEF message: truncated record header at offset %d", offset)
		}

		header := ndefMessage[offset]
		// MB := (header & 0x80) != 0 // Message Begin
		ME := (header & 0x40) != 0 // Message End
		// CF := (header & 0x20) != 0 // Chunk Flag
		SR := (header & 0x10) != 0 // Short Record
		IL := (header & 0x08) != 0 // ID Length Present
		TNF := header & 0x07       // Type Name Format

		currentPos := offset + 1

		// Read type length
		if currentPos+1 > len(ndefMessage) {
			return nil, fmt.Errorf("invalid NDEF message: truncated type length at offset %d", currentPos-1)
		}
		typeLength := int(ndefMessage[currentPos])
		currentPos++

		// Read payload length
		var payloadLength int
		if SR {
			if currentPos+1 > len(ndefMessage) {
				return nil, fmt.Errorf("invalid NDEF message: truncated short record payload length at offset %d", currentPos-1)
			}
			payloadLength = int(ndefMessage[currentPos])
			currentPos++
		} else {
			if currentPos+4 > len(ndefMessage) {
				return nil, fmt.Errorf("invalid NDEF message: truncated non-short record payload length at offset %d", currentPos-1)
			}
			payloadLength = int(binary.BigEndian.Uint32(ndefMessage[currentPos : currentPos+4]))
			currentPos += 4
		}

		// Read ID length (if present)
		var idLength int
		if IL {
			if currentPos+1 > len(ndefMessage) {
				return nil, fmt.Errorf("invalid NDEF message: truncated ID length at offset %d", currentPos-1)
			}
			idLength = int(ndefMessage[currentPos])
			currentPos++
		}

		// Read type
		if currentPos+typeLength > len(ndefMessage) {
			return nil, fmt.Errorf("invalid NDEF message: truncated type field at offset %d", currentPos-1)
		}
		recordType := make([]byte, typeLength)
		copy(recordType, ndefMessage[currentPos:currentPos+typeLength])
		currentPos += typeLength

		// Read ID
		var recordID []byte
		if IL && idLength > 0 {
			if currentPos+idLength > len(ndefMessage) {
				return nil, fmt.Errorf("invalid NDEF message: truncated ID field at offset %d", currentPos-1)
			}
			recordID = make([]byte, idLength)
			copy(recordID, ndefMessage[currentPos:currentPos+idLength])
			currentPos += idLength
		}

		// Read payload
		if currentPos+payloadLength > len(ndefMessage) {
			return nil, fmt.Errorf("invalid NDEF message: truncated payload at offset %d", currentPos-1)
		}
		recordPayload := make([]byte, payloadLength)
		copy(recordPayload, ndefMessage[currentPos:currentPos+payloadLength])
		currentPos += payloadLength

		// Add record to list
		records = append(records, NDEFRecord{
			TNF:     TNF,
			Type:    recordType,
			ID:      recordID,
			Payload: recordPayload,
		})

		offset = currentPos

		// Stop if this is the last record
		if ME {
			break
		}
	}

	return records, nil
}

// encodeNDEFRecords encodes a slice of NDEFRecord structs into raw NDEF message bytes.
func encodeNDEFRecords(records []NDEFRecord) ([]byte, error) {
	if len(records) == 0 {
		return nil, fmt.Errorf("cannot encode empty record list")
	}

	var result []byte

	for i, record := range records {
		isFirst := i == 0
		isLast := i == len(records)-1

		payloadLen := len(record.Payload)
		typeLen := len(record.Type)
		idLen := len(record.ID)

		isShortRecord := payloadLen <= 255
		hasID := idLen > 0

		// Build header byte
		header := record.TNF & 0x07 // TNF is lower 3 bits
		if isFirst {
			header |= 0x80 // MB = 1 (Message Begin)
		}
		if isLast {
			header |= 0x40 // ME = 1 (Message End)
		}
		if isShortRecord {
			header |= 0x10 // SR = 1 (Short Record)
		}
		if hasID {
			header |= 0x08 // IL = 1 (ID Length present)
		}

		// Calculate total record size
		recordSize := 1 // header
		recordSize += 1 // type length
		if isShortRecord {
			recordSize += 1 // payload length (1 byte)
		} else {
			recordSize += 4 // payload length (4 bytes)
		}
		if hasID {
			recordSize += 1 // id length
		}
		recordSize += typeLen
		recordSize += idLen
		recordSize += payloadLen

		// Allocate buffer for this record
		recordBytes := make([]byte, recordSize)
		pos := 0

		// Write header
		recordBytes[pos] = header
		pos++

		// Write type length
		recordBytes[pos] = byte(typeLen)
		pos++

		// Write payload length
		if isShortRecord {
			recordBytes[pos] = byte(payloadLen)
			pos++
		} else {
			binary.BigEndian.PutUint32(recordBytes[pos:pos+4], uint32(payloadLen))
			pos += 4
		}

		// Write ID length (if present)
		if hasID {
			recordBytes[pos] = byte(idLen)
			pos++
		}

		// Write type
		copy(recordBytes[pos:], record.Type)
		pos += typeLen

		// Write ID (if present)
		if hasID {
			copy(recordBytes[pos:], record.ID)
			pos += idLen
		}

		// Write payload
		copy(recordBytes[pos:], record.Payload)

		// Append to result
		result = append(result, recordBytes...)
	}

	return result, nil
}
