package protocol

import "time"

// DeviceCapabilities defines the capabilities of a connected NFC device.
type DeviceCapabilities struct {
	CanRead  bool   `json:"canRead"`
	CanWrite bool   `json:"canWrite"`
	NFCType  string `json:"nfcType"` // "nfca", "nfcb", "nfcf", "nfcv", "isodep", etc.
}

// DeviceRegistrationRequest is sent by a device to register with the server.
type DeviceRegistrationRequest struct {
	DeviceName   string             `json:"deviceName"`   // e.g., "John's iPhone 12"
	Platform     string             `json:"platform"`     // "ios" or "android"
	AppVersion   string             `json:"appVersion"`   // e.g., "1.0.0"
	Capabilities DeviceCapabilities `json:"capabilities"` // Device capabilities
	Metadata     map[string]string  `json:"metadata"`     // Optional metadata
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

// DeviceTagData is sent by a device when a tag is scanned.
type DeviceTagData struct {
	DeviceID    string            `json:"deviceID"`    // Device that scanned the tag
	UID         string            `json:"uid"`         // Tag UID (hex format)
	Technology  string            `json:"technology"`  // "ISO14443A", "ISO14443B", etc.
	Type        string            `json:"type"`        // "MIFARE Classic 1K", "Type4", etc.
	ATR         string            `json:"atr"`         // Answer to Reset (if applicable)
	ScannedAt   time.Time         `json:"scannedAt"`   // Timestamp of scan
	NDEFMessage *NDEFMessageInput `json:"ndefMessage"` // Parsed NDEF data (if available)
	RawData     []byte            `json:"rawData"`     // Raw tag data (base64 encoded)
}

// DeviceHeartbeat is sent by a device periodically.
type DeviceHeartbeat struct {
	DeviceID  string    `json:"deviceID"`
	Timestamp time.Time `json:"timestamp"`
}

// DeviceTagRemovedData is sent by a device when a tag leaves the NFC field.
type DeviceTagRemovedData struct {
	DeviceID  string    `json:"deviceID"`
	UID       string    `json:"uid"`       // UID of the removed tag
	RemovedAt time.Time `json:"removedAt"` // Timestamp of removal
}

// DeviceWriteRequest is sent by server to a device for writing (future feature).
type DeviceWriteRequest struct {
	RequestID   string            `json:"requestID"`   // Unique request ID for correlation
	DeviceID    string            `json:"deviceID"`    // Target device
	NDEFMessage *NDEFMessageInput `json:"ndefMessage"` // Data to write
}

// DeviceWriteResponse is sent by a device after a write operation (future feature).
type DeviceWriteResponse struct {
	RequestID string `json:"requestID"`
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
}
