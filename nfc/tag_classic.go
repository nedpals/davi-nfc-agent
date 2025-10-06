package nfc

import (
	"encoding/binary"
	"fmt"
	"log"

	"github.com/clausecker/freefare"
)

// classicAdapter implements ClassicTag for MIFARE Classic tags.
type classicAdapter struct {
	tag freefare.ClassicTag
}

// Ensure classicAdapter implements ClassicTag
var _ ClassicTag = (*classicAdapter)(nil)

// newClassicAdapter creates a new adapter for a MIFARE Classic tag.
func newClassicAdapter(tag freefare.ClassicTag) *classicAdapter {
	return &classicAdapter{tag: tag}
}

func (c *classicAdapter) UID() string {
	return c.tag.UID()
}

func (c *classicAdapter) Type() string {
	switch c.tag.Type() {
	case freefare.Classic1k:
		return "MIFARE Classic 1K"
	case freefare.Classic4k:
		return "MIFARE Classic 4K"
	default:
		return fmt.Sprintf("MIFARE Classic (type %d)", c.tag.Type())
	}
}

func (c *classicAdapter) NumericType() int {
	return int(c.tag.Type())
}

func (c *classicAdapter) GetFreefareTag() freefare.Tag {
	return c.tag
}

func (c *classicAdapter) Connect() error {
	return c.tag.Connect()
}

func (c *classicAdapter) Disconnect() error {
	return c.tag.Disconnect()
}

func (c *classicAdapter) Transceive(data []byte) ([]byte, error) {
	return nil, fmt.Errorf("Transceive not directly supported for classicAdapter; use Read/Write or device-level Transceive")
}

func (c *classicAdapter) Read(sector, block uint8, key []byte, keyType int) ([]byte, error) {
	if len(key) != 6 {
		return nil, fmt.Errorf("classicAdapter.Read: key length must be 6 bytes, got %d", len(key))
	}
	var keyArray [6]byte
	copy(keyArray[:], key)

	if err := c.tag.Connect(); err != nil {
		return nil, fmt.Errorf("classicAdapter.Read connect error: %w", err)
	}
	defer c.tag.Disconnect()

	trailerBlockNum := freefare.ClassicSectorLastBlock(sector)
	errAuth := c.tag.Authenticate(trailerBlockNum, keyArray, keyType)
	if errAuth != nil {
		return nil, fmt.Errorf("classicAdapter.Read authentication error for sector %d (auth block %d): %w", sector, trailerBlockNum, errAuth)
	}

	absoluteBlockNumber, errCalc := ClassicSectorBlockToLinear(c.NumericType(), sector, block)
	if errCalc != nil {
		return nil, fmt.Errorf("classicAdapter.Read error calculating absolute block number for sector %d, block %d: %w", sector, block, errCalc)
	}

	data, errRead := c.tag.ReadBlock(absoluteBlockNumber)
	if errRead != nil {
		return nil, fmt.Errorf("classicAdapter.Read error reading block %d (sector %d, rel block %d): %w", absoluteBlockNumber, sector, block, errRead)
	}
	return data[:], nil
}

func (c *classicAdapter) Write(sector, block uint8, data []byte, key []byte, keyType int) error {
	if len(data) != 16 {
		return fmt.Errorf("classicAdapter.Write error: data length must be 16 bytes, got %d", len(data))
	}
	if len(key) != 6 {
		return fmt.Errorf("classicAdapter.Write: key length must be 6 bytes, got %d", len(key))
	}
	var keyArray [6]byte
	copy(keyArray[:], key)

	if err := c.tag.Connect(); err != nil {
		return fmt.Errorf("classicAdapter.Write connect error: %w", err)
	}
	defer c.tag.Disconnect()

	trailerBlockNum := freefare.ClassicSectorLastBlock(sector)
	errAuth := c.tag.Authenticate(trailerBlockNum, keyArray, keyType)
	if errAuth != nil {
		return fmt.Errorf("classicAdapter.Write authentication error for sector %d (auth block %d): %w", sector, trailerBlockNum, errAuth)
	}

	absoluteBlockNumber, errCalc := ClassicSectorBlockToLinear(c.NumericType(), sector, block)
	if errCalc != nil {
		return fmt.Errorf("classicAdapter.Write error calculating absolute block number for sector %d, block %d: %w", sector, block, errCalc)
	}

	var dataArray [16]byte
	copy(dataArray[:], data)

	errWrite := c.tag.WriteBlock(absoluteBlockNumber, dataArray)
	if errWrite != nil {
		return fmt.Errorf("classicAdapter.Write error writing to block %d (sector %d, rel block %d): %w", absoluteBlockNumber, sector, block, errWrite)
	}
	return nil
}

func (c *classicAdapter) ReadData() ([]byte, error) {
	if err := c.tag.Connect(); err != nil {
		return nil, fmt.Errorf("classicAdapter.ReadData connect error: %w", err)
	}
	defer c.tag.Disconnect()

	mad, errMad := c.tag.ReadMad()
	if errMad != nil {
		madSector := byte(0x00)
		if c.NumericType() == int(freefare.Classic4k) {
			madSector = byte(0x10)
		}
		trailerBlockNum := freefare.ClassicSectorLastBlock(madSector)
		if errAuth := c.tag.Authenticate(trailerBlockNum, FactoryKey, int(freefare.KeyA)); errAuth == nil {
			log.Printf("classicAdapter.ReadData: MAD read failed (%v), but factory key auth succeeded. Assuming factory mode.", errMad)
			return nil, nil
		}
		return nil, fmt.Errorf("classicAdapter.ReadData MAD read error: %w", errMad)
	}

	buffer := make([]byte, 4096)
	bufLen, errReadApp := c.tag.ReadApplication(mad, freefare.MadNFCForumAid, buffer, PublicKey, int(freefare.KeyA))
	if errReadApp != nil {
		return nil, fmt.Errorf("classicAdapter.ReadData read NDEF application error: %w", errReadApp)
	}
	if bufLen == 0 {
		log.Println("classicAdapter.ReadData: No data in NDEF application.")
		return nil, nil
	}

	ndefApplicationData := buffer[:bufLen]

	// Parse TLV structure
	offset := 0
	for offset < len(ndefApplicationData) {
		if offset+1 > len(ndefApplicationData) {
			return nil, fmt.Errorf("TLV structure error at offset %d (type missing)", offset)
		}
		tlvType := ndefApplicationData[offset]

		if tlvType == 0x00 {
			offset++
			continue
		}
		if tlvType == 0xFE {
			break
		}

		lenFieldStart := offset + 1
		if lenFieldStart >= len(ndefApplicationData) {
			return nil, fmt.Errorf("TLV type 0x%X at offset %d: length field missing", tlvType, offset)
		}

		var msgLength int
		var lengthFieldSize int

		if ndefApplicationData[lenFieldStart] == 0xFF {
			if lenFieldStart+2 >= len(ndefApplicationData) {
				return nil, fmt.Errorf("TLV type 0x%X at offset %d: long format length bytes missing", tlvType, offset)
			}
			msgLength = int(binary.BigEndian.Uint16(ndefApplicationData[lenFieldStart+1 : lenFieldStart+3]))
			lengthFieldSize = 3
		} else {
			msgLength = int(ndefApplicationData[lenFieldStart])
			lengthFieldSize = 1
		}

		valueStart := lenFieldStart + lengthFieldSize
		if valueStart+msgLength > len(ndefApplicationData) {
			return nil, fmt.Errorf("TLV type 0x%X at offset %d: value (len %d) exceeds buffer bounds (app data len %d)", tlvType, offset, msgLength, len(ndefApplicationData))
		}

		message := ndefApplicationData[valueStart : valueStart+msgLength]

		if tlvType == 0x03 {
			return message, nil
		}
		offset = valueStart + msgLength
	}
	log.Println("classicAdapter.ReadData: No NDEF Message TLV (type 0x03) found.")
	return nil, nil
}

func (c *classicAdapter) IsWritable() (bool, error) {
	if err := c.tag.Connect(); err != nil {
		return false, fmt.Errorf("classicAdapter.IsWritable connect error: %w", err)
	}
	defer c.tag.Disconnect()

	// Check if sector 0 can be authenticated with factory key (indicates writable factory mode)
	sector0TrailerBlock := freefare.ClassicSectorLastBlock(0)
	if err := c.tag.Authenticate(sector0TrailerBlock, FactoryKey, int(freefare.KeyA)); err == nil {
		return true, nil // Factory mode - definitely writable
	}

	// Check if any NDEF sectors (1-15 for 1K, 1-39 for 4K) can be authenticated with write key
	maxSectorIdx := 15
	if c.NumericType() == int(freefare.Classic4k) {
		maxSectorIdx = 39
	}

	// Try to authenticate with PublicKey for NDEF sectors
	// If we can authenticate, we can potentially write (unless access bits prevent it)
	for sectorIdx := 1; sectorIdx <= maxSectorIdx; sectorIdx++ {
		currentSector := byte(sectorIdx)

		// Skip MAD sectors (0 and 16)
		if currentSector == 0 || (c.NumericType() == int(freefare.Classic4k) && currentSector == 16) {
			continue
		}

		trailerBlockNum := freefare.ClassicSectorLastBlock(currentSector)

		// Try authenticating with PublicKey (used for NDEF sectors)
		if err := c.tag.Authenticate(trailerBlockNum, PublicKey, int(freefare.KeyA)); err != nil {
			continue // Can't authenticate, skip this sector
		}

		// Successfully authenticated - check if we have write permissions
		// Try to read the trailer block to check access bits
		trailerData, err := c.tag.ReadBlock(trailerBlockNum)
		if err != nil {
			continue
		}

		// Parse access bits from trailer (bytes 6, 7, 8)
		// If all access bits are 0xFF (or 0x07 pattern), it's read-only
		// Otherwise, there are write permissions
		if trailerData[6] != 0xFF || trailerData[7] != 0x07 {
			return true, nil // Has write permissions
		}
	}

	return false, nil // No writable blocks found
}

func (c *classicAdapter) CanMakeReadOnly() (bool, error) {
	if err := c.tag.Connect(); err != nil {
		return false, fmt.Errorf("classicAdapter.CanMakeReadOnly connect error: %w", err)
	}
	defer c.tag.Disconnect()

	// To make a tag read-only, we need write access to sector trailers
	// Check if we have write permissions for at least one NDEF sector trailer

	maxSectorIdx := 15
	if c.NumericType() == int(freefare.Classic4k) {
		maxSectorIdx = 39
	}

	// Check NDEF sectors (1-15 for 1K, 1-39 for 4K, excluding MAD sectors)
	for sectorIdx := 1; sectorIdx <= maxSectorIdx; sectorIdx++ {
		currentSector := byte(sectorIdx)

		// Skip MAD sectors (0 and 16)
		if currentSector == 0 || (c.NumericType() == int(freefare.Classic4k) && currentSector == 16) {
			continue
		}

		trailerBlockNum := freefare.ClassicSectorLastBlock(currentSector)

		// Try to authenticate with PublicKey (Key A)
		if err := c.tag.Authenticate(trailerBlockNum, PublicKey, int(freefare.KeyA)); err != nil {
			continue // Can't authenticate, try next sector
		}

		// Try to read the trailer block to check if we have write access
		trailerData, err := c.tag.ReadBlock(trailerBlockNum)
		if err != nil {
			continue
		}

		// If we can read the trailer and it's not already locked to read-only,
		// we can potentially make it read-only
		// Check if access bits are not already set to read-only (0xFF, 0x07)
		if trailerData[6] != 0xFF || trailerData[7] != 0x07 {
			// Tag is not fully read-only yet, and we have access, so we can make it read-only
			return true, nil
		}
	}

	// All accessible sectors are already read-only or we can't access any sectors
	return false, nil
}

func (c *classicAdapter) MakeReadOnly() error {
	if err := c.tag.Connect(); err != nil {
		return fmt.Errorf("classicAdapter.MakeReadOnly connect error: %w", err)
	}
	defer c.tag.Disconnect()

	maxSectorIdx := 15
	if c.NumericType() == int(freefare.Classic4k) {
		maxSectorIdx = 39
	}

	// Lock all NDEF sectors (1-15 for 1K, 1-39 for 4K, excluding MAD sectors)
	for sectorIdx := 1; sectorIdx <= maxSectorIdx; sectorIdx++ {
		currentSector := byte(sectorIdx)

		// Skip MAD sectors (0 and 16)
		if currentSector == 0 || (c.NumericType() == int(freefare.Classic4k) && currentSector == 16) {
			continue
		}

		c.tag.Disconnect()
		if err := c.tag.Connect(); err != nil {
			return fmt.Errorf("MakeReadOnly: failed to connect for sector %d: %w", currentSector, err)
		}

		trailerBlockNum := freefare.ClassicSectorLastBlock(currentSector)

		// Try to authenticate with PublicKey (Key A)
		if err := c.tag.Authenticate(trailerBlockNum, PublicKey, int(freefare.KeyA)); err != nil {
			log.Printf("MakeReadOnly: failed to authenticate sector %d with PublicKey: %v. Skipping sector.", currentSector, err)
			continue
		}

		// Set trailer block to read-only mode
		// Access conditions: 0xFF for data blocks (read-only), 0x07 for trailer (read-only)
		// These access bits prevent any future writes
		trailerData := [16]byte{}
		MifareClassicTrailerBlock2(&trailerData, PublicKey, 0xFF, 0x07, 0x88, 0xC1, PublicKey)

		if err := c.tag.WriteBlock(trailerBlockNum, trailerData); err != nil {
			return fmt.Errorf("MakeReadOnly: failed to write read-only trailer for sector %d: %w", currentSector, err)
		}

		log.Printf("classicAdapter.MakeReadOnly: Locked sector %d to read-only", currentSector)
	}

	log.Println("classicAdapter.MakeReadOnly: Tag successfully locked to read-only mode")
	return nil
}

func (c *classicAdapter) WriteData(data []byte) error {
	if err := c.tag.Connect(); err != nil {
		return fmt.Errorf("classicAdapter.WriteData connect error: %w", err)
	}
	defer c.tag.Disconnect()

	sector0TrailerBlock := freefare.ClassicSectorLastBlock(0)
	authErr := c.tag.Authenticate(sector0TrailerBlock, FactoryKey, int(freefare.KeyA))

	if authErr == nil {
		log.Println("classicAdapter.WriteData: Card in factory mode. Initializing for NDEF...")
		maxSectorIdx := 15
		if c.NumericType() == int(freefare.Classic4k) {
			maxSectorIdx = 39
		}

		for sectorIdx := 0; sectorIdx <= maxSectorIdx; sectorIdx++ {
			currentSector := byte(sectorIdx)
			currentSectorTrailerBlock := freefare.ClassicSectorLastBlock(currentSector)

			c.tag.Disconnect()
			if errCon := c.tag.Connect(); errCon != nil {
				return fmt.Errorf("WriteData: failed to connect for sector %d init: %w", currentSector, errCon)
			}
			if errAuthSector := c.tag.Authenticate(currentSectorTrailerBlock, FactoryKey, int(freefare.KeyA)); errAuthSector != nil {
				return fmt.Errorf("WriteData: failed to authenticate sector %d with factory key: %w", currentSector, errAuthSector)
			}

			trailerData := [16]byte{}
			if currentSector == 0 || (c.NumericType() == int(freefare.Classic4k) && currentSector == 16) {
				MifareClassicTrailerBlock2(&trailerData, DefaultKeyA, 0x78, 0x77, 0x88, 0xC1, FactoryKey)
			} else {
				MifareClassicTrailerBlock2(&trailerData, PublicKey, 0x7F, 0x07, 0x88, 0x40, FactoryKey)
			}

			if errWrite := c.tag.WriteBlock(currentSectorTrailerBlock, trailerData); errWrite != nil {
				return fmt.Errorf("WriteData: failed to write trailer for sector %d: %w", currentSector, errWrite)
			}
			log.Printf("classicAdapter.WriteData: Initialized trailer for sector %d", currentSector)
		}
		log.Println("classicAdapter.WriteData: Card initialized from factory mode.")
	} else {
		log.Printf("classicAdapter.WriteData: Card not in factory mode (auth error: %v) or already initialized.", authErr)
	}

	if len(data) > 0 {
		log.Printf("classicAdapter.WriteData: Attempting to write NDEF data (%d bytes)...", len(data))

		ndefMsgLen := len(data)
		var tlvPayload []byte
		tlvPayload = append(tlvPayload, 0x03)
		if ndefMsgLen < 255 {
			tlvPayload = append(tlvPayload, byte(ndefMsgLen))
		} else {
			tlvPayload = append(tlvPayload, 0xFF)
			tlvPayload = append(tlvPayload, byte(ndefMsgLen>>8))
			tlvPayload = append(tlvPayload, byte(ndefMsgLen&0xFF))
		}
		tlvPayload = append(tlvPayload, data...)
		tlvPayload = append(tlvPayload, 0xFE)

		bytesWritten := 0
		totalBytesToWrite := len(tlvPayload)
		maxSectors := 15
		if c.NumericType() == int(freefare.Classic4k) {
			maxSectors = 39
		}

		for sectorIdx := 0; sectorIdx <= maxSectors && bytesWritten < totalBytesToWrite; sectorIdx++ {
			currentSector := byte(sectorIdx)

			if currentSector == 0 || (c.NumericType() == int(freefare.Classic4k) && currentSector == 16) {
				continue
			}

			c.tag.Disconnect()
			if errCon := c.tag.Connect(); errCon != nil {
				return fmt.Errorf("WriteData NDEF: failed to connect for sector %d: %w", currentSector, errCon)
			}

			trailerBlockNum := freefare.ClassicSectorLastBlock(currentSector)
			if errAuth := c.tag.Authenticate(trailerBlockNum, PublicKey, int(freefare.KeyA)); errAuth != nil {
				log.Printf("WriteData NDEF: failed to authenticate NDEF sector %d with PublicKey: %v. Skipping sector.", currentSector, errAuth)
				continue
			}
			log.Printf("WriteData NDEF: Authenticated NDEF sector %d with PublicKey", currentSector)

			numDataBlocksInSector := 3
			if c.NumericType() == int(freefare.Classic4k) && currentSector >= 32 {
				numDataBlocksInSector = 15
			}

			for blockIdx := 0; blockIdx < numDataBlocksInSector && bytesWritten < totalBytesToWrite; blockIdx++ {
				currentBlockInSector := byte(blockIdx)
				absoluteBlockNumber, errCalc := ClassicSectorBlockToLinear(c.NumericType(), currentSector, currentBlockInSector)
				if errCalc != nil {
					return fmt.Errorf("WriteData NDEF: error calculating absolute block for sector %d, block %d: %w", currentSector, currentBlockInSector, errCalc)
				}

				var blockData [16]byte
				chunkSize := 16
				remainingToWriteInPayload := totalBytesToWrite - bytesWritten
				if chunkSize > remainingToWriteInPayload {
					chunkSize = remainingToWriteInPayload
				}
				copy(blockData[:chunkSize], tlvPayload[bytesWritten:bytesWritten+chunkSize])

				if errWrite := c.tag.WriteBlock(absoluteBlockNumber, blockData); errWrite != nil {
					return fmt.Errorf("WriteData NDEF: failed to write to abs block %d (sector %d, rel block %d): %w", absoluteBlockNumber, currentSector, currentBlockInSector, errWrite)
				}
				log.Printf("WriteData NDEF: Wrote %d bytes to abs block %d (sector %d, rel block %d)", chunkSize, absoluteBlockNumber, currentSector, currentBlockInSector)
				bytesWritten += chunkSize
			}
		}

		if bytesWritten < totalBytesToWrite {
			return fmt.Errorf("WriteData NDEF: failed to write all NDEF data. Wrote %d of %d bytes. Card may be full or some sectors un-writable", bytesWritten, totalBytesToWrite)
		}

		log.Printf("classicAdapter.WriteData: Successfully wrote %d bytes of NDEF TLV data.", bytesWritten)
	} else if data != nil {
		log.Printf("classicAdapter.WriteData: Received empty data. Writing empty NDEF message TLV + Terminator.")
		emptyNdefPayload := []byte{0x03, 0x00, 0xFE}

		bytesWritten := 0
		totalBytesToWrite := len(emptyNdefPayload)
		maxSectors := 15
		if c.NumericType() == int(freefare.Classic4k) {
			maxSectors = 39
		}

		for sectorIdx := 0; sectorIdx <= maxSectors && bytesWritten < totalBytesToWrite; sectorIdx++ {
			currentSector := byte(sectorIdx)
			if currentSector == 0 || (c.NumericType() == int(freefare.Classic4k) && currentSector == 16) {
				continue
			}
			c.tag.Disconnect()
			if errCon := c.tag.Connect(); errCon != nil {
				return fmt.Errorf("WriteData NDEF (empty): failed to connect for sector %d: %w", currentSector, errCon)
			}
			trailerBlockNum := freefare.ClassicSectorLastBlock(currentSector)
			if errAuth := c.tag.Authenticate(trailerBlockNum, PublicKey, int(freefare.KeyA)); errAuth != nil {
				log.Printf("WriteData NDEF (empty): failed to authenticate NDEF sector %d with PublicKey: %v. Skipping sector.", currentSector, errAuth)
				continue
			}
			numDataBlocksInSector := 3
			if c.NumericType() == int(freefare.Classic4k) && currentSector >= 32 {
				numDataBlocksInSector = 15
			}
			for blockIdx := 0; blockIdx < numDataBlocksInSector && bytesWritten < totalBytesToWrite; blockIdx++ {
				currentBlockInSector := byte(blockIdx)
				absoluteBlockNumber, _ := ClassicSectorBlockToLinear(c.NumericType(), currentSector, currentBlockInSector)
				var blockData [16]byte
				chunkSize := 16
				remainingToWriteInPayload := totalBytesToWrite - bytesWritten
				if chunkSize > remainingToWriteInPayload {
					chunkSize = remainingToWriteInPayload
				}
				copy(blockData[:chunkSize], emptyNdefPayload[bytesWritten:bytesWritten+chunkSize])
				if errWrite := c.tag.WriteBlock(absoluteBlockNumber, blockData); errWrite != nil {
					return fmt.Errorf("WriteData NDEF (empty): failed to write to abs block %d: %w", absoluteBlockNumber, errWrite)
				}
				bytesWritten += chunkSize
			}
		}
		if bytesWritten < totalBytesToWrite {
			return fmt.Errorf("WriteData NDEF (empty): failed to write all data. Wrote %d of %d bytes", bytesWritten, totalBytesToWrite)
		}
		log.Printf("classicAdapter.WriteData: Successfully wrote empty NDEF message.")
	}

	if authErr != nil && data == nil {
		return fmt.Errorf("card not in factory mode (auth error: %w) and no data to write", authErr)
	}

	return nil
}
