package nfc

import (
	"encoding/binary"
	"fmt"
	"strings"
	"unicode/utf16"
)

// ParseNdefMessageForTextRecord parses an NDEF message and returns the text from the first Text Record.
// ndefMessage is the byte array representing one or more NDEF records.
func ParseNdefMessageForTextRecord(ndefMessage []byte) (string, error) {
	if len(ndefMessage) == 0 {
		return "", nil // No message, no text
	}

	offset := 0
	for offset < len(ndefMessage) {
		if offset+1 > len(ndefMessage) { // Need at least header byte
			return "", fmt.Errorf("invalid NDEF message: truncated record header at offset %d", offset)
		}
		header := ndefMessage[offset]
		// MB := (header & 0x80) != 0 // Message Begin
		// ME := (header & 0x40) != 0 // Message End
		// CF := (header & 0x20) != 0 // Chunk Flag
		SR := (header & 0x10) != 0 // Short Record
		IL := (header & 0x08) != 0 // ID Length Present
		TNF := header & 0x07       // Type Name Format

		currentPos := offset + 1 // Current position after header byte

		if currentPos+1 > len(ndefMessage) { // Need type length byte
			return "", fmt.Errorf("invalid NDEF message: truncated type length at offset %d", currentPos-1)
		}
		typeLength := int(ndefMessage[currentPos])
		currentPos++

		var payloadLength int
		if SR {
			if currentPos+1 > len(ndefMessage) { // Need payload length byte (1 byte for SR)
				return "", fmt.Errorf("invalid NDEF message: truncated short record payload length at offset %d", currentPos-1)
			}
			payloadLength = int(ndefMessage[currentPos])
			currentPos++
		} else { // Non-short record, payload length is 4 bytes
			if currentPos+4 > len(ndefMessage) { // Need payload length bytes (4 bytes for non-SR)
				return "", fmt.Errorf("invalid NDEF message: truncated non-short record payload length at offset %d", currentPos-1)
			}
			payloadLength = int(binary.BigEndian.Uint32(ndefMessage[currentPos : currentPos+4]))
			currentPos += 4
		}

		var idLength int
		if IL {
			if currentPos+1 > len(ndefMessage) { // Need ID length byte
				return "", fmt.Errorf("invalid NDEF message: truncated ID length at offset %d", currentPos-1)
			}
			idLength = int(ndefMessage[currentPos])
			currentPos++
		}

		// Check bounds for type, ID, and payload
		if currentPos+typeLength > len(ndefMessage) {
			return "", fmt.Errorf("invalid NDEF message: truncated type field at offset %d", currentPos-1)
		}
		recordType := ndefMessage[currentPos : currentPos+typeLength]
		currentPos += typeLength

		if currentPos+idLength > len(ndefMessage) {
			return "", fmt.Errorf("invalid NDEF message: truncated ID field at offset %d", currentPos-1)
		}
		// recordID := ndefMessage[currentPos : currentPos+idLength] // We don't use ID for now
		currentPos += idLength

		if currentPos+payloadLength > len(ndefMessage) {
			return "", fmt.Errorf("invalid NDEF message: truncated payload at offset %d", currentPos-1)
		}
		recordPayload := ndefMessage[currentPos : currentPos+payloadLength]

		// Check if it's a Text Record (TNF Well Known, Type 'T')
		if TNF == 0x01 && typeLength == 1 && recordType[0] == 'T' {
			return parseTextRecordPayload(recordPayload)
		}

		offset = currentPos + payloadLength
	}
	return "", nil // No text record found or text was empty
}

// EncodeNdefMessageWithTextRecord creates an NDEF message containing a single Text Record.
func EncodeNdefMessageWithTextRecord(text string, langCodeStr string) []byte {
	textRecordPayload := makeTextRecordPayload(text, langCodeStr)

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

func makeTextRecordPayload(text string, langCodeStr string) []byte {
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
