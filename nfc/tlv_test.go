package nfc

import (
	"bytes"
	"testing"
)

func TestTLVEncode_ShortMessage(t *testing.T) {
	data := []byte{0x01, 0x02, 0x03, 0x04}
	result := TLVEncode(data, TLVNDEF)

	// Expected: Type (0x03) + Length (0x04) + Data + Terminator (0xFE)
	expected := []byte{0x03, 0x04, 0x01, 0x02, 0x03, 0x04, 0xFE}
	if !bytes.Equal(result, expected) {
		t.Errorf("Expected %v, got %v", expected, result)
	}
}

func TestTLVEncode_LongMessage(t *testing.T) {
	// Create data that requires long format (>254 bytes)
	data := make([]byte, 300)
	for i := range data {
		data[i] = byte(i % 256)
	}

	result := TLVEncode(data, TLVNDEF)

	// Expected: Type (0x03) + 0xFF + Length (2 bytes big-endian) + Data + Terminator (0xFE)
	if result[0] != 0x03 {
		t.Errorf("Expected type 0x03, got 0x%02X", result[0])
	}
	if result[1] != 0xFF {
		t.Errorf("Expected long format marker 0xFF, got 0x%02X", result[1])
	}
	// Length should be 300 = 0x012C
	if result[2] != 0x01 || result[3] != 0x2C {
		t.Errorf("Expected length bytes 0x01 0x2C, got 0x%02X 0x%02X", result[2], result[3])
	}
	// Check data starts at offset 4
	if !bytes.Equal(result[4:4+len(data)], data) {
		t.Error("Data mismatch in long format TLV")
	}
	// Check terminator
	if result[len(result)-1] != 0xFE {
		t.Errorf("Expected terminator 0xFE, got 0x%02X", result[len(result)-1])
	}
}

func TestTLVEncode_EmptyMessage(t *testing.T) {
	result := TLVEncode([]byte{}, TLVNDEF)

	// Expected: Type (0x03) + Length (0x00) + Terminator (0xFE)
	expected := []byte{0x03, 0x00, 0xFE}
	if !bytes.Equal(result, expected) {
		t.Errorf("Expected %v, got %v", expected, result)
	}
}

func TestTLVDecode_ShortMessage(t *testing.T) {
	// TLV structure: Type 0x03 (NDEF), Length 4, Data [0x01, 0x02, 0x03, 0x04], Terminator 0xFE
	tlvData := []byte{0x03, 0x04, 0x01, 0x02, 0x03, 0x04, 0xFE}

	result, tlvType := TLVDecode(tlvData)

	if tlvType != TLVNDEF {
		t.Errorf("Expected type 0x03, got 0x%02X", tlvType)
	}
	expected := []byte{0x01, 0x02, 0x03, 0x04}
	if !bytes.Equal(result, expected) {
		t.Errorf("Expected %v, got %v", expected, result)
	}
}

func TestTLVDecode_LongFormat(t *testing.T) {
	// Create TLV with long format length
	data := make([]byte, 300)
	for i := range data {
		data[i] = byte(i % 256)
	}

	// Build TLV manually: Type + 0xFF + Length (2 bytes) + Data + Terminator
	tlvData := []byte{0x03, 0xFF, 0x01, 0x2C} // Type + long format + length 300
	tlvData = append(tlvData, data...)
	tlvData = append(tlvData, 0xFE)

	result, tlvType := TLVDecode(tlvData)

	if tlvType != TLVNDEF {
		t.Errorf("Expected type 0x03, got 0x%02X", tlvType)
	}
	if !bytes.Equal(result, data) {
		t.Error("Data mismatch in long format decode")
	}
}

func TestTLVRecordLength_Short(t *testing.T) {
	tlvData := []byte{0x03, 0x04, 0x01, 0x02, 0x03, 0x04}
	fls, fvs := TLVRecordLength(tlvData)

	if fls != 1 {
		t.Errorf("Expected field length start 1, got %d", fls)
	}
	if fvs != 2 {
		t.Errorf("Expected field value start 2, got %d", fvs)
	}
}

func TestTLVRecordLength_Long(t *testing.T) {
	tlvData := []byte{0x03, 0xFF, 0x01, 0x2C, 0x00} // Long format
	fls, fvs := TLVRecordLength(tlvData)

	if fls != 1 {
		t.Errorf("Expected field length start 1, got %d", fls)
	}
	if fvs != 4 {
		t.Errorf("Expected field value start 4, got %d", fvs)
	}
}

func TestTLV_Roundtrip(t *testing.T) {
	// Test short message roundtrip
	original := []byte{0xD1, 0x01, 0x04, 0x54, 0x02, 0x65, 0x6E, 0x48, 0x69}

	encoded := TLVEncode(original, TLVNDEF)
	decoded, tlvType := TLVDecode(encoded)

	if tlvType != TLVNDEF {
		t.Errorf("Expected type 0x03, got 0x%02X", tlvType)
	}
	if !bytes.Equal(decoded, original) {
		t.Errorf("Roundtrip failed: original %v, got %v", original, decoded)
	}
}

func TestTLV_RoundtripLong(t *testing.T) {
	// Test long message roundtrip
	original := make([]byte, 512)
	for i := range original {
		original[i] = byte(i % 256)
	}

	encoded := TLVEncode(original, TLVNDEF)
	decoded, _ := TLVDecode(encoded)

	if !bytes.Equal(decoded, original) {
		t.Error("Long format roundtrip failed")
	}
}

func TestTLVFindNDEF_WithNullTLVs(t *testing.T) {
	// Data with null TLVs before NDEF
	data := []byte{
		0x00,       // Null TLV
		0x00,       // Null TLV
		0x03, 0x04, // NDEF TLV, length 4
		0x01, 0x02, 0x03, 0x04, // NDEF data
		0xFE, // Terminator
	}

	result, found := TLVFindNDEF(data)

	if !found {
		t.Error("Expected to find NDEF TLV")
	}
	expected := []byte{0x01, 0x02, 0x03, 0x04}
	if !bytes.Equal(result, expected) {
		t.Errorf("Expected %v, got %v", expected, result)
	}
}

func TestTLVFindNDEF_NoNDEF(t *testing.T) {
	// Data with only terminator
	data := []byte{0x00, 0x00, 0xFE}

	result, found := TLVFindNDEF(data)

	if found {
		t.Error("Expected not to find NDEF TLV")
	}
	if result != nil {
		t.Errorf("Expected nil result, got %v", result)
	}
}

func TestTLVGetLength_Short(t *testing.T) {
	data := []byte{0x03, 0x10} // Type + short length
	length := TLVGetLength(data)

	if length != 16 {
		t.Errorf("Expected length 16, got %d", length)
	}
}

func TestTLVGetLength_Long(t *testing.T) {
	data := []byte{0x03, 0xFF, 0x01, 0x00} // Type + long format + length 256
	length := TLVGetLength(data)

	if length != 256 {
		t.Errorf("Expected length 256, got %d", length)
	}
}
