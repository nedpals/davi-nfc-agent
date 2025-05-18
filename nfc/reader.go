package nfc

import (
	"fmt"
	"log"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/clausecker/freefare"
	// "github.com/clausecker/nfc/v2" // Not directly used by reader, but by manager
)

// NFCReader manages NFC device interactions and broadcasts tag data.
type NFCReader struct {
	device            DeviceInterface  // Interface for the NFC device
	nfcManager        ManagerInterface // Interface for managing NFC devices
	hasDevice         bool
	dataChan          chan NFCData      // Broadcasts successfully read NFC data
	statusChan        chan DeviceStatus // Broadcasts device status updates
	stopChan          chan struct{}     // Signals the worker to stop
	cache             *TagCache         // Caches tag data
	deviceStatus      DeviceStatus      // Internal tracking of device status
	statusMux         sync.RWMutex
	devicePath        string        // Path of the connected device
	cardPresent       bool          // Internal tracking of card presence
	isWriting         bool          // Tracks if a write operation is in progress
	operationMutex    sync.Mutex    // Protects tag operations (read/write)
	operationTimeout  time.Duration // Timeout for tag operations
	cooldownTimer     *time.Timer   // Timer for device error cooldown
	inCooldown        bool          // Flag indicating if device is in cooldown
	deviceCheckTicker *time.Ticker  // Ticker for periodic device checks
	cardCheckTicker   *time.Ticker  // Ticker for periodic card presence checks (based on cache)
}

// NewNFCReader creates and initializes a new NFCReader instance.
func NewNFCReader(deviceStr string, manager ManagerInterface, opTimeout time.Duration) (*NFCReader, error) {
	if manager == nil {
		return nil, fmt.Errorf("NFCManagerInterface cannot be nil")
	}
	if opTimeout <= 0 {
		opTimeout = 5 * time.Second // Default operation timeout
	}

	reader := &NFCReader{
		nfcManager:       manager,
		hasDevice:        false,
		dataChan:         make(chan NFCData, 1),      // Buffered to prevent blocking on send if no listener
		statusChan:       make(chan DeviceStatus, 1), // Buffered for status updates
		stopChan:         make(chan struct{}),
		cache:            NewTagCache(),
		devicePath:       deviceStr,
		cardPresent:      false,
		deviceStatus:     DeviceStatus{Connected: false, Message: "Initializing...", CardPresent: false},
		operationTimeout: opTimeout,
	}

	// Initial device connection attempt (non-blocking for constructor)
	go func() {
		if deviceStr == "" {
			devices, err := manager.ListDevices()
			if err != nil {
				log.Printf("Error listing devices during initial scan: %v", err)
			} else if len(devices) == 0 {
				log.Println("No NFC devices found during initial scan.")
			} else {
				reader.devicePath = devices[0] // Use the first device found
				log.Printf("No device specified, found and will attempt to use: %s", reader.devicePath)
			}
		}

		if reader.devicePath != "" {
			log.Printf("Attempting to open initial device: %s", reader.devicePath)
			if err := reader.tryConnect(); err != nil {
				log.Printf("Failed to connect to device %s initially: %v. Worker will keep trying.", reader.devicePath, err)
				// Status already set by tryConnect
			} else {
				log.Printf("Successfully opened device: %s", reader.devicePath)
				// Status and device info logged by tryConnect
			}
		} else {
			log.Println("No device string specified and no devices automatically found. Worker will attempt to find one.")
			reader.setDeviceStatus(false, "No device connected, searching...")
		}
	}()

	return reader, nil
}

// Close releases resources. Does not stop the worker, use Stop() for that.
func (r *NFCReader) Close() {
	log.Println("NFCReader Close called (resource cleanup).")
	r.statusMux.Lock()
	if r.hasDevice && r.device != nil {
		log.Println("Closing NFC device in Close().")
		if err := r.device.Close(); err != nil {
			log.Printf("Error closing NFC device in Close(): %v", err)
		}
		r.device = nil
		r.hasDevice = false
	}
	r.statusMux.Unlock()
	// Note: Channels dataChan, statusChan are not closed here as they might be read by other goroutines.
	// They are managed by the lifecycle of the NFCReader user.
}

// Stop gracefully shuts down the NFCReader worker.
func (r *NFCReader) Stop() {
	log.Println("Stopping NFCReader...")
	select {
	case <-r.stopChan:
		log.Println("Stop channel already closed or closing.")
		return // Already stopping or stopped
	default:
		close(r.stopChan)
		log.Println("Stop channel successfully closed.")
	}
	// Worker's defer will handle device closing and final status.
}

// Start begins the NFC reading process in a separate goroutine.
func (r *NFCReader) Start() {
	log.Println("NFCReader Start called, starting worker.")
	go r.worker()
}

// Data returns a channel that provides NFCData as tags are read.
func (r *NFCReader) Data() <-chan NFCData {
	return r.dataChan
}

// StatusUpdates returns a channel that provides DeviceStatus updates.
func (r *NFCReader) StatusUpdates() <-chan DeviceStatus {
	return r.statusChan
}

// GetDeviceStatus returns the current device status.
func (r *NFCReader) GetDeviceStatus() DeviceStatus {
	r.statusMux.RLock()
	defer r.statusMux.RUnlock()
	return r.deviceStatus
}

func (r *NFCReader) reconnect() error {
	log.Printf("Attempting to reconnect device (path hint: %s)...", r.devicePath)
	r.cache.Clear()
	r.setCardPresent(false) // Updates status and sends to statusChan

	r.statusMux.Lock()
	if r.hasDevice && r.device != nil {
		log.Println("Closing existing device connection before reconnect attempt.")
		r.device.Close() // Ignore error, device might be in bad state
		r.device = nil
		r.hasDevice = false
	}
	r.statusMux.Unlock()

	var lastErr error
	for attempt := 1; attempt <= MaxReconnectTries; attempt++ {
		r.setDeviceStatus(false, fmt.Sprintf("Attempting to reconnect (attempt %d/%d)", attempt, MaxReconnectTries))

		if err := r.tryConnect(); err == nil {
			log.Printf("Reconnection attempt %d successful.", attempt)
			// Check for tags to update card presence immediately
			tags, errTags := r.getTags()
			if errTags == nil && len(tags) > 0 {
				if len(tags) > 0 { // Ensure tags is not empty before accessing tags[0]
					r.cache.UpdateLastSeenTime(tags[0].UID()) // Mark activity using first tag's UID
				}
				r.setCardPresent(true)
				log.Println("Card detected immediately after reconnect.")
			} else {
				r.setCardPresent(false)
				if errTags != nil {
					log.Printf("Error getting tags after reconnect: %v", errTags)
				}
			}
			return nil
		} else {
			lastErr = err
			log.Printf("Reconnection attempt %d failed: %v", attempt, err)
		}

		select {
		case <-r.stopChan:
			log.Println("Reconnect: Stop signal received, aborting reconnection.")
			return fmt.Errorf("reconnection aborted by stop signal")
		case <-time.After(ReconnectDelay * time.Duration(attempt)): // Basic exponential backoff
		}
	}

	errMsg := fmt.Sprintf("failed to reconnect device (path hint: %s) after %d attempts: %v", r.devicePath, MaxReconnectTries, lastErr)
	r.setDeviceStatus(false, "Failed to reconnect after multiple attempts")
	log.Println(errMsg)
	return fmt.Errorf(errMsg)
}

func (r *NFCReader) forceReconnectDevice() error {
	log.Println("Attempting to force reconnect device...")
	r.statusMux.Lock()
	if r.hasDevice && r.device != nil {
		log.Println("Closing current device for force reconnect.")
		r.device.Close() // Ignore error
		r.device = nil
		r.hasDevice = false
	}
	r.statusMux.Unlock()

	// Some devices (like ACR122U) might need a longer pause.
	log.Println("Waiting for device to reset after close...")
	select {
	case <-time.After(time.Second * 3):
	case <-r.stopChan:
		log.Println("Force Reconnect: Stop signal received during wait, aborting.")
		return fmt.Errorf("force reconnection aborted by stop signal")
	}

	r.cache.Clear()
	r.setCardPresent(false)

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		log.Printf("Force reconnect attempt %d/3...", attempt+1)
		if err := r.tryConnect(); err != nil {
			lastErr = err
			log.Printf("Force reconnect attempt %d failed: %v", attempt+1, err)
			select {
			case <-r.stopChan:
				log.Println("Force Reconnect: Stop signal received, aborting.")
				return fmt.Errorf("force reconnection aborted by stop signal")
			case <-time.After(time.Second * time.Duration(attempt+1)):
			}
			continue
		}
		log.Println("Force reconnect successful.")
		return nil
	}

	errMsg := "force reconnect failed after multiple attempts"
	if lastErr != nil {
		errMsg = fmt.Sprintf("%s: %v", errMsg, lastErr)
	}
	r.setDeviceStatus(false, "Force reconnect failed")
	log.Println(errMsg)
	return fmt.Errorf(errMsg)
}

func (r *NFCReader) worker() {
	log.Println("NFCReader worker started.")
	defer log.Println("NFCReader worker stopped.")

	r.deviceCheckTicker = time.NewTicker(DeviceCheckInterval)
	r.cardCheckTicker = time.NewTicker(250 * time.Millisecond) // For updating r.cardPresent from cache
	r.cooldownTimer = time.NewTimer(0)                         // Timer for cooldown period
	if !r.cooldownTimer.Stop() {
		select {
		case <-r.cooldownTimer.C:
		default:
		} // Drain if already fired
	}
	r.inCooldown = false
	retryCount := 0

	defer func() {
		r.deviceCheckTicker.Stop()
		r.cardCheckTicker.Stop()
		r.cooldownTimer.Stop()

		r.statusMux.Lock()
		if r.hasDevice && r.device != nil {
			if err := r.device.Close(); err != nil {
				log.Printf("NFCReader worker defer: Error closing device: %v", err)
			}
			r.device = nil
			r.hasDevice = false
		}
		r.statusMux.Unlock()
		r.setDeviceStatus(false, "Worker stopped, device disconnected.")
	}()

	for {
		select {
		case <-r.stopChan:
			return

		case <-r.deviceCheckTicker.C:
			r.statusMux.RLock()
			hasDev := r.hasDevice
			inCool := r.inCooldown
			r.statusMux.RUnlock()

			if !hasDev && !inCool {
				log.Println("Device check ticker: No device or not in cooldown, attempting connect.")
				if err := r.tryConnect(); err != nil {
					log.Printf("Device check ticker: Connection attempt failed: %v", err)
				} else {
					log.Println("Device check ticker: Connection successful.")
					retryCount = 0
				}
			}

		case <-r.cardCheckTicker.C:
			currentCacheCardPresent := r.cache.IsCardPresent()
			r.statusMux.RLock()
			cardPres := r.cardPresent
			r.statusMux.RUnlock()

			if cardPres != currentCacheCardPresent {
				r.setCardPresent(currentCacheCardPresent) // This updates deviceStatus and broadcasts
				if currentCacheCardPresent {
					uid, _ := r.cache.GetLastScanned()
					log.Printf("Card presence changed via cache: DETECTED (UID: %s)", uid)
				} else {
					log.Println("Card presence changed via cache: REMOVED/timed out")
				}
			}

		case <-r.cooldownTimer.C:
			log.Println("Device cooldown period ended.")
			r.statusMux.Lock()
			r.inCooldown = false
			r.statusMux.Unlock()
			if err := r.forceReconnectDevice(); err != nil {
				log.Printf("Reconnection after cooldown failed: %v.", err)
			}

		default:
			r.statusMux.RLock()
			hasDev := r.hasDevice
			dev := r.device
			inCool := r.inCooldown
			isWrite := r.isWriting
			r.statusMux.RUnlock()

			if !hasDev || dev == nil || inCool {
				time.Sleep(200 * time.Millisecond)
				continue
			}
			if isWrite {
				time.Sleep(50 * time.Millisecond)
				continue
			}

			tags, err := r.getTags() // Uses FreefareTagProvider
			if err != nil {
				log.Printf("Error getting tags: %v", err)
				originalErrorString := err.Error()

				if IsIOError(err) || IsDeviceConfigError(err) {
					log.Printf("Device error detected (IO/Config): %v. Closing device.", err)
					r.statusMux.Lock()
					if r.hasDevice && r.device != nil {
						r.device.Close()
					}
					r.device = nil
					r.hasDevice = false
					r.isWriting = false
					r.statusMux.Unlock()
					r.setDeviceStatus(false, fmt.Sprintf("Device error: %v", err))

					if strings.Contains(originalErrorString, "Operation not permitted") ||
						strings.Contains(originalErrorString, "broken pipe") ||
						strings.Contains(originalErrorString, "RDR_to_PC_DataBlock") {
						r.statusMux.Lock()
						if !r.inCooldown {
							r.inCooldown = true
							cooldownPeriod := time.Second * 10
							log.Printf("ACR122-like error. Entering cooldown for %v", cooldownPeriod)
							r.cooldownTimer.Reset(cooldownPeriod)
						}
						r.statusMux.Unlock()
						continue
					}
					log.Println("Attempting force reconnect after IO/Config error...")
					time.Sleep(time.Second * 1) // Brief pause
					if errReconnect := r.forceReconnectDevice(); errReconnect != nil {
						log.Printf("Force reconnection failed after IO/Config error: %v", errReconnect)
					}
					continue
				}

				if IsTimeoutError(err) || IsDeviceClosedError(err) {
					log.Printf("Device error (Timeout/Closed): %v", err)
					delay := time.Duration(math.Pow(2, float64(retryCount))) * BaseDelay
					if retryCount < MaxRetries {
						retryCount++
						log.Printf("Retrying connection (attempt %d/%d) in %v...", retryCount, MaxRetries, delay)
						select {
						case <-time.After(delay):
						case <-r.stopChan:
							return
						}
						r.statusMux.Lock()
						r.isWriting = false
						r.statusMux.Unlock()
						if errReconnect := r.reconnect(); errReconnect != nil {
							log.Printf("Device reconnection failed: %v", errReconnect)
						} else {
							log.Println("Reconnected successfully.")
							retryCount = 0
						}
					} else {
						log.Printf("Max retries reached for Timeout/Closed error: %v. Closing device.", err)
						r.statusMux.Lock()
						if r.hasDevice && r.device != nil {
							r.device.Close()
						}
						r.device = nil
						r.hasDevice = false
						if !r.inCooldown { // Enter long cooldown
							r.inCooldown = true
							r.cooldownTimer.Reset(time.Second * 30)
							log.Println("Entering long cooldown after max retries for Timeout/Closed error.")
						}
						r.statusMux.Unlock()
						r.setDeviceStatus(false, "Max retries for device error.")
					}
					continue
				}
				log.Printf("Unhandled error from getTags: %v. Sending to dataChan.", err)
				r.dataChan <- NFCData{Err: fmt.Errorf("get tags error: %v", err)}
				time.Sleep(time.Second)
				continue
			}
			retryCount = 0 // Reset error count on successful getTags

			if len(tags) > 0 {
				atLeastOneValidTagProcessed := false
				for _, tag := range tags { // tag is FreefareTagProvider
					if tag.NumericType() != int(freefare.Classic1k) && tag.NumericType() != int(freefare.Classic4k) {
						continue
					}
					uid := tag.UID()
					text, readErr := r.readTagData(tag) // tag is FreefareTagProvider

					if uid != "" { // A tag was physically present enough to get UID
						r.cache.UpdateLastSeenTime(uid) // Mark activity for this tag
						atLeastOneValidTagProcessed = true
					}

					if readErr != nil {
						log.Printf("Error reading data for tag UID %s: %v", uid, readErr)
						r.dataChan <- NFCData{UID: uid, Text: "", Err: readErr}
						continue
					}

					if r.cache.HasChanged(uid, text) {
						log.Printf("Tag data changed or new tag: UID %s", uid)
						r.dataChan <- NFCData{UID: uid, Text: text, Err: nil}
					}
				}
				if !atLeastOneValidTagProcessed && r.cardPresent {
					// No classic tags found, but cardPresent was true. Cache timeout will handle this.
				}
			}
			time.Sleep(100 * time.Millisecond) // Polling interval
		}
	}
}

func (r *NFCReader) setDeviceStatus(connected bool, message string) {
	r.statusMux.Lock()
	r.deviceStatus.Connected = connected
	r.deviceStatus.Message = message
	// r.deviceStatus.CardPresent is managed by setCardPresent
	currentStatus := r.deviceStatus // Make a copy to send
	r.statusMux.Unlock()

	select {
	case r.statusChan <- currentStatus:
	default:
		log.Println("Warning: Device status channel full or no listener.")
	}
}

func (r *NFCReader) readTagData(tag TagInterface) (string, error) {
	rawNdefMessage, err := tag.ReadData() // Use the ReadData method from TagInterface
	if err != nil {
		// Log the specific UID if available, though tag.UID() might also error if tag is problematic
		// For now, keep it simple.
		return "", fmt.Errorf("readTagData: error from tag.ReadData(): %w", err)
	}

	if rawNdefMessage == nil {
		// This can occur for factory mode cards or cards with no NDEF application.
		// The underlying ReadData implementation (e.g., in RealClassicTagAdapter) should log details.
		log.Println("readTagData: received nil NDEF message, likely factory mode or no NDEF data.")
		return "", nil // No error, but no data to parse
	}

	// Parse the raw NDEF message for a text record
	text, parseErr := ParseNdefMessageForTextRecord(rawNdefMessage)
	if parseErr != nil {
		return "", fmt.Errorf("readTagData: error parsing NDEF message: %w", parseErr)
	}
	return text, nil
}

func (r *NFCReader) tryConnect() error {
	r.statusMux.RLock()
	hasDev := r.hasDevice
	currentDevice := r.device
	r.statusMux.RUnlock()

	var initErr error // Declare initErr here for wider scope
	if hasDev && currentDevice != nil {
		// Quick check if device is responsive. This can be slow or hang.
		// Consider timeout if it becomes an issue.
		initErr = currentDevice.InitiatorInit() // Assign to outer initErr
		if initErr == nil {
			log.Println("Device already connected and responsive.")
			// Ensure status is correct if it was previously disconnected message
			r.setDeviceStatus(true, fmt.Sprintf("Connected to %s", currentDevice.String()))
			return nil
		}
		log.Printf("Device was marked connected, but Init failed: %v. Attempting full reconnect.", initErr)
		r.statusMux.Lock()
		currentDevice.Close() // Ignore error
		r.device = nil
		r.hasDevice = false
		r.statusMux.Unlock()
	}

	r.setCardPresent(false) // Update status before connection attempt

	devicePathToConnect := r.devicePath // Use stored path if available
	if devicePathToConnect == "" {
		devices, errList := r.nfcManager.ListDevices()
		if errList != nil {
			errMsg := fmt.Sprintf("Error listing NFC devices: %v", errList)
			r.setDeviceStatus(false, errMsg)
			return fmt.Errorf(errMsg)
		}
		if len(devices) == 0 {
			errMsg := "No NFC devices found by manager."
			r.setDeviceStatus(false, "Waiting for NFC reader...")
			return fmt.Errorf(errMsg)
		}
		devicePathToConnect = devices[0] // Try first available
		log.Printf("No specific device path, trying first available: %s", devicePathToConnect)
	}

	log.Printf("Attempting to connect to device: %s", devicePathToConnect)
	newDevice, errOpen := r.nfcManager.OpenDevice(devicePathToConnect)
	if errOpen != nil {
		errMsg := fmt.Sprintf("Failed to open device %s: %v", devicePathToConnect, errOpen)
		r.setDeviceStatus(false, errMsg)
		return fmt.Errorf(errMsg)
	}

	if errInit := newDevice.InitiatorInit(); errInit != nil {
		newDevice.Close() // Close if init fails
		errMsg := fmt.Sprintf("Failed to initialize device %s: %v", devicePathToConnect, errInit)
		r.setDeviceStatus(false, errMsg)
		return fmt.Errorf(errMsg)
	}

	r.statusMux.Lock()
	r.device = newDevice
	r.hasDevice = true
	r.devicePath = devicePathToConnect // Update to successfully connected path
	r.statusMux.Unlock()

	r.cache.Clear() // Clear cache on successful new connection
	r.LogDeviceInfo()
	r.setDeviceStatus(true, fmt.Sprintf("Connected to %s", newDevice.String()))
	return nil
}

// LogDeviceInfo logs information about the connected NFC device.
func (r *NFCReader) LogDeviceInfo() {
	r.statusMux.RLock()
	defer r.statusMux.RUnlock()
	if !r.hasDevice || r.device == nil {
		return
	}
	name := r.device.String()
	connString := r.device.Connection() // Assuming DeviceInterface has Connection()
	log.Printf("Connected NFC device: %s (Connection: %s, Path: %s)", name, connString, r.devicePath)
}

// GetLastScannedData retrieves the last scanned UID and text from the cache.
func (r *NFCReader) GetLastScannedData() (string, string) {
	return r.cache.GetLastScanned()
}

func (r *NFCReader) setCardPresent(present bool) {
	r.statusMux.Lock()
	if r.cardPresent == present && r.deviceStatus.CardPresent == present { // Avoid redundant updates
		r.statusMux.Unlock()
		return
	}
	r.cardPresent = present
	r.deviceStatus.CardPresent = present
	if present {
		uid, _ := r.cache.GetLastScanned()
		if uid != "" {
			r.deviceStatus.Message = fmt.Sprintf("Card detected (UID: %s)", uid)
		} else {
			r.deviceStatus.Message = "Card detected"
		}
	} else {
		r.deviceStatus.Message = "Card removed"
		r.cache.Clear() // Clear cache when card is definitively removed
	}
	currentStatus := r.deviceStatus // Make a copy to send
	r.statusMux.Unlock()

	select {
	case r.statusChan <- currentStatus:
	default:
		log.Println("Warning: Card presence status channel full or no listener.")
	}
}

// writeData initializes a factory mode card to NDEF format.
// Actual NDEF message writing is a separate step (conceptual in original).
func (r *NFCReader) writeData(tag TagInterface, text string) error {
	var ndefMessage []byte

	if text != "" {
		// Encode the text string into an NDEF message payload.
		// Assuming EncodeTextPayload is available in the nfc package (e.g., from ndef_utils.go).
		// Defaulting to "en" language code.
		// Corrected to use EncodeNdefMessageWithTextRecord from ndef_utils.go
		ndefMessage = EncodeNdefMessageWithTextRecord(text, "en")
		// err is not returned by EncodeNdefMessageWithTextRecord, so no error check needed here.
		log.Printf("writeData: Encoded text to NDEF message of %d bytes.", len(ndefMessage))
	} else {
		log.Println("writeData: No text provided; will call WriteData with nil to ensure card is NDEF ready/initialized.")
		// Passing nil or empty slice to WriteData on RealClassicTagAdapter
		// should trigger initialization logic if the card is in factory mode.
		ndefMessage = nil
	}

	// Call the WriteData method from TagInterface
	// This will handle factory initialization if needed (e.g., in RealClassicTagAdapter)
	// and then write the NDEF message.
	if err := tag.WriteData(ndefMessage); err != nil {
		return fmt.Errorf("writeData: error from tag.WriteData(%d bytes): %w", len(ndefMessage), err)
	}

	log.Println("writeData: tag.WriteData() completed successfully.")
	return nil
}

// WriteCardData attempts to write data to a detected NFC card.
// Currently, this means initializing a factory mode card.
func (r *NFCReader) WriteCardData(text string) error {
	return r.withTagOperation(func() error {
		r.statusMux.RLock()
		hasDev := r.hasDevice
		r.statusMux.RUnlock()
		if !hasDev {
			return fmt.Errorf("no NFC device connected")
		}

		r.statusMux.Lock()
		r.isWriting = true
		r.statusMux.Unlock()
		defer func() {
			r.statusMux.Lock()
			r.isWriting = false
			r.statusMux.Unlock()
		}()

		tags, err := r.getTags()
		if err != nil {
			return fmt.Errorf("failed to get tags for writing: %w", err)
		}
		if len(tags) == 0 {
			return fmt.Errorf("no card detected for writing")
		}

		for _, tag := range tags { // tag is FreefareTagProvider
			if tag.NumericType() == int(freefare.Classic1k) || tag.NumericType() == int(freefare.Classic4k) { // Use NumericType
				log.Printf("Attempting to write to compatible tag UID: %s", tag.UID())
				// The 'text' argument is passed but currently unused by writeData's main logic.
				return r.writeData(tag, text)
			}
		}
		return fmt.Errorf("no compatible (MIFARE Classic) card found for writing")
	})
}

// withTagOperation performs a protected tag operation with timeout.
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
		// Attempt to signal the operation to stop if possible (e.g. context cancellation)
		// For now, just return timeout. The operation might still be running.
		return fmt.Errorf("operation timed out after %v", r.operationTimeout)
	}
}

// getTags retrieves available tags from the connected NFC device.
func (r *NFCReader) getTags() ([]TagInterface, error) {
	r.statusMux.RLock()
	hasDev := r.hasDevice
	dev := r.device
	manager := r.nfcManager
	r.statusMux.RUnlock()

	if !hasDev || dev == nil {
		return nil, fmt.Errorf("getTags: no device connected or device is nil")
	}
	if manager == nil {
		return nil, fmt.Errorf("getTags: nfcManager is nil")
	}

	tags, err := manager.GetTags(dev) // ManagerInterface.GetTags now returns []TagInterface
	if err != nil {
		return nil, fmt.Errorf("getTags: error from nfcManager.GetTags: %w", err)
	}
	return tags, nil
}
