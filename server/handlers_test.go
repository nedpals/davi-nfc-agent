package server

import (
	"testing"

	"github.com/dotside-studios/davi-nfc-agent/nfc"
)

// TestBuildNDEFMessage tests NDEF message building logic
func TestBuildNDEFMessage(t *testing.T) {
	tests := []struct {
		name        string
		request     WriteRequest
		expectError bool
		checkMsg    func(*testing.T, *nfc.NDEFMessage)
	}{
		{
			name: "Single text record",
			request: WriteRequest{
				Records: []WriteRecord{
					{Type: "text", Content: "Hello, NFC!"},
				},
			},
			expectError: false,
			checkMsg: func(t *testing.T, msg *nfc.NDEFMessage) {
				records := msg.Records()
				if len(records) != 1 {
					t.Errorf("Expected 1 record, got %d", len(records))
				}
				text, _ := records[0].GetText()
				if text != "Hello, NFC!" {
					t.Errorf("Expected 'Hello, NFC!', got '%s'", text)
				}
			},
		},
		{
			name: "Multiple records",
			request: WriteRequest{
				Records: []WriteRecord{
					{Type: "text", Content: "First"},
					{Type: "text", Content: "Second"},
				},
			},
			expectError: false,
			checkMsg: func(t *testing.T, msg *nfc.NDEFMessage) {
				records := msg.Records()
				if len(records) != 2 {
					t.Errorf("Expected 2 records, got %d", len(records))
				}
			},
		},
		{
			name: "URI record",
			request: WriteRequest{
				Records: []WriteRecord{
					{Type: "uri", Content: "https://example.com"},
				},
			},
			expectError: false,
			checkMsg: func(t *testing.T, msg *nfc.NDEFMessage) {
				records := msg.Records()
				if len(records) != 1 {
					t.Errorf("Expected 1 record, got %d", len(records))
				}
				uri, _ := records[0].GetURI()
				if uri != "https://example.com" {
					t.Errorf("Expected 'https://example.com', got '%s'", uri)
				}
			},
		},
		{
			name: "Mixed record types",
			request: WriteRequest{
				Records: []WriteRecord{
					{Type: "text", Content: "Hello"},
					{Type: "uri", Content: "https://example.com"},
				},
			},
			expectError: false,
			checkMsg: func(t *testing.T, msg *nfc.NDEFMessage) {
				records := msg.Records()
				if len(records) != 2 {
					t.Errorf("Expected 2 records, got %d", len(records))
				}
			},
		},
		{
			name: "Unsupported record type",
			request: WriteRequest{
				Records: []WriteRecord{
					{Type: "unknown", Content: "test"},
				},
			},
			expectError: true,
		},
		{
			name: "Empty records array",
			request: WriteRequest{
				Records: []WriteRecord{},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := BuildNDEFMessage(tt.request)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if msg == nil {
				t.Fatal("Expected NDEF message, got nil")
			}

			if tt.checkMsg != nil {
				tt.checkMsg(t, msg)
			}
		})
	}
}
