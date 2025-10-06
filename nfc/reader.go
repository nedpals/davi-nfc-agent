package nfc

import (
	"fmt"
	"log"
	"math"
	"strings"
	"sync"
	"time"
)

// Polling intervals
const (
	DefaultPollingInterval      = 100 * time.Millisecond
	DeviceIdleCheckInterval     = 200 * time.Millisecond
	WriteCheckInterval          = 50 * time.Millisecond
	CardCheckTickerInterval     = 250 * time.Millisecond
	DeviceResetWaitTime         = 3 * time.Second
	DeviceErrorCooldownPeriod   = 10 * time.Second
	MaxRetriesCooldownPeriod    = 30 * time.Second
	PostErrorPauseTime          = 1 * time.Second
	UnhandledErrorRetryInterval = 1 * time.Second
)

// NFCReader manages NFC device interactions and broadcasts tag data.
type NFCReader struct {
	device            Device  // Interface for the NFC device
	nfcManager        Manager // Interface for managing NFC devices
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
	workerWg          sync.WaitGroup // Tracks worker goroutine completion
}

// NewNFCReader creates and initializes a new NFCReader instance.
func NewNFCReader(deviceStr string, manager Manager, opTimeout time.Duration) (*NFCReader, error) {
	if manager == nil {
		return nil, fmt.Errorf("NFCManager cannot be nil")
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

// Stop gracefully shuts down the NFCReader worker and waits for it to complete.
func (r *NFCReader) Stop() {
	log.Println("Stopping NFCReader...")
	select {
	case <-r.stopChan:
		log.Println("Stop channel already closed or closing.")
		return // Already stopping or stopped
	default:
		close(r.stopChan)
		log.Println("Stop channel successfully closed, waiting for worker to finish...")
	}
	// Wait for the worker to finish
	r.workerWg.Wait()
	log.Println("NFCReader worker stopped successfully.")
	// Worker's defer will handle device closing and final status.
}

// Start begins the NFC reading process in a separate goroutine.
func (r *NFCReader) Start() {
	log.Println("NFCReader Start called, starting worker.")
	r.workerWg.Add(1)
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

// reconnectDevice attempts to reconnect to the NFC device with configurable retry logic.
// If forceMode is true, waits for device reset and uses fewer retries.
func (r *NFCReader) reconnectDevice(forceMode bool) error {
	logPrefix := "Reconnect"
	maxAttempts := MaxReconnectTries
	if forceMode {
		logPrefix = "Force reconnect"
		maxAttempts = 3
	}

	log.Printf("%s: Attempting to reconnect device (path hint: %s)...", logPrefix, r.devicePath)
	r.cache.Clear()
	r.setCardPresent(false)

	// Close existing device
	r.statusMux.Lock()
	if r.hasDevice && r.device != nil {
		log.Printf("%s: Closing existing device connection.", logPrefix)
		r.device.Close() // Ignore error, device might be in bad state
		r.device = nil
		r.hasDevice = false
	}
	r.statusMux.Unlock()

	// For force mode, wait for device reset (ACR122U and similar devices)
	if forceMode {
		log.Println("Waiting for device to reset after close...")
		select {
		case <-time.After(DeviceResetWaitTime):
		case <-r.stopChan:
			log.Printf("%s: Stop signal received during wait, aborting.", logPrefix)
			return fmt.Errorf("reconnection aborted by stop signal")
		}
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		r.setDeviceStatus(false, fmt.Sprintf("Attempting to reconnect (attempt %d/%d)", attempt, maxAttempts))

		connectErr := r.tryConnect()
		if connectErr == nil {
			log.Printf("%s: Attempt %d successful.", logPrefix, attempt)

			// Check for tags to update card presence immediately
			tags, errTags := r.getTags()
			if errTags == nil && len(tags) > 0 {
				r.cache.UpdateLastSeenTime(tags[0].UID())
				r.setCardPresent(true)
				log.Println("Card detected immediately after reconnect.")
			} else {
				r.setCardPresent(false)
				if errTags != nil {
					log.Printf("Error getting tags after reconnect: %v", errTags)
				}
			}
			return nil
		}

		lastErr = connectErr
		log.Printf("%s: Attempt %d failed: %v", logPrefix, attempt, connectErr)

		// Calculate backoff delay
		var backoffDelay time.Duration
		if forceMode {
			backoffDelay = time.Second * time.Duration(attempt)
		} else {
			backoffDelay = ReconnectDelay * time.Duration(attempt)
		}

		select {
		case <-r.stopChan:
			log.Printf("%s: Stop signal received, aborting reconnection.", logPrefix)
			return fmt.Errorf("reconnection aborted by stop signal")
		case <-time.After(backoffDelay):
		}
	}

	errMsg := fmt.Sprintf("%s failed after %d attempts: %v", logPrefix, maxAttempts, lastErr)
	r.setDeviceStatus(false, fmt.Sprintf("%s failed after multiple attempts", logPrefix))
	log.Println(errMsg)
	return fmt.Errorf(errMsg)
}

// reconnect is a convenience wrapper for normal reconnection.
func (r *NFCReader) reconnect() error {
	return r.reconnectDevice(false)
}

// forceReconnectDevice is a convenience wrapper for forced reconnection with device reset.
func (r *NFCReader) forceReconnectDevice() error {
	return r.reconnectDevice(true)
}

// handleDeviceCheck attempts to connect to the device if not connected and not in cooldown.
func (r *NFCReader) handleDeviceCheck(retryCount *int) {
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
			*retryCount = 0
		}
	}
}

// handleCardCheck updates card presence based on cache status.
func (r *NFCReader) handleCardCheck() {
	currentCacheCardPresent := r.cache.IsCardPresent()
	r.statusMux.RLock()
	cardPres := r.cardPresent
	r.statusMux.RUnlock()

	if cardPres != currentCacheCardPresent {
		r.setCardPresent(currentCacheCardPresent)
		if currentCacheCardPresent {
			uid := r.cache.GetLastScanned()
			log.Printf("Card presence changed via cache: DETECTED (UID: %s)", uid)
		} else {
			log.Println("Card presence changed via cache: REMOVED/timed out")
		}
	}
}

// handleCooldownEnd handles the end of a device cooldown period.
func (r *NFCReader) handleCooldownEnd() {
	log.Println("Device cooldown period ended.")
	r.statusMux.Lock()
	r.inCooldown = false
	r.statusMux.Unlock()
	if err := r.forceReconnectDevice(); err != nil {
		log.Printf("Reconnection after cooldown failed: %v.", err)
	}
}

// handleDeviceErrors processes errors from getTags and determines recovery action.
// Returns true if the error was handled and the caller should continue the loop.
func (r *NFCReader) handleDeviceErrors(err error, retryCount *int) bool {
	log.Printf("Error getting tags: %v", err)
	originalErrorString := err.Error()

	// Handle IO/Config errors
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

		// Check for ACR122-specific errors that need cooldown
		if strings.Contains(originalErrorString, "Operation not permitted") ||
			strings.Contains(originalErrorString, "broken pipe") ||
			strings.Contains(originalErrorString, "RDR_to_PC_DataBlock") {
			r.statusMux.Lock()
			if !r.inCooldown {
				r.inCooldown = true
				log.Printf("ACR122-like error. Entering cooldown for %v", DeviceErrorCooldownPeriod)
				r.cooldownTimer.Reset(DeviceErrorCooldownPeriod)
			}
			r.statusMux.Unlock()
			return true
		}

		log.Println("Attempting force reconnect after IO/Config error...")
		time.Sleep(PostErrorPauseTime)
		if errReconnect := r.forceReconnectDevice(); errReconnect != nil {
			log.Printf("Force reconnection failed after IO/Config error: %v", errReconnect)
		}
		return true
	}

	// Handle Timeout/Closed errors with retry logic
	if IsTimeoutError(err) || IsDeviceClosedError(err) {
		log.Printf("Device error (Timeout/Closed): %v", err)
		delay := time.Duration(math.Pow(2, float64(*retryCount))) * BaseDelay
		if *retryCount < MaxRetries {
			*retryCount++
			log.Printf("Retrying connection (attempt %d/%d) in %v...", *retryCount, MaxRetries, delay)
			select {
			case <-time.After(delay):
			case <-r.stopChan:
				return false // Signal to exit worker
			}
			r.statusMux.Lock()
			r.isWriting = false
			r.statusMux.Unlock()
			if errReconnect := r.reconnect(); errReconnect != nil {
				log.Printf("Device reconnection failed: %v", errReconnect)
			} else {
				log.Println("Reconnected successfully.")
				*retryCount = 0
			}
		} else {
			log.Printf("Max retries reached for Timeout/Closed error: %v. Closing device.", err)
			r.statusMux.Lock()
			if r.hasDevice && r.device != nil {
				r.device.Close()
			}
			r.device = nil
			r.hasDevice = false
			if !r.inCooldown {
				r.inCooldown = true
				r.cooldownTimer.Reset(MaxRetriesCooldownPeriod)
				log.Println("Entering long cooldown after max retries for Timeout/Closed error.")
			}
			r.statusMux.Unlock()
			r.setDeviceStatus(false, "Max retries for device error.")
		}
		return true
	}

	// Unhandled error
	log.Printf("Unhandled error from getTags: %v. Sending to dataChan.", err)
	r.dataChan <- NFCData{Card: nil, Err: fmt.Errorf("get tags error: %v", err)}
	time.Sleep(UnhandledErrorRetryInterval)
	return true
}

// handleTagPolling processes detected tags and sends data to the channel.
func (r *NFCReader) handleTagPolling(tags []Tag) {
	for _, tag := range tags {
		uid := tag.UID()

		if uid != "" {
			r.cache.UpdateLastSeenTime(uid)
		}

		// Read data from tag first
		data, readErr := tag.ReadData()
		if readErr != nil {
			log.Printf("Error reading data for tag UID %s (Type: %s): %v", uid, tag.Type(), readErr)
			// Still create card even on error, but with no pre-loaded data
			card := NewCard(tag)
			r.dataChan <- NFCData{Card: card, Err: readErr}
			continue
		}

		// Create Card with pre-loaded data
		card := NewCard(tag)
		card.preloadData(data)

		// Check if this is a new/different card
		if r.cache.HasChanged(uid) {
			log.Printf("Tag data changed or new tag: UID %s (Type: %s)", uid, tag.Type())
			r.dataChan <- NFCData{Card: card, Err: nil}
		}
		time.Sleep(DefaultPollingInterval)
	}
}

func (r *NFCReader) worker() {
	log.Println("NFCReader worker started.")
	defer log.Println("NFCReader worker stopped.")

	r.deviceCheckTicker = time.NewTicker(DeviceCheckInterval)
	r.cardCheckTicker = time.NewTicker(CardCheckTickerInterval)
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
		r.workerWg.Done()
		log.Println("Worker goroutine finished.")
	}()

	for {
		select {
		case <-r.stopChan:
			return

		case <-r.deviceCheckTicker.C:
			r.handleDeviceCheck(&retryCount)

		case <-r.cardCheckTicker.C:
			r.handleCardCheck()

		case <-r.cooldownTimer.C:
			r.handleCooldownEnd()

		default:
			r.statusMux.RLock()
			hasDev := r.hasDevice
			dev := r.device
			inCool := r.inCooldown
			isWrite := r.isWriting
			r.statusMux.RUnlock()

			if !hasDev || dev == nil || inCool {
				time.Sleep(DeviceIdleCheckInterval)
				continue
			}
			if isWrite {
				time.Sleep(WriteCheckInterval)
				continue
			}

			tags, err := r.getTags()
			if err != nil {
				if !r.handleDeviceErrors(err, &retryCount) {
					return // Stop signal received during error handling
				}
				continue
			}
			retryCount = 0

			if len(tags) > 0 {
				r.handleTagPolling(tags)
			}
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
	connString := r.device.Connection() // Assuming Device has Connection()
	log.Printf("Connected NFC device: %s (Connection: %s, Path: %s)", name, connString, r.devicePath)
}

// GetLastScannedData retrieves the last scanned UID from the cache.
func (r *NFCReader) GetLastScannedData() string {
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
		uid := r.cache.GetLastScanned()
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
func (r *NFCReader) writeData(tag Tag, text string) error {
	var ndefMessage []byte

	if text != "" {
		// Encode the text string into an NDEF message payload.
		// Assuming EncodeTextPayload is available in the nfc package (e.g., from ndef_utils.go).
		// Defaulting to "en" language code.
		// Corrected to use EncodeNdefMessageWithTextRecord from ndef_utils.go
		ndefMessage = EncodeNdefMessageWithTextRecord(text, "en")
		// err is not returned by EncodeNdefMessageWithTextRecord, so no error check needed here.
		log.Printf("writeData (UID: %s, Type: %s): Encoded text to NDEF message of %d bytes.", tag.UID(), tag.Type(), len(ndefMessage))
	} else {
		log.Printf("writeData (UID: %s, Type: %s): No text provided; will call WriteData with nil/empty to ensure card is NDEF ready/initialized if applicable.", tag.UID(), tag.Type())
		// Passing nil or empty slice to WriteData on RealClassicTagAdapter
		// should trigger initialization logic if the card is in factory mode.
		ndefMessage = nil
	}

	// Call the WriteData method from Tag
	// This will handle factory initialization if needed (e.g., in RealClassicTagAdapter)
	// and then write the NDEF message.
	if err := tag.WriteData(ndefMessage); err != nil {
		return fmt.Errorf("writeData (UID: %s, Type: %s): error from tag.WriteData(%d bytes): %w", tag.UID(), tag.Type(), len(ndefMessage), err)
	}

	log.Printf("writeData (UID: %s, Type: %s): tag.WriteData() completed successfully.", tag.UID(), tag.Type())
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

		for _, tag := range tags { // tag is Tag
			// Attempt to write to any tag that supports WriteData.
			// The current implementation of WriteData in RealClassicTagAdapter handles MIFARE Classic.
			// Other tag types would need their own WriteData implementations.
			// We could add a check here: if _, ok := tag.(ClassicTag); ok { ... }
			// Or, more generally, just try and let the tag report if it's not supported.
			log.Printf("Attempting to write to tag UID: %s, Type: %s", tag.UID(), tag.Type())
			errWrite := r.writeData(tag, text)
			if errWrite == nil {
				log.Printf("Successfully wrote to tag UID: %s", tag.UID())
				return nil // Success
			}
			log.Printf("Failed to write to tag UID %s (Type: %s): %v. Trying next tag if any.", tag.UID(), tag.Type(), errWrite)
			// If it's a MIFARE Classic tag and it failed, we might not want to try others unless specifically designed.
			// For now, let's assume we try the first one that might work.
			// If the error is critical (e.g. card removed), it might be better to return it immediately.
		}
		return fmt.Errorf("no compatible or writable tag found, or write operation failed for all detected tags")
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
func (r *NFCReader) getTags() ([]Tag, error) {
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

	tags, err := manager.GetTags(dev) // Manager.GetTags now returns []Tag
	if err != nil {
		return nil, fmt.Errorf("getTags: error from nfcManager.GetTags: %w", err)
	}
	return tags, nil
}
