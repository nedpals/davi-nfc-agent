// Package main provides an NFC card reader agent with WebSocket broadcasting capabilities.
// It supports reading NDEF formatted text from Mifare Classic tags and broadcasts the data
// to connected WebSocket clients.
package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time" // Added for operation timeout in NFCReader

	"fyne.io/systray"
	"github.com/gorilla/websocket"

	"github.com/nedpals/davi-nfc-agent/nfc" // Import the new nfc package using module path
)

var (
	upgrader = websocket.Upgrader{
		CheckOrigin: checkOrigin,
	}
	clients           = make(map[*websocket.Conn]bool)
	clientsMux        sync.RWMutex
	defaultPort       = 18080
	additionalOrigins string
	currentReader     *nfc.NFCReader // Use nfc.NFCReader
	currentDevice     string
	allowedCardTypes  = make(map[string]bool) // Empty means all types allowed
	devicePathFlag    string
	portFlag          int
	systrayFlag       bool
	apiSecretFlag     string
	nfcManager        nfc.Manager
	lastBroadcastCard *nfc.Card // Track last broadcast card for systray display
	lastCardMux       sync.RWMutex

	// Session management
	sessionToken   string
	sessionOrigin  string // Bound origin for the session
	sessionIP      string // Bound IP address for the session
	sessionMux     sync.RWMutex
	sessionTimeout = 60 * time.Second
	sessionTimer   *time.Timer
)

// setLastBroadcastCard safely sets the last broadcast card
func setLastBroadcastCard(card *nfc.Card) {
	lastCardMux.Lock()
	defer lastCardMux.Unlock()
	lastBroadcastCard = card
}

// getLastBroadcastCard safely gets the last broadcast card
func getLastBroadcastCard() *nfc.Card {
	lastCardMux.RLock()
	defer lastCardMux.RUnlock()
	return lastBroadcastCard
}

// generateSessionToken generates a cryptographically secure random session token
func generateSessionToken() string {
	// Generate a random 32-byte token and encode as hex
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		log.Fatalf("Failed to generate session token: %v", err)
	}
	return fmt.Sprintf("%x", b)
}

// acquireSession attempts to acquire the session token
// Returns the token if successful, or empty string if already claimed
// origin and remoteAddr are used for optional session binding
func acquireSession(secret string, origin string, remoteAddr string) string {
	sessionMux.Lock()
	defer sessionMux.Unlock()

	// Check if API secret is required and validate
	if apiSecretFlag != "" && secret != apiSecretFlag {
		return ""
	}

	// If no active session, create one
	if sessionToken == "" {
		sessionToken = generateSessionToken()
		sessionOrigin = origin
		sessionIP = remoteAddr

		// Reset the session timeout timer
		if sessionTimer != nil {
			sessionTimer.Stop()
		}
		sessionTimer = time.AfterFunc(sessionTimeout, func() {
			releaseSession()
			log.Println("Session timeout - token released")
		})

		log.Printf("Session acquired: %s (origin: %s, ip: %s)", sessionToken[:8]+"...", origin, remoteAddr)
		return sessionToken
	}

	// Session already claimed
	return ""
}

// validateSession checks if the provided token matches the current session
// and optionally validates origin and IP binding
func validateSession(token string, origin string, remoteAddr string) bool {
	sessionMux.RLock()
	defer sessionMux.RUnlock()

	// Check token match
	if sessionToken == "" || sessionToken != token {
		return false
	}

	// Validate origin binding if it was set during acquisition
	if sessionOrigin != "" && origin != sessionOrigin {
		log.Printf("Session validation failed: origin mismatch (expected: %s, got: %s)", sessionOrigin, origin)
		return false
	}

	// Validate IP binding if it was set during acquisition
	if sessionIP != "" && remoteAddr != sessionIP {
		log.Printf("Session validation failed: IP mismatch (expected: %s, got: %s)", sessionIP, remoteAddr)
		return false
	}

	return true
}

// releaseSession releases the current session token
func releaseSession() {
	sessionMux.Lock()
	defer sessionMux.Unlock()

	if sessionToken != "" {
		log.Printf("Session released: %s", sessionToken[:8]+"...")
		sessionToken = ""
		sessionOrigin = ""
		sessionIP = ""

		if sessionTimer != nil {
			sessionTimer.Stop()
			sessionTimer = nil
		}
	}
}

// refreshSessionTimeout resets the session timeout timer
func refreshSessionTimeout() {
	sessionMux.Lock()
	defer sessionMux.Unlock()

	if sessionTimer != nil {
		sessionTimer.Reset(sessionTimeout)
	}
}

// sendErrorResponse sends an error response to a WebSocket connection
func sendErrorResponse(conn *websocket.Conn, responseType string, errMsg string) {
	conn.WriteJSON(WebSocketMessage{
		Type: responseType,
		Payload: map[string]interface{}{
			"success": false,
			"error":   errMsg,
		},
	})
}

// buildNDEFMessageWithOptions builds an NDEF message and write options based on the request
func buildNDEFMessageWithOptions(writeReq WriteRequest) (*nfc.NDEFMessage, nfc.WriteOptions, error) {
	// Determine record type
	recordType := writeReq.RecordType
	if recordType == "" {
		recordType = "text" // default to text
	}

	// Determine language
	language := writeReq.Language
	if language == "" {
		language = "en"
	}

	// Build NDEF record using the new builder API
	var newRecord nfc.NDEFRecordBuilder
	switch recordType {
	case "text":
		newRecord = &nfc.NDEFText{Content: writeReq.Text, Language: language}
	case "uri":
		newRecord = &nfc.NDEFURI{Content: writeReq.Text}
	default:
		return nil, nfc.WriteOptions{}, fmt.Errorf("unsupported record type: %s", recordType)
	}

	// Build message
	builder := &nfc.NDEFMessageBuilder{
		Records: []nfc.NDEFRecordBuilder{newRecord},
	}
	ndefMsg := builder.MustBuild()

	// Determine write options
	var writeOpts nfc.WriteOptions
	if writeReq.Replace {
		log.Printf("WriteRequest: Replacing entire NDEF message (destructive)")
		writeOpts = nfc.WriteOptions{Overwrite: true, Index: -1}
	} else if writeReq.Append {
		log.Printf("WriteRequest: Appending new %s record", recordType)
		writeOpts = nfc.WriteOptions{Overwrite: false, Index: -1}
	} else if writeReq.RecordIndex != nil {
		log.Printf("WriteRequest: Updating record at index %d", *writeReq.RecordIndex)
		writeOpts = nfc.WriteOptions{Overwrite: false, Index: *writeReq.RecordIndex}
	} else {
		log.Printf("WriteRequest: Auto-detecting write mode")
		writeOpts = nfc.WriteOptions{Overwrite: true, Index: -1}
	}

	return ndefMsg, writeOpts, nil
}

// handleWriteRequest processes a write request from a WebSocket client
func handleWriteRequest(conn *websocket.Conn, wsMessage WebSocketMessage, ctx context.Context) {
	var writeReq WriteRequest
	payloadJSON, err := json.Marshal(wsMessage.Payload)
	if err != nil {
		log.Printf("Failed to marshal write payload: %v", err)
		sendErrorResponse(conn, "writeResponse", "Failed to parse request")
		return
	}

	if err := json.Unmarshal(payloadJSON, &writeReq); err != nil {
		log.Printf("Failed to parse write request: %v", err)
		sendErrorResponse(conn, "writeResponse", "Failed to parse request")
		return
	}

	// Get reader from context
	reader := getReaderFromContext(ctx)
	if reader == nil {
		sendErrorResponse(conn, "writeResponse", "NFC reader not available")
		return
	}

	// Build NDEF message and write options
	ndefMsg, writeOpts, err := buildNDEFMessageWithOptions(writeReq)
	if err != nil {
		log.Printf("Failed to build NDEF message: %v", err)
		sendErrorResponse(conn, "writeResponse", err.Error())
		return
	}

	// Use the new WriteMessageWithOptions to preserve NDEF structure
	err = reader.WriteMessageWithOptions(ndefMsg, writeOpts)
	if err != nil {
		log.Printf("Write failed: %v", err)
		sendErrorResponse(conn, "writeResponse", err.Error())
		return
	}

	// Success response
	conn.WriteJSON(WebSocketMessage{
		Type: "writeResponse",
		Payload: map[string]interface{}{
			"success": true,
			"error":   nil,
		},
	})
}

// checkOrigin implements CORS checking for WebSocket connections.
// It allows connections from localhost:3000 and any origins specified in additionalOrigins.
func checkOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // Allow requests with no origin
	}

	u, err := url.Parse(origin)
	if err != nil {
		return false
	}

	// Always allow localhost:3000
	if u.Host == "localhost:3000" {
		return true
	}

	// Check additional origins if provided
	if additionalOrigins != "" {
		for _, allowed := range strings.Split(additionalOrigins, ",") {
			if strings.TrimSpace(allowed) == origin {
				return true
			}
		}
	}

	return false
}

// handleWebSocket upgrades HTTP connections to WebSocket connections and manages
// the client connection lifecycle. It sends the last known tag data upon connection
// and removes the client when the connection closes.
func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Add CORS headers
	w.Header().Set("Access-Control-Allow-Origin", r.Header.Get("Origin"))
	w.Header().Set("Access-Control-Allow-Credentials", "true")

	// Validate session token
	token := r.URL.Query().Get("token")
	if token == "" {
		token = r.Header.Get("X-Session-Token")
	}

	// Get origin and remote address for validation
	origin := r.Header.Get("Origin")
	remoteAddr := r.RemoteAddr

	if !validateSession(token, origin, remoteAddr) {
		log.Printf("Invalid or missing session token")
		http.Error(w, "Unauthorized: Invalid or missing session token", http.StatusUnauthorized)
		return
	}

	// Refresh session timeout on successful connection
	refreshSessionTimeout()

	reader := getReaderFromContext(r.Context())
	if reader == nil {
		log.Printf("No NFC reader in context")
		http.Error(w, "Server configuration error", http.StatusInternalServerError)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	defer func() {
		conn.Close()
		// Release session when WebSocket disconnects
		releaseSession()
	}()

	clientsMux.Lock()
	clients[conn] = true
	clientsMux.Unlock()

	// Send initial device status
	status := reader.GetDeviceStatus() // Use nfc.NFCReader method
	conn.WriteJSON(WebSocketMessage{
		Type:    "deviceStatus",
		Payload: status,
	})

	// Get last scanned data from cache
	uid := reader.GetLastScannedData() // Use nfc.NFCReader method
	if uid != "" {
		conn.WriteJSON(WebSocketMessage{
			Type: "tagData",
			Payload: map[string]interface{}{
				"uid":  uid,
				"text": "", // Text not cached, will be sent on next scan
				"err":  nil,
			},
		})
	}

	// Keep connection alive and handle incoming messages
	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			clientsMux.Lock()
			delete(clients, conn)
			clientsMux.Unlock()
			break
		}

		// Refresh session timeout on any incoming message
		refreshSessionTimeout()

		if messageType == websocket.TextMessage {
			var wsMessage WebSocketMessage
			if err := json.Unmarshal(message, &wsMessage); err != nil {
				log.Printf("Failed to parse WebSocket message: %v", err)
				continue
			}

			switch wsMessage.Type {
			case "writeRequest":
				handleWriteRequest(conn, wsMessage, r.Context())
			case "release":
				// Client explicitly releases the session
				releaseSession()
				conn.WriteJSON(WebSocketMessage{
					Type: "releaseResponse",
					Payload: map[string]interface{}{
						"success": true,
					},
				})
			}
		}
	}
}

// broadcastToClients sends NFCData to all connected WebSocket clients.
// It handles client disconnections and removes disconnected clients from the pool.
func broadcastToClients(data nfc.NFCData) { // Use nfc.NFCData
	var errStr *string = nil
	if data.Err != nil {
		errStr = new(string)
		*errStr = data.Err.Error()
	}

	var payload map[string]interface{}

	if data.Card != nil {
		// Update last broadcast card for systray display
		setLastBroadcastCard(data.Card)

		// Build the structured payload
		payload = map[string]interface{}{
			"uid":        data.Card.UID,
			"type":       data.Card.Type,
			"technology": data.Card.Technology,
			"scannedAt":  data.Card.ScannedAt.Format("2006-01-02T15:04:05Z07:00"),
			"err":        errStr,
		}

		// Try to read and parse message from card
		if msg, err := data.Card.ReadMessage(); err == nil {
			var text string
			var messageInfo map[string]interface{}

			if ndefMsg, ok := msg.(*nfc.NDEFMessage); ok {
				// NDEF message - extract text and build records array
				text, _ = ndefMsg.GetText()

				records := make([]map[string]interface{}, 0, len(ndefMsg.Records()))
				for _, record := range ndefMsg.Records() {
					recordInfo := map[string]interface{}{
						"tnf":  record.TNF,
						"type": string(record.Type),
					}

					// Add ID if present
					if len(record.ID) > 0 {
						recordInfo["id"] = string(record.ID)
					}

					// Extract type-specific data
					if recordText, ok := record.GetText(); ok {
						recordInfo["text"] = recordText
					} else if recordURI, ok := record.GetURI(); ok {
						recordInfo["uri"] = recordURI
					}

					// Always include raw payload (as base64 for binary safety)
					recordInfo["payload"] = record.Payload

					records = append(records, recordInfo)
				}

				messageInfo = map[string]interface{}{
					"type":    "ndef",
					"records": records,
				}
			} else if textMsg, ok := msg.(*nfc.TextMessage); ok {
				// Raw text message
				text = textMsg.Text
				messageInfo = map[string]interface{}{
					"type": "raw",
					"data": textMsg.Bytes(),
				}
			}

			payload["message"] = messageInfo
			payload["text"] = text
		} else {
			// Error reading message
			payload["text"] = ""
		}
	} else {
		// No card data available
		payload = map[string]interface{}{
			"uid":  "",
			"text": "",
			"err":  errStr,
		}
	}

	message := WebSocketMessage{
		Type:    "tagData",
		Payload: payload,
	}

	clientsMux.Lock()
	defer clientsMux.Unlock()

	for client := range clients {
		err := client.WriteJSON(message)
		if err != nil {
			log.Printf("WebSocket write error: %v", err)
			client.Close()
			delete(clients, client)
		}
	}
}

// getReaderFromContext retrieves the NFCReader instance from the context.
// Returns nil if no reader is found in the context.
func getReaderFromContext(ctx context.Context) *nfc.NFCReader { // Use nfc.NFCReader
	reader, _ := ctx.Value(readerContextKey).(*nfc.NFCReader) // Use nfc.NFCReader
	return reader
}

// WebSocketMessage represents a message sent to WebSocket clients.
type WebSocketMessage struct {
	Type    string `json:"type"`
	Payload any    `json:"payload"`
}

// WriteRequest represents a request to write data to an NFC card.
type WriteRequest struct {
	// Text is the text string to write
	Text string `json:"text"`

	// RecordIndex specifies which NDEF record to update (0-based)
	// Required for updating existing records
	RecordIndex *int `json:"recordIndex,omitempty"`

	// RecordType specifies the type of record to update/create
	// Options: "text" (default), "uri"
	RecordType string `json:"recordType,omitempty"`

	// Language code for text records (default: "en")
	Language string `json:"language,omitempty"`

	// Append adds a new record instead of replacing
	// Set to true to safely add records without overwriting
	Append bool `json:"append,omitempty"`

	// Replace replaces the entire NDEF message (destructive)
	// Must be explicitly set to true to overwrite all existing data
	Replace bool `json:"replace,omitempty"`
}

// gracefulShutdown attempts to close all active connections and resources
func gracefulShutdown(reader *nfc.NFCReader) { // Use nfc.NFCReader
	log.Println("Performing graceful shutdown...")

	// Close all WebSocket connections
	clientsMux.Lock()
	for client := range clients {
		client.Close()
		delete(clients, client)
	}
	clientsMux.Unlock()

	// Close the NFC reader if it exists
	if reader != nil {
		reader.Close()
	}
}

func onReady() {
	systray.SetIcon(iconData)
	systray.SetTitle("NFC Agent")
	systray.SetTooltip("NFC Card Reader Agent")

	// Status section
	mStatus := systray.AddMenuItem("Starting...", "Agent Status")
	mStatus.Disable()

	mConnection := systray.AddMenuItem("Connection: Disconnected", "Connection Status")
	mConnection.Disable()

	mMode := systray.AddMenuItem("Mode: Read/Write", "Current Mode")
	mMode.Disable()

	systray.AddSeparator()

	// Card info section
	mCardUID := systray.AddMenuItem("Card UID: None", "Current card UID")
	mCardUID.Disable()

	mCardType := systray.AddMenuItem("Card Type: None", "Current card type")
	mCardType.Disable()

	systray.AddSeparator()

	// Device management section
	mDeviceMenu := systray.AddMenuItem("Device", "Select NFC Device")
	mRefreshDevices := mDeviceMenu.AddSubMenuItem("Refresh Devices", "Refresh device list")
	mDeviceMenu.AddSubMenuItemCheckbox("Auto-detect", "Auto-detect device", true)

	systray.AddSeparator()

	// Mode toggle section
	mModeMenu := systray.AddMenuItem("Switch Mode", "Change operation mode")
	mReadWriteMode := mModeMenu.AddSubMenuItemCheckbox("Read/Write Mode", "Allow both read and write", true)
	mReadMode := mModeMenu.AddSubMenuItemCheckbox("Read Only Mode", "Only allow reading", false)
	mWriteMode := mModeMenu.AddSubMenuItemCheckbox("Write Only Mode", "Only allow writing", false)

	systray.AddSeparator()

	// Card type filtering section
	mCardFilterMenu := systray.AddMenuItem("Card Type Filter", "Filter cards by type")
	mFilterAll := mCardFilterMenu.AddSubMenuItemCheckbox("All Types", "Allow all card types", true)
	mFilterMifare := mCardFilterMenu.AddSubMenuItemCheckbox("MIFARE Classic", "Allow MIFARE Classic only", false)
	mFilterMifareUltra := mCardFilterMenu.AddSubMenuItemCheckbox("MIFARE Ultralight", "Allow MIFARE Ultralight only", false)
	mFilterDESFire := mCardFilterMenu.AddSubMenuItemCheckbox("DESFire", "Allow DESFire only", false)
	mFilterType4 := mCardFilterMenu.AddSubMenuItemCheckbox("Type 4", "Allow Type 4 only", false)

	systray.AddSeparator()

	// Agent control section
	mStart := systray.AddMenuItem("Start Agent", "Start the NFC agent")
	mStop := systray.AddMenuItem("Stop Agent", "Stop the NFC agent")
	mStart.Disable() // Disable start since we're auto-starting
	mStop.Disable()  // Will be enabled once agent starts

	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit", "Quit the application")

	// Initialize device list
	deviceMenuItems := make(map[string]*systray.MenuItem)
	updateDeviceList := func() {
		// Clear existing device menu items
		for _, item := range deviceMenuItems {
			item.Hide()
		}
		deviceMenuItems = make(map[string]*systray.MenuItem)

		// Get available devices
		devices, err := nfcManager.ListDevices()
		if err != nil {
			log.Printf("Error listing devices: %v", err)
			return
		}

		// Add device menu items
		for _, device := range devices {
			deviceName := device
			isChecked := (currentDevice == deviceName) || (currentDevice == "" && len(deviceMenuItems) == 0)
			item := mDeviceMenu.AddSubMenuItemCheckbox(deviceName, "Select this device", isChecked)
			deviceMenuItems[deviceName] = item

			if isChecked && currentDevice == "" {
				currentDevice = deviceName
			}
		}
	}

	// Auto-start the agent
	go func() {
		if err := startAgent(); err == nil {
			mStatus.SetTitle("Running")
			mConnection.SetTitle("Connection: Connected")
			if currentDevice != "" {
				mConnection.SetTitle("Connection: Connected (" + currentDevice + ")")
			}
			mStop.Enable()
		} else {
			mStatus.SetTitle("Failed to Start")
			mConnection.SetTitle("Connection: Failed")
			mStart.Enable()
		}
		updateDeviceList()
	}()

	// Goroutine to update card info display
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		lastUID := ""
		lastType := ""

		for range ticker.C {
			card := getLastBroadcastCard()
			var uid, cardType string
			if card != nil {
				uid = card.UID
				cardType = card.Type
			}

			if uid != lastUID {
				if uid == "" {
					mCardUID.SetTitle("Card UID: None")
				} else {
					mCardUID.SetTitle("Card UID: " + uid)
				}
				lastUID = uid
			}

			if cardType != lastType {
				if cardType == "" {
					mCardType.SetTitle("Card Type: None")
				} else {
					mCardType.SetTitle("Card Type: " + cardType)
				}
				lastType = cardType
			}
		}
	}()

	go func() {
		for {
			select {
			case <-mStart.ClickedCh:
				if err := startAgent(); err == nil {
					mStatus.SetTitle("Running")
					mConnection.SetTitle("Connection: Connected")
					if currentDevice != "" {
						mConnection.SetTitle("Connection: Connected (" + currentDevice + ")")
					}
					mStart.Disable()
					mStop.Enable()
				} else {
					mStatus.SetTitle("Failed to Start")
					mConnection.SetTitle("Connection: Failed")
				}
			case <-mStop.ClickedCh:
				stopAgent()
				mStatus.SetTitle("Stopped")
				mConnection.SetTitle("Connection: Disconnected")
				setLastBroadcastCard(nil)
				mStop.Disable()
				mStart.Enable()
			case <-mRefreshDevices.ClickedCh:
				updateDeviceList()
			case <-mReadWriteMode.ClickedCh:
				if !mReadWriteMode.Checked() && currentReader != nil {
					currentReader.SetMode(nfc.ModeReadWrite)
					mMode.SetTitle("Mode: Read/Write")
					mReadWriteMode.Check()
					mReadMode.Uncheck()
					mWriteMode.Uncheck()
					log.Println("Switched to read/write mode")
				}
			case <-mReadMode.ClickedCh:
				if !mReadMode.Checked() && currentReader != nil {
					currentReader.SetMode(nfc.ModeReadOnly)
					mMode.SetTitle("Mode: Read Only")
					mReadMode.Check()
					mReadWriteMode.Uncheck()
					mWriteMode.Uncheck()
					log.Println("Switched to read-only mode")
				}
			case <-mWriteMode.ClickedCh:
				if !mWriteMode.Checked() && currentReader != nil {
					currentReader.SetMode(nfc.ModeWriteOnly)
					mMode.SetTitle("Mode: Write Only")
					mWriteMode.Check()
					mReadWriteMode.Uncheck()
					mReadMode.Uncheck()
					log.Println("Switched to write-only mode")
				}
			case <-mFilterAll.ClickedCh:
				mFilterAll.Check()
				mFilterMifare.Uncheck()
				mFilterMifareUltra.Uncheck()
				mFilterDESFire.Uncheck()
				mFilterType4.Uncheck()
				allowedCardTypes = make(map[string]bool) // Empty means all allowed
				log.Println("Card filter: All types allowed")
			case <-mFilterMifare.ClickedCh:
				mFilterAll.Uncheck()
				if mFilterMifare.Checked() {
					mFilterMifare.Uncheck()
					delete(allowedCardTypes, "MIFARE Classic 1K")
					delete(allowedCardTypes, "MIFARE Classic 4K")
				} else {
					mFilterMifare.Check()
					allowedCardTypes["MIFARE Classic 1K"] = true
					allowedCardTypes["MIFARE Classic 4K"] = true
				}
				// Check if no filters active, then revert to All
				if len(allowedCardTypes) == 0 {
					mFilterAll.Check()
				}
				log.Printf("Card filter updated: %v", allowedCardTypes)
			case <-mFilterMifareUltra.ClickedCh:
				mFilterAll.Uncheck()
				if mFilterMifareUltra.Checked() {
					mFilterMifareUltra.Uncheck()
					delete(allowedCardTypes, "MIFARE Ultralight")
				} else {
					mFilterMifareUltra.Check()
					allowedCardTypes["MIFARE Ultralight"] = true
				}
				// Check if no filters active, then revert to All
				if len(allowedCardTypes) == 0 {
					mFilterAll.Check()
				}
				log.Printf("Card filter updated: %v", allowedCardTypes)
			case <-mFilterDESFire.ClickedCh:
				mFilterAll.Uncheck()
				if mFilterDESFire.Checked() {
					mFilterDESFire.Uncheck()
					delete(allowedCardTypes, "DESFire")
				} else {
					mFilterDESFire.Check()
					allowedCardTypes["DESFire"] = true
				}
				// Check if no filters active, then revert to All
				if len(allowedCardTypes) == 0 {
					mFilterAll.Check()
				}
				log.Printf("Card filter updated: %v", allowedCardTypes)
			case <-mFilterType4.ClickedCh:
				mFilterAll.Uncheck()
				if mFilterType4.Checked() {
					mFilterType4.Uncheck()
					delete(allowedCardTypes, string(nfc.TagTypeType4))
				} else {
					mFilterType4.Check()
					allowedCardTypes[string(nfc.TagTypeType4)] = true
				}
				// Check if no filters active, then revert to All
				if len(allowedCardTypes) == 0 {
					mFilterAll.Check()
				}
				log.Printf("Card filter updated: %v", allowedCardTypes)
			case <-mQuit.ClickedCh:
				systray.Quit()
				return
			}

			// Handle device selection
			for deviceName, menuItem := range deviceMenuItems {
				select {
				case <-menuItem.ClickedCh:
					if currentDevice != deviceName {
						// Uncheck all devices
						for _, item := range deviceMenuItems {
							item.Uncheck()
						}
						// Check selected device
						menuItem.Check()
						currentDevice = deviceName

						// Restart agent with new device
						wasRunning := currentReader != nil
						if wasRunning {
							stopAgent()
							if err := startAgentWithDevice(currentDevice); err == nil {
								mStatus.SetTitle("Running")
								mConnection.SetTitle("Connection: Connected (" + currentDevice + ")")
								mStop.Enable()
								mStart.Disable()
							} else {
								mStatus.SetTitle("Failed to Start")
								mConnection.SetTitle("Connection: Failed")
								mStart.Enable()
								mStop.Disable()
							}
						}
					}
				default:
					// No click event for this menu item
				}
			}
		}
	}()
}

func onExit() {
	stopAgent()
}

func startAgent() error {
	return startAgentWithDevice(currentDevice)
}

func startAgentWithDevice(devicePath string) error {
	var err error

	// Use the device path if specified, otherwise use auto-detect
	if devicePath == "" {
		devicePath = devicePathFlag
	}

	currentReader, err = nfc.NewNFCReader(devicePath, nfcManager, 5*time.Second)
	if err != nil {
		log.Printf("Error initializing NFC reader: %v", err)
		return err
	}

	// Store the actual device being used
	if currentReader != nil && devicePath != "" {
		currentDevice = devicePath
	}

	// Start server in a goroutine so it doesn't block systray operations
	go startServer(currentReader, portFlag)
	return nil
}

func stopAgent() {
	log.Println("Stopping agent...")

	// First stop the server to prevent new operations
	stopServer()

	// Then stop the NFC reader (this will wait for worker to finish and close device)
	if currentReader != nil {
		currentReader.Stop()
		currentReader = nil
		log.Println("Agent stopped successfully")
	}
}

func main() {
	// Command line flags
	flag.StringVar(&devicePathFlag, "device", "", "Path to NFC device (optional)")
	flag.IntVar(&portFlag, "port", defaultPort, "Port to listen on for the web interface")
	flag.BoolVar(&systrayFlag, "cli", false, "Run in CLI mode (default: system tray mode)")
	flag.StringVar(&apiSecretFlag, "api-secret", "", "API secret for session handshake (optional)")
	flag.Parse()

	// Initialize the global NFC manager
	nfcManager = nfc.NewManager()

	// Run in CLI mode only if explicitly requested
	if systrayFlag {
		// Regular CLI mode
		reader, err := nfc.NewNFCReader(devicePathFlag, nfcManager, 5*time.Second) // Use nfc.NewNFCReader, add timeout
		if err != nil {
			log.Fatalf("Error initializing NFC reader: %v", err)
		}
		defer reader.Close()

		// Set up signal handling for graceful shutdown
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

		// Start server in a goroutine
		go startServer(reader, portFlag)

		// Wait for shutdown signal
		<-sigChan
		log.Println("Shutdown signal received, stopping server...")
		stopServer()
	} else {
		// Default systray mode
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

		go func() {
			<-sigChan
			systray.Quit()
		}()

		systray.Run(onReady, onExit)
	}
}
