package nfc

import (
	"testing"
)

// TestNDEFMessageToBuilder tests the ToBuilder conversion
func TestNDEFMessageToBuilder(t *testing.T) {
	// Create a message with various record types
	originalBuilder := &NDEFMessageBuilder{
		Records: []NDEFRecordBuilder{
			&NDEFText{Content: "Hello World", Language: "en"},
			&NDEFURI{Content: "https://example.com"},
			&NDEFMIME{Type: "application/json", Data: []byte(`{"key":"value"}`)},
		},
	}
	original := originalBuilder.MustBuild()

	// Convert to builder
	builder := original.ToBuilder()

	if len(builder.Records) != 3 {
		t.Fatalf("Expected 3 records in builder, got %d", len(builder.Records))
	}

	// Check first record (Text)
	if text, ok := builder.Records[0].(*NDEFText); ok {
		if text.Content != "Hello World" {
			t.Errorf("Text content mismatch: expected 'Hello World', got '%s'", text.Content)
		}
		if text.Language != "en" {
			t.Errorf("Text language mismatch: expected 'en', got '%s'", text.Language)
		}
	} else {
		t.Errorf("First record should be *NDEFText, got %T", builder.Records[0])
	}

	// Check second record (URI)
	if uri, ok := builder.Records[1].(*NDEFURI); ok {
		if uri.Content != "https://example.com" {
			t.Errorf("URI content mismatch: expected 'https://example.com', got '%s'", uri.Content)
		}
	} else {
		t.Errorf("Second record should be *NDEFURI, got %T", builder.Records[1])
	}

	// Check third record (MIME)
	if mime, ok := builder.Records[2].(*NDEFMIME); ok {
		if mime.Type != "application/json" {
			t.Errorf("MIME type mismatch: expected 'application/json', got '%s'", mime.Type)
		}
		if string(mime.Data) != `{"key":"value"}` {
			t.Errorf("MIME data mismatch")
		}
	} else {
		t.Errorf("Third record should be *NDEFMIME, got %T", builder.Records[2])
	}
}

// TestRecordToBuilder_TextRecord tests conversion of text records
func TestRecordToBuilder_TextRecord(t *testing.T) {
	// Create a text record
	text := &NDEFText{Content: "Test", Language: "fr"}
	record := text.ToRecord()

	// Convert back to builder
	builder := recordToBuilder(record)

	if builder == nil {
		t.Fatal("recordToBuilder returned nil")
	}

	textBuilder, ok := builder.(*NDEFText)
	if !ok {
		t.Fatalf("Expected *NDEFText, got %T", builder)
	}

	if textBuilder.Content != "Test" {
		t.Errorf("Content mismatch: expected 'Test', got '%s'", textBuilder.Content)
	}
	if textBuilder.Language != "fr" {
		t.Errorf("Language mismatch: expected 'fr', got '%s'", textBuilder.Language)
	}
}

// TestRecordToBuilder_URIRecord tests conversion of URI records
func TestRecordToBuilder_URIRecord(t *testing.T) {
	uri := &NDEFURI{Content: "https://test.com"}
	record := uri.ToRecord()

	builder := recordToBuilder(record)

	if builder == nil {
		t.Fatal("recordToBuilder returned nil")
	}

	uriBuilder, ok := builder.(*NDEFURI)
	if !ok {
		t.Fatalf("Expected *NDEFURI, got %T", builder)
	}

	if uriBuilder.Content != "https://test.com" {
		t.Errorf("Content mismatch: expected 'https://test.com', got '%s'", uriBuilder.Content)
	}
}

// TestRecordToBuilder_MIMERecord tests conversion of MIME records
func TestRecordToBuilder_MIMERecord(t *testing.T) {
	data := []byte("test data")
	mime := &NDEFMIME{Type: "text/plain", Data: data}
	record := mime.ToRecord()

	builder := recordToBuilder(record)

	if builder == nil {
		t.Fatal("recordToBuilder returned nil")
	}

	mimeBuilder, ok := builder.(*NDEFMIME)
	if !ok {
		t.Fatalf("Expected *NDEFMIME, got %T", builder)
	}

	if mimeBuilder.Type != "text/plain" {
		t.Errorf("Type mismatch: expected 'text/plain', got '%s'", mimeBuilder.Type)
	}
	if string(mimeBuilder.Data) != string(data) {
		t.Errorf("Data mismatch")
	}
}

// TestRecordToBuilder_ExternalRecord tests conversion of external records
func TestRecordToBuilder_ExternalRecord(t *testing.T) {
	data := []byte("custom")
	ext := &NDEFExternal{Domain: "example.com:app", Data: data}
	record := ext.ToRecord()

	builder := recordToBuilder(record)

	if builder == nil {
		t.Fatal("recordToBuilder returned nil")
	}

	extBuilder, ok := builder.(*NDEFExternal)
	if !ok {
		t.Fatalf("Expected *NDEFExternal, got %T", builder)
	}

	if extBuilder.Domain != "example.com:app" {
		t.Errorf("Domain mismatch: expected 'example.com:app', got '%s'", extBuilder.Domain)
	}
	if string(extBuilder.Data) != string(data) {
		t.Errorf("Data mismatch")
	}
}

// TestRecordToBuilder_EmptyRecord tests conversion of empty records
func TestRecordToBuilder_EmptyRecord(t *testing.T) {
	empty := &NDEFEmpty{}
	record := empty.ToRecord()

	builder := recordToBuilder(record)

	if builder == nil {
		t.Fatal("recordToBuilder returned nil")
	}

	_, ok := builder.(*NDEFEmpty)
	if !ok {
		t.Fatalf("Expected *NDEFEmpty, got %T", builder)
	}
}

// TestRecordToBuilder_UnknownRecord tests handling of unknown record types
func TestRecordToBuilder_UnknownRecord(t *testing.T) {
	// Create a record with unknown TNF
	record := NDEFRecord{
		TNF:     0x05, // Unknown type
		Type:    []byte("unknown"),
		Payload: []byte("data"),
	}

	builder := recordToBuilder(record)

	if builder != nil {
		t.Errorf("Expected nil for unknown record type, got %T", builder)
	}
}

// TestToBuilder_RoundTrip tests that we can go from builder -> message -> builder -> message
func TestToBuilder_RoundTrip(t *testing.T) {
	// Start with a builder
	originalBuilder := &NDEFMessageBuilder{
		Records: []NDEFRecordBuilder{
			&NDEFText{Content: "Round trip test", Language: "en"},
			&NDEFURI{Content: "https://roundtrip.test"},
		},
	}

	// Build message
	msg1 := originalBuilder.MustBuild()

	// Convert back to builder
	builder2 := msg1.ToBuilder()

	// Build again
	msg2 := builder2.MustBuild()

	// Encode both and compare
	data1, err1 := msg1.Encode()
	if err1 != nil {
		t.Fatalf("Failed to encode msg1: %v", err1)
	}

	data2, err2 := msg2.Encode()
	if err2 != nil {
		t.Fatalf("Failed to encode msg2: %v", err2)
	}

	if string(data1) != string(data2) {
		t.Errorf("Round trip produced different encoded data")
	}
}

// TestToBuilder_Manipulation tests modifying a message via builder
func TestToBuilder_Manipulation(t *testing.T) {
	// Create original message
	originalBuilder := &NDEFMessageBuilder{
		Records: []NDEFRecordBuilder{
			&NDEFText{Content: "Original", Language: "en"},
		},
	}
	original := originalBuilder.MustBuild()

	// Convert to builder and append a record
	builder := original.ToBuilder()
	builder.Records = append(builder.Records, &NDEFURI{Content: "https://added.com"})

	// Build new message
	modified := builder.MustBuild()

	// Check it has 2 records
	records := modified.Records()
	if len(records) != 2 {
		t.Fatalf("Expected 2 records after append, got %d", len(records))
	}

	// Verify first record unchanged
	text, ok := records[0].GetText()
	if !ok {
		t.Fatal("First record should be text")
	}
	if text != "Original" {
		t.Errorf("First record text changed: got '%s'", text)
	}

	// Verify second record was added
	uri, ok := records[1].GetURI()
	if !ok {
		t.Fatal("Second record should be URI")
	}
	if uri != "https://added.com" {
		t.Errorf("Second record URI mismatch: got '%s'", uri)
	}
}

// TestToBuilder_ReplaceRecord tests replacing a record at specific index
func TestToBuilder_ReplaceRecord(t *testing.T) {
	// Create original message with 2 records
	originalBuilder := &NDEFMessageBuilder{
		Records: []NDEFRecordBuilder{
			&NDEFText{Content: "First", Language: "en"},
			&NDEFText{Content: "Second", Language: "en"},
		},
	}
	original := originalBuilder.MustBuild()

	// Convert to builder and replace first record
	builder := original.ToBuilder()
	builder.Records[0] = &NDEFURI{Content: "https://replaced.com"}

	// Build new message
	modified := builder.MustBuild()

	// Check first record is now URI
	records := modified.Records()
	uri, ok := records[0].GetURI()
	if !ok {
		t.Fatal("First record should be URI after replacement")
	}
	if uri != "https://replaced.com" {
		t.Errorf("Replaced URI mismatch: got '%s'", uri)
	}

	// Check second record unchanged
	text, ok := records[1].GetText()
	if !ok {
		t.Fatal("Second record should still be text")
	}
	if text != "Second" {
		t.Errorf("Second record changed unexpectedly: got '%s'", text)
	}
}

// TestToBuilder_EmptyMessage tests ToBuilder on empty message
func TestToBuilder_EmptyMessage(t *testing.T) {
	msg := NewNDEFMessage()
	msg.AddText("Test", "en")

	builder := msg.ToBuilder()

	if len(builder.Records) != 1 {
		t.Errorf("Expected 1 record, got %d", len(builder.Records))
	}
}

// BenchmarkToBuilder benchmarks the ToBuilder conversion
func BenchmarkToBuilder(b *testing.B) {
	builder := &NDEFMessageBuilder{
		Records: []NDEFRecordBuilder{
			&NDEFText{Content: "Benchmark", Language: "en"},
			&NDEFURI{Content: "https://bench.test"},
			&NDEFMIME{Type: "text/plain", Data: []byte("data")},
		},
	}
	msg := builder.MustBuild()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = msg.ToBuilder()
	}
}

// BenchmarkRecordToBuilder benchmarks individual record conversion
func BenchmarkRecordToBuilder(b *testing.B) {
	text := &NDEFText{Content: "Benchmark", Language: "en"}
	record := text.ToRecord()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = recordToBuilder(record)
	}
}
