package nfc

import "fmt"

// Message represents data that can be written to/read from a card.
// Different implementations handle different encoding schemes.
type Message interface {
	// Encode converts the message to bytes for writing to card
	Encode() ([]byte, error)

	// Type returns the message type for debugging
	Type() string
}

// TextMessage represents raw bytes from cards that don't support NDEF.
// This is a fallback message type that stores both the raw data and decoded text.
type TextMessage struct {
	Data []byte // Raw bytes from the card
	Text string // Decoded text representation
}

// NewTextMessage creates a new text message from raw bytes.
// It automatically decodes the bytes to a string.
func NewTextMessage(data []byte) *TextMessage {
	return &TextMessage{
		Data: data,
		Text: string(data),
	}
}

// NewTextMessageFromString creates a new text message from a string.
func NewTextMessageFromString(text string) *TextMessage {
	return &TextMessage{
		Data: []byte(text),
		Text: text,
	}
}

// Encode returns the raw bytes as-is (no encoding).
func (t *TextMessage) Encode() ([]byte, error) {
	return t.Data, nil
}

// Type returns "raw" for debugging.
func (t *TextMessage) Type() string {
	return "raw"
}

// String returns the decoded text.
func (t *TextMessage) String() string {
	return t.Text
}

// Bytes returns the raw bytes.
func (t *TextMessage) Bytes() []byte {
	return t.Data
}

// NDEFMessage represents a structured NDEF message with multiple records.
// This allows complex messages with multiple record types (text, URI, MIME, etc.)
type NDEFMessage struct {
	records []NDEFRecord
}

// NDEFRecord represents a single NDEF record within a message.
type NDEFRecord struct {
	TNF     byte   // Type Name Format (0x00-0x07)
	Type    []byte // Record type (e.g., "T" for text, "U" for URI)
	ID      []byte // Optional record ID
	Payload []byte // Record payload data
}

// GetText extracts text from a Text Record (TNF=0x01, Type='T').
// Returns (text, true) if this is a text record, or ("", false) otherwise.
func (r *NDEFRecord) GetText() (string, bool) {
	if !r.IsTextRecord() {
		return "", false
	}
	text, err := parseTextRecordPayload(r.Payload)
	if err != nil {
		return "", false
	}
	return text, true
}

// GetURI extracts URI from a URI Record (TNF=0x01, Type='U').
// Returns (uri, true) if this is a URI record, or ("", false) otherwise.
func (r *NDEFRecord) GetURI() (string, bool) {
	if !r.IsURIRecord() {
		return "", false
	}
	uri, err := parseURIRecordPayload(r.Payload)
	if err != nil {
		return "", false
	}
	return uri, true
}

// IsTextRecord returns true if this is a Text Record.
func (r *NDEFRecord) IsTextRecord() bool {
	return r.TNF == 0x01 && len(r.Type) == 1 && r.Type[0] == 'T'
}

// IsURIRecord returns true if this is a URI Record.
func (r *NDEFRecord) IsURIRecord() bool {
	return r.TNF == 0x01 && len(r.Type) == 1 && r.Type[0] == 'U'
}

// NewNDEFMessage creates a new empty NDEF message.
func NewNDEFMessage() *NDEFMessage {
	return &NDEFMessage{records: []NDEFRecord{}}
}

// AddRecord adds a raw NDEF record to the message.
func (m *NDEFMessage) AddRecord(record NDEFRecord) *NDEFMessage {
	m.records = append(m.records, record)
	return m
}

// AddText adds an NDEF Text Record to the message.
func (m *NDEFMessage) AddText(text, langCode string) *NDEFMessage {
	if langCode == "" {
		langCode = "en"
	}
	payload := MakeTextRecordPayload(text, langCode)
	m.records = append(m.records, NDEFRecord{
		TNF:     0x01, // Well Known
		Type:    []byte("T"),
		Payload: payload,
	})
	return m
}

// AddURI adds an NDEF URI Record to the message.
func (m *NDEFMessage) AddURI(uri string) *NDEFMessage {
	payload := MakeURIRecordPayload(uri)
	m.records = append(m.records, NDEFRecord{
		TNF:     0x01, // Well Known
		Type:    []byte("U"),
		Payload: payload,
	})
	return m
}

// Encode converts the NDEF message to bytes.
func (m *NDEFMessage) Encode() ([]byte, error) {
	if len(m.records) == 0 {
		return nil, fmt.Errorf("cannot encode empty NDEF message")
	}
	return encodeNDEFRecords(m.records)
}

// Type returns "ndef" for debugging.
func (m *NDEFMessage) Type() string {
	return "ndef"
}

// Records returns the list of NDEF records in this message.
func (m *NDEFMessage) Records() []NDEFRecord {
	return m.records
}

// GetText returns the text content from the first Text Record in the message.
func (m *NDEFMessage) GetText() (string, error) {
	for _, r := range m.records {
		if text, ok := r.GetText(); ok {
			return text, nil
		}
	}
	return "", fmt.Errorf("no text record found in NDEF message")
}

// GetURI returns the URI from the first URI Record in the message.
func (m *NDEFMessage) GetURI() (string, error) {
	for _, r := range m.records {
		if uri, ok := r.GetURI(); ok {
			return uri, nil
		}
	}
	return "", fmt.Errorf("no URI record found in NDEF message")
}

// DecodeNDEF parses raw bytes into an NDEFMessage.
// Returns error if the data is not valid NDEF format.
func DecodeNDEF(data []byte) (*NDEFMessage, error) {
	records, err := parseNDEFRecords(data)
	if err != nil {
		return nil, err
	}
	return &NDEFMessage{records: records}, nil
}

// DecodeText creates a TextMessage from raw bytes (no parsing).
// This is used for cards that don't support NDEF.
func DecodeText(data []byte) *TextMessage {
	return NewTextMessage(data)
}

// MakeURIRecordPayload creates the payload for an NDEF URI record.
func MakeURIRecordPayload(uri string) []byte {
	// URI record format: [identifier code][URI string]
	// Identifier code 0x00 means no prefix abbreviation
	payload := make([]byte, 1+len(uri))
	payload[0] = 0x00 // No abbreviation
	copy(payload[1:], []byte(uri))
	return payload
}

// parseURIRecordPayload extracts URI from an NDEF URI record payload.
func parseURIRecordPayload(payload []byte) (string, error) {
	if len(payload) < 1 {
		return "", fmt.Errorf("URI record payload too short")
	}

	identifierCode := payload[0]
	uriBytes := payload[1:]

	// Handle URI prefix abbreviations
	var prefix string
	switch identifierCode {
	case 0x00:
		prefix = ""
	case 0x01:
		prefix = "http://www."
	case 0x02:
		prefix = "https://www."
	case 0x03:
		prefix = "http://"
	case 0x04:
		prefix = "https://"
	// Add more as needed
	default:
		prefix = ""
	}

	return prefix + string(uriBytes), nil
}
