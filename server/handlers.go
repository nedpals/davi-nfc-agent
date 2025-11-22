package server

import (
	"fmt"
	"log"

	"github.com/nedpals/davi-nfc-agent/nfc"
)

// WriteRequest represents a request to write data to an NFC card.
type WriteRequest struct {
	// Text is the text string to write
	Text string `json:"text"`

	// RecordIndex specifies which NDEF record to update (0-based)
	// Required for updating existing records
	RecordIndex *int `json:"recordIndex,omitempty"`

	// RecordType specifies the type of record to update/create
	// Options: "text" (default), "uri"
	RecordType string `json:"recordType,omitempty"`

	// Language code for text records (default: "en")
	Language string `json:"language,omitempty"`

	// Append adds a new record instead of replacing
	// Set to true to safely add records without overwriting
	Append bool `json:"append,omitempty"`

	// Replace replaces the entire NDEF message (destructive)
	// Must be explicitly set to true to overwrite all existing data
	Replace bool `json:"replace,omitempty"`
}

// BuildNDEFMessage builds an NDEF message and write options based on the request.
func BuildNDEFMessage(writeReq WriteRequest) (*nfc.NDEFMessage, nfc.WriteOptions, error) {
	// Determine record type
	recordType := writeReq.RecordType
	if recordType == "" {
		recordType = "text" // default to text
	}

	// Determine language
	language := writeReq.Language
	if language == "" {
		language = "en"
	}

	// Build NDEF record using the new builder API
	var newRecord nfc.NDEFRecordBuilder
	switch recordType {
	case "text":
		newRecord = &nfc.NDEFText{Content: writeReq.Text, Language: language}
	case "uri":
		newRecord = &nfc.NDEFURI{Content: writeReq.Text}
	default:
		return nil, nfc.WriteOptions{}, fmt.Errorf("unsupported record type: %s", recordType)
	}

	// Build message
	builder := &nfc.NDEFMessageBuilder{
		Records: []nfc.NDEFRecordBuilder{newRecord},
	}
	ndefMsg := builder.MustBuild()

	// Determine write options
	var writeOpts nfc.WriteOptions
	if writeReq.Replace {
		log.Printf("WriteRequest: Replacing entire NDEF message (destructive)")
		writeOpts = nfc.WriteOptions{Overwrite: true, Index: -1}
	} else if writeReq.Append {
		log.Printf("WriteRequest: Appending new %s record", recordType)
		writeOpts = nfc.WriteOptions{Overwrite: false, Index: -1}
	} else if writeReq.RecordIndex != nil {
		log.Printf("WriteRequest: Updating record at index %d", *writeReq.RecordIndex)
		writeOpts = nfc.WriteOptions{Overwrite: false, Index: *writeReq.RecordIndex}
	} else {
		log.Printf("WriteRequest: Auto-detecting write mode")
		writeOpts = nfc.WriteOptions{Overwrite: true, Index: -1}
	}

	return ndefMsg, writeOpts, nil
}

// HandleWriteRequest processes a write request and performs the NFC write operation.
func HandleWriteRequest(reader *nfc.NFCReader, writeReq WriteRequest) error {
	// Build NDEF message and write options
	ndefMsg, writeOpts, err := BuildNDEFMessage(writeReq)
	if err != nil {
		return fmt.Errorf("failed to build NDEF message: %w", err)
	}

	// Use the new WriteMessageWithOptions to preserve NDEF structure
	err = reader.WriteMessageWithOptions(ndefMsg, writeOpts)
	if err != nil {
		return fmt.Errorf("write failed: %w", err)
	}

	return nil
}
