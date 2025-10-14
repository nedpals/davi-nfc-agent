package nfc

import (
	"fmt"
	"log"
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
	deviceManager     *DeviceManager
	dataChan          chan NFCData      // Broadcasts successfully read NFC data
	statusChan        chan DeviceStatus // Broadcasts device status updates
	stopChan          chan struct{}     // Signals the worker to stop
	cache             *TagCache         // Caches tag data
	statusMux         sync.RWMutex
	cardPresent       bool           // Internal tracking of card presence
	isWriting         bool           // Tracks if a write operation is in progress
	operationMutex    sync.Mutex     // Protects tag operations (read/write)
	operationTimeout  time.Duration  // Timeout for tag operations
	deviceCheckTicker *time.Ticker   // Ticker for periodic device checks
	cardCheckTicker   *time.Ticker   // Ticker for periodic card presence checks (based on cache)
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

	deviceManager := NewDeviceManager(manager, deviceStr)

	reader := &NFCReader{
		deviceManager:    deviceManager,
		dataChan:         make(chan NFCData, 1),      // Buffered to prevent blocking on send if no listener
		statusChan:       make(chan DeviceStatus, 1), // Buffered for status updates
		stopChan:         make(chan struct{}),
		cache:            NewTagCache(),
		cardPresent:      false,
		operationTimeout: opTimeout,
	}

	// Attempt initial connection synchronously
	// If it fails, the worker will retry via handleDeviceCheck
	retryCount := 0
	reader.handleDeviceCheck(&retryCount)

	return reader, nil
}

// Close releases resources. Does not stop the worker, use Stop() for that.
func (r *NFCReader) Close() {
	log.Println("NFCReader Close called (resource cleanup).")
	r.deviceManager.Close()
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

// GetDeviceStatus returns the current device status by querying live state.
func (r *NFCReader) GetDeviceStatus() DeviceStatus {
	cardPres := r.readCardPresent()
	connected := r.deviceManager.HasDevice()
	var message string
	if connected {
		dev := r.deviceManager.Device()
		if dev != nil {
			message = fmt.Sprintf("Connected to %s", dev.String())
		} else {
			message = "Connected"
		}
	} else if r.deviceManager.InCooldown() {
		message = "Device in cooldown"
	} else {
		message = "Not connected"
	}

	return DeviceStatus{
		Connected:   connected,
		Message:     message,
		CardPresent: cardPres,
	}
}

// readCardPresent safely reads the cardPresent flag.
func (r *NFCReader) readCardPresent() bool {
	r.statusMux.RLock()
	defer r.statusMux.RUnlock()
	return r.cardPresent
}

// handleDeviceCheck attempts to connect to the device if not connected and not in cooldown.
func (r *NFCReader) handleDeviceCheck(retryCount *int) {
	if !r.deviceManager.HasDevice() && !r.deviceManager.InCooldown() {
		log.Println("Device check ticker: No device or not in cooldown, attempting connect.")
		if err := r.deviceManager.TryConnect(); err != nil {
			log.Printf("Device check ticker: Connection attempt failed: %v", err)
			r.broadcastDeviceStatus(fmt.Sprintf("Connection failed: %v", err))
		} else {
			log.Println("Device check ticker: Connection successful.")
			*retryCount = 0
			r.LogDeviceInfo()
			r.broadcastDeviceStatus() // Use default message from GetDeviceStatus
		}
	}
}

// handleCardCheck updates card presence based on cache status.
func (r *NFCReader) handleCardCheck() {
	currentCacheCardPresent := r.cache.IsCardPresent()
	cardPres := r.readCardPresent()
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

// handleDeviceErrors processes errors from getTags and determines recovery action.
// Returns true if the error was handled and the caller should continue the loop.
func (r *NFCReader) handleDeviceErrors(err error, retryCount *int) bool {
	// Clear write flag on error
	r.statusMux.Lock()
	r.isWriting = false
	r.statusMux.Unlock()

	// Delegate error handling to DeviceManager
	newRetryCount, needsCooldown := r.deviceManager.HandleError(err, *retryCount, r.stopChan)
	*retryCount = newRetryCount

	if needsCooldown {
		r.broadcastDeviceStatus("Device in cooldown")
		return true
	}

	// Check if device was reconnected successfully
	if r.deviceManager.HasDevice() {
		r.broadcastDeviceStatus() // Use default message from GetDeviceStatus
		*retryCount = 0
		return true
	}

	// For unhandled errors, send to data channel
	if !IsIOError(err) && !IsDeviceConfigError(err) && !IsTimeoutError(err) && !IsDeviceClosedError(err) {
		log.Printf("Unhandled error from getTags: %v. Sending to dataChan.", err)
		r.dataChan <- NFCData{Card: nil, Err: fmt.Errorf("get tags error: %v", err)}
		time.Sleep(UnhandledErrorRetryInterval)
	}

	return true
}

// handleTagPolling processes detected tags and sends data to the channel.
func (r *NFCReader) handleTagPolling(tags []Tag) {
	for _, tag := range tags {
		uid := tag.UID()

		if uid != "" {
			r.cache.UpdateLastSeenTime(uid)
		}

		// Create Card wrapper
		card := NewCard(tag)
		if _, err := card.ReadMessage(); err != nil {
			log.Printf("Error reading data for card UID %s (Type: %s): %v", uid, card.Type, err)
			// Send card with error
			r.dataChan <- NFCData{Card: card, Err: err}
			continue
		}

		if r.cache.HasChanged(uid) {
			log.Printf("Card data changed or new card: UID %s (Type: %s)", uid, card.Type)
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
	retryCount := 0

	defer func() {
		r.deviceCheckTicker.Stop()
		r.cardCheckTicker.Stop()
		r.deviceManager.Close()
		r.broadcastDeviceStatus("Worker stopped, device disconnected.")
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

		case <-r.deviceManager.CooldownChannel():
			r.deviceManager.EndCooldown(r.stopChan)

		default:
			hasDev := r.deviceManager.HasDevice()
			inCool := r.deviceManager.InCooldown()

			r.statusMux.RLock()
			isWrite := r.isWriting
			r.statusMux.RUnlock()

			if !hasDev || inCool {
				time.Sleep(DeviceIdleCheckInterval)
				continue
			}
			if isWrite {
				time.Sleep(WriteCheckInterval)
				continue
			}

			tags, err := r.GetTags()
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

// broadcastDeviceStatus broadcasts a device status update.
// It queries the current live state via GetDeviceStatus().
// An optional custom message can be provided to override the default message.
func (r *NFCReader) broadcastDeviceStatus(customMessage ...string) {
	status := r.GetDeviceStatus()

	// Allow override for specific messages like "Reconnecting...", "Failed to connect", etc.
	if len(customMessage) > 0 && customMessage[0] != "" {
		status.Message = customMessage[0]
	}

	select {
	case r.statusChan <- status:
	default:
		log.Println("Warning: Device status channel full or no listener.")
	}
}

// LogDeviceInfo logs information about the connected NFC device.
func (r *NFCReader) LogDeviceInfo() {
	dev := r.deviceManager.Device()
	if dev == nil {
		return
	}
	name := dev.String()
	connString := dev.Connection()
	devicePath := r.deviceManager.DevicePath()
	log.Printf("Connected NFC device: %s (Connection: %s, Path: %s)", name, connString, devicePath)
}

// GetLastScannedData retrieves the last scanned UID from the cache.
func (r *NFCReader) GetLastScannedData() string {
	return r.cache.GetLastScanned()
}

func (r *NFCReader) setCardPresent(present bool) {
	r.statusMux.Lock()
	if r.cardPresent == present { // Avoid redundant updates
		r.statusMux.Unlock()
		return
	}
	r.cardPresent = present
	r.statusMux.Unlock()

	// Construct message based on card presence
	var message string
	if present {
		uid := r.cache.GetLastScanned()
		if uid != "" {
			message = fmt.Sprintf("Card detected (UID: %s)", uid)
		} else {
			message = "Card detected"
		}
	} else {
		message = "Card removed"
		r.cache.Clear() // Clear cache when card is definitively removed
	}

	// Broadcast status with custom message
	r.broadcastDeviceStatus(message)
}

// writeData writes text to a card using the Card WriteMessage API.
func (r *NFCReader) writeData(card *Card, text string) error {
	if text == "" {
		log.Printf("writeData (UID: %s, Type: %s): No text provided; writing empty NDEF message to initialize card if needed.", card.UID, card.Type)
	} else {
		log.Printf("writeData (UID: %s, Type: %s): Writing text message: %q", card.UID, card.Type, text)
	}

	// Encode text as NDEF message
	ndefMessage := &NDEFMessageBuilder{
		Records: []NDEFRecordBuilder{
			&NDEFText{Content: text, Language: "en"},
		},
	}

	// Use Card's WriteMessage which handles encoding and writing
	if err := card.WriteMessage(ndefMessage); err != nil {
		return fmt.Errorf("writeData (UID: %s, Type: %s): error from card.WriteMessage: %w", card.UID, card.Type, err)
	}

	log.Printf("writeData (UID: %s, Type: %s): card write completed successfully.", card.UID, card.Type)
	return nil
}

// WriteCardData attempts to write data to a detected NFC card.
// Currently, this means initializing a factory mode card.
func (r *NFCReader) WriteCardData(text string) error {
	return r.withTagOperation(func() error {
		if !r.deviceManager.HasDevice() {
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

		tags, err := r.GetTags()
		if err != nil {
			return fmt.Errorf("failed to get tags for writing: %w", err)
		}
		if len(tags) == 0 {
			return fmt.Errorf("no card detected for writing")
		}

		for _, tag := range tags {
			// Create Card wrapper for the tag
			card := NewCard(tag)

			log.Printf("Attempting to write to card UID: %s, Type: %s", card.UID, card.Type)
			errWrite := r.writeData(card, text)
			if errWrite == nil {
				log.Printf("Successfully wrote to card UID: %s", card.UID)
				return nil // Success
			}
			log.Printf("Failed to write to card UID %s (Type: %s): %v. Trying next card if any.", card.UID, card.Type, errWrite)
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

// GetTags retrieves available tags from the connected NFC device.
func (r *NFCReader) GetTags() ([]Tag, error) {
	dev := r.deviceManager.Device()
	if dev == nil {
		return nil, fmt.Errorf("getTags: no device connected or device is nil")
	}

	tags, err := dev.GetTags()
	if err != nil {
		return nil, fmt.Errorf("getTags: error from device.GetTags: %w", err)
	}
	return tags, nil
}
