package nfc

import (
	"bytes"
	"testing"

	"github.com/clausecker/freefare"
)

func TestFreefareTLVencode_ShortMessage(t *testing.T) {
	data := []byte{0x01, 0x02, 0x03, 0x04}
	result := freefare.TLVencode(data, 0x03)

	// Expected: Type 0x03, Length 4, Data, Terminator 0xFE
	expected := []byte{0x03, 0x04, 0x01, 0x02, 0x03, 0x04, 0xFE}
	if !bytes.Equal(result, expected) {
		t.Errorf("Expected %v, got %v", expected, result)
	}
}

func TestFreefareTLVencode_LongMessage(t *testing.T) {
	// Create a message longer than 254 bytes
	data := make([]byte, 300)
	for i := range data {
		data[i] = byte(i % 256)
	}

	result := freefare.TLVencode(data, 0x03)

	// Check header: Type 0x03, 0xFF, Length MSB (0x01), Length LSB (0x2C = 300)
	if result[0] != 0x03 {
		t.Errorf("Expected type 0x03, got 0x%02X", result[0])
	}
	if result[1] != 0xFF {
		t.Errorf("Expected long format marker 0xFF, got 0x%02X", result[1])
	}
	if result[2] != 0x01 || result[3] != 0x2C {
		t.Errorf("Expected length 0x012C (300), got 0x%02X%02X", result[2], result[3])
	}

	// Check terminator
	if result[len(result)-1] != 0xFE {
		t.Errorf("Expected terminator 0xFE, got 0x%02X", result[len(result)-1])
	}

	// Check data
	if !bytes.Equal(result[4:4+300], data) {
		t.Error("Data mismatch in long format TLV")
	}
}

func TestFreefareTLVencode_EmptyMessage(t *testing.T) {
	result := freefare.TLVencode([]byte{}, 0x03)

	// Expected: Type 0x03, Length 0, Terminator 0xFE
	expected := []byte{0x03, 0x00, 0xFE}
	if !bytes.Equal(result, expected) {
		t.Errorf("Expected %v, got %v", expected, result)
	}
}

func TestFreefareTLVdecode_ShortMessage(t *testing.T) {
	// TLV structure: Type 0x03 (NDEF), Length 4, Data [0x01, 0x02, 0x03, 0x04], Terminator 0xFE
	tlvData := []byte{0x03, 0x04, 0x01, 0x02, 0x03, 0x04, 0xFE}

	result, tlvType := freefare.TLVdecode(tlvData)
	if tlvType != 0x03 {
		t.Errorf("Expected type 0x03, got 0x%02X", tlvType)
	}

	expected := []byte{0x01, 0x02, 0x03, 0x04}
	if !bytes.Equal(result, expected) {
		t.Errorf("Expected %v, got %v", expected, result)
	}
}

func TestFreefareTLVdecode_LongFormat(t *testing.T) {
	// Long format: Type 0x03, Length 0xFF followed by 2-byte length (256 bytes)
	ndefData := make([]byte, 256)
	for i := range ndefData {
		ndefData[i] = byte(i)
	}

	tlvData := []byte{0x03, 0xFF, 0x01, 0x00} // Type, Long format marker, Length MSB, Length LSB
	tlvData = append(tlvData, ndefData...)
	tlvData = append(tlvData, 0xFE) // Terminator

	result, tlvType := freefare.TLVdecode(tlvData)
	if tlvType != 0x03 {
		t.Errorf("Expected type 0x03, got 0x%02X", tlvType)
	}

	if !bytes.Equal(result, ndefData) {
		t.Errorf("Long format NDEF data mismatch")
	}
}

func TestFreefareTLVrecordLength_Short(t *testing.T) {
	tlvData := []byte{0x03, 0x04, 0x01, 0x02, 0x03, 0x04}
	fls, fvs := freefare.TLVrecordLength(tlvData)

	if fls != 1 {
		t.Errorf("Expected field length size 1, got %d", fls)
	}
	if fvs != 4 {
		t.Errorf("Expected field value size 4, got %d", fvs)
	}
}

func TestFreefareTLVrecordLength_Long(t *testing.T) {
	tlvData := []byte{0x03, 0xFF, 0x01, 0x00} // 256 bytes
	fls, fvs := freefare.TLVrecordLength(tlvData)

	if fls != 3 {
		t.Errorf("Expected field length size 3, got %d", fls)
	}
	if fvs != 256 {
		t.Errorf("Expected field value size 256, got %d", fvs)
	}
}

func TestFreefareTLV_Roundtrip(t *testing.T) {
	// Test that encode -> decode returns original data
	original := []byte{0xD1, 0x01, 0x0C, 0x55, 0x01, 0x65, 0x78, 0x61, 0x6D, 0x70, 0x6C, 0x65, 0x2E, 0x63, 0x6F, 0x6D}

	encoded := freefare.TLVencode(original, 0x03)
	decoded, tlvType := freefare.TLVdecode(encoded)

	if tlvType != 0x03 {
		t.Errorf("Expected type 0x03, got 0x%02X", tlvType)
	}

	if !bytes.Equal(decoded, original) {
		t.Errorf("Roundtrip failed: expected %v, got %v", original, decoded)
	}
}

func TestFreefareTLV_RoundtripLong(t *testing.T) {
	// Test roundtrip with long format
	original := make([]byte, 500)
	for i := range original {
		original[i] = byte(i % 256)
	}

	encoded := freefare.TLVencode(original, 0x03)
	decoded, _ := freefare.TLVdecode(encoded)

	if !bytes.Equal(decoded, original) {
		t.Error("Long roundtrip failed: data mismatch")
	}
}

func TestNtag215Constants(t *testing.T) {
	// Verify NTAG215 memory layout constants
	if ntag215TotalPages != 135 {
		t.Errorf("Expected 135 total pages, got %d", ntag215TotalPages)
	}
	if ntag215UserStartPage != 4 {
		t.Errorf("Expected user start page 4, got %d", ntag215UserStartPage)
	}
	if ntag215UserEndPage != 129 {
		t.Errorf("Expected user end page 129, got %d", ntag215UserEndPage)
	}
	if ntag215UserPages != 126 {
		t.Errorf("Expected 126 user pages, got %d", ntag215UserPages)
	}

	// Verify user data capacity: 126 pages * 4 bytes = 504 bytes
	userDataCapacity := ntag215UserPages * 4
	if userDataCapacity != 504 {
		t.Errorf("Expected 504 bytes user capacity, got %d", userDataCapacity)
	}
}

func TestCardTypeNtag215Constant(t *testing.T) {
	if CardTypeNtag215 != "NTAG215" {
		t.Errorf("Expected CardTypeNtag215 to be 'NTAG215', got '%s'", CardTypeNtag215)
	}

	// Verify it's in GetAllCardTypes
	allTypes := GetAllCardTypes()
	found := false
	for _, ct := range allTypes {
		if ct == CardTypeNtag215 {
			found = true
			break
		}
	}
	if !found {
		t.Error("CardTypeNtag215 should be in GetAllCardTypes()")
	}
}

// MockNtagTag tests

func TestMockNtagTag_BasicOperations(t *testing.T) {
	tag := NewMockNtagTag("04112233445566")

	// Test UID
	if uid := tag.UID(); uid != "04112233445566" {
		t.Errorf("Expected UID '04112233445566', got '%s'", uid)
	}

	// Test Type
	if tagType := tag.Type(); tagType != CardTypeNtag215 {
		t.Errorf("Expected type '%s', got '%s'", CardTypeNtag215, tagType)
	}

	// Test NumericType
	if numType := tag.NumericType(); numType != 100 {
		t.Errorf("Expected numeric type 100, got %d", numType)
	}

	// Test Connect
	if err := tag.Connect(); err != nil {
		t.Errorf("Connect() failed: %v", err)
	}

	// Test Disconnect
	if err := tag.Disconnect(); err != nil {
		t.Errorf("Disconnect() failed: %v", err)
	}
}

func TestMockNtagTag_ReadWritePage(t *testing.T) {
	tag := NewMockNtagTag("04112233445566")
	tag.Connect()

	// Write data to page 4
	writeData := [4]byte{0x03, 0x10, 0xD1, 0x01}
	err := tag.WritePage(4, writeData)
	if err != nil {
		t.Errorf("WritePage() failed: %v", err)
	}

	// Read data back
	readData, err := tag.ReadPage(4)
	if err != nil {
		t.Errorf("ReadPage() failed: %v", err)
	}

	if readData != writeData {
		t.Errorf("Expected %v, got %v", writeData, readData)
	}
}

func TestMockNtagTag_ReadWriteNotConnected(t *testing.T) {
	tag := NewMockNtagTag("04112233445566")
	// Don't connect

	// Try to read - should fail
	_, err := tag.ReadPage(4)
	if err == nil {
		t.Error("ReadPage() should fail when not connected")
	}

	// Try to write - should fail
	err = tag.WritePage(4, [4]byte{0x01, 0x02, 0x03, 0x04})
	if err == nil {
		t.Error("WritePage() should fail when not connected")
	}
}

func TestMockNtagTag_MemoryBounds(t *testing.T) {
	tag := NewMockNtagTag("04112233445566")
	tag.Connect()

	// Try to read page beyond max (135)
	_, err := tag.ReadPage(135)
	if err == nil {
		t.Error("ReadPage(135) should fail - out of bounds")
	}

	// Try to write to page beyond max
	err = tag.WritePage(135, [4]byte{0x01, 0x02, 0x03, 0x04})
	if err == nil {
		t.Error("WritePage(135) should fail - out of bounds")
	}

	// Valid page 134 should work
	tag.SetPageData(134, [4]byte{0xAA, 0xBB, 0xCC, 0xDD})
	data, err := tag.ReadPage(134)
	if err != nil {
		t.Errorf("ReadPage(134) should succeed: %v", err)
	}
	if data != [4]byte{0xAA, 0xBB, 0xCC, 0xDD} {
		t.Errorf("Expected [AA BB CC DD], got %v", data)
	}
}

func TestMockNtagTag_HeaderProtection(t *testing.T) {
	tag := NewMockNtagTag("04112233445566")
	tag.Connect()

	// Pages 0-3 should be write-protected
	for page := byte(0); page < 4; page++ {
		err := tag.WritePage(page, [4]byte{0x01, 0x02, 0x03, 0x04})
		if err == nil {
			t.Errorf("WritePage(%d) should fail - header area is read-only", page)
		}
	}

	// Page 4 should be writable
	err := tag.WritePage(4, [4]byte{0x01, 0x02, 0x03, 0x04})
	if err != nil {
		t.Errorf("WritePage(4) should succeed: %v", err)
	}
}

func TestMockNtagTag_ReadOnly(t *testing.T) {
	tag := NewMockNtagTag("04112233445566")
	tag.Connect()

	// Write should work initially
	err := tag.WritePage(4, [4]byte{0x01, 0x02, 0x03, 0x04})
	if err != nil {
		t.Errorf("WritePage() should succeed: %v", err)
	}

	// Make tag read-only
	tag.MakeReadOnly()

	// Write should fail now
	err = tag.WritePage(5, [4]byte{0x05, 0x06, 0x07, 0x08})
	if err == nil {
		t.Error("WritePage() should fail on read-only tag")
	}

	// Read should still work
	_, err = tag.ReadPage(4)
	if err != nil {
		t.Errorf("ReadPage() should still work on read-only tag: %v", err)
	}
}

func TestMockNtagTag_NDEFWorkflow(t *testing.T) {
	tag := NewMockNtagTag("04112233445566")
	tag.Connect()

	// Create an NDEF message (URI record for "example.com")
	ndefMessage := []byte{
		0xD1,       // MB=1, ME=1, CF=0, SR=1, IL=0, TNF=1 (Well-Known)
		0x01,       // Type Length = 1
		0x0C,       // Payload Length = 12
		0x55,       // Type = 'U' (URI)
		0x01,       // URI prefix = http://www.
		'e', 'x', 'a', 'm', 'p', 'l', 'e', '.', 'c', 'o', 'm',
	}

	// Encode as TLV
	tlvData := freefare.TLVencode(ndefMessage, 0x03)

	// Write TLV to pages starting at page 4
	page := byte(4)
	for offset := 0; offset < len(tlvData); offset += 4 {
		var pageData [4]byte
		end := offset + 4
		if end > len(tlvData) {
			end = len(tlvData)
		}
		copy(pageData[:], tlvData[offset:end])
		tag.SetPageData(page, pageData)
		page++
	}

	// Now read it back and verify
	var readData []byte
	for p := byte(4); p < page; p++ {
		data, err := tag.ReadPage(p)
		if err != nil {
			t.Errorf("ReadPage(%d) failed: %v", p, err)
		}
		readData = append(readData, data[:]...)
	}

	// Decode TLV
	decoded, tlvType := freefare.TLVdecode(readData)
	if tlvType != 0x03 {
		t.Errorf("Expected TLV type 0x03 (NDEF), got 0x%02X", tlvType)
	}

	if !bytes.Equal(decoded, ndefMessage) {
		t.Errorf("NDEF message mismatch.\nExpected: %v\nGot: %v", ndefMessage, decoded)
	}
}

func TestMockNtagTag_SetGetPageData(t *testing.T) {
	tag := NewMockNtagTag("04112233445566")

	// Set page data directly (no connection required)
	testData := [4]byte{0xDE, 0xAD, 0xBE, 0xEF}
	tag.SetPageData(10, testData)

	// Get it back
	data, exists := tag.GetPageData(10)
	if !exists {
		t.Error("Page 10 should exist after SetPageData")
	}
	if data != testData {
		t.Errorf("Expected %v, got %v", testData, data)
	}

	// Non-existent page
	_, exists = tag.GetPageData(20)
	if exists {
		t.Error("Page 20 should not exist")
	}
}

func TestMockNtagTag_ClearPageData(t *testing.T) {
	tag := NewMockNtagTag("04112233445566")

	// Set some pages
	tag.SetPageData(4, [4]byte{0x01, 0x02, 0x03, 0x04})
	tag.SetPageData(5, [4]byte{0x05, 0x06, 0x07, 0x08})

	// Clear all
	tag.ClearPageData()

	// Verify cleared
	_, exists := tag.GetPageData(4)
	if exists {
		t.Error("Page 4 should not exist after ClearPageData")
	}
	_, exists = tag.GetPageData(5)
	if exists {
		t.Error("Page 5 should not exist after ClearPageData")
	}
}

func TestMockNtagTag_CallLog(t *testing.T) {
	tag := NewMockNtagTag("04112233445566")
	tag.ClearCallLog()

	tag.Connect()
	tag.ReadPage(4)
	tag.WritePage(5, [4]byte{0x01, 0x02, 0x03, 0x04})
	tag.Disconnect()

	callLog := tag.GetCallLog()
	expectedCalls := []string{
		"Connect",
		"ReadPage(4)",
		"WritePage(5)",
		"Disconnect",
	}

	if len(callLog) != len(expectedCalls) {
		t.Errorf("Expected %d calls, got %d: %v", len(expectedCalls), len(callLog), callLog)
	}

	for i, call := range expectedCalls {
		if i >= len(callLog) {
			break
		}
		if callLog[i] != call {
			t.Errorf("Expected call %d to be '%s', got '%s'", i, call, callLog[i])
		}
	}
}

func TestMockDevice_WithNtagTag(t *testing.T) {
	device := NewMockDevice()
	ntag := NewMockNtagTag("04112233445566")
	device.AddTag(ntag)

	tags, err := device.GetTags()
	if err != nil {
		t.Errorf("GetTags() failed: %v", err)
	}

	if len(tags) != 1 {
		t.Errorf("Expected 1 tag, got %d", len(tags))
	}

	if tags[0].Type() != CardTypeNtag215 {
		t.Errorf("Expected tag type '%s', got '%s'", CardTypeNtag215, tags[0].Type())
	}

	if tags[0].UID() != "04112233445566" {
		t.Errorf("Expected UID '04112233445566', got '%s'", tags[0].UID())
	}
}
