package nfc

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// DeviceCapabilities defines the capabilities of a smartphone NFC device.
type DeviceCapabilities struct {
	CanRead  bool   `json:"canRead"`
	CanWrite bool   `json:"canWrite"`
	NFCType  string `json:"nfcType"` // "nfca", "nfcb", "nfcf", "nfcv", "isodep", etc.
}

// DeviceRegistrationRequest is sent by mobile app to register as an NFC device.
type DeviceRegistrationRequest struct {
	DeviceName   string                 `json:"deviceName"`   // e.g., "John's iPhone 12"
	Platform     string                 `json:"platform"`     // "ios" or "android"
	AppVersion   string                 `json:"appVersion"`   // e.g., "1.0.0"
	Capabilities DeviceCapabilities     `json:"capabilities"` // Device capabilities
	Metadata     map[string]string      `json:"metadata"`     // Optional metadata
}

// DeviceRegistrationResponse is sent by server after successful registration.
type DeviceRegistrationResponse struct {
	DeviceID     string     `json:"deviceID"`     // Unique device identifier (UUID)
	SessionToken string     `json:"sessionToken"` // Authentication token (optional future use)
	ServerInfo   ServerInfo `json:"serverInfo"`
}

// ServerInfo contains information about the server.
type ServerInfo struct {
	Version      string   `json:"version"`
	SupportedNFC []string `json:"supportedNFC"` // ["mifare", "desfire", etc.]
}

// SmartphoneTagData is sent by mobile app when a tag is scanned.
type SmartphoneTagData struct {
	DeviceID    string           `json:"deviceID"`    // Device that scanned the tag
	UID         string           `json:"uid"`         // Tag UID (hex format)
	Technology  string           `json:"technology"`  // "ISO14443A", "ISO14443B", etc.
	Type        string           `json:"type"`        // "MIFARE Classic 1K", "Type4", etc.
	ATR         string           `json:"atr"`         // Answer to Reset (if applicable)
	ScannedAt   time.Time        `json:"scannedAt"`   // Timestamp of scan
	NDEFMessage *NDEFMessageData `json:"ndefMessage"` // Parsed NDEF data (if available)
	RawData     []byte           `json:"rawData"`     // Raw tag data (base64 encoded)
}

// NDEFMessageData represents NDEF message from mobile app.
type NDEFMessageData struct {
	Records []NDEFRecordData `json:"records"`
}

// NDEFRecordData represents a single NDEF record from mobile app.
type NDEFRecordData struct {
	TNF        uint8  `json:"tnf"`        // Type Name Format
	Type       []byte `json:"type"`       // Record type
	ID         []byte `json:"id"`         // Record ID (optional)
	Payload    []byte `json:"payload"`    // Record payload
	RecordType string `json:"recordType"` // "text", "uri", "mime", etc.
	Content    string `json:"content"`    // Decoded content (text/URI)
	Language   string `json:"language"`   // For text records
}

// DeviceHeartbeat is sent by mobile app periodically.
type DeviceHeartbeat struct {
	DeviceID  string    `json:"deviceID"`
	Timestamp time.Time `json:"timestamp"`
}

// DeviceWriteRequest is sent by server to mobile app (future feature).
type DeviceWriteRequest struct {
	RequestID   string           `json:"requestID"`   // Unique request ID for correlation
	DeviceID    string           `json:"deviceID"`    // Target device
	NDEFMessage *NDEFMessageData `json:"ndefMessage"` // Data to write
	Options     WriteOptions     `json:"options"`     // Write options
}

// DeviceWriteResponse is sent by mobile app to server (future feature).
type DeviceWriteResponse struct {
	RequestID string `json:"requestID"`
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
}

// ConvertSmartphoneTagData converts mobile app tag data to internal Tag.
func ConvertSmartphoneTagData(data SmartphoneTagData) (Tag, error) {
	// Validate required fields
	if data.UID == "" {
		return nil, fmt.Errorf("tag UID is required")
	}
	if data.Technology == "" {
		return nil, fmt.Errorf("tag technology is required")
	}

	// Normalize UID format
	uid, err := parseSmartphoneUID(data.UID)
	if err != nil {
		return nil, fmt.Errorf("invalid UID format: %w", err)
	}

	// Parse NDEF message if present
	var ndefMsg *NDEFMessage
	var ndefData []byte
	if data.NDEFMessage != nil {
		ndefMsg, err = ConvertNDEFMessageData(data.NDEFMessage)
		if err != nil {
			return nil, fmt.Errorf("failed to parse NDEF message: %w", err)
		}
		// Encode NDEF message to bytes
		ndefData, err = ndefMsg.Encode()
		if err != nil {
			return nil, fmt.Errorf("failed to encode NDEF message: %w", err)
		}
	}

	// Create SmartphoneTag instance
	tag := &SmartphoneTag{
		uid:          uid,
		tagType:      data.Type,
		technology:   data.Technology,
		ndefData:     ndefData,
		ndefMsg:      ndefMsg,
		rawData:      data.RawData,
		scannedAt:    data.ScannedAt,
		sourceDevice: data.DeviceID,
	}

	return tag, nil
}

// ConvertNDEFMessageData converts mobile app NDEF format to internal NDEFMessage.
func ConvertNDEFMessageData(data *NDEFMessageData) (*NDEFMessage, error) {
	if data == nil || len(data.Records) == 0 {
		return nil, fmt.Errorf("empty NDEF message")
	}

	msg := NewNDEFMessage()
	for i, recordData := range data.Records {
		record, err := ConvertNDEFRecordData(recordData)
		if err != nil {
			return nil, fmt.Errorf("failed to convert record %d: %w", i, err)
		}
		msg.AddRecord(*record)
	}

	return msg, nil
}

// ConvertNDEFRecordData converts single NDEF record from mobile app format.
func ConvertNDEFRecordData(data NDEFRecordData) (*NDEFRecord, error) {
	// Validate TNF
	if data.TNF > 0x07 {
		return nil, fmt.Errorf("invalid TNF value: 0x%02X", data.TNF)
	}

	record := &NDEFRecord{
		TNF:     data.TNF,
		Type:    data.Type,
		ID:      data.ID,
		Payload: data.Payload,
	}

	return record, nil
}

// parseSmartphoneUID parses and normalizes UID from various formats.
// Supports: "04:AB:CD:EF", "04ABCDEF", "04 AB CD EF"
// Returns: normalized colon-separated uppercase hex (e.g., "04:AB:CD:EF")
func parseSmartphoneUID(uid string) (string, error) {
	if uid == "" {
		return "", fmt.Errorf("empty UID")
	}

	// Remove common separators and spaces
	cleaned := strings.ReplaceAll(uid, ":", "")
	cleaned = strings.ReplaceAll(cleaned, " ", "")
	cleaned = strings.ReplaceAll(cleaned, "-", "")
	cleaned = strings.ToUpper(cleaned)

	// Validate hex characters
	validHex := regexp.MustCompile(`^[0-9A-F]+$`)
	if !validHex.MatchString(cleaned) {
		return "", fmt.Errorf("UID contains invalid characters: %s", uid)
	}

	// UID length should be even (each byte = 2 hex chars)
	if len(cleaned)%2 != 0 {
		return "", fmt.Errorf("UID has odd number of hex characters: %s", uid)
	}

	// Typical NFC UID lengths: 4, 7, or 10 bytes (8, 14, or 20 hex chars)
	// But we'll accept any even length
	if len(cleaned) < 2 {
		return "", fmt.Errorf("UID too short: %s", uid)
	}

	// Format as colon-separated pairs
	var result strings.Builder
	for i := 0; i < len(cleaned); i += 2 {
		if i > 0 {
			result.WriteByte(':')
		}
		result.WriteString(cleaned[i : i+2])
	}

	return result.String(), nil
}
