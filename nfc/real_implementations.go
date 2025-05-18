package nfc

import (
	"encoding/binary" // For RealClassicTagAdapter.ReadData TLV parsing
	"fmt"
	"log"
	"time" // For RealManager ListDevices retry

	"github.com/clausecker/freefare"
	"github.com/clausecker/nfc/v2"
)

// RealDevice implements DeviceInterface using an actual nfc.Device.
type RealDevice struct {
	device nfc.Device
}

func NewRealDevice(dev nfc.Device) *RealDevice {
	return &RealDevice{device: dev}
}

func (rd *RealDevice) Close() error {
	return rd.device.Close()
}

func (rd *RealDevice) InitiatorInit() error {
	return rd.device.InitiatorInit()
}

func (rd *RealDevice) String() string {
	return rd.device.String()
}

func (rd *RealDevice) Connection() string {
	return rd.device.Connection()
}

// RealClassicTagAdapter implements FreefareTagProvider using an actual freefare.ClassicTag.
type RealClassicTagAdapter struct {
	tag freefare.ClassicTag
}

func NewRealClassicTagAdapter(tag freefare.ClassicTag) *RealClassicTagAdapter {
	return &RealClassicTagAdapter{tag: tag}
}

func (rcta *RealClassicTagAdapter) UID() string {
	return rcta.tag.UID()
}

// Type returns a string representation of the tag type.
func (rcta *RealClassicTagAdapter) Type() string {
	switch rcta.tag.Type() {
	case freefare.Classic1k:
		return "MIFARE Classic 1K"
	case freefare.Classic4k:
		return "MIFARE Classic 4K"
	case freefare.Ultralight:
		return "MIFARE Ultralight"
	case freefare.UltralightC:
		return "MIFARE Ultralight C"
	case freefare.DESFire:
		return "MIFARE DESFire"
	default:
		return fmt.Sprintf("Unknown tag type: %d", rcta.tag.Type())
	}
}

// NumericType returns the integer representation of the tag type from freefare.
func (rcta *RealClassicTagAdapter) NumericType() int {
	return int(rcta.tag.Type())
}

// GetFreefareTag returns the underlying freefare.Tag.
func (rcta *RealClassicTagAdapter) GetFreefareTag() freefare.Tag {
	return rcta.tag
}

func (rcta *RealClassicTagAdapter) Connect() error {
	return rcta.tag.Connect()
}

func (rcta *RealClassicTagAdapter) Disconnect() error {
	return rcta.tag.Disconnect()
}

// Read implements the TagInterface Read method.
// It reads a specific block from a sector using the provided key.
func (rcta *RealClassicTagAdapter) Read(sector, block uint8, key []byte, keyType int) ([]byte, error) {
	if len(key) != 6 {
		return nil, fmt.Errorf("RealClassicTagAdapter.Read: key length must be 6 bytes, got %d", len(key))
	}
	var keyArray [6]byte
	copy(keyArray[:], key)

	if err := rcta.tag.Connect(); err != nil {
		return nil, fmt.Errorf("RealClassicTagAdapter.Read connect error: %w", err)
	}
	defer rcta.tag.Disconnect()

	trailerBlockNum := freefare.ClassicSectorLastBlock(sector)
	// Ensure trailerBlockNum is valid, though ClassicSectorLastBlock should handle typical sector values.
	// freefare.ClassicTag.Authenticate expects uint8 for block number.

	errAuth := rcta.tag.Authenticate(trailerBlockNum, keyArray, keyType)
	if errAuth != nil {
		return nil, fmt.Errorf("RealClassicTagAdapter.Read authentication error for sector %d (auth block %d): %w", sector, trailerBlockNum, errAuth)
	}

	absoluteBlockNumber, errCalc := ClassicSectorBlockToLinear(rcta.NumericType(), sector, block) // Use local util and NumericType
	if errCalc != nil {
		return nil, fmt.Errorf("RealClassicTagAdapter.Read error calculating absolute block number for sector %d, block %d: %w", sector, block, errCalc)
	}

	data, errRead := rcta.tag.ReadBlock(absoluteBlockNumber)
	if errRead != nil {
		return nil, fmt.Errorf("RealClassicTagAdapter.Read error reading block %d (sector %d, rel block %d): %w", absoluteBlockNumber, sector, block, errRead)
	}
	return data[:], nil
}

// Write implements the TagInterface Write method.
// It writes data to a specific block in a sector using the provided key.
func (rcta *RealClassicTagAdapter) Write(sector, block uint8, data []byte, key []byte, keyType int) error {
	if len(data) != 16 {
		return fmt.Errorf("RealClassicTagAdapter.Write error: data length must be 16 bytes, got %d", len(data))
	}
	if len(key) != 6 {
		return fmt.Errorf("RealClassicTagAdapter.Write: key length must be 6 bytes, got %d", len(key))
	}
	var keyArray [6]byte
	copy(keyArray[:], key)

	if err := rcta.tag.Connect(); err != nil {
		return fmt.Errorf("RealClassicTagAdapter.Write connect error: %w", err)
	}
	defer rcta.tag.Disconnect()

	trailerBlockNum := freefare.ClassicSectorLastBlock(sector)

	errAuth := rcta.tag.Authenticate(trailerBlockNum, keyArray, keyType)
	if errAuth != nil {
		return fmt.Errorf("RealClassicTagAdapter.Write authentication error for sector %d (auth block %d): %w", sector, trailerBlockNum, errAuth)
	}

	absoluteBlockNumber, errCalc := ClassicSectorBlockToLinear(rcta.NumericType(), sector, block) // Use local util and NumericType
	if errCalc != nil {
		return fmt.Errorf("RealClassicTagAdapter.Write error calculating absolute block number for sector %d, block %d: %w", sector, block, errCalc)
	}

	var dataArray [16]byte
	copy(dataArray[:], data)

	errWrite := rcta.tag.WriteBlock(absoluteBlockNumber, dataArray)
	if errWrite != nil {
		return fmt.Errorf("RealClassicTagAdapter.Write error writing to block %d (sector %d, rel block %d): %w", absoluteBlockNumber, sector, block, errWrite)
	}
	return nil
}

// ReadData reads the raw NDEF message from the MIFARE Classic tag.
func (rcta *RealClassicTagAdapter) ReadData() ([]byte, error) {
	if err := rcta.tag.Connect(); err != nil {
		return nil, fmt.Errorf("RealClassicTagAdapter.ReadData connect error: %w", err)
	}
	defer rcta.tag.Disconnect()

	mad, errMad := rcta.tag.ReadMad()
	if errMad != nil {
		// Check for factory mode
		madSector := byte(0x00)
		if rcta.NumericType() == int(freefare.Classic4k) {
			madSector = byte(0x10)
		}
		trailerBlockNum := freefare.ClassicSectorLastBlock(madSector)
		// Authenticate with factory key to check for factory mode
		if errAuth := rcta.tag.Authenticate(trailerBlockNum, FactoryKey, int(freefare.KeyA)); errAuth == nil {
			log.Printf("RealClassicTagAdapter.ReadData: MAD read failed (%v), but factory key auth succeeded. Assuming factory mode.", errMad)
			return nil, nil // Factory mode, no NDEF data yet
		}
		return nil, fmt.Errorf("RealClassicTagAdapter.ReadData MAD read error: %w (factory key auth also failed: %v)", errMad, errMad) // Propagate original MAD error
	}

	// Buffer for NDEF application data
	buffer := make([]byte, 4096)

	// Read NDEF application (AID 0x0003 for NFC Forum Application)
	bufLen, errReadApp := rcta.tag.ReadApplication(mad, freefare.MadNFCForumAid, buffer, PublicKey, int(freefare.KeyA))
	if errReadApp != nil {
		return nil, fmt.Errorf("RealClassicTagAdapter.ReadData read NDEF application error: %w", errReadApp)
	}
	if bufLen == 0 {
		log.Println("RealClassicTagAdapter.ReadData: No data in NDEF application.")
		return nil, nil
	}

	ndefApplicationData := buffer[:bufLen]

	// Parse TLV structure within the NDEF application data
	// Expecting an NDEF Message TLV (Type 0x03)
	offset := 0
	for offset < len(ndefApplicationData) {
		if offset+1 > len(ndefApplicationData) { // Need Type
			return nil, fmt.Errorf("TLV structure error at offset %d (type missing)", offset)
		}
		tlvType := ndefApplicationData[offset]

		if tlvType == 0x00 { // NULL TLV
			offset++
			continue
		}
		if tlvType == 0xFE { // Terminator TLV
			break
		}

		lenFieldStart := offset + 1
		if lenFieldStart >= len(ndefApplicationData) { // Need at least 1 byte for length
			return nil, fmt.Errorf("TLV type 0x%X at offset %d: length field missing", tlvType, offset)
		}

		var msgLength int
		var lengthFieldSize int

		if ndefApplicationData[lenFieldStart] == 0xFF { // Long format for length (3 bytes: 0xFF + 2 bytes length)
			if lenFieldStart+2 >= len(ndefApplicationData) { // Need 2 more bytes for length
				return nil, fmt.Errorf("TLV type 0x%X at offset %d: long format length bytes missing", tlvType, offset)
			}
			msgLength = int(binary.BigEndian.Uint16(ndefApplicationData[lenFieldStart+1 : lenFieldStart+3]))
			lengthFieldSize = 3
		} else { // Short format for length (1 byte)
			msgLength = int(ndefApplicationData[lenFieldStart])
			lengthFieldSize = 1
		}

		valueStart := lenFieldStart + lengthFieldSize
		if valueStart+msgLength > len(ndefApplicationData) {
			return nil, fmt.Errorf("TLV type 0x%X at offset %d: value (len %d) exceeds buffer bounds (app data len %d)", tlvType, offset, msgLength, len(ndefApplicationData))
		}

		message := ndefApplicationData[valueStart : valueStart+msgLength]

		if tlvType == 0x03 { // NDEF Message TLV
			return message, nil // Return the raw NDEF message
		}
		// Skip other TLV types for now, advance to next TLV
		offset = valueStart + msgLength
	}
	log.Println("RealClassicTagAdapter.ReadData: No NDEF Message TLV (type 0x03) found.")
	return nil, nil
}

// WriteData initializes a factory mode card for NDEF, or prepares for NDEF message writing.
// The `data` parameter should be the raw NDEF message to write.
// Full NDEF message writing to data blocks is complex and currently a placeholder.
func (rcta *RealClassicTagAdapter) WriteData(data []byte) error {
	if err := rcta.tag.Connect(); err != nil {
		return fmt.Errorf("RealClassicTagAdapter.WriteData connect error: %w", err)
	}
	defer rcta.tag.Disconnect()

	sector0TrailerBlock := freefare.ClassicSectorLastBlock(0)
	authErr := rcta.tag.Authenticate(sector0TrailerBlock, FactoryKey, int(freefare.KeyA))

	if authErr == nil { // Card is in factory mode
		log.Println("RealClassicTagAdapter.WriteData: Card in factory mode. Initializing for NDEF...")
		maxSectorIdx := 15 // Default for 1K card
		if rcta.NumericType() == int(freefare.Classic4k) {
			maxSectorIdx = 39 // For 4K card
		}

		for sectorIdx := 0; sectorIdx <= maxSectorIdx; sectorIdx++ {
			currentSector := byte(sectorIdx)
			currentSectorTrailerBlock := freefare.ClassicSectorLastBlock(currentSector)

			// Each sector operation might need its own connect/auth cycle.
			rcta.tag.Disconnect() // Disconnect before re-authenticating or moving to a new sector.
			if errCon := rcta.tag.Connect(); errCon != nil {
				return fmt.Errorf("WriteData: failed to connect for sector %d init: %w", currentSector, errCon)
			}
			if errAuthSector := rcta.tag.Authenticate(currentSectorTrailerBlock, FactoryKey, int(freefare.KeyA)); errAuthSector != nil {
				return fmt.Errorf("WriteData: failed to authenticate sector %d with factory key: %w", currentSector, errAuthSector)
			}

			trailerData := [16]byte{}
			// MAD sectors (0 for 1K/4K, 16 for 4K MAD2)
			if currentSector == 0 || (rcta.NumericType() == int(freefare.Classic4k) && currentSector == 16) {
				MifareClassicTrailerBlock2(&trailerData, DefaultKeyA, 0x78, 0x77, 0x88, 0xC1, FactoryKey) // GPB C1 for MAD
			} else { // Application sectors (for NDEF data)
				MifareClassicTrailerBlock2(&trailerData, PublicKey, 0x7F, 0x07, 0x88, 0x40, FactoryKey) // GPB 0x40 for NDEF
			}

			if errWrite := rcta.tag.WriteBlock(currentSectorTrailerBlock, trailerData); errWrite != nil {
				return fmt.Errorf("WriteData: failed to write trailer for sector %d: %w", currentSector, errWrite)
			}
			log.Printf("RealClassicTagAdapter.WriteData: Initialized trailer for sector %d", currentSector)
		}
		log.Println("RealClassicTagAdapter.WriteData: Card initialized from factory mode.")
		// After initialization, if data is provided, proceed to write it.
	} else {
		// Not factory mode, or auth failed. If data is present, we might try to write it directly.
		log.Printf("RealClassicTagAdapter.WriteData: Card not in factory mode (auth error: %v) or already initialized.", authErr)
	}

	// Actual NDEF message writing part (if `data` is not nil)
	if data != nil && len(data) > 0 {
		log.Printf("RealClassicTagAdapter.WriteData: Attempting to write NDEF data (%d bytes)...", len(data))

		// 1. Construct NDEF Message TLV + Terminator TLV
		ndefMsgLen := len(data)
		var tlvPayload []byte
		tlvPayload = append(tlvPayload, 0x03) // NDEF Message TLV type
		if ndefMsgLen < 255 {
			tlvPayload = append(tlvPayload, byte(ndefMsgLen)) // Length (1 byte)
		} else {
			tlvPayload = append(tlvPayload, 0xFF) // Length (3 bytes: FF + 2 bytes actual length)
			tlvPayload = append(tlvPayload, byte(ndefMsgLen>>8))
			tlvPayload = append(tlvPayload, byte(ndefMsgLen&0xFF))
		}
		tlvPayload = append(tlvPayload, data...) // The NDEF message itself
		tlvPayload = append(tlvPayload, 0xFE)    // Terminator TLV

		bytesWritten := 0
		totalBytesToWrite := len(tlvPayload)
		maxSectors := 15 // Default for 1K
		if rcta.NumericType() == int(freefare.Classic4k) {
			maxSectors = 39
		}

		for sectorIdx := 0; sectorIdx <= maxSectors && bytesWritten < totalBytesToWrite; sectorIdx++ {
			currentSector := byte(sectorIdx)

			// Skip MAD sectors (0 for 1K/4K, 16 for 4K MAD2)
			if currentSector == 0 || (rcta.NumericType() == int(freefare.Classic4k) && currentSector == 16) {
				continue
			}

			// Disconnect and reconnect before authenticating the new NDEF sector with PublicKey.
			// This ensures a clean authentication state.
			rcta.tag.Disconnect()
			if errCon := rcta.tag.Connect(); errCon != nil {
				return fmt.Errorf("WriteData NDEF: failed to connect for sector %d: %w", currentSector, errCon)
			}

			trailerBlockNum := freefare.ClassicSectorLastBlock(currentSector)
			if errAuth := rcta.tag.Authenticate(trailerBlockNum, PublicKey, int(freefare.KeyA)); errAuth != nil {
				log.Printf("WriteData NDEF: failed to authenticate NDEF sector %d with PublicKey: %v. Skipping sector.", currentSector, errAuth)
				// If a sector auth fails, we might not be able to write the full message.
				// Depending on desired behavior, could return error here or try to continue.
				// For now, log and skip, the final check `bytesWritten < totalBytesToWrite` will catch it.
				continue
			}
			log.Printf("WriteData NDEF: Authenticated NDEF sector %d with PublicKey", currentSector)

			numDataBlocksInSector := 3 // Blocks 0, 1, 2 for normal sectors
			if rcta.NumericType() == int(freefare.Classic4k) && currentSector >= 32 {
				numDataBlocksInSector = 15 // Blocks 0-14 for large sectors in 4K card (sectors 32-39)
			}

			for blockIdx := 0; blockIdx < numDataBlocksInSector && bytesWritten < totalBytesToWrite; blockIdx++ {
				currentBlockInSector := byte(blockIdx)
				absoluteBlockNumber, errCalc := ClassicSectorBlockToLinear(rcta.NumericType(), currentSector, currentBlockInSector)
				if errCalc != nil {
					// This error should ideally not happen if sector/block logic is correct
					return fmt.Errorf("WriteData NDEF: error calculating absolute block for sector %d, block %d: %w", currentSector, currentBlockInSector, errCalc)
				}

				var blockData [16]byte // Initialize with all zeros
				chunkSize := 16
				remainingToWriteInPayload := totalBytesToWrite - bytesWritten
				if chunkSize > remainingToWriteInPayload {
					chunkSize = remainingToWriteInPayload
				}
				copy(blockData[:chunkSize], tlvPayload[bytesWritten:bytesWritten+chunkSize])
				// If chunkSize < 16, the rest of blockData remains 0x00, padding the block.

				if errWrite := rcta.tag.WriteBlock(absoluteBlockNumber, blockData); errWrite != nil {
					return fmt.Errorf("WriteData NDEF: failed to write to abs block %d (sector %d, rel block %d): %w", absoluteBlockNumber, currentSector, currentBlockInSector, errWrite)
				}
				log.Printf("WriteData NDEF: Wrote %d bytes to abs block %d (sector %d, rel block %d)", chunkSize, absoluteBlockNumber, currentSector, currentBlockInSector)
				bytesWritten += chunkSize
			}
		}

		if bytesWritten < totalBytesToWrite {
			return fmt.Errorf("WriteData NDEF: failed to write all NDEF data. Wrote %d of %d bytes. Card may be full or some sectors un-writable", bytesWritten, totalBytesToWrite)
		}

		log.Printf("RealClassicTagAdapter.WriteData: Successfully wrote %d bytes of NDEF TLV data.", bytesWritten)
		// NDEF data written successfully, the function will return nil later if no other errors occurred
	} else if data != nil { // data is not nil, but len(data) == 0. Write empty NDEF message.
		log.Printf("RealClassicTagAdapter.WriteData: Received empty data. Writing empty NDEF message TLV + Terminator.")
		// This is effectively the same as data != nil && len(data) > 0 with an empty `data` array.
		// The logic above handles len(data) == 0 correctly by creating a TLV for an empty message.
		// To avoid code duplication, we can call this function recursively, or slightly restructure.
		// For now, let's ensure the above block is hit for data=[]byte{}.
		// The condition `if data != nil && len(data) > 0` will not be met for `len(data) == 0`.
		// We need to handle the case of writing an "empty" NDEF message (TLV type 0x03, length 0, value empty, then terminator 0xFE)
		// This is a specific sequence: [0x03, 0x00, 0xFE]
		emptyNdefPayload := []byte{0x03, 0x00, 0xFE}

		// Re-using parts of the logic above, simplified for this specific payload
		bytesWritten := 0
		totalBytesToWrite := len(emptyNdefPayload)
		maxSectors := 15
		if rcta.NumericType() == int(freefare.Classic4k) {
			maxSectors = 39
		}

		for sectorIdx := 0; sectorIdx <= maxSectors && bytesWritten < totalBytesToWrite; sectorIdx++ {
			currentSector := byte(sectorIdx)
			if currentSector == 0 || (rcta.NumericType() == int(freefare.Classic4k) && currentSector == 16) {
				continue
			}
			rcta.tag.Disconnect()
			if errCon := rcta.tag.Connect(); errCon != nil {
				return fmt.Errorf("WriteData NDEF (empty): failed to connect for sector %d: %w", currentSector, errCon)
			}
			trailerBlockNum := freefare.ClassicSectorLastBlock(currentSector)
			if errAuth := rcta.tag.Authenticate(trailerBlockNum, PublicKey, int(freefare.KeyA)); errAuth != nil {
				log.Printf("WriteData NDEF (empty): failed to authenticate NDEF sector %d with PublicKey: %v. Skipping sector.", currentSector, errAuth)
				continue
			}
			numDataBlocksInSector := 3
			if rcta.NumericType() == int(freefare.Classic4k) && currentSector >= 32 {
				numDataBlocksInSector = 15
			}
			for blockIdx := 0; blockIdx < numDataBlocksInSector && bytesWritten < totalBytesToWrite; blockIdx++ {
				currentBlockInSector := byte(blockIdx)
				absoluteBlockNumber, _ := ClassicSectorBlockToLinear(rcta.NumericType(), currentSector, currentBlockInSector) // Error already checked broadly
				var blockData [16]byte
				chunkSize := 16
				remainingToWriteInPayload := totalBytesToWrite - bytesWritten
				if chunkSize > remainingToWriteInPayload {
					chunkSize = remainingToWriteInPayload
				}
				copy(blockData[:chunkSize], emptyNdefPayload[bytesWritten:bytesWritten+chunkSize])
				if errWrite := rcta.tag.WriteBlock(absoluteBlockNumber, blockData); errWrite != nil {
					return fmt.Errorf("WriteData NDEF (empty): failed to write to abs block %d: %w", absoluteBlockNumber, errWrite)
				}
				bytesWritten += chunkSize
			}
		}
		if bytesWritten < totalBytesToWrite {
			return fmt.Errorf("WriteData NDEF (empty): failed to write all data. Wrote %d of %d bytes", bytesWritten, totalBytesToWrite)
		}
		log.Printf("RealClassicTagAdapter.WriteData: Successfully wrote empty NDEF message.")
	}

	// If factory auth failed AND no data was provided (or data was empty and successfully written), return the original auth error.
	if authErr != nil && data == nil {
		return fmt.Errorf("card not in factory mode (auth error: %w) and no data to write", authErr)
	}
	// If factory auth failed AND data was provided but writing it failed, that error would have been returned already.
	// If factory auth succeeded (or card not in factory mode but authErr was for something else like already initialized)
	// AND data writing succeeded (or no data to write), then return nil.

	return nil // Success if initialized, or NDEF written, or no data to write and not a blocking factory auth error
}

// FreefareTagProvider specific methods (pass-through to freefare.ClassicTag)
func (rcta *RealClassicTagAdapter) ReadMad() (*freefare.Mad, error) {
	return rcta.tag.ReadMad()
}

func (rcta *RealClassicTagAdapter) ReadApplication(mad *freefare.Mad, aid freefare.MadAid, buffer []byte, key [6]byte, keyType int) (int, error) {
	return rcta.tag.ReadApplication(mad, aid, buffer, key, keyType)
}

func (rcta *RealClassicTagAdapter) Authenticate(block byte, key [6]byte, keyType int) error {
	return rcta.tag.Authenticate(block, key, keyType)
}

func (rcta *RealClassicTagAdapter) TrailerBlockPermission(block byte, perm uint16, keyType int) (bool, error) {
	return rcta.tag.TrailerBlockPermission(block, perm, keyType)
}

func (rcta *RealClassicTagAdapter) WriteBlock(block byte, data [16]byte) error {
	return rcta.tag.WriteBlock(block, data)
}

// RealManager implements ManagerInterface using actual nfc and freefare libraries.
type RealManager struct{}

func NewRealManager() *RealManager {
	return &RealManager{}
}

func (rm *RealManager) OpenDevice(deviceStr string) (DeviceInterface, error) {
	dev, err := nfc.Open(deviceStr)
	if err != nil {
		return nil, err
	}
	return NewRealDevice(dev), nil
}

func (rm *RealManager) ListDevices() ([]string, error) {
	var devices []string
	var err error
	// Using DeviceEnumRetries from common.go (implicitly, as it's in same package nfc)
	for i := 0; i < DeviceEnumRetries; i++ {
		devices, err = nfc.ListDevices()
		if err == nil {
			return devices, nil
		}
		time.Sleep(time.Millisecond * 100)
	}
	return nil, fmt.Errorf("failed to list NFC devices after %d retries: %w", DeviceEnumRetries, err)
}

// GetTags returns a list of FreefareTagProvider, currently filtering for MIFARE Classic.
func (rm *RealManager) GetTags(dev DeviceInterface) ([]TagInterface, error) {
	realDevWrapper, ok := dev.(*RealDevice)
	if !ok {
		// This case should ideally not happen if DeviceInterface is correctly managed.
		// If dev is not *RealDevice, we cannot get the underlying nfc.Device.
		return nil, fmt.Errorf("GetTags requires a *RealDevice instance to access the underlying nfc.Device for freefare.GetTags")
	}

	tags, err := freefare.GetTags(realDevWrapper.device)
	if err != nil {
		return nil, err
	}

	var result []TagInterface
	for _, tag := range tags {
		if classicTag, ok := tag.(freefare.ClassicTag); ok {
			result = append(result, NewRealClassicTagAdapter(classicTag))
		}
		// Future: Add adapters for other tag types (e.g., Desfire, Ultralight)
		// The ManagerInterface.GetTags would then return []TagInterface.
	}
	return result, nil
}
