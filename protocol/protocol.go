// Package protocol provides NFC message types for external tools.
// This package is designed to be importable without pulling in server dependencies.
package protocol

import "time"

// TagInputRequest is the request structure for the POST /api/v1/tag endpoint.
// External tools can use this to inject NFC tag data into the agent.
type TagInputRequest struct {
	// UID is the tag's unique identifier in hex format (e.g., "04:AB:CD:EF:12:34:56")
	// Supports formats: "04:AB:CD:EF", "04ABCDEF", "04 AB CD EF", "04-AB-CD-EF"
	UID string `json:"uid"`

	// Type is the tag type string (e.g., "MIFARE Classic 1K", "Type4", "NTAG215")
	// Optional - defaults to "Unknown"
	Type string `json:"type,omitempty"`

	// Technology is the NFC technology family (e.g., "ISO14443A", "ISO14443B")
	// Optional - will be inferred from Type if not provided
	Technology string `json:"technology,omitempty"`

	// Message contains NDEF message data (optional)
	Message *NDEFMessageInput `json:"message,omitempty"`

	// ScannedAt is the timestamp when the tag was scanned
	// Optional - defaults to current server time
	ScannedAt *time.Time `json:"scannedAt,omitempty"`

	// Source identifies where this tag data came from (e.g., "http-api", "manual-tool")
	// Optional - defaults to "http-api"
	Source string `json:"source,omitempty"`
}

// TagInputResponse is the response structure for the POST /api/v1/tag endpoint.
type TagInputResponse struct {
	Success   bool   `json:"success"`
	Message   string `json:"message,omitempty"`
	Error     string `json:"error,omitempty"`
	ErrorCode string `json:"errorCode,omitempty"`
	UID       string `json:"uid,omitempty"` // Echo back the normalized UID
}

// Error codes for TagInputResponse
const (
	ErrCodeInvalidUID     = "INVALID_UID"
	ErrCodeInvalidNDEF    = "INVALID_NDEF"
	ErrCodeInvalidRequest = "INVALID_REQUEST"
	ErrCodeInternalError  = "INTERNAL_ERROR"
)
