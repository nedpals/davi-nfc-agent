package protocol

// NDEFMessageInput represents an NDEF message for input.
type NDEFMessageInput struct {
	Records []NDEFRecordInput `json:"records"`
}

// NDEFRecordInput represents a single NDEF record for input.
// Supports both high-level (type+content) and low-level (TNF+payload) formats.
type NDEFRecordInput struct {
	// High-level format (preferred for simple records)
	RecordType string `json:"recordType,omitempty"` // "text", "uri", "mime", "external"
	Content    string `json:"content,omitempty"`    // Text content or URI
	Language   string `json:"language,omitempty"`   // Language code for text (default: "en")
	MimeType   string `json:"mimeType,omitempty"`   // MIME type for mime records

	// Low-level format (for advanced use cases)
	TNF     *uint8 `json:"tnf,omitempty"`     // Type Name Format (0x00-0x07)
	Type    []byte `json:"type,omitempty"`    // NDEF record type bytes
	ID      []byte `json:"id,omitempty"`      // Optional record ID
	Payload []byte `json:"payload,omitempty"` // Raw payload bytes (base64 in JSON)
}

// NDEFRecordPayload is the JSON-friendly representation of an NDEF record.
// Used in WebSocket broadcasts and API responses.
type NDEFRecordPayload struct {
	Type     string `json:"type"`              // "text", "uri", etc.
	Content  string `json:"content,omitempty"` // Decoded content
	Language string `json:"language,omitempty"`
	TNF      uint8  `json:"tnf"`
	ID       string `json:"id,omitempty"`
	Payload  []byte `json:"payload"`
}

// NDEFMessagePayload is the JSON-friendly representation of an NDEF message.
type NDEFMessagePayload struct {
	Type    string              `json:"type"` // "ndef"
	Records []NDEFRecordPayload `json:"records"`
}
