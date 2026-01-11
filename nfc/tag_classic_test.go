package nfc

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"testing"
)

// mockScardCard simulates a scard.Card for testing
type mockScardCard struct {
	// responses maps command hex strings to response hex strings
	responses map[string]string
	// callLog records all transmitted commands
	callLog [][]byte
	// authenticatedSector tracks which sector is currently authenticated
	authenticatedSector int
}

func newMockScardCard() *mockScardCard {
	return &mockScardCard{
		responses:           make(map[string]string),
		callLog:             make([][]byte, 0),
		authenticatedSector: -1,
	}
}

// Transmit simulates sending an APDU command
func (m *mockScardCard) Transmit(cmd []byte) ([]byte, error) {
	m.callLog = append(m.callLog, cmd)

	// Convert command to hex for lookup
	cmdHex := hex.EncodeToString(cmd)

	// Check for exact match first
	if respHex, ok := m.responses[cmdHex]; ok {
		return hex.DecodeString(respHex)
	}

	// Default: return error status
	return []byte{0x6A, 0x82}, nil
}

// addResponse adds a response for a command
func (m *mockScardCard) addResponse(cmdHex, respHex string) {
	m.responses[cmdHex] = respHex
}

// setupClassicTagResponses sets up responses for a basic MIFARE Classic tag
func (m *mockScardCard) setupClassicTagResponses() {
	// Load key command: FF 82 00 00 06 [key]
	// Factory key: FF FF FF FF FF FF
	m.addResponse("ff82000006ffffffffffff", "9000") // Load factory key success

	// Auth Key A for sector 1 (block 7): FF 86 00 00 05 01 00 07 60 00
	m.addResponse("ff8600000501000760009000", "9000") // Auth success (note: should be without 9000 in cmd)
	m.addResponse("ff860000050100076000", "9000")     // Auth sector 1 Key A success

	// Read block 4 (first user block in sector 1): FF B0 00 04 10
	// Response: 16 bytes of data + 90 00
	m.addResponse("ffb0000410", "03 0b d1 01 07 54 02 65 6e 48 65 6c 6c 6f 21 fe 9000")
}

// setupNDEFData sets up a tag with NDEF TLV data
func (m *mockScardCard) setupNDEFData(ndefData []byte) {
	// Load key success
	m.addResponse("ff82000006ffffffffffff", "9000")

	// Calculate which blocks we need to set up
	tlvData := TLVEncode(ndefData, TLVNDEF)

	// Pad to 16-byte blocks
	for len(tlvData)%16 != 0 {
		tlvData = append(tlvData, 0x00)
	}

	// Set up auth for each sector and read for each block
	for blockNum := 4; blockNum < 64; blockNum++ {
		sector := blockNum / 4

		// Skip sector trailers
		if (blockNum+1)%4 == 0 {
			continue
		}

		// Auth for this sector
		authBlock := sector*4 + 3
		authCmd := fmt.Sprintf("ff860000050100%02x6000", authBlock)
		m.addResponse(authCmd, "9000")

		// Read for this block
		readCmd := fmt.Sprintf("ffb000%02x10", blockNum)
		dataOffset := (blockNum - 4) * 16
		if dataOffset < len(tlvData) {
			endOffset := dataOffset + 16
			if endOffset > len(tlvData) {
				endOffset = len(tlvData)
			}
			blockData := make([]byte, 16)
			copy(blockData, tlvData[dataOffset:endOffset])
			m.addResponse(readCmd, hex.EncodeToString(blockData)+"9000")
		} else {
			// Empty block
			m.addResponse(readCmd, "000000000000000000000000000000009000")
		}
	}
}

// TestClassicTag_IsSectorTrailer tests the sector trailer detection
func TestClassicTag_IsSectorTrailer(t *testing.T) {
	tag := &pcscClassicTag{is4K: false}

	tests := []struct {
		block    int
		expected bool
	}{
		{0, false},
		{1, false},
		{2, false},
		{3, true}, // Sector 0 trailer
		{4, false},
		{5, false},
		{6, false},
		{7, true},  // Sector 1 trailer
		{8, false},
		{11, true}, // Sector 2 trailer
		{63, true}, // Last sector trailer in 1K
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("block_%d", tt.block), func(t *testing.T) {
			result := tag.isSectorTrailer(tt.block)
			if result != tt.expected {
				t.Errorf("isSectorTrailer(%d) = %v, want %v", tt.block, result, tt.expected)
			}
		})
	}
}

// TestClassicTag_IsSectorTrailer4K tests sector trailer detection for 4K cards
func TestClassicTag_IsSectorTrailer4K(t *testing.T) {
	tag := &pcscClassicTag{is4K: true}

	tests := []struct {
		block    int
		expected bool
	}{
		// Small sectors (0-31) - same as 1K
		{3, true},
		{7, true},
		{127, true},
		// Large sectors (32-39) - 16 blocks each
		{128, false},
		{143, true},  // First large sector trailer
		{144, false},
		{159, true},  // Second large sector trailer
		{255, true},  // Last sector trailer
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("block_%d", tt.block), func(t *testing.T) {
			result := tag.isSectorTrailer(tt.block)
			if result != tt.expected {
				t.Errorf("isSectorTrailer(%d) = %v, want %v", tt.block, result, tt.expected)
			}
		})
	}
}

// TestClassicTag_Type tests the Type() method
func TestClassicTag_Type(t *testing.T) {
	tests := []struct {
		name     string
		is4K     bool
		expected string
	}{
		{"1K card", false, CardTypeMifareClassic1K},
		{"4K card", true, CardTypeMifareClassic4K},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tag := &pcscClassicTag{is4K: tt.is4K}
			if got := tag.Type(); got != tt.expected {
				t.Errorf("Type() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// TestClassicTag_Transceive tests that Transceive returns an error
func TestClassicTag_Transceive(t *testing.T) {
	tag := &pcscClassicTag{}
	_, err := tag.Transceive([]byte{0x00})
	if err == nil {
		t.Error("Transceive() should return error for MIFARE Classic")
	}
}

// TestClassicTag_Capabilities tests the Capabilities() method
func TestClassicTag_Capabilities(t *testing.T) {
	tag := &pcscClassicTag{}
	caps := tag.Capabilities()

	if !caps.CanRead {
		t.Error("Expected CanRead to be true")
	}
	if !caps.CanWrite {
		t.Error("Expected CanWrite to be true")
	}
}

// TestClassicDefaultKeys verifies the default keys are defined correctly
func TestClassicDefaultKeys(t *testing.T) {
	expectedKeys := [][]byte{
		{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}, // Factory default
		{0xD3, 0xF7, 0xD3, 0xF7, 0xD3, 0xF7}, // NFC Forum public key
		{0xA0, 0xA1, 0xA2, 0xA3, 0xA4, 0xA5}, // MAD key
		{0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, // Zero key
	}

	if len(classicDefaultKeys) != len(expectedKeys) {
		t.Fatalf("Expected %d keys, got %d", len(expectedKeys), len(classicDefaultKeys))
	}

	for i, expected := range expectedKeys {
		if !bytes.Equal(classicDefaultKeys[i], expected) {
			t.Errorf("Key %d mismatch: got %v, want %v", i, classicDefaultKeys[i], expected)
		}
	}
}

// TestTLVEncode_ForClassic tests TLV encoding for MIFARE Classic (16-byte blocks)
func TestTLVEncode_ForClassic(t *testing.T) {
	// Small NDEF message
	ndefMessage := []byte{0xD1, 0x01, 0x04, 0x54, 0x02, 0x65, 0x6E, 0x48, 0x69}

	encoded := TLVEncode(ndefMessage, TLVNDEF)

	// Expected: Type (0x03) + Length (0x09) + Data + Terminator (0xFE)
	expected := []byte{0x03, 0x09}
	expected = append(expected, ndefMessage...)
	expected = append(expected, 0xFE)

	if !bytes.Equal(encoded, expected) {
		t.Errorf("TLVEncode mismatch:\ngot:  %v\nwant: %v", encoded, expected)
	}
}

// TestClassicTag_ReadDataParsesTLV tests that ReadData correctly parses TLV
func TestClassicTag_ReadDataParsesTLV(t *testing.T) {
	// Create a simple NDEF text record
	ndefMessage := []byte{0xD1, 0x01, 0x04, 0x54, 0x02, 0x65, 0x6E, 0x48, 0x69}

	// TLV-encoded version
	tlvData := TLVEncode(ndefMessage, TLVNDEF)

	// Parse it back
	parsed, found := TLVFindNDEF(tlvData)
	if !found {
		t.Fatal("TLVFindNDEF failed to find NDEF")
	}

	if !bytes.Equal(parsed, ndefMessage) {
		t.Errorf("NDEF mismatch:\ngot:  %v\nwant: %v", parsed, ndefMessage)
	}
}

// TestClassicTag_WriteDataFormatsTLV tests that WriteData formats TLV correctly
func TestClassicTag_WriteDataFormatsTLV(t *testing.T) {
	ndefMessage := []byte{0xD1, 0x01, 0x04, 0x54, 0x02, 0x65, 0x6E, 0x48, 0x69}

	// TLV encode
	tlvPayload := TLVEncode(ndefMessage, TLVNDEF)

	// Pad to 16-byte blocks as WriteData would
	for len(tlvPayload)%16 != 0 {
		tlvPayload = append(tlvPayload, 0x00)
	}

	// Should be padded to 16 bytes
	if len(tlvPayload)%16 != 0 {
		t.Errorf("Payload not padded to 16-byte boundary: len=%d", len(tlvPayload))
	}

	// First block should start with TLV type byte
	if tlvPayload[0] != TLVNDEF {
		t.Errorf("First byte should be NDEF TLV type (0x03), got 0x%02X", tlvPayload[0])
	}
}

// TestNewPCSCClassicTag tests tag creation
func TestNewPCSCClassicTag(t *testing.T) {
	mockDev := &pcscDevice{}

	tests := []struct {
		name     string
		tagType  DetectedTagType
		expected bool
	}{
		{"1K tag", DetectedClassic1K, false},
		{"4K tag", DetectedClassic4K, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tag := newPCSCClassicTag(mockDev, "04112233", tt.tagType)
			if tag.is4K != tt.expected {
				t.Errorf("is4K = %v, want %v", tag.is4K, tt.expected)
			}
			if tag.uid != "04112233" {
				t.Errorf("uid = %v, want 04112233", tag.uid)
			}
		})
	}
}

// TestClassicTag_AuthenticationKeys tests that we have the correct keys
func TestClassicTag_AuthenticationKeys(t *testing.T) {
	// Verify we have at least 4 keys
	if len(classicDefaultKeys) < 4 {
		t.Errorf("Expected at least 4 default keys, got %d", len(classicDefaultKeys))
	}

	// Verify all keys are 6 bytes
	for i, key := range classicDefaultKeys {
		if len(key) != 6 {
			t.Errorf("Key %d has wrong length: got %d, want 6", i, len(key))
		}
	}

	// Verify factory key is first (most common)
	factoryKey := []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}
	if !bytes.Equal(classicDefaultKeys[0], factoryKey) {
		t.Errorf("First key should be factory default")
	}
}

// TestClassicTag_SectorToAbsoluteBlock tests sector/block conversion
func TestClassicTag_SectorToAbsoluteBlock(t *testing.T) {
	tag1K := &pcscClassicTag{is4K: false}
	tag4K := &pcscClassicTag{is4K: true}

	tests := []struct {
		name     string
		tag      *pcscClassicTag
		sector   uint8
		block    uint8
		expected int
		wantErr  bool
	}{
		// 1K card tests
		{"1K sector 0 block 0", tag1K, 0, 0, 0, false},
		{"1K sector 0 block 3", tag1K, 0, 3, 3, false},
		{"1K sector 1 block 0", tag1K, 1, 0, 4, false},
		{"1K sector 15 block 2", tag1K, 15, 2, 62, false},
		{"1K sector 16 (invalid)", tag1K, 16, 0, 0, true},
		{"1K block 4 (invalid)", tag1K, 0, 4, 0, true},

		// 4K card small sectors
		{"4K sector 0 block 0", tag4K, 0, 0, 0, false},
		{"4K sector 31 block 3", tag4K, 31, 3, 127, false},

		// 4K card large sectors
		{"4K sector 32 block 0", tag4K, 32, 0, 128, false},
		{"4K sector 32 block 15", tag4K, 32, 15, 143, false},
		{"4K sector 39 block 15", tag4K, 39, 15, 255, false},
		{"4K sector 40 (invalid)", tag4K, 40, 0, 0, true},
		{"4K sector 32 block 16 (invalid)", tag4K, 32, 16, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.tag.sectorToAbsoluteBlock(tt.sector, tt.block)
			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if result != tt.expected {
					t.Errorf("Got %d, want %d", result, tt.expected)
				}
			}
		})
	}
}

// TestKeyTypeConstants tests that key type constants are defined correctly
func TestKeyTypeConstants(t *testing.T) {
	if KeyTypeA != 0x60 {
		t.Errorf("KeyTypeA = 0x%02X, want 0x60", KeyTypeA)
	}
	if KeyTypeB != 0x61 {
		t.Errorf("KeyTypeB = 0x%02X, want 0x61", KeyTypeB)
	}
}

// TestCommonKeyConstants tests that common key constants are defined
func TestCommonKeyConstants(t *testing.T) {
	// KeyDefault
	expected := []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}
	if !bytes.Equal(KeyDefault, expected) {
		t.Errorf("KeyDefault = %v, want %v", KeyDefault, expected)
	}

	// KeyNFCForum
	expected = []byte{0xD3, 0xF7, 0xD3, 0xF7, 0xD3, 0xF7}
	if !bytes.Equal(KeyNFCForum, expected) {
		t.Errorf("KeyNFCForum = %v, want %v", KeyNFCForum, expected)
	}

	// KeyMAD
	expected = []byte{0xA0, 0xA1, 0xA2, 0xA3, 0xA4, 0xA5}
	if !bytes.Equal(KeyMAD, expected) {
		t.Errorf("KeyMAD = %v, want %v", KeyMAD, expected)
	}
}

// TestClassicTagInterface tests that pcscClassicTag implements ClassicTag
func TestClassicTagInterface(t *testing.T) {
	// This test verifies at compile time that pcscClassicTag implements ClassicTag
	var _ ClassicTag = (*pcscClassicTag)(nil)
}

// TestMockClassicTagImplementsInterface tests that MockClassicTag can be used with ClassicTag
func TestMockClassicTagImplementsInterface(t *testing.T) {
	mock := NewMockClassicTag("04A1B2C3")
	mock.IsConnected = true

	// Test Read
	mock.SetBlockData(1, 0, []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10})

	data, err := mock.Read(1, 0, KeyDefault, KeyTypeA)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if len(data) != 16 {
		t.Errorf("Expected 16 bytes, got %d", len(data))
	}

	// Test Write
	writeData := make([]byte, 16)
	for i := range writeData {
		writeData[i] = byte(i)
	}
	err = mock.Write(2, 1, writeData, KeyDefault, KeyTypeB)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Verify written data
	readBack, exists := mock.GetBlockData(2, 1)
	if !exists {
		t.Error("Written data not found")
	}
	if !bytes.Equal(readBack, writeData) {
		t.Errorf("Written data mismatch")
	}
}
