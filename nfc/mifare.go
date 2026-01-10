package nfc

import (
	"fmt"
	"log"

	"github.com/clausecker/freefare"
)

// SearchSectorKey tries to find a key with full write permissions for a given sector.
// The tag provider's Connect/Disconnect methods are used internally.
// It now accepts ClassicTag instead of the more generic FreefareTagProvider.
func SearchSectorKey(tag ClassicTag, sector byte, foundKey *[6]byte, foundKeyType *int) error {
	block := freefare.ClassicSectorLastBlock(sector)

	// Get the underlying freefare.ClassicTag via the FreefareTagProvider interface embedded in ClassicTag
	rawTag := tag.GetFreefareTag()
	classicTag, ok := rawTag.(freefare.ClassicTag)
	if !ok {
		// This should ideally not happen if the ClassicTag implementation is correct
		return fmt.Errorf("SearchSectorKey: provided tag is not a MIFARE Classic tag internally")
	}

	for _, keyToTry := range DefaultKeys { // Uses DefaultKeys from keys.go
		// Try KeyA
		if err := classicTag.Connect(); err != nil {
			log.Printf("SearchSectorKey: connect error before KeyA auth with %X for sector %d: %v", keyToTry, sector, err)
			classicTag.Disconnect() // Attempt to clean up
			continue
		}
		authErr := classicTag.Authenticate(block, keyToTry, int(freefare.KeyA))
		if authErr == nil {
			permA, _ := classicTag.TrailerBlockPermission(block, uint16(freefare.WriteKeyA), int(freefare.KeyA))
			permAB, _ := classicTag.TrailerBlockPermission(block, uint16(freefare.WriteAccessBits), int(freefare.KeyA))
			permB, _ := classicTag.TrailerBlockPermission(block, uint16(freefare.WriteKeyB), int(freefare.KeyA))
			classicTag.Disconnect() // Disconnect after permission check
			if permA && permAB && permB {
				*foundKey = keyToTry
				*foundKeyType = int(freefare.KeyA)
				return nil
			}
		} else {
			classicTag.Disconnect() // Disconnect if auth failed
		}

		// Try KeyB
		if err := classicTag.Connect(); err != nil {
			log.Printf("SearchSectorKey: connect error before KeyB auth with %X for sector %d: %v", keyToTry, sector, err)
			classicTag.Disconnect()
			continue
		}
		authErr = classicTag.Authenticate(block, keyToTry, int(freefare.KeyB))
		if authErr == nil {
			permA, _ := classicTag.TrailerBlockPermission(block, uint16(freefare.WriteKeyA), int(freefare.KeyB))
			permAB, _ := classicTag.TrailerBlockPermission(block, uint16(freefare.WriteAccessBits), int(freefare.KeyB))
			permB, _ := classicTag.TrailerBlockPermission(block, uint16(freefare.WriteKeyB), int(freefare.KeyB))
			classicTag.Disconnect() // Disconnect after permission check
			if permA && permAB && permB {
				*foundKey = keyToTry
				*foundKeyType = int(freefare.KeyB)
				return nil
			}
		} else {
			classicTag.Disconnect() // Disconnect if auth failed
		}
	}
	return fmt.Errorf("no known authentication key with full permissions for sector %d", sector)
}

// MifareClassicTrailerBlock constructs a MIFARE Classic trailer block bytes based on abstract access conditions.
// keyA, keyB: 6-byte keys.
// ab0, ab1, ab2, abTb: 3-bit access conditions for data blocks 0, 1, 2 and sector trailer respectively.
// gpb: General Purpose Byte.
func MifareClassicTrailerBlock(trailer *[16]byte, keyA [6]byte, ab0, ab1, ab2, abTb uint8, gpb uint8, keyB [6]byte) {
	copy(trailer[0:6], keyA[:])

	// Convert 3-bit control values (abX) into the 12-bit access_bits format
	// where each abX = (C1 C2 C3) maps to specific bits in access_bits.
	// This interpretation is based on freefare's C utility mifare_classic_encode_access_conditions.
	// access_bits[0..2] = C1_0, C2_0, C3_0 for block 0
	// access_bits[3..5] = C1_1, C2_1, C3_1 for block 1
	// ...and so on.
	// Let's define access_bits such that bit 0 is C1_0, bit 1 is C2_0, bit 2 is C3_0,
	// bit 3 is C1_1, etc.
	var accessBits uint16 // Using uint16 for 12 bits
	accessBits |= uint16(ab0&0x7) << 0
	accessBits |= uint16(ab1&0x7) << 3
	accessBits |= uint16(ab2&0x7) << 6
	accessBits |= uint16(abTb&0x7) << 9

	// Encode these 12 accessBits into 3 bytes (trailer[6], trailer[7], trailer[8])
	// according to MIFARE Classic specification / freefare C code (mifare_classic_access_bits_to_bytes).
	// trailer[6] = ((~C1_1 & 0x1) << 7) | ((~C2_1 & 0x1) << 6) | ((~C3_1 & 0x1) << 5) | ((~C1_0 & 0x1) << 4) |
	//              (( C1_1 & 0x1) << 3) | (( C2_1 & 0x1) << 2) | (( C3_1 & 0x1) << 1) | (( C1_0 & 0x1) << 0)
	// This is complex. The freefare C code `mifare_classic_access_bits_to_bytes` is:
	// bytes[0] = (uint8_t) (((~access_bits >> 0) & 0x0F) << 4 | ((access_bits >>  0) & 0x0F)); // For bits 0-3 of access_bits
	// bytes[1] = (uint8_t) (((~access_bits >> 4) & 0x0F) << 4 | ((access_bits >>  4) & 0x0F)); // For bits 4-7 of access_bits
	// bytes[2] = (uint8_t) (((~access_bits >> 8) & 0x0F) << 4 | ((access_bits >>  8) & 0x0F)); // For bits 8-11 of access_bits
	// Here, access_bits is the 12-bit value.

	trailer[6] = byte(((^accessBits>>0)&0x0F)<<4 | ((accessBits >> 0) & 0x0F))
	trailer[7] = byte(((^accessBits>>4)&0x0F)<<4 | ((accessBits >> 4) & 0x0F))
	trailer[8] = byte(((^accessBits>>8)&0x0F)<<4 | ((accessBits >> 8) & 0x0F))

	trailer[9] = gpb
	copy(trailer[10:16], keyB[:])
}

// MifareClassicTrailerBlock2 sets trailer block bytes directly.
// accByte6, accByte7, accByte8 are the literal access bytes (trailer[6], trailer[7], trailer[8]).
// gpb is trailer[9]. This is used when the exact byte values for access conditions are known.
func MifareClassicTrailerBlock2(trailer *[16]byte, keyA [6]byte, accByte6, accByte7, accByte8, gpb uint8, keyB [6]byte) {
	copy(trailer[0:6], keyA[:])
	trailer[6] = accByte6
	trailer[7] = accByte7
	trailer[8] = accByte8
	trailer[9] = gpb
	copy(trailer[10:16], keyB[:])
}

// ClassicSectorBlockToLinear converts a sector and block number to a linear block number.
// This is necessary because MIFARE Classic 4K cards have different sector sizes.
func ClassicSectorBlockToLinear(tagType int, sector, block uint8) (byte, error) {
	if tagType == int(freefare.Classic1k) {
		if sector > 31 {
			return 0, fmt.Errorf("invalid sector %d for MIFARE Classic 1K (0-31)", sector)
		}
		if block > 3 {
			return 0, fmt.Errorf("invalid block %d for sector %d in MIFARE Classic 1K (0-3)", block, sector)
		}
		return (sector * 4) + block, nil
	} else if tagType == int(freefare.Classic4k) {
		if sector > 39 {
			return 0, fmt.Errorf("invalid sector %d for MIFARE Classic 4K (0-39)", sector)
		}
		if sector < 32 { // Sectors 0-31 have 4 blocks each
			if block > 3 {
				return 0, fmt.Errorf("invalid block %d for sector %d in MIFARE Classic 4K (0-3 for sectors 0-31)", block, sector)
			}
			return (sector * 4) + block, nil
		} else { // Sectors 32-39 have 16 blocks each
			if block > 15 {
				return 0, fmt.Errorf("invalid block %d for sector %d in MIFARE Classic 4K (0-15 for sectors 32-39)", block, sector)
			}
			// Blocks for sectors 0-31: 32 sectors * 4 blocks/sector = 128 blocks
			// Blocks for sectors 32 onwards: (sector - 32) * 16 blocks/sector
			return 128 + ((sector - 32) * 16) + block, nil
		}
	}
	return 0, fmt.Errorf("unsupported tag type for ClassicSectorBlockToLinear: %d", tagType)
}
