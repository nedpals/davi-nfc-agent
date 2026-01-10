package remotenfc

import (
	"testing"
	"time"

	"github.com/dotside-studios/davi-nfc-agent/nfc"
)

func TestTagImplementsNFCTag(t *testing.T) {
	// Verify Tag implements nfc.Tag interface
	var _ nfc.Tag = (*Tag)(nil)
}

func TestTagUID(t *testing.T) {
	tag := &Tag{
		uid:        "04:AB:CD:EF",
		tagType:    "Type4",
		technology: "ISO14443A",
	}

	if tag.UID() != "04:AB:CD:EF" {
		t.Errorf("UID() = %v, want %v", tag.UID(), "04:AB:CD:EF")
	}
}

func TestTagType(t *testing.T) {
	tag := &Tag{
		uid:        "04:AB:CD:EF",
		tagType:    "MIFARE Classic 1K",
		technology: "ISO14443A",
	}

	if tag.Type() != "MIFARE Classic 1K" {
		t.Errorf("Type() = %v, want %v", tag.Type(), "MIFARE Classic 1K")
	}
}

func TestTagNumericType(t *testing.T) {
	tag := &Tag{
		uid:        "04:AB:CD:EF",
		tagType:    "Type4",
		technology: "ISO14443A",
	}

	// Tag should return 0 for numeric type
	if tag.NumericType() != 0 {
		t.Errorf("NumericType() = %v, want 0", tag.NumericType())
	}
}

func TestTagReadDataWithNDEF(t *testing.T) {
	ndefData := []byte{0x01, 0x02, 0x03, 0x04}
	tag := &Tag{
		uid:        "04:AB:CD:EF",
		tagType:    "Type4",
		technology: "ISO14443A",
		ndefData:   ndefData,
		rawData:    []byte("raw"),
	}

	data, err := tag.ReadData()
	if err != nil {
		t.Errorf("ReadData() failed: %v", err)
	}

	if len(data) != len(ndefData) {
		t.Errorf("ReadData() returned %d bytes, want %d", len(data), len(ndefData))
	}

	for i := range ndefData {
		if data[i] != ndefData[i] {
			t.Errorf("ReadData()[%d] = %v, want %v", i, data[i], ndefData[i])
			break
		}
	}
}

func TestTagReadDataWithoutNDEF(t *testing.T) {
	rawData := []byte("raw data")
	tag := &Tag{
		uid:        "04:AB:CD:EF",
		tagType:    "Type4",
		technology: "ISO14443A",
		rawData:    rawData,
	}

	data, err := tag.ReadData()
	if err != nil {
		t.Errorf("ReadData() failed: %v", err)
	}

	if len(data) != len(rawData) {
		t.Errorf("ReadData() returned %d bytes, want %d", len(data), len(rawData))
	}
}

func TestTagWriteData(t *testing.T) {
	tag := &Tag{
		uid:        "04:AB:CD:EF",
		tagType:    "Type4",
		technology: "ISO14443A",
	}

	// WriteData should not be supported
	err := tag.WriteData([]byte("test"))
	if err == nil {
		t.Error("WriteData() should return error (not supported)")
	}
}

func TestTagTransceive(t *testing.T) {
	tag := &Tag{
		uid:        "04:AB:CD:EF",
		tagType:    "Type4",
		technology: "ISO14443A",
	}

	// Transceive should not be supported
	_, err := tag.Transceive([]byte{0x01, 0x02})
	if err == nil {
		t.Error("Transceive() should return error (not supported)")
	}
}

func TestTagConnect(t *testing.T) {
	tag := &Tag{
		uid:        "04:AB:CD:EF",
		tagType:    "Type4",
		technology: "ISO14443A",
	}

	// Connect should be a no-op
	err := tag.Connect()
	if err != nil {
		t.Errorf("Connect() failed: %v", err)
	}
}

func TestTagDisconnect(t *testing.T) {
	tag := &Tag{
		uid:        "04:AB:CD:EF",
		tagType:    "Type4",
		technology: "ISO14443A",
	}

	// Disconnect should be a no-op
	err := tag.Disconnect()
	if err != nil {
		t.Errorf("Disconnect() failed: %v", err)
	}
}

func TestTagIsWritable(t *testing.T) {
	tag := &Tag{
		uid:        "04:AB:CD:EF",
		tagType:    "Type4",
		technology: "ISO14443A",
	}

	// Should return false
	writable, err := tag.IsWritable()
	if err != nil {
		t.Errorf("IsWritable() failed: %v", err)
	}
	if writable {
		t.Error("IsWritable() should return false")
	}
}

func TestTagCanMakeReadOnly(t *testing.T) {
	tag := &Tag{
		uid:        "04:AB:CD:EF",
		tagType:    "Type4",
		technology: "ISO14443A",
	}

	// Should return false
	canMakeReadOnly, err := tag.CanMakeReadOnly()
	if err != nil {
		t.Errorf("CanMakeReadOnly() failed: %v", err)
	}
	if canMakeReadOnly {
		t.Error("CanMakeReadOnly() should return false")
	}
}

func TestTagMakeReadOnly(t *testing.T) {
	tag := &Tag{
		uid:        "04:AB:CD:EF",
		tagType:    "Type4",
		technology: "ISO14443A",
	}

	// Should return error
	err := tag.MakeReadOnly()
	if err == nil {
		t.Error("MakeReadOnly() should return error (not supported)")
	}
}

func TestTagGetNDEFMessage(t *testing.T) {
	ndefMsg := nfc.NewNDEFMessage()
	ndefMsg.AddText("Test", "en")

	tag := &Tag{
		uid:        "04:AB:CD:EF",
		tagType:    "Type4",
		technology: "ISO14443A",
		ndefMsg:    ndefMsg,
	}

	// Should return the NDEF message
	msg, err := tag.GetNDEFMessage()
	if err != nil {
		t.Errorf("GetNDEFMessage() failed: %v", err)
	}
	if msg == nil {
		t.Error("GetNDEFMessage() returned nil")
	}

	// Test with no NDEF message
	tag2 := &Tag{
		uid:        "04:AB:CD:EF",
		tagType:    "Type4",
		technology: "ISO14443A",
	}

	_, err = tag2.GetNDEFMessage()
	if err == nil {
		t.Error("GetNDEFMessage() should return error when no NDEF available")
	}
}

func TestTagScannedAt(t *testing.T) {
	now := time.Now()
	tag := &Tag{
		uid:        "04:AB:CD:EF",
		tagType:    "Type4",
		technology: "ISO14443A",
		scannedAt:  now,
	}

	scannedAt := tag.ScannedAt()
	if !scannedAt.Equal(now) {
		t.Errorf("ScannedAt() = %v, want %v", scannedAt, now)
	}
}

func TestTagSourceDevice(t *testing.T) {
	tag := &Tag{
		uid:          "04:AB:CD:EF",
		tagType:      "Type4",
		technology:   "ISO14443A",
		sourceDevice: "device-123",
	}

	sourceDevice := tag.SourceDevice()
	if sourceDevice != "device-123" {
		t.Errorf("SourceDevice() = %v, want %v", sourceDevice, "device-123")
	}
}

func TestTagThreadSafety(t *testing.T) {
	tag := &Tag{
		uid:        "04:AB:CD:EF",
		tagType:    "Type4",
		technology: "ISO14443A",
		ndefData:   []byte{0x01, 0x02},
	}

	// Test concurrent reads
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			_, err := tag.ReadData()
			if err != nil {
				t.Errorf("Concurrent ReadData() failed: %v", err)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}
