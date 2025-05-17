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

// NFCDeviceInterface defines the operations needed from an NFC device.
type NFCDeviceInterface interface {
	Close() error
	InitiatorInit() error
	String() string
	Connection() string
}

// FreefareTagInterface defines the operations needed from a Freefare tag.
// Uses basic types for parameters where specific freefare types were undefined.
type FreefareTagInterface interface {
	UID() string
	Type() int
	Connect() error
	Disconnect() error
	ReadMad() (*freefare.Mad, error)
	ReadApplication(mad *freefare.Mad, aid freefare.MadAid, buffer []byte, key [6]byte, keyType int) (int, error)
	Authenticate(block byte, key [6]byte, keyType int) error
	TrailerBlockPermission(block byte, perm uint16, keyType int) (bool, error) // Changed perm to uint16
	WriteBlock(block byte, data [16]byte) error
}

// RealNFCDevice is a wrapper around nfc.Device to implement NFCDeviceInterface
type RealNFCDevice struct {
	device nfc.Device
}

func NewRealNFCDevice(dev nfc.Device) *RealNFCDevice {
	return &RealNFCDevice{device: dev}
}

func (r *RealNFCDevice) Close() error         { return r.device.Close() }
func (r *RealNFCDevice) InitiatorInit() error { return r.device.InitiatorInit() }
func (r *RealNFCDevice) String() string       { return r.device.String() }
func (r *RealNFCDevice) Connection() string   { return r.device.Connection() }

// RealClassicTag is a wrapper around freefare.ClassicTag to implement FreefareTagInterface
type RealClassicTag struct {
	tag freefare.ClassicTag
}

func NewRealClassicTag(tag freefare.ClassicTag) *RealClassicTag {
	return &RealClassicTag{tag: tag}
}

func (r *RealClassicTag) UID() string { return r.tag.UID() }

// Assuming r.tag.Type() returns a value comparable to freefare.Classic1k,
// and that this type is compatible with int (e.g. freefare.TagType is an int alias
// or constants like freefare.Classic1k are ints).
func (r *RealClassicTag) Type() int {
	// This is a potential mismatch if r.tag.Type() doesn't return int.
	// However, if freefare.TagType was undefined, we use int as a placeholder.
	// The comparison `tagIf.Type() != freefare.Classic1k` must still work.
	// This implies freefare.Classic1k is also an int or compatible.
	return int(r.tag.Type()) // Explicit cast if r.tag.Type() is, e.g., freefare.TagType (an alias for int)
}
func (r *RealClassicTag) Connect() error    { return r.tag.Connect() }
func (r *RealClassicTag) Disconnect() error { return r.tag.Disconnect() }
func (r *RealClassicTag) ReadMad() (*freefare.Mad, error) {
	mad, err := r.tag.ReadMad()
	return mad, err
}
func (r *RealClassicTag) ReadApplication(mad *freefare.Mad, aid freefare.MadAid, buffer []byte, key [6]byte, keyType int) (int, error) {
	// Assuming the underlying freefare.ClassicTag.ReadApplication takes these types.
	// key is [6]byte, keyType is int.
	return r.tag.ReadApplication(mad, aid, buffer, key, keyType)
}
func (r *RealClassicTag) Authenticate(block byte, key [6]byte, keyType int) error {
	// Assuming underlying method takes byte, [6]byte, int
	return r.tag.Authenticate(block, key, keyType)
}
func (r *RealClassicTag) TrailerBlockPermission(block byte, perm uint16, keyType int) (bool, error) { // Changed perm to uint16
	// Assuming underlying method takes byte, uint16, int
	return r.tag.TrailerBlockPermission(block, perm, keyType)
}
func (r *RealClassicTag) WriteBlock(block byte, data [16]byte) error {
	// Assuming underlying method takes byte, [16]byte
	return r.tag.WriteBlock(block, data)
}

// NFCManagerInterface abstracts NFC device listing, opening, and tag retrieval.
type NFCManagerInterface interface {
	OpenDevice(deviceStr string) (NFCDeviceInterface, error)
	ListDevices() ([]string, error)
	GetTags(dev NFCDeviceInterface) ([]FreefareTagInterface, error)
}

// RealNFCManager implements NFCManagerInterface using the actual nfc and freefare libraries.
type RealNFCManager struct{}

func (m *RealNFCManager) OpenDevice(deviceStr string) (NFCDeviceInterface, error) {
	dev, err := nfc.Open(deviceStr)
	if err != nil {
		return nil, err
	}
	return NewRealNFCDevice(dev), nil
}

func (m *RealNFCManager) ListDevices() ([]string, error) {
	var devices []string
	var err error
	for i := 0; i < deviceEnumRetries; i++ {
		devices, err = nfc.ListDevices()
		if err == nil {
			return devices, nil
		}
		time.Sleep(time.Millisecond * 100)
	}
	return nil, fmt.Errorf("failed to list NFC devices after %d retries: %w", deviceEnumRetries, err)
}

func (m *RealNFCManager) GetTags(devInterface NFCDeviceInterface) ([]FreefareTagInterface, error) {
	realDevWrapper, ok := devInterface.(*RealNFCDevice)
	if !ok {
		return nil, fmt.Errorf("GetTags requires a RealNFCDevice instance for the current implementation")
	}

	tags, err := freefare.GetTags(realDevWrapper.device)
	if err != nil {
		return nil, err
	}
	var result []FreefareTagInterface
	for _, tag := range tags {
		if classicTag, ok := tag.(freefare.ClassicTag); ok {
			result = append(result, NewRealClassicTag(classicTag))
		}
	}
	return result, nil
}

// NFCReader manages NFC device interactions and broadcasts tag data to connected clients.
type NFCReader struct {
	// device is the underlying NFC device
	device NFCDeviceInterface // Changed type
	// nfcManager handles device opening and tag discovery
	nfcManager NFCManagerInterface // New field
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
func NewNFCReader(deviceStr string, manager NFCManagerInterface) (*NFCReader, error) {
	if manager == nil { // Add a guard for nil manager
		return nil, fmt.Errorf("NFCManagerInterface cannot be nil")
	}

	reader := &NFCReader{
		nfcManager:       manager, // Store the manager
		hasDevice:        false,
		dataChan:         make(chan NFCData),
		stopChan:         make(chan struct{}),
		cache:            newTagCache(),
		devicePath:       deviceStr,
		cardPresent:      false,
		deviceStatus:     DeviceStatus{Connected: false, Message: "No device connected", CardPresent: false},
		operationTimeout: 5 * time.Second,
	}

	// If no device specified, try to find one
	if deviceStr == "" {
		devices, err := manager.ListDevices() // Use manager
		if err != nil {
			log.Printf("Error listing devices during initial scan: %v", err)
			// Continue without a device, worker will try to connect
		} else if len(devices) == 0 {
			log.Println("No NFC devices found during initial scan.")
			// Continue without a device, worker will try to connect
		} else {
			deviceStr = devices[0]
			reader.devicePath = deviceStr // Update devicePath if found
			log.Printf("No device specified, found and using: %s", deviceStr)
		}
	}

	if deviceStr != "" {
		log.Printf("Attempting to open specified/found device: %s", deviceStr)
		device, err := manager.OpenDevice(deviceStr) // Use manager
		if err != nil {
			log.Printf("Failed to open device %s initially: %v. Worker will attempt to connect.", deviceStr, err)
			// Return reader, worker will attempt to connect later
		} else {
			log.Printf("Successfully opened device: %s", deviceStr)
			reader.device = device
			reader.hasDevice = true
			reader.deviceStatus = DeviceStatus{Connected: true, Message: "Device connected", CardPresent: false}
			reader.logDeviceInfo() // Log info for the successfully opened device
		}
	} else {
		log.Println("No device string specified and no devices found by manager. Worker will attempt to find one.")
		reader.deviceStatus = DeviceStatus{Connected: false, Message: "No device connected", CardPresent: false}
	}

	return reader, nil
}

// Close releases resources and stops the NFC reader worker.
func (r *NFCReader) Close() {
	log.Println("NFCReader Close called.")
	// This method is primarily for resource cleanup when NFCReader is discarded.
	// For stopping the worker goroutine, a dedicated Stop() method is better.
	if r.hasDevice && r.device != nil {
		log.Println("Closing NFC device in Close().")
		if err := r.device.Close(); err != nil {
			log.Printf("Error closing NFC device in Close(): %v", err)
		}
		r.device = nil
		r.hasDevice = false
	}
}

// Stop gracefully shuts down the NFCReader.
func (r *NFCReader) Stop() {
	log.Println("Stopping NFCReader...")
	// Signal the worker goroutine to stop
	select {
	case <-r.stopChan:
		log.Println("Stop channel already closed or closing.")
	default:
		close(r.stopChan)
		log.Println("Stop channel successfully closed.")
	}

	// The worker's defer r.Close() should handle device closing.
	// Explicitly setting hasDevice to false here might be redundant if worker handles it.
	// However, it ensures the state is updated if Stop is called externally.
	r.statusMux.Lock()
	r.hasDevice = false // Reflect that we are stopping.
	r.statusMux.Unlock()
	log.Println("NFCReader stop process initiated.")
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
	log.Printf("Attempting to reconnect device (path hint: %s)...", r.devicePath)
	r.cache.clear()
	r.setCardPresent(false) // Updates status and broadcasts

	for attempt := 1; attempt <= maxReconnectTries; attempt++ {
		log.Printf("Reconnect attempt %d/%d for device path: %s", attempt, maxReconnectTries, r.devicePath)
		if r.hasDevice && r.device != nil {
			log.Println("Closing existing device connection before reconnect attempt.")
			if err := r.device.Close(); err != nil {
				log.Printf("Error closing device during reconnect: %v", err)
				// Continue attempt, as device might be in a bad state anyway
			}
			r.device = nil // Ensure it's nil before trying to connect
			r.hasDevice = false
		}

		r.setDeviceStatus(false, fmt.Sprintf("Attempting to reconnect (attempt %d/%d)", attempt, maxReconnectTries))

		// tryConnect will use r.devicePath if set, or find a new device.
		if err := r.tryConnect(); err == nil { // tryConnect uses the manager
			log.Printf("Reconnection attempt %d successful.", attempt)
			// Check for tags to update card presence immediately
			tags, errTags := r.getTags() // Uses manager via r.device
			if errTags == nil && len(tags) > 0 {
				r.cache.mu.Lock()
				r.cache.lastSeenTime = time.Now() // Mark activity
				r.cache.mu.Unlock()
				r.setCardPresent(true)
				log.Println("Card detected immediately after reconnect.")
			} else {
				r.setCardPresent(false)
				if errTags != nil {
					log.Printf("Error getting tags after reconnect: %v", errTags)
				} else {
					log.Println("No card detected immediately after reconnect.")
				}
			}
			return nil
		} else {
			log.Printf("Reconnection attempt %d failed: %v", attempt, err)
		}

		select {
		case <-r.stopChan:
			log.Println("Reconnect: Stop signal received, aborting reconnection.")
			return fmt.Errorf("reconnection aborted by stop signal")
		case <-time.After(reconnectDelay * time.Duration(attempt)): // Exponential backoff could be used here too
			// continue loop
		}
	}

	errMsg := fmt.Sprintf("failed to reconnect device (path hint: %s) after %d attempts", r.devicePath, maxReconnectTries)
	r.setDeviceStatus(false, "Failed to reconnect after multiple attempts")
	log.Println(errMsg)
	return fmt.Errorf(errMsg)
}

// forceReconnectDevice attempts to force a device reconnection by fully closing and reopening
func (r *NFCReader) forceReconnectDevice() error {
	log.Println("Attempting to force reconnect device...")
	if r.hasDevice && r.device != nil {
		log.Println("Closing current device for force reconnect.")
		if err := r.device.Close(); err != nil {
			log.Printf("Error closing device during force reconnect: %v", err)
		}
		r.device = nil
		r.hasDevice = false
		// Some devices (like ACR122U) might need a longer pause after being closed before they can be reopened.
		log.Println("Waiting for device to reset completely after close...")
		time.Sleep(time.Second * 3)
	}

	r.cache.clear()
	r.setCardPresent(false) // Update status

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ { // Try a few times
		log.Printf("Force reconnect attempt %d/3...", attempt+1)
		if err := r.tryConnect(); err != nil { // tryConnect uses the manager
			lastErr = err
			log.Printf("Force reconnect attempt %d failed: %v", attempt+1, err)
			select {
			case <-r.stopChan:
				log.Println("Force Reconnect: Stop signal received, aborting.")
				return fmt.Errorf("force reconnection aborted by stop signal")
			case <-time.After(time.Second * time.Duration(attempt+1)): // Increasing delay
				// continue loop
			}
			continue
		}
		log.Println("Force reconnect successful.")
		return nil // Successfully reconnected
	}

	errMsg := "force reconnect failed after multiple attempts"
	if lastErr != nil {
		errMsg = fmt.Sprintf("%s: %v", errMsg, lastErr)
	}
	r.setDeviceStatus(false, "Force reconnect failed") // Update status
	log.Println(errMsg)
	return fmt.Errorf(errMsg)
}

// worker is the main processing loop for reading NFC tags.
// It handles device errors, reconnection, and broadcasts tag data.
func (r *NFCReader) worker() {
	retryCount := 0
	deviceCheckTicker := time.NewTicker(deviceCheckInterval)
	// cardCheckTicker checks cache status to update r.cardPresent and broadcast if it changed
	cardCheckTicker := time.NewTicker(250 * time.Millisecond)
	cooldownTimer := time.NewTimer(0) // Timer for cooldown period after certain errors
	if !cooldownTimer.Stop() {        // Ensure timer is drained if it fired very early
		// If Stop returns false, the timer has already fired or been stopped.
		// If it fired, its channel might have a value. Drain it.
		select {
		case <-cooldownTimer.C:
		default:
		}
	}
	inCooldown := false

	// Defer cleanups
	defer deviceCheckTicker.Stop()
	defer cardCheckTicker.Stop()
	defer cooldownTimer.Stop()
	// Ensure device is closed when worker exits, regardless of how.
	// This is critical for releasing hardware resources.
	defer func() {
		log.Println("NFCReader worker defer: Closing device if connected.")
		if r.hasDevice && r.device != nil {
			if err := r.device.Close(); err != nil {
				log.Printf("NFCReader worker defer: Error closing device: %v", err)
			}
			r.device = nil
			r.hasDevice = false
		}
		log.Println("NFCReader worker defer: Setting final device status to disconnected.")
		// Set a final disconnected status unless already appropriately set.
		// This might be redundant if Stop() or Close() already did this.
		r.setDeviceStatus(false, "Worker stopped, device disconnected.")
	}()

	log.Println("NFCReader worker started.")

	for {
		select {
		case <-r.stopChan:
			log.Println("NFCReader worker stopping via stopChan.")
			return // Exit the worker loop

		case <-deviceCheckTicker.C:
			if !r.hasDevice && !inCooldown {
				log.Println("Device check ticker: No device or not in cooldown, attempting connect.")
				if err := r.tryConnect(); err != nil { // tryConnect uses manager
					log.Printf("Device check ticker: Connection attempt failed: %v", err)
					// Failure to connect is logged by tryConnect, status updated there.
				} else {
					log.Println("Device check ticker: Connection successful or already connected.")
					retryCount = 0 // Reset error-related retry count on successful connection
				}
			}

		case <-cardCheckTicker.C:
			// Check card presence based on cache's lastSeenTime
			// This decouples UI update of card presence from actual tag polling loop
			currentCacheCardPresent := r.cache.isCardPresent()
			if r.cardPresent != currentCacheCardPresent {
				r.setCardPresent(currentCacheCardPresent) // This updates deviceStatus and broadcasts
				if currentCacheCardPresent {
					log.Printf("Card presence changed: DETECTED (UID: %s)", r.cache.lastUID)
				} else {
					log.Println("Card presence changed: REMOVED/timed out")
				}
			}

		case <-cooldownTimer.C:
			log.Println("Device cooldown period ended.")
			inCooldown = false
			// Attempt to reconnect after cooldown
			if err := r.forceReconnectDevice(); err != nil { // uses manager
				log.Printf("Reconnection after cooldown failed: %v. May re-enter cooldown if error persists.", err)
				// Consider if this failure should trigger another cooldown or if deviceCheckTicker will handle it.
			}

		default:
			// Main polling logic if device is connected and not in cooldown
			if !r.hasDevice || r.device == nil || inCooldown {
				time.Sleep(200 * time.Millisecond) // Sleep if no device or in cooldown
				continue
			}

			if r.isWriting { // Skip polling if a write operation is in progress
				time.Sleep(50 * time.Millisecond)
				continue
			}

			tags, err := r.getTags() // Uses manager via r.device

			if err != nil {
				log.Printf("Error getting tags: %v", err)
				originalErrorString := err.Error() // For string matching specific errors

				// Handle IO/Config errors (often indicate device disconnection)
				if isIOError(err) || isDeviceConfigError(err) {
					log.Printf("Device error detected (IO/Config): %v. Closing device.", err)
					if r.hasDevice && r.device != nil {
						r.device.Close() // Close the problematic device
					}
					r.device = nil
					r.hasDevice = false
					r.setDeviceStatus(false, fmt.Sprintf("Device error: %v", err))
					r.isWriting = false // Reset writing state

					// Cooldown logic for specific errors (e.g., ACR122 related)
					if strings.Contains(originalErrorString, "Operation not permitted") ||
						strings.Contains(originalErrorString, "broken pipe") ||
						strings.Contains(originalErrorString, "RDR_to_PC_DataBlock") {
						if !inCooldown { // Avoid resetting timer if already in cooldown
							inCooldown = true
							cooldownPeriod := time.Second * 10
							log.Printf("ACR122-like error detected. Entering cooldown for %v", cooldownPeriod)
							cooldownTimer.Reset(cooldownPeriod)
						}
						continue // Skip immediate reconnect attempt, wait for cooldown
					}

					// For other IO/Config errors, attempt force reconnect after a short delay
					log.Println("Attempting force reconnect after IO/Config error...")
					time.Sleep(time.Second * 2)                                        // Brief pause before trying
					if errReconnect := r.forceReconnectDevice(); errReconnect != nil { // uses manager
						log.Printf("Force reconnection failed after IO/Config error: %v", errReconnect)
						// If force reconnect fails, might enter cooldown or rely on deviceCheckTicker
					}
					continue // Move to next iteration
				}

				// Handle Timeout/Closed errors (might be recoverable with retries)
				if isTimeoutError(err) || isDeviceClosedError(err) {
					log.Printf("Device error (Timeout/Closed): %v", err)
					delay := time.Duration(math.Pow(2, float64(retryCount))) * baseDelay
					if retryCount < maxRetries {
						retryCount++
						log.Printf("Retrying connection (attempt %d/%d) in %v...", retryCount, maxRetries, delay)

						select {
						case <-time.After(delay):
							// Proceed with reconnect attempt
						case <-r.stopChan:
							log.Println("Worker stopping during retry delay for Timeout/Closed error.")
							return
						}

						r.isWriting = false                                     // Reset writing state
						if errReconnect := r.reconnect(); errReconnect != nil { // uses manager
							log.Printf("Device reconnection failed after Timeout/Closed error: %v", errReconnect)
							// Do not reset retryCount here, let it increment for next error
						} else {
							log.Println("Reconnected successfully after Timeout/Closed error.")
							retryCount = 0 // Reset on successful reconnect
						}
					} else { // Max retries reached
						log.Printf("Max retries reached for Timeout/Closed error: %v. Closing device.", err)
						if r.hasDevice && r.device != nil {
							r.device.Close()
						}
						r.device = nil
						r.hasDevice = false
						r.setDeviceStatus(false, "Max retries reached for device error.")
						if !inCooldown { // Enter a longer cooldown if not already in one
							inCooldown = true
							cooldownTimer.Reset(time.Second * 30)
							log.Println("Entering long cooldown after max retries for Timeout/Closed error.")
						}
					}
					continue // Move to next iteration
				}

				// For other unhandled errors from getTags
				log.Printf("Unhandled error type from getTags: %v. Sending error to dataChan.", err)
				r.dataChan <- NFCData{Err: fmt.Errorf("get tags error: %v", err)}
				time.Sleep(time.Second) // Wait a bit before trying again
				continue                // Move to next iteration
			}
			// Successful tag retrieval (or no tags found, which is not an error)
			retryCount = 0 // Reset error-related retry count if getTags was successful

			if len(tags) > 0 {
				atLeastOneValidTagProcessed := false
				for _, tagIf := range tags { // tagIf is FreefareTagInterface
					// The comparison `tagIf.Type() != freefare.Classic1k` requires freefare.Classic1k
					// to be comparable with the result of tagIf.Type() (which is int via interface).
					if tagIf.Type() != int(freefare.Classic1k) && tagIf.Type() != int(freefare.Classic4k) {
						// log.Printf("Skipping non-Classic tag type: %v", tagIf.Type())
						continue
					}

					uid := tagIf.UID()
					text, readErr := r.readTagData(tagIf) // Pass interface directly

					// Regardless of readErr, if a tag was physically present enough to get UID, update cache
					if uid != "" {
						r.cache.mu.Lock()
						r.cache.lastSeenTime = time.Now() // Mark activity for this tag
						r.cache.mu.Unlock()
						atLeastOneValidTagProcessed = true // A tag was seen
					}

					if readErr != nil {
						log.Printf("Error reading data for tag UID %s: %v", uid, readErr)
						// Send error, but UID might still be useful
						r.dataChan <- NFCData{UID: uid, Text: "", Err: readErr}
						continue // Next tag
					}

					// Successfully read tag data
					if r.cache.hasChanged(uid, text) {
						log.Printf("Tag data changed or new tag: UID %s", uid)
						r.dataChan <- NFCData{UID: uid, Text: text, Err: nil}
					} else {
						// log.Printf("Tag data unchanged for UID %s", uid)
					}
				}
				// If any tag was processed, it implies a card is (or was very recently) present.
				// The cardCheckTicker will handle the r.cardPresent state based on cache.lastSeenTime.
				// No direct call to r.setCardPresent(true) here; let cache logic drive it.
				if !atLeastOneValidTagProcessed && r.cardPresent {
					// This case is tricky: getTags returned 0 classic tags, but cardPresent was true.
					// This implies the card might have just been removed.
					// The cache.isCardPresent() and cardCheckTicker will handle this timeout.
				}

			} else { // No tags found by getTags()
				// If no tags are found, cache.isCardPresent() will eventually reflect this
				// via the cardCheckTicker (due to time.Since(r.cache.lastSeenTime) growing).
				// No explicit r.setCardPresent(false) here unless we are certain.
			}
			time.Sleep(100 * time.Millisecond) // Polling interval when device is active
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

// readTagData reads and decodes NDEF formatted text from a Mifare Classic tag.
func (r *NFCReader) readTagData(tag FreefareTagInterface) (string, error) {
	if err := tag.Connect(); err != nil {
		return "", fmt.Errorf("readTagData connect error: %v", err)
	}
	defer tag.Disconnect()

	mad, errMad := tag.ReadMad() // Changed 'err' to 'errMad' to avoid conflict
	if errMad != nil {
		// Attempt to confirm factory mode if MAD read fails
		madSector := byte(0x00)
		// Assuming tag.Type() (which is int via interface) can be compared to freefare.Classic4k (also cast to int)
		if tag.Type() == int(freefare.Classic4k) {
			madSector = byte(0x10)
		}
		trailerBlockNum := freefare.ClassicSectorLastBlock(madSector)

		var errAuth error // Declare errAuth to be in scope for the final error message
		// Assuming factoryKey is [6]byte and freefare.KeyA is int for keyType
		if errAuth = tag.Authenticate(trailerBlockNum, factoryKey, int(freefare.KeyA)); errAuth == nil {
			log.Printf("MAD read failed (%v), but factory key auth on sector %d trailer succeeded. Assuming factory mode.", errMad, madSector)
			return "", nil
		}
		return "", fmt.Errorf("MAD read error: %v (factory key auth also failed: %v)", errMad, errAuth)
	}

	buffer := make([]byte, 4096)

	// Assuming publicKey is [6]byte and freefare.KeyA is int for keyType
	bufLen, errReadApp := tag.ReadApplication(mad, freefare.MadNFCForumAid, buffer, publicKey, int(freefare.KeyA)) // Changed 'err' to 'errReadApp'
	if errReadApp != nil {
		return "", fmt.Errorf("readTagData read application error: %v", errReadApp)
	}

	buffer = buffer[:bufLen]
	var ndefMessage []byte
loop:
	for offset := 0; offset < len(buffer); {
		message, tlvType := freefare.TLVdecode(buffer[offset:])
		if message == nil {
			return "", fmt.Errorf("TLVdecode returned nil message at offset %d", offset)
		}
		msgLen := len(message)
		lenFieldSize := getLengthFieldSize(msgLen)

		// Check for potential out-of-bounds before slicing or advancing offset
		if offset+1+lenFieldSize > len(buffer) || offset+1+lenFieldSize+msgLen > len(buffer) {
			return "", fmt.Errorf("TLV entry at offset %d (len %d, lenField %d) exceeds buffer bounds (len %d)", offset, msgLen, lenFieldSize, len(buffer))
		}

		offset += 1 + lenFieldSize + msgLen

		switch tlvType {
		case 0x03: // NDEF Message TLV
			ndefMessage = append(ndefMessage, message...)
		case 0x00: // NULL TLV - skip
			continue
		case 0xFE: // Terminator TLV
			break loop
		default:
			log.Printf("Unknown TLV type: 0x%X at offset %d", tlvType, offset-(1+lenFieldSize+msgLen))
			return "", fmt.Errorf("unknown TLV type: 0x%X", tlvType)
		}
	}

	if len(ndefMessage) == 0 {
		log.Println("No NDEF message found in application data.")
		return "", nil
	}
	return decodeTextPayload(ndefMessage)
}

// tryConnect attempts to connect to an NFC device
func (r *NFCReader) tryConnect() error {
	if r.hasDevice && r.device != nil {
		// Perform a quick check if the device is still responsive
		// This InitiatorInit can be slow or hang if device is in a bad state.
		// Consider a timeout wrapper if this becomes an issue.
		initErr := r.device.InitiatorInit() // Capture error here
		if initErr == nil {
			log.Println("Device already connected and seems responsive (InitiatorInit OK).")
			return nil
		}
		log.Printf("Device was marked as connected, but InitiatorInit failed: %v. Attempting full reconnect.", initErr)
		// Proceed to close and reopen
		if errClose := r.device.Close(); errClose != nil {
			log.Printf("Error closing problematic device before reconnect: %v", errClose)
		}
		r.device = nil      // Ensure device is nil before attempting to open a new one
		r.hasDevice = false // Mark as not having a device
	}

	// Set card presence to false before attempting connection, update status
	r.setCardPresent(false)
	// Do not clear cache here; cache should only be cleared on successful (re)connection
	// or when a card is definitively removed.

	devices, errList := r.nfcManager.ListDevices()
	if errList != nil {
		errMsg := fmt.Sprintf("Error listing NFC devices: %v", errList)
		r.setDeviceStatus(false, errMsg) // Update status
		return fmt.Errorf(errMsg)
	}
	if len(devices) == 0 {
		errMsg := "No NFC devices found by manager."
		r.setDeviceStatus(false, "Waiting for NFC reader to be connected...") // Update status
		return fmt.Errorf(errMsg)
	}

	deviceStrToConnect := ""
	// Prefer r.devicePath if it's set and found in the list
	if r.devicePath != "" {
		for _, dev := range devices {
			if dev == r.devicePath {
				deviceStrToConnect = dev
				break
			}
		}
		if deviceStrToConnect == "" {
			log.Printf("Preferred device path %s not found in current device list. Will try first available.", r.devicePath)
		}
	}

	if deviceStrToConnect == "" { // If no preferred path, or preferred not found, use the first device
		deviceStrToConnect = devices[0]
		log.Printf("No preferred device or preferred not found. Using first available device: %s", deviceStrToConnect)
	}

	log.Printf("Attempting to connect to device: %s", deviceStrToConnect)
	newDevice, errOpen := r.nfcManager.OpenDevice(deviceStrToConnect)
	if errOpen != nil {
		errMsg := fmt.Sprintf("Failed to open device %s: %v", deviceStrToConnect, errOpen)
		r.setDeviceStatus(false, errMsg) // Update status
		return fmt.Errorf(errMsg)
	}

	if errInit := newDevice.InitiatorInit(); errInit != nil {
		newDevice.Close() // Close the device if init fails
		errMsg := fmt.Sprintf("Failed to initialize device %s: %v", deviceStrToConnect, errInit)
		r.setDeviceStatus(false, errMsg) // Update status
		return fmt.Errorf(errMsg)
	}

	// Successfully connected and initialized
	r.device = newDevice
	r.hasDevice = true
	r.devicePath = deviceStrToConnect // Update to the successfully connected device path
	r.cache.clear()                   // Clear cache on successful new connection/reconnection

	deviceName := newDevice.String()
	connectionInfo := newDevice.Connection()
	log.Printf("NFC device connected - Name: %s, Connection: %s, Path: %s",
		deviceName, connectionInfo, deviceStrToConnect)
	// setDeviceStatus also updates r.cardPresent based on its internal logic, ensure it's consistent
	r.setDeviceStatus(true, fmt.Sprintf("Connected to %s via %s", deviceName, connectionInfo))
	// After successful connection, card is not yet known to be present until a scan happens.
	// setCardPresent(false) was called earlier, which is correct.
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

func searchSectorKey(tag FreefareTagInterface, sector byte, key *[6]byte, keyType *int) error {
	block := freefare.ClassicSectorLastBlock(sector)
	// log.Printf("searchSectorKey: last block for sector %d is %d", sector, block)

	// The original C code disconnects/reconnects frequently. This might be specific to libfreefare's C state machine.
	// In Go, with interfaces, we assume Connect/Disconnect manage state appropriately.
	// However, to mimic behavior closely if issues arise, this pattern is kept but simplified.
	// Initial disconnect, then connect before each auth attempt.

	if err := tag.Disconnect(); err != nil {
		if !strings.Contains(err.Error(), "tag state error") && !strings.Contains(err.Error(), "not connected") {
			log.Printf("searchSectorKey: warning during initial disconnect for sector %d: %v", sector, err)
		}
	}

	for i := 0; i < len(defaultKeys); i++ {
		currentKeyToTry := defaultKeys[i]

		// Try KeyA
		if err := tag.Connect(); err != nil {
			log.Printf("searchSectorKey: connect error before KeyA auth with %X for sector %d: %v", currentKeyToTry, sector, err)
			tag.Disconnect() // Attempt to clean up
			continue
		}
		// Assuming freefare.KeyA is an int constant for keyType
		authnErr := tag.Authenticate(block, currentKeyToTry, int(freefare.KeyA))
		if authnErr == nil {
			// Assuming freefare.WriteKeyA, freefare.WriteAccessBits, freefare.WriteKeyB are int constants for perm
			// And freefare.KeyA is int for keyType used in TrailerBlockPermission
			writeAPerm, _ := tag.TrailerBlockPermission(block, uint16(freefare.WriteKeyA), int(freefare.KeyA))          // Cast to uint16
			accessBitPerm, _ := tag.TrailerBlockPermission(block, uint16(freefare.WriteAccessBits), int(freefare.KeyA)) // Cast to uint16
			writeBPerm, _ := tag.TrailerBlockPermission(block, uint16(freefare.WriteKeyB), int(freefare.KeyA))          // Cast to uint16

			if writeAPerm && accessBitPerm && writeBPerm {
				*key = currentKeyToTry
				*keyType = int(freefare.KeyA)
				return nil
			}
		}
		tag.Disconnect() // Disconnect after KeyA attempt (successful auth but no full perm, or auth fail)

		// Try KeyB
		if err := tag.Connect(); err != nil {
			log.Printf("searchSectorKey: connect error before KeyB auth with %X for sector %d: %v", currentKeyToTry, sector, err)
			tag.Disconnect()
			continue
		}
		// Assuming freefare.KeyB is an int constant for keyType
		authnErr = tag.Authenticate(block, currentKeyToTry, int(freefare.KeyB))
		if authnErr == nil {
			// Assuming freefare.WriteKeyA etc are int for perm, and freefare.KeyB is int for keyType
			writeAPerm, _ := tag.TrailerBlockPermission(block, uint16(freefare.WriteKeyA), int(freefare.KeyB))          // Cast to uint16
			accessBitPerm, _ := tag.TrailerBlockPermission(block, uint16(freefare.WriteAccessBits), int(freefare.KeyB)) // Cast to uint16
			writeBPerm, _ := tag.TrailerBlockPermission(block, uint16(freefare.WriteKeyB), int(freefare.KeyB))          // Cast to uint16

			if writeAPerm && accessBitPerm && writeBPerm {
				*key = currentKeyToTry
				*keyType = int(freefare.KeyB)
				return nil
			}
		}
		tag.Disconnect() // Disconnect after KeyB attempt
	}

	return fmt.Errorf("searchSectorKey error: no known authentication key with full permissions for sector %d", sector)
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

func (r *NFCReader) writeData(tag FreefareTagInterface, text string) error {
	// This function primarily initializes a factory mode card.
	// The `text` argument is not used in the current implementation after initialization.

	if err := tag.Connect(); err != nil {
		return fmt.Errorf("writeData connect error: %v", err)
	}
	// Defer disconnect to ensure it happens, but individual operations might manage it too.
	// Let's make it more granular.
	// defer tag.Disconnect()

	sector0TrailerBlock := freefare.ClassicSectorLastBlock(0)
	// Authenticate with factoryKey on sector 0 trailer to check for factory mode
	authErr := tag.Authenticate(sector0TrailerBlock, factoryKey, int(freefare.KeyA))
	if authErr == nil {
		log.Println("writeData: Detected factory mode card (authenticated sector 0 with factoryKey), proceeding with initialization...")

		maxSectorIdx := 15 // Sectors 0-15 for 1K card (total 16 sectors)
		// Assuming tag.Type() returns int, and freefare.Classic4k is an int constant
		if tag.Type() == int(freefare.Classic4k) {
			maxSectorIdx = 39 // Sectors 0-39 for 4K card (total 40 sectors)
		}

		for sectorIdx := 0; sectorIdx <= maxSectorIdx; sectorIdx++ {
			currentSector := byte(sectorIdx)
			currentSectorTrailerBlock := freefare.ClassicSectorLastBlock(currentSector)

			// Each sector operation might need its own connect/auth cycle, especially after writing.
			// Disconnect before potential re-authentication for the same or new sector.
			tag.Disconnect()
			if errCon := tag.Connect(); errCon != nil {
				return fmt.Errorf("writeData: failed to connect before auth for sector %d: %v", currentSector, errCon)
			}

			// Authenticate current sector's trailer block with factoryKey before writing new trailer.
			// Assuming factoryKey is [6]byte and freefare.KeyA is int for keyType
			if errAuthSector := tag.Authenticate(currentSectorTrailerBlock, factoryKey, int(freefare.KeyA)); errAuthSector != nil {
				// If this is sector 0 and we already passed the initial factory check, this shouldn't fail with factoryKey.
				// For other sectors, this is the expected auth.
				return fmt.Errorf("writeData: failed to authenticate sector %d with factory key: %v", currentSector, errAuthSector)
			}

			trailerData := [16]byte{}
			var keyA, keyB [6]byte

			// Define keys and access bits based on whether it's a MAD sector or application sector
			// Assuming tag.Type() returns int, and freefare.Classic4k is an int constant
			if currentSector == 0 || (tag.Type() == int(freefare.Classic4k) && currentSector == 16) { // MAD sectors (0 for 1K/4K, 16 for 4K MAD2)
				keyA = defaultKeyA // NFC Forum MAD Key A (A0A1A2A3A4A5)
				keyB = factoryKey  // Key B for MAD can be factoryKey or a specific one (e.g., defaultKeyB for NDEF compatibility)
				// Access bits for MAD: KeyA r/w for trailer, KeyA r/w for keys, KeyA r for data blocks.
				// GPB C1 (0xC1) for MAD: Key A may change Key A and Key B. Access bits are R/W with Key A. Data blocks are R/O with Key A.
				MifareClassicTrailerBlock2(&trailerData, keyA, 0x78, 0x77, 0x88, 0xC1, keyB)
				log.Printf("writeData: Preparing MAD trailer for sector %d with KeyA=%X, KeyB=%X", currentSector, keyA, keyB)
			} else { // Application sectors (for NDEF data)
				keyA = publicKey  // Public Key for NDEF applications (e.g., D3F7D3F7D3F7)
				keyB = factoryKey // Key B can be factoryKey or same as KeyA for NDEF.
				// Access bits for NDEF data: KeyA r/w for trailer, KeyA r/w for keys, KeyA r/w for data blocks.
				// GPB 40 (0x40) for NDEF: Key A may change Key A and Key B. Access bits are R/W with Key A. Data blocks are R/W with Key A.
				MifareClassicTrailerBlock2(&trailerData, keyA, 0x7F, 0x07, 0x88, 0x40, keyB)
				log.Printf("writeData: Preparing NDEF App trailer for sector %d with KeyA=%X, KeyB=%X", currentSector, keyA, keyB)
			}

			if errWrite := tag.WriteBlock(currentSectorTrailerBlock, trailerData); errWrite != nil {
				return fmt.Errorf("writeData: failed to write trailer for sector %d: %v", currentSector, errWrite)
			}
			log.Printf("writeData: Successfully wrote trailer for sector %d", currentSector)

			// After writing a trailer, the keys for that sector have changed.
			// Any subsequent operations on this sector (even reading) would need to auth with the new keys.
		}
		log.Println("writeData: Card initialized successfully from factory mode.")
		// The card is now initialized with MAD/NDEF keys and permissions.
		// Actual NDEF message writing (the `text` content) is a separate step.
	} else {
		log.Printf("writeData: Card does not appear to be in factory mode (authentication of sector 0 with factoryKey failed: %v). Skipping initialization.", authErr)
		// If not factory mode, one might attempt to write NDEF data directly if keys are known.
		// This function's current scope is primarily initialization.
	}

	tag.Disconnect() // Ensure final disconnect

	// If the goal was to also write `text` as NDEF data after initialization:
	if text != "" {
		log.Println("writeData: NDEF text writing part is conceptual. Card initialized (if was factory), but actual NDEF message from 'text' not written by this function.")
		// This would involve:
		// 1. Re-Connect and Authenticate with appropriate NDEF application keys (e.g., publicKey on relevant sectors).
		// 2. Encode `text` into an NDEF message structure (TLV format).
		// 3. Write this NDEF message to the data blocks of NDEF application sectors.
		// This is a more complex operation, potentially using freefare.Mad methods or manual block writes.
		// For example: return r.writeNdefMessageToTag(tag, text) // Call to a dedicated NDEF writing function
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

		tags, err := r.getTags()
		if err != nil {
			return fmt.Errorf("failed to get tags: %v", err)
		}

		if len(tags) == 0 {
			return fmt.Errorf("no card detected")
		}

		for _, tag := range tags {
			if tag.Type() != int(freefare.Classic1k) && tag.Type() != int(freefare.Classic4k) {
				continue
			}

			return r.writeData(tag, text)
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

// getTags retrieves available tags from the connected NFC device.
// It now uses the NFCManagerInterface.
func (r *NFCReader) getTags() ([]FreefareTagInterface, error) {
	if !r.hasDevice || r.device == nil {
		return nil, fmt.Errorf("getTags: no device connected or device is nil")
	}
	// r.device is NFCDeviceInterface. r.nfcManager.GetTags expects this.
	tags, err := r.nfcManager.GetTags(r.device)
	if err != nil {
		return nil, fmt.Errorf("getTags: error from nfcManager.GetTags: %w", err)
	}
	return tags, nil
}
