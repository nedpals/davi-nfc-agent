package server

import (
	"testing"

	"github.com/nedpals/davi-nfc-agent/nfc"
)

// TestBuildNDEFMessage tests NDEF message building logic (now in server package)
func TestBuildNDEFMessage(t *testing.T) {
	tests := []struct {
		name        string
		request     WriteRequest
		expectError bool
		checkOpts   func(*testing.T, nfc.WriteOptions)
	}{
		{
			name: "Simple text record",
			request: WriteRequest{
				Text: "Hello, World!",
			},
			expectError: false,
			checkOpts: func(t *testing.T, opts nfc.WriteOptions) {
				if !opts.Overwrite {
					t.Error("Expected Overwrite to be true for simple write")
				}
				if opts.Index != -1 {
					t.Error("Expected Index to be -1 for simple write")
				}
			},
		},
		{
			name: "Append mode",
			request: WriteRequest{
				Text:   "Additional text",
				Append: true,
			},
			expectError: false,
			checkOpts: func(t *testing.T, opts nfc.WriteOptions) {
				if opts.Overwrite {
					t.Error("Expected Overwrite to be false for append")
				}
				if opts.Index != -1 {
					t.Error("Expected Index to be -1 for append")
				}
			},
		},
		{
			name: "Replace mode",
			request: WriteRequest{
				Text:    "New content",
				Replace: true,
			},
			expectError: false,
			checkOpts: func(t *testing.T, opts nfc.WriteOptions) {
				if !opts.Overwrite {
					t.Error("Expected Overwrite to be true for replace")
				}
				if opts.Index != -1 {
					t.Error("Expected Index to be -1 for replace")
				}
			},
		},
		{
			name: "Update specific record",
			request: WriteRequest{
				Text:        "Updated text",
				RecordIndex: intPtr(0),
			},
			expectError: false,
			checkOpts: func(t *testing.T, opts nfc.WriteOptions) {
				if opts.Overwrite {
					t.Error("Expected Overwrite to be false for record update")
				}
				if opts.Index != 0 {
					t.Errorf("Expected Index to be 0, got %d", opts.Index)
				}
			},
		},
		{
			name: "URI record",
			request: WriteRequest{
				Text:       "https://example.com",
				RecordType: "uri",
			},
			expectError: false,
			checkOpts: func(t *testing.T, opts nfc.WriteOptions) {
				if !opts.Overwrite {
					t.Error("Expected Overwrite to be true for simple URI write")
				}
			},
		},
		{
			name: "Unsupported record type",
			request: WriteRequest{
				Text:       "test",
				RecordType: "unknown",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, opts, err := BuildNDEFMessage(tt.request)

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

			if tt.checkOpts != nil {
				tt.checkOpts(t, opts)
			}
		})
	}
}

func intPtr(i int) *int {
	return &i
}
