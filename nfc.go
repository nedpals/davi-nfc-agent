package main

import (
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"strings"
	"sync"
	"time"
	"unicode/utf16"

	"github.com/clausecker/freefare"
	"github.com/clausecker/nfc/v2"
)

const (
	maxRetries          = 5
	baseDelay           = 500 * time.Millisecond
	maxReconnectTries   = 10
	reconnectDelay      = time.Second * 2
	deviceCheckInterval = time.Second * 2 // Interval to check for new devices
	deviceEnumRetries   = 3               // Number of retries for device enumeration
)

var (
	defaultKeyA = [6]byte{0xa0, 0xa1, 0xa2, 0xa3, 0xa4, 0xa5} // MiFare Application default key a
	defaultKeyB = [6]byte{0xd3, 0xf7, 0xd3, 0xf7, 0xd3, 0xf7} // NFC Forum default key B
	factoryKey  = [6]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff} // Factory default key
	publicKey   = defaultKeyB
	defaultKeys = [][6]byte{
		{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, // Factory default key
		{0xd3, 0xf7, 0xd3, 0xf7, 0xd3, 0xf7}, // NFC Forum default key B
		{0xa0, 0xa1, 0xa2, 0xa3, 0xa4, 0xa5}, // MiFare Application default key A
		{0xb0, 0xb1, 0xb2, 0xb3, 0xb4, 0xb5},
		{0x4d, 0x3a, 0x99, 0xc3, 0x51, 0xdd},
		{0x1a, 0x98, 0x2c, 0x7e, 0x45, 0x9a},
		{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff},
		{0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
	}
)

type contextKey string

const readerContextKey contextKey = "nfcReader"

// NFCData represents the data read from an NFC tag including any potential errors.
type NFCData struct {
	UID  string
	Text string
	Err  error
}

// tagCache provides thread-safe caching of NFC tag data and tracks the last successful scan.
type tagCache struct {
	lastSeen     map[string]string // map[UID]Text
	lastUID      string            // Most recently scanned valid UID
	lastText     string            // Most recently scanned valid text
	mu           sync.RWMutex
	lastSeenTime time.Time
}

// newTagCache creates and initializes a new tagCache instance.
func newTagCache() *tagCache {
	return &tagCache{
		lastSeen:     make(map[string]string),
		lastSeenTime: time.Time{},
	}
}

// getLastScanned returns the UID and text of the last successfully scanned tag.
func (c *tagCache) getLastScanned() (string, string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastUID, c.lastText
}

// hasChanged checks if the given UID and text combination differs from the cached version
// and updates the cache if it has changed. It returns true if the data has changed.
func (c *tagCache) hasChanged(uid, text string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Always update lastSeenTime for any valid card detection
	c.lastSeenTime = time.Now()

	// For factory mode cards, cache UID but no text
	if text == "" {
		c.lastUID = uid
		// Only report change if it's a different card
		_, exists := c.lastSeen[uid]
		return !exists
	}

	lastText, exists := c.lastSeen[uid]
	if !exists || lastText != text {
		c.lastSeen[uid] = text
		c.lastUID = uid
		c.lastText = text
		return true
	}
	return false
}

// clear removes all entries from the cache and resets the last scanned data.
func (c *tagCache) clear() {
	c.mu.Lock()
	c.lastSeen = make(map[string]string)
	c.lastUID = ""
	c.lastText = ""
	c.lastSeenTime = time.Time{}
	c.mu.Unlock()
}

// isCardPresent checks if a card is still present based on the last seen time.
func (c *tagCache) isCardPresent() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return !c.lastSeenTime.IsZero() && time.Since(c.lastSeenTime) < time.Second
}

// NFCReader manages NFC device interactions and broadcasts tag data to connected clients.
type NFCReader struct {
	// device is the underlying NFC device
	device nfc.Device
	// hasDevice indicates if the device is currently connected
	hasDevice bool
	// dataChan is used to broadcast NFC tag data
	dataChan chan NFCData
	// stopChan signals the worker to stop
	stopChan chan struct{}
	// cache stores previously seen tag data
	cache *tagCache
	// deviceStatus tracks the connection status of the device
	deviceStatus DeviceStatus
	// statusMux provides thread-safe access to deviceStatus
	statusMux sync.RWMutex
	// scanner provides device discovery and monitoring capabilities
	scanner *DeviceScanner
	// devicePath stores the path of the connected device
	devicePath string
	// cardPresent indicates if a card is currently present
	cardPresent bool
	// isWriting tracks write operations
	isWriting bool
	// operationMutex protects all tag operations
	operationMutex sync.Mutex
	// operationTimeout is the timeout for tag operations
	operationTimeout time.Duration
}

// NewNFCReader creates and initializes a new NFCReader instance.
// It attempts to open the NFC device specified by deviceStr.
func NewNFCReader(deviceStr string) (*NFCReader, error) {
	scanner := NewDeviceScanner()
	reader := &NFCReader{
		hasDevice:        false,
		dataChan:         make(chan NFCData),
		stopChan:         make(chan struct{}),
		cache:            newTagCache(),
		scanner:          scanner,
		devicePath:       deviceStr,
		cardPresent:      false,
		deviceStatus:     DeviceStatus{Connected: false, Message: "No device connected", CardPresent: false},
		operationTimeout: 5 * time.Second,
	}

	// If no device specified, try to find one
	if deviceStr == "" {
		devices := scanner.scanDevices()
		if len(devices) > 0 {
			deviceStr = devices[0]
		}
	}

	device, err := nfc.Open(deviceStr)
	if err != nil {
		return reader, nil
	}

	reader.device = device
	reader.hasDevice = true
	reader.deviceStatus = DeviceStatus{Connected: true, Message: "Device connected", CardPresent: false}

	return reader, nil
}

// Close releases resources and stops the NFC reader worker.
func (r *NFCReader) Close() {
	close(r.stopChan)
	r.device.Close()
}

// Start begins the NFC reading process in a separate goroutine.
func (r *NFCReader) Start() {
	go r.worker()
}

// Data returns a channel that provides NFCData as tags are read.
func (r *NFCReader) Data() <-chan NFCData {
	return r.dataChan
}

// isTimeoutError checks if an error is related to USB timeout or device communication timeout.
func isTimeoutError(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "Operation timed out") ||
		strings.Contains(err.Error(), "operation timed out") ||
		strings.Contains(err.Error(), "Unable to write to USB")) || strings.Contains(err.Error(), "timeout")
}

// isDeviceClosedError checks if an error indicates the device has been closed.
func isDeviceClosedError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "device closed")
}

// isIOError checks if an error is related to device I/O problems (usually disconnection)
func isIOError(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "input / output error") ||
		strings.Contains(err.Error(), "Input/output error") ||
		strings.Contains(err.Error(), "i/o error") ||
		strings.Contains(err.Error(), "broken pipe") ||
		strings.Contains(err.Error(), "Operation not permitted"))
}

// isDeviceConfigError checks if an error is related to device configuration issues
func isDeviceConfigError(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "device not configured") ||
		strings.Contains(err.Error(), "Device not configured") ||
		strings.Contains(err.Error(), "Unable to write to USB") ||
		strings.Contains(err.Error(), "RDR_to_PC_DataBlock"))
}

// reconnect attempts to re-establish connection with the NFC device.
// It will retry up to maxReconnectTries times with delays between attempts.
func (r *NFCReader) reconnect() error {
	// Clear cache and reset card presence state
	r.cache.clear()
	r.setCardPresent(false)

	for attempt := 1; attempt <= maxReconnectTries; attempt++ {
		if r.hasDevice {
			r.hasDevice = false
			r.device.Close()
		}

		r.setDeviceStatus(false, fmt.Sprintf("Attempting to reconnect (attempt %d/%d)", attempt, maxReconnectTries))

		err := r.tryConnect()
		if err == nil {
			// Only verify device presence without attempting to read
			tags, err := r.getTags()
			if err == nil && len(tags) > 0 {
				r.setCardPresent(true)
			}
			return nil
		}

		log.Printf("Reconnection attempt %d failed: %v", attempt, err)
		time.Sleep(reconnectDelay)
	}

	r.setDeviceStatus(false, "Failed to reconnect after multiple attempts")
	return fmt.Errorf("failed to reconnect after %d attempts", maxReconnectTries)
}

// forceReconnectDevice attempts to force a device reconnection by fully closing and reopening
func (r *NFCReader) forceReconnectDevice() error {
	if r.hasDevice {
		r.device.Close()
		r.hasDevice = false

		// Add a longer delay for ACR122 readers to fully reset
		log.Println("Waiting for device to reset completely...")
		time.Sleep(time.Second * 3)
	}

	// Clear all state
	r.cache.clear()
	r.setCardPresent(false)

	// Try to reconnect with multiple attempts
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		log.Printf("Force reconnect attempt %d/3...", attempt+1)

		if err := r.tryConnect(); err != nil {
			lastErr = err
			time.Sleep(time.Second)
			continue
		}

		// If we successfully reconnected, try to initialize device
		if err := r.device.InitiatorInit(); err != nil {
			log.Printf("Device initialized but InitiatorInit failed: %v", err)
			r.device.Close()
			r.hasDevice = false
			lastErr = err
			time.Sleep(time.Second)
			continue
		}

		log.Println("Force reconnect successful")
		return nil
	}

	return fmt.Errorf("force reconnect failed after multiple attempts: %v", lastErr)
}

// worker is the main processing loop for reading NFC tags.
// It handles device errors, reconnection, and broadcasts tag data.
func (r *NFCReader) worker() {
	retryCount := 0
	deviceCheckTicker := time.NewTicker(deviceCheckInterval)
	cardCheckTicker := time.NewTicker(100 * time.Millisecond)
	cooldownTimer := time.NewTimer(0)
	<-cooldownTimer.C // Consume the initial timer event
	inCooldown := false

	defer deviceCheckTicker.Stop()
	defer cardCheckTicker.Stop()
	defer cooldownTimer.Stop()

	for {
		select {
		case <-r.stopChan:
			return
		case <-deviceCheckTicker.C:
			if !r.hasDevice && !inCooldown {
				if err := r.tryConnect(); err != nil {
					continue
				}
				retryCount = 0
			}
		case <-cardCheckTicker.C:
			// Check card presence based on cache
			cardPresent := r.cache.isCardPresent()
			r.setCardPresent(cardPresent)
		case <-cooldownTimer.C:
			inCooldown = false
			log.Println("Device cooldown period ended, attempting to reconnect")
			if err := r.forceReconnectDevice(); err != nil {
				log.Printf("Reconnection after cooldown failed: %v", err)
			}
		default:
			if !r.hasDevice || inCooldown {
				time.Sleep(deviceCheckInterval)
				continue
			}

			// Skip reading if a write operation is in progress
			if r.isWriting {
				time.Sleep(50 * time.Millisecond)
				continue
			}

			tags, err := r.getTags()

			if err != nil {
				if isIOError(err) || isDeviceConfigError(err) {
					// Immediate device disconnection handling
					log.Printf("Device error detected: %v", err)
					r.hasDevice = false
					r.device.Close()
					r.setDeviceStatus(false, fmt.Sprintf("Device error: %v", err))
					r.isWriting = false // Reset writing state on disconnect

					// Enable cooldown to avoid rapid reconnection attempts with ACR122
					if strings.Contains(err.Error(), "Operation not permitted") ||
						strings.Contains(err.Error(), "broken pipe") ||
						strings.Contains(err.Error(), "RDR_to_PC_DataBlock") {
						inCooldown = true
						cooldownPeriod := time.Second * 10 // Longer cooldown for ACR122 specific errors
						log.Printf("ACR122 specific error detected. Entering cooldown for %v", cooldownPeriod)
						cooldownTimer.Reset(cooldownPeriod)
						continue
					}

					// Force reconnection attempt with delay
					time.Sleep(time.Second * 2)
					if err := r.forceReconnectDevice(); err != nil {
						log.Printf("Force reconnection failed: %v", err)
						time.Sleep(deviceCheckInterval)
					}
					continue
				}

				if isTimeoutError(err) || isDeviceClosedError(err) {
					delay := time.Duration(math.Pow(2, float64(retryCount))) * baseDelay
					if retryCount < maxRetries {
						retryCount++
						log.Printf("Device error detected, attempt %d/%d. Retrying in %v...",
							retryCount, maxRetries, delay)
						time.Sleep(delay)
						r.isWriting = false // Reset writing state on error

						if err := r.reconnect(); err != nil {
							log.Printf("Device reconnection failed: %v", err)
							continue
						}
						retryCount = 0
						continue
					}
				}

				// For other errors, notify clients but don't treat as disconnection
				r.dataChan <- NFCData{Err: fmt.Errorf("get tags error: %v", err)}
				time.Sleep(time.Second)
				continue
			}

			// Process tags without immediate presence update
			if len(tags) > 0 {
				for _, tag := range tags {
					if tag.Type() != freefare.Classic1k && tag.Type() != freefare.Classic4k {
						continue
					}

					classicTag, ok := tag.(freefare.ClassicTag)
					if !ok {
						continue
					}

					uid := classicTag.UID()
					text, err := r.readTagData(classicTag)

					// Update card presence regardless of read result
					r.setCardPresent(true)

					if err != nil {
						r.dataChan <- NFCData{
							UID:  uid,
							Text: "",
							Err:  err,
						}
						continue
					}

					if r.cache.hasChanged(uid, text) {
						r.dataChan <- NFCData{
							UID:  uid,
							Text: text,
							Err:  nil,
						}
					}
				}
			} else {
				r.setCardPresent(false)
			}

			time.Sleep(50 * time.Millisecond)
		}
	}
}

// setDeviceStatus updates the device status and broadcasts it to clients.
func (r *NFCReader) setDeviceStatus(connected bool, message string) {
	r.statusMux.Lock()
	r.deviceStatus = DeviceStatus{Connected: connected, Message: message}
	r.statusMux.Unlock()
	broadcastDeviceStatus(r.deviceStatus)
}

// getDeviceStatus returns the current device status.
func (r *NFCReader) getDeviceStatus() DeviceStatus {
	r.statusMux.RLock()
	defer r.statusMux.RUnlock()
	return r.deviceStatus
}

// readData reads and decodes NDEF formatted text from a Mifare Classic tag.
func readData(tag freefare.ClassicTag) (string, error) {
	if err := tag.Connect(); err != nil {
		return "", fmt.Errorf("readData connect error: %v", err)
	}
	defer tag.Disconnect()

	mad, err := tag.ReadMad()
	if err != nil {
		if err := tag.Connect(); err != nil {
			return "", fmt.Errorf("MAD connect error: %v", err)
		}

		// Try factory key authentication to confirm factory mode
		madSector := byte(0x00)
		if tag.Type() == freefare.Classic4k {
			madSector = byte(0x10)
		}

		if err := tag.Authenticate(freefare.ClassicSectorLastBlock(madSector), factoryKey, freefare.KeyA); err == nil {
			// Card is confirmed to be in factory mode
			return "", nil
		}
		return "", fmt.Errorf("MAD read error: %v", err)
	}

	buffer := make([]byte, 4096)

	bufLen, err := tag.ReadApplication(mad, freefare.MadNFCForumAid, buffer, publicKey, freefare.KeyA)
	if err != nil {
		// return "", fmt.Errorf("card is in factory mode")
		return "", fmt.Errorf("readData read application error: %v", err)
	}

	buffer = buffer[:bufLen]
	var ndefMessage []byte

	// First, extract the NDEF message from TLV blocks
loop:
	for offset := 0; offset < len(buffer); {
		message, tlvType := freefare.TLVdecode(buffer[offset:])
		msgLen := len(message)
		offset += getLengthFieldSize(msgLen) + msgLen + 1

		switch tlvType {
		case 0x03: // NDEF Message TLV
			ndefMessage = append(ndefMessage, message...)
		case 0x00: // NULL TLV - skip
			continue
		case 0xFE: // Terminator TLV
			break loop
		default:
			return "", fmt.Errorf("unknown TLV type: %x", tlvType)
		}
	}

	return decodeTextPayload(ndefMessage)
}

// DeviceScanner provides device discovery and monitoring capabilities
type DeviceScanner struct {
	devices []string
	mu      sync.RWMutex
}

func NewDeviceScanner() *DeviceScanner {
	return &DeviceScanner{
		devices: make([]string, 0),
	}
}

// scanDevices attempts to enumerate all available NFC devices
func (s *DeviceScanner) scanDevices() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	var devices []string
	for i := 0; i < deviceEnumRetries; i++ {
		if devs, err := nfc.ListDevices(); err == nil {
			devices = devs
			break
		}
		time.Sleep(time.Millisecond * 100)
	}

	s.devices = devices
	return devices
}

// tryConnect attempts to connect to an NFC device
func (r *NFCReader) tryConnect() error {
	if r.hasDevice {
		return nil
	}

	// Reset card presence state and clear cache
	r.setCardPresent(false)
	r.cache.clear()

	// Scan for available devices
	devices := r.scanner.scanDevices()
	if len(devices) == 0 {
		r.setDeviceStatus(false, "Waiting for NFC reader to be connected...")
		return fmt.Errorf("no NFC devices found")
	}

	// Try to use the same device if available
	deviceStr := devices[0]
	for _, dev := range devices {
		if dev == r.devicePath {
			deviceStr = dev
			break
		}
	}

	device, err := nfc.Open(deviceStr)
	if err != nil {
		r.setDeviceStatus(false, fmt.Sprintf("Failed to connect to device %s: %v", deviceStr, err))
		return fmt.Errorf("failed to open device %s: %v", deviceStr, err)
	}

	// Try to initialize the device
	if err := device.InitiatorInit(); err != nil {
		device.Close()
		r.setDeviceStatus(false, fmt.Sprintf("Failed to initialize device %s: %v", deviceStr, err))
		return fmt.Errorf("failed to initialize device %s: %v", deviceStr, err)
	}

	r.device = device
	r.hasDevice = true
	r.devicePath = deviceStr
	r.cardPresent = false

	// Log device information
	deviceName := device.String()
	connectionInfo := device.Connection()
	log.Printf("NFC device connected - Name: %s, Connection: %s, Path: %s",
		deviceName, connectionInfo, deviceStr)

	r.setDeviceStatus(true, fmt.Sprintf("Connected to %s via %s", deviceName, connectionInfo))
	return nil
}

// logDeviceInfo logs detailed information about the connected NFC device
func (r *NFCReader) logDeviceInfo() {
	if !r.hasDevice {
		return
	}

	name := r.device.String()
	connString := r.device.Connection()
	log.Printf("Connected NFC device: %s (Connection: %s)", name, connString)
}

// setCardPresent updates the card presence status and broadcasts it to clients.
func (r *NFCReader) setCardPresent(present bool) {
	r.statusMux.Lock()
	if r.cardPresent != present {
		r.cardPresent = present
		status := r.deviceStatus
		status.CardPresent = present
		if present {
			status.Message = "Card detected"
		} else {
			status.Message = "Card removed"
			// Clear cache when card is removed
			r.cache.clear()
		}
		r.deviceStatus = status
		r.statusMux.Unlock()
		broadcastDeviceStatus(status)
	} else {
		r.statusMux.Unlock()
	}
}

// decodeTextPayload extracts text from an NDEF text record payload.
// It handles both UTF-8 and UTF-16 encodings and language code records.
func decodeTextPayload(payload []byte) (string, error) {
	if len(payload) < 3 {
		return "", fmt.Errorf("NDEF message too short")
	}

	// NDEF Record Header bits
	// Byte 0: [MB(1) ME(1) CF(1) SR(1) IL(1) TNF(3)]
	// Byte 1: Type Length
	// Byte 2: Payload Length (if SR=1)
	// Byte 3+TypeLength+IL: Payload

	// recordHeader := payload[0]
	typeLength := int(payload[1])
	payloadLength := int(payload[2])

	// Calculate where payload starts:
	// - Skip header byte (1)
	// - Skip type length byte (1)
	// - Skip payload length byte (1)
	// - Skip type bytes (typeLength)
	payloadStart := 3 + typeLength

	payload = payload[payloadStart : payloadStart+payloadLength]

	// First byte contains encoding and language length
	status := payload[0]
	langLength := int(status & 0x3F)
	isUTF16 := (status & 0x80) != 0

	// Skip status byte and language code
	textStart := 0
	if status == 2 {
		textStart = 1 + langLength
	}

	if textStart >= len(payload) {
		fmt.Println(string(payload))
		fmt.Println(textStart)
		return "", fmt.Errorf("payload too short")
	}

	textBytes := payload[textStart:]
	if isUTF16 {
		if len(textBytes) < 2 {
			return "", fmt.Errorf("invalid UTF-16 text")
		}
		return decodeUTF16(textBytes), nil
	}

	return string(textBytes), nil
}

// decodeUTF16 converts UTF-16 encoded bytes to a Go string.
func decodeUTF16(b []byte) string {
	if len(b)%2 != 0 {
		return "" // Invalid UTF-16 length
	}

	u16s := make([]uint16, 0, len(b)/2)
	for i := 0; i < len(b); i += 2 {
		u16s = append(u16s, binary.LittleEndian.Uint16(b[i:i+2]))
	}

	runes := utf16.Decode(u16s)
	return strings.TrimSpace(string(runes))
}

// getLengthFieldSize returns the size of the TLV length field based on the length value.
func getLengthFieldSize(length int) int {
	if length > 0xFF {
		return 3
	}
	return 1
}

type MifareClassicKeyAndType struct {
	Key  [6]byte
	Type int
}

func searchSectorKey(tag freefare.ClassicTag, sector byte, key *[6]byte, keyType *int) error {
	block := freefare.ClassicSectorLastBlock(sector)

	fmt.Printf("searchSectorKey: last block for sector %d is %d\n", sector, block)

	if err := tag.Disconnect(); err != nil {
		// Ignore tag state errors
		if !strings.Contains(err.Error(), "tag state error") {
			return fmt.Errorf("searchSectorKey first disconnect error: %v", err)
		}
	}

	/*
		     * FIXME (from libfreefare/mifare-class-write-ndef.c): We should not assume that
			 * if we have full access to trailer block we also have a full access to data blocks.
	*/

	for i := 0; i < len(defaultKeys); i++ {
		connectErr := tag.Connect()
		fmt.Printf("searchSectorKey: Trying key %X (index %d) with KeyA\n", defaultKeys[i], i)
		authnErr := tag.Authenticate(block, defaultKeys[i], freefare.KeyA)

		if connectErr == nil && authnErr == nil {
			fmt.Printf("searchSectorKey: Authentication with key %X (KeyA) successful\n", defaultKeys[i])
			writeAPerm, _ := tag.TrailerBlockPermission(block, freefare.WriteKeyA, freefare.KeyA)
			accessBitPerm, _ := tag.TrailerBlockPermission(block, freefare.WriteAccessBits, freefare.KeyA)
			writeBPerm, _ := tag.TrailerBlockPermission(block, freefare.WriteKeyB, freefare.KeyA)

			if writeAPerm && accessBitPerm && writeBPerm {
				// equivalent to memcpy(key, &default_keys[i], sizeof(MifareClassicKey));
				fmt.Printf("searchSectorKey: Found suitable KeyA: %X for sector %d\n", defaultKeys[i], sector)
				*key = defaultKeys[i]
				*keyType = freefare.KeyA
				return nil
			} else {
				fmt.Printf("searchSectorKey: KeyA %X doesn't have full access\n", defaultKeys[i]) // ADD
			}
		} else {
			if authnErr != nil {
				fmt.Printf("searchSectorKey: Authentication with key %X (KeyA) failed: %v\n", defaultKeys[i], authnErr) // ADD
			} else {
				fmt.Printf("searchSectorKey: Connect failed: %v\n", connectErr) // ADD
			}
		}

		tag.Disconnect()

		connectErr = tag.Connect()
		fmt.Printf("searchSectorKey: Trying key %X (index %d) with KeyB\n", defaultKeys[i], i)
		authnErr = tag.Authenticate(block, defaultKeys[i], freefare.KeyB)

		if connectErr == nil && authnErr == nil {
			fmt.Printf("searchSectorKey: Authentication with key %X (KeyB) successful\n", defaultKeys[i])
			writeAPerm, _ := tag.TrailerBlockPermission(block, freefare.WriteKeyA, freefare.KeyB)
			accessBitPerm, _ := tag.TrailerBlockPermission(block, freefare.WriteAccessBits, freefare.KeyB)
			writeBPerm, _ := tag.TrailerBlockPermission(block, freefare.WriteKeyB, freefare.KeyB)

			if writeAPerm && accessBitPerm && writeBPerm {
				// equivalent to memcpy(key, &default_keys[i], sizeof(MifareClassicKey));
				fmt.Printf("searchSectorKey: Found suitable KeyB: %X for sector %d\n", defaultKeys[i], sector)
				*key = defaultKeys[i]
				*keyType = freefare.KeyB
				return nil
			} else {
				fmt.Printf("searchSectorKey: KeyB %X doesn't have full access\n", defaultKeys[i])
			}
		} else {
			if authnErr != nil {
				fmt.Printf("searchSectorKey: Authentication with key %X (KeyB) failed: %v\n", defaultKeys[i], authnErr) // ADD
			} else {
				fmt.Printf("searchSectorKey: Connect failed: %v\n", connectErr) // ADD
			}
		}

		tag.Disconnect()
	}

	return fmt.Errorf("searchSectorKey error: no known authentication key for sector %d", sector)
}

func MifareClassicTrailerBlock(block *[16]byte, keyA [6]byte, ab0, ab1, ab2, abTb uint8, gpb uint8, keyB [6]byte) {
	if len(block) < 16 {
		panic("block must be at least 16 bytes")
	}

	// Copy Key A
	// NOTE: ugly but necessary due to Go's array copy limitations
	for i := 0; i < 6; i++ {
		block[i] = keyA[i]
	}

	// Calculate access bits components
	calcBits := func(ab uint8) uint32 {
		c3 := (ab & 0x4) >> 2 // Extract C3 (bit 2)
		c2 := (ab & 0x2) >> 1 // Extract C2 (bit 1)
		c1 := ab & 0x1        // Extract C1 (bit 0)
		return (uint32(c3) << 8) | (uint32(c2) << 4) | uint32(c1)
	}

	ab0Bits := calcBits(ab0)
	ab1Bits := calcBits(ab1) << 1
	ab2Bits := calcBits(ab2) << 2
	abTbBits := calcBits(abTb) << 3

	// Combine all access components
	accessBits := ab0Bits | ab1Bits | ab2Bits | abTbBits
	accessInverted := (^accessBits) & 0xfff // Mask to 12 bits

	// Pack into 24-bit value and convert to little-endian
	combined := (uint32(accessBits) << 12) | uint32(accessInverted)
	leBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(leBytes, combined)

	// Copy access bytes and GPB
	for i := 0; i < 3; i++ {
		block[6+i] = leBytes[i]
	}
	block[9] = gpb

	// Copy Key B
	for i := 0; i < 6; i++ {
		block[10+i] = keyB[i]
	}
}

func MifareClassicTrailerBlock2(block *[16]byte, keyA [6]byte, ab0, ab1, ab2, gpb uint8, keyB [6]byte) {
	if len(block) < 16 {
		panic("block must be at least 16 bytes")
	}

	// Copy Key A
	// NOTE: ugly but necessary due to Go's array copy limitations
	for i := 0; i < 6; i++ {
		block[i] = keyA[i]
	}

	// Copy access bytes and GPB
	leBytes := [4]byte{ab0, ab1, ab2, gpb}
	for i := 0; i < 4; i++ {
		block[6+i] = leBytes[i]
	}

	// Copy Key B
	for i := 0; i < 6; i++ {
		block[10+i] = keyB[i]
	}
}

// func fixMadTrailerBlock(tag freefare.ClassicTag, sector byte, key [6]byte, keyType int) error {
// 	block := [16]byte{}

// 	MifareClassicTrailerBlock(block, defaultKeyA, 0x0, 0x1, 0x1, 0x6, 0x00, defaultKeyB)

// 	if err := tag.Authenticate(freefare.ClassicSectorLastBlock(sector), key, keyType); err != nil {
// 		return fmt.Errorf("fixMadTrailerBlock error: %v", err)
// 	}

// 	if err := tag.WriteBlock(freefare.ClassicSectorLastBlock(sector), block); err != nil {
// 		return fmt.Errorf("fixMadTrailerBlock error: %v", err)
// 	}

// 	return nil
// }

func writeData(classicTag freefare.ClassicTag, text string) error {
	// Notes from stackoverflow:
	// Your remark about an Android phone being able to read and write this tag
	// suggests it is formatted to contain NDEF data. Instead of the factory default key 0xFFFFFFFFFFFF,
	// you could try to use the MIFARE Application Directory key 0xA0A1A2A3A4A5 for the first sector
	// (blocks 0-3) and the NFC Forum key 0xD3F7D3F7D3F7 for the following sectors.
	// See NFC Type MIFARE Classic Tag Operation for more details.
	// - https://stackoverflow.com/a/9386977

	if err := classicTag.Connect(); err != nil {
		return fmt.Errorf("writeData connect error: %v", err)
	}
	defer classicTag.Disconnect()

	// Check if card is in factory mode by trying factory key on sector 0
	if err := classicTag.Authenticate(0, factoryKey, freefare.KeyA); err == nil {
		// Card is in factory mode, initialize it with proper keys and format
		fmt.Println("Detected factory mode card, initializing...")

		// Initialize sectors with proper keys
		maxSector := 16
		if classicTag.Type() == freefare.Classic4k {
			maxSector = 40
		}

		for sector := 0; sector < maxSector; sector++ {
			// Skip MAD2 sector in 4K cards if we're not at that stage yet
			if sector == 0x10 && classicTag.Type() == freefare.Classic4k {
				continue
			}

			// Authenticate with factory key
			block := freefare.ClassicSectorLastBlock(byte(sector))
			if err := classicTag.Authenticate(block, factoryKey, freefare.KeyA); err != nil {
				return fmt.Errorf("failed to authenticate sector %d: %v", sector, err)
			}

			// Prepare trailer block with proper keys and access bits
			trailer := [16]byte{}
			var keyA, keyB [6]byte

			if sector == 0 { // MAD sector
				keyA = defaultKeyA
				keyB = factoryKey
				MifareClassicTrailerBlock2(&trailer, keyA, 0x78, 0x77, 0x88, 0xc1, keyB)
			} else { // Application sectors
				keyA = publicKey
				keyB = factoryKey
				MifareClassicTrailerBlock2(&trailer, keyA, 0x7f, 0x07, 0x88, 0x40, keyB)
			}

			// Write the trailer block
			if err := classicTag.WriteBlock(block, trailer); err != nil {
				return fmt.Errorf("failed to write trailer for sector %d: %v", sector, err)
			}

			// Format data blocks (except sector 0 which needs MAD)
			// if sector != 0 {
			// 	if err := classicTag.FormatSector(byte(sector)); err != nil {
			// 		return fmt.Errorf("failed to format sector %d: %v", sector, err)
			// 	}
			// }
		}

		// For 4K cards, handle MAD2 sector separately
		if classicTag.Type() == freefare.Classic4k {
			block := freefare.ClassicSectorLastBlock(0x10)
			if err := classicTag.Authenticate(block, factoryKey, freefare.KeyA); err != nil {
				return fmt.Errorf("failed to authenticate MAD2 sector: %v", err)
			}

			trailer := [16]byte{}
			MifareClassicTrailerBlock2(&trailer, defaultKeyA, 0x78, 0x77, 0x88, 0xc1, defaultKeyB)
			if err := classicTag.WriteBlock(block, trailer); err != nil {
				return fmt.Errorf("failed to write MAD2 trailer: %v", err)
			}
		}

		fmt.Println("Card initialized successfully")
	}

	// Proceed with normal write operation
	// 40 keys for 40 sectors (max for Mifare Classic 4K)
	cardWriteKeys := make([]MifareClassicKeyAndType, 40)

	// Determine MAD sectors based on card type
	madSector1 := byte(0)    // First MAD sector (sector 0)
	madSector2 := byte(0x10) // Second MAD sector (sector 16, only for 4K cards)
	isMad2 := classicTag.Type() == freefare.Classic4k

	// Set default keys for all sectors
	for i := 0; i < 40; i++ {
		// For MAD sectors, use factory key, otherwise use public key
		if i == int(madSector1) || (isMad2 && i == int(madSector2)) {
			cardWriteKeys[i].Key = factoryKey
		} else {
			cardWriteKeys[i].Key = publicKey
		}

		cardWriteKeys[i].Type = freefare.KeyA
	}

	// First, authenticate and handle MAD sector 0 (never erase this sector)
	if err := searchSectorKey(classicTag, madSector1, &cardWriteKeys[madSector1].Key, &cardWriteKeys[madSector1].Type); err != nil {
		return fmt.Errorf("cannot authenticate MAD sector 0: %v", err)
	}

	// For 4K cards, also handle MAD2 in sector 16
	if isMad2 {
		if err := searchSectorKey(classicTag, madSector2, &cardWriteKeys[madSector2].Key, &cardWriteKeys[madSector2].Type); err != nil {
			return fmt.Errorf("cannot authenticate MAD2 sector: %v", err)
		}
	}

	// Encode the text as an NDEF record
	encoded := freefare.TLVencode(encodeTextPayload(text), 0x03)

	// Read existing MAD
	mad, err := classicTag.ReadMad()
	if err != nil {
		// If MAD doesn't exist or is corrupt, create a new one
		madVersion := byte(1)
		if isMad2 {
			madVersion = 2
		}
		mad = freefare.NewMad(madVersion)
	}

	// Clear existing NDEF application if present
	if sectors := mad.FindApplication(freefare.MadNFCForumAid); len(sectors) > 0 {
		for _, sector := range sectors {
			// Skip MAD sectors
			if sector == madSector1 || (isMad2 && sector == madSector2) {
				continue
			}

			if err := searchSectorKey(classicTag, sector, &cardWriteKeys[sector].Key, &cardWriteKeys[sector].Type); err != nil {
				continue // Skip sectors we can't authenticate
			}

			if err := classicTag.Authenticate(freefare.ClassicSectorLastBlock(sector), cardWriteKeys[sector].Key, cardWriteKeys[sector].Type); err != nil {
				continue
			}

			// Format non-MAD sectors only
			// if err := classicTag.FormatSector(sector); err != nil {
			// 	log.Printf("Warning: couldn't format sector %d: %v", sector, err)
			// }

			// If sector is an application sector, set keya to public key
			// if sector != madSector1 && sector != madSector2 {
			// classicTag.
			// }
		}
	}

	// Mark available sectors in MAD
	maxSectors := 16 // Classic 1K
	if isMad2 {
		maxSectors = 40 // Classic 4K
	}

	// Find available sectors for writing
	availableSectors := []byte{}
	for s := 1; s < maxSectors; s++ { // Start from 1 to skip sector 0
		if s == int(madSector2) && isMad2 {
			continue // Skip MAD2 sector for 4K cards
		}

		sector := byte(s)
		if err := searchSectorKey(classicTag, sector, &cardWriteKeys[sector].Key, &cardWriteKeys[sector].Type); err == nil {
			availableSectors = append(availableSectors, sector)
		}
	}

	if len(availableSectors) == 0 {
		return fmt.Errorf("no writable sectors available")
	}

	// Calculate required sectors for the data
	requiredSectors := (len(encoded) + 47) / 48 // 48 bytes per sector (3 blocks of 16 bytes)
	if requiredSectors > len(availableSectors) {
		return fmt.Errorf("not enough space: need %d sectors, have %d", requiredSectors, len(availableSectors))
	}

	// Update MAD to mark sectors as NDEF
	for _, sector := range availableSectors[:requiredSectors] {
		mad.SetAid(sector, freefare.MadNFCForumAid)
	}

	// Write updated MAD
	if err := classicTag.WriteMad(mad, cardWriteKeys[madSector1].Key, cardWriteKeys[madSector2].Key); err != nil {
		return fmt.Errorf("failed to write MAD: %v", err)
	}

	// Write the NDEF data
	if _, err := classicTag.WriteApplication(mad, freefare.MadNFCForumAid, encoded, factoryKey, freefare.KeyB); err != nil {
		return fmt.Errorf("failed to write NDEF data: %v", err)
	}

	return nil
}

// encodeTextPayload converts a string into an NDEF text record payload.
// It encodes the text in UTF-8 format with 'en' as the default language code.
func encodeTextPayload(text string) []byte {
	// Use "en" as the language code
	langCode := []byte("en")

	// Calculate total size: 1 (status byte) + len(langCode) + len(text)
	textPayloadSize := 1 + len(langCode) + len(text)
	textPayload := make([]byte, textPayloadSize)

	// Status byte:
	// - Bit 7: 0 = UTF-8 encoding
	// - Bit 6: 0 (RFU)
	// - Bits 5-0: language code length (max 63)
	textPayload[0] = byte(len(langCode)) // UTF-8 encoding (bit 7 = 0)

	// Copy language code
	copy(textPayload[1:], langCode)

	// Copy the text
	copy(textPayload[1+len(langCode):], []byte(text))

	// Now wrap in NDEF record structure
	recordHeader := byte(0xD1) // MB=1, ME=1, CF=0, SR=1, IL=0, TNF=001
	typeLength := byte(1)      // 1 byte for the type
	payloadLength := byte(textPayloadSize)
	recordType := byte(0x54) // "T" for Text record

	// Full message size: 3 header bytes + 1 type byte + payload
	ndefMessage := make([]byte, 3+1+textPayloadSize)
	ndefMessage[0] = recordHeader
	ndefMessage[1] = typeLength
	ndefMessage[2] = payloadLength
	ndefMessage[3] = recordType
	copy(ndefMessage[4:], textPayload)

	return ndefMessage
}

// WriteCardData attempts to write text data to a detected NFC card.
func (r *NFCReader) WriteCardData(text string) error {
	return r.withTagOperation(func() error {
		if !r.hasDevice {
			return fmt.Errorf("no NFC device connected")
		}

		r.isWriting = true
		defer func() {
			r.isWriting = false
		}()

		tags, err := freefare.GetTags(r.device)
		if err != nil {
			return fmt.Errorf("failed to get tags: %v", err)
		}

		if len(tags) == 0 {
			return fmt.Errorf("no card detected")
		}

		for _, tag := range tags {
			if tag.Type() != freefare.Classic1k && tag.Type() != freefare.Classic4k {
				continue
			}

			classicTag, ok := tag.(freefare.ClassicTag)
			if !ok {
				continue
			}

			return writeData(classicTag, text)
		}

		return fmt.Errorf("no compatible cards found")
	})
}

// withTagOperation performs a protected tag operation with timeout
func (r *NFCReader) withTagOperation(operation func() error) error {
	r.operationMutex.Lock()
	defer r.operationMutex.Unlock()

	done := make(chan error, 1)
	go func() {
		done <- operation()
	}()

	select {
	case err := <-done:
		return err
	case <-time.After(r.operationTimeout):
		return fmt.Errorf("operation timed out")
	}
}

// getTags safely gets tags from the device with timeout
func (r *NFCReader) getTags() ([]freefare.Tag, error) {
	var tags []freefare.Tag
	err := r.withTagOperation(func() error {
		var err error
		tags, err = freefare.GetTags(r.device)
		return err
	})
	return tags, err
}

// readTagData safely reads data from a tag
func (r *NFCReader) readTagData(tag freefare.ClassicTag) (string, error) {
	var result string
	err := r.withTagOperation(func() error {
		var err error
		result, err = readData(tag)
		return err
	})
	return result, err
}

// RepairSector0 attempts to repair a corrupted sector 0 of a card with the specified UID
func (r *NFCReader) RepairSector0(uidStr string) error {
	fmt.Printf("Waiting for card with UID: %s\n", uidStr)
	fmt.Println("Please place the card on the reader...")

	tags, err := r.getTags()
	if err != nil {
		return fmt.Errorf("error getting tags: %v", err)
	}

	fmt.Printf("Found %d tag(s)\n", len(tags))

	for _, t := range tags {
		classicTag, ok := t.(freefare.ClassicTag)
		if !ok {
			continue
		}

		uid := classicTag.UID()
		if uid != uidStr {
			fmt.Printf("Found card with UID %s, but waiting for %s\n", uid, uidStr)
			continue
		}

		fmt.Printf("Found matching card with UID: %s\n", uid)
		return repairMifareClassicSector0(classicTag)
	}

	return fmt.Errorf("no card with UID %s found", uidStr)
}

// repairMifareClassicSector0 attempts to fix a corrupted sector 0 of a Mifare Classic card
func repairMifareClassicSector0(tag freefare.ClassicTag) error {
	if err := tag.Connect(); err != nil {
		return fmt.Errorf("repairMifareClassicSector0 connect error: %v", err)
	}
	defer tag.Disconnect()

	fmt.Println("Attempting to repair sector 0...")

	// Try to authenticate with any of the known keys
	var useKey [6]byte = [6]byte{}
	var keyType int = -1

	// First, authenticate and handle MAD sector 0 (never erase this sector)
	if err := searchSectorKey(tag, 0x00, &useKey, &keyType); err != nil {
		return fmt.Errorf("cannot authenticate MAD sector 0: %v", err)
	}

	// For 4K cards, also handle MAD2 in sector 16
	if tag.Type() == freefare.Classic4k {
		if err := searchSectorKey(tag, 0x10, &useKey, &keyType); err != nil {
			return fmt.Errorf("cannot authenticate MAD2 sector: %v", err)
		}
	}

	if keyType == -1 {
		return fmt.Errorf("could not authenticate to sector 0 with any known key")
	}

	fmt.Printf("Successfully authenticated with key type %d: %X\n", keyType, useKey)

	// Read manufacturer block (block 0)
	_, err := tag.ReadBlock(0)
	if err != nil {
		return fmt.Errorf("error reading manufacturer block: %v", err)
	}

	// Read blocks 1 and 2
	block1, err := tag.ReadBlock(1)
	if err != nil {
		fmt.Printf("Warning: Cannot read block 1: %v\n", err)
		// Initialize with zeros if we can't read it
		block1 = [16]byte{}
	}

	block2, err := tag.ReadBlock(2)
	if err != nil {
		fmt.Printf("Warning: Cannot read block 2: %v\n", err)
		// Initialize with zeros if we can't read it
		block2 = [16]byte{}
	}

	// Read the trailer block
	if _, err := tag.ReadBlock(3); err != nil {
		fmt.Printf("Warning: Cannot read trailer block: %v\n", err)
		// Initialize with default trailer if we can't read it
		// trailerBlock = [16]byte{}
	}

	// Prepare a fixed trailer block with proper access bits
	// Using the NFC Forum default keys and access bits
	fixedTrailer := [16]byte{}
	MifareClassicTrailerBlock(
		&fixedTrailer,
		publicKey,   // Key A - NFC Forum default
		0x7,         // Access bits for block 0 (read-only manufacturer block)
		0x8,         // Access bits for block 1 (free read/write with key A or B)
		0x8,         // Access bits for block 2 (free read/write with key A or B)
		0x8,         // Access bits for trailer block
		0x00,        // GPB
		defaultKeyB, // Key B
	)

	// Write the fixed trailer block
	fmt.Println("Writing fixed trailer block to sector 0...")
	if err := tag.WriteBlock(3, fixedTrailer); err != nil {
		return fmt.Errorf("error writing trailer block: %v", err)
	}

	fmt.Println("Sector 0 trailer block repaired successfully")

	// Try to write blocks 1 and 2 with their original data or zeros if they were unreadable
	if err := tag.WriteBlock(1, block1); err != nil {
		fmt.Printf("Warning: Could not restore block 1: %v\n", err)
	}

	if err := tag.WriteBlock(2, block2); err != nil {
		fmt.Printf("Warning: Could not restore block 2: %v\n", err)
	}

	fmt.Println("Card repair operation completed")
	return nil
}
