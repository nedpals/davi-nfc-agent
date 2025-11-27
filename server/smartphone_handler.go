package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/nedpals/davi-nfc-agent/nfc"
)

// SmartphoneDeviceHandler handles all smartphone device WebSocket connections and management.
type SmartphoneDeviceHandler struct {
	manager           *nfc.SmartphoneManager
	deviceSessions    map[string]*websocket.Conn // deviceID -> websocket conn
	deviceSessionsMux sync.RWMutex
	connToDeviceID    map[*websocket.Conn]string // reverse lookup: conn -> deviceID
	upgrader          websocket.Upgrader
}

// NewSmartphoneDeviceHandler creates a new smartphone device handler.
func NewSmartphoneDeviceHandler(manager *nfc.SmartphoneManager) *SmartphoneDeviceHandler {
	return &SmartphoneDeviceHandler{
		manager:        manager,
		deviceSessions: make(map[string]*websocket.Conn),
		connToDeviceID: make(map[*websocket.Conn]string),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins
			},
		},
	}
}

// HandleWebSocket handles WebSocket connections from mobile devices.
// No authentication required - plug and play for seamless mobile integration.
func (h *SmartphoneDeviceHandler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {

	// Upgrade connection to WebSocket
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[smartphone] WebSocket upgrade error: %v", err)
		return
	}

	log.Printf("[smartphone] WebSocket connected from %s", r.RemoteAddr)

	var deviceID string
	defer func() {
		conn.Close()
		if deviceID != "" {
			h.handleDeviceDisconnect(deviceID)
		}
		log.Printf("[smartphone] WebSocket disconnected: %s", deviceID)
	}()

	// Wait for registerDevice message
	messageType, message, err := conn.ReadMessage()
	if err != nil {
		log.Printf("[smartphone] Failed to read registration message: %v", err)
		h.sendError(conn, "", "READ_ERROR", "Failed to read message")
		return
	}

	if messageType != websocket.TextMessage {
		log.Printf("[smartphone] Expected text message, got type %d", messageType)
		h.sendError(conn, "", "INVALID_MESSAGE_TYPE", "Expected text message")
		return
	}

	var wsRequest WebsocketRequest
	if err := json.Unmarshal(message, &wsRequest); err != nil {
		log.Printf("[smartphone] Failed to parse registration message: %v", err)
		h.sendError(conn, "", "PARSE_ERROR", "Invalid message format")
		return
	}

	if wsRequest.Type != WSMessageTypeRegisterDevice {
		log.Printf("[smartphone] Expected '%s', got '%s'", WSMessageTypeRegisterDevice, wsRequest.Type)
		h.sendError(conn, wsRequest.ID, "INVALID_MESSAGE_TYPE", fmt.Sprintf("Expected '%s' message", WSMessageTypeRegisterDevice))
		return
	}

	// Handle device registration
	deviceID, err = h.handleRegisterDevice(conn, wsRequest)
	if err != nil {
		log.Printf("[smartphone] Registration failed: %v", err)
		return
	}

	// Handle device messages in loop
	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			break
		}

		if messageType == websocket.TextMessage {
			var wsRequest WebsocketRequest
			if err := json.Unmarshal(message, &wsRequest); err != nil {
				log.Printf("[smartphone] Failed to parse message: %v", err)
				h.sendError(conn, "", "PARSE_ERROR", "Invalid message format")
				continue
			}

			// Handle request based on type
			switch wsRequest.Type {
			case WSMessageTypeTagScanned:
				h.handleTagScanned(conn, wsRequest)
			case WSMessageTypeDeviceHeartbeat:
				h.handleDeviceHeartbeat(conn, wsRequest)
			case WSMessageTypeWriteResponse:
				// Future feature
				log.Printf("[smartphone] Write response received (not yet implemented)")
			default:
				log.Printf("[smartphone] Unknown message type: %s", wsRequest.Type)
				h.sendError(conn, wsRequest.ID, "UNKNOWN_TYPE", fmt.Sprintf("Unknown message type: %s", wsRequest.Type))
			}
		}
	}
}

// handleRegisterDevice processes device registration from mobile app.
func (h *SmartphoneDeviceHandler) handleRegisterDevice(conn *websocket.Conn, req WebsocketRequest) (string, error) {
	// Parse DeviceRegistrationRequest from payload
	payloadBytes, err := json.Marshal(req.Payload)
	if err != nil {
		h.sendError(conn, req.ID, "INVALID_PAYLOAD", "Failed to process payload")
		return "", fmt.Errorf("failed to marshal payload: %w", err)
	}

	var regReq nfc.DeviceRegistrationRequest
	if err := json.Unmarshal(payloadBytes, &regReq); err != nil {
		h.sendError(conn, req.ID, "INVALID_PAYLOAD", "Invalid registration request format")
		return "", fmt.Errorf("failed to parse registration request: %w", err)
	}

	// Validate request
	if regReq.DeviceName == "" {
		h.sendError(conn, req.ID, "INVALID_REQUEST", "Device name is required")
		return "", fmt.Errorf("device name is required")
	}
	if regReq.Platform != "ios" && regReq.Platform != "android" {
		h.sendError(conn, req.ID, "INVALID_REQUEST", "Platform must be 'ios' or 'android'")
		return "", fmt.Errorf("invalid platform: %s", regReq.Platform)
	}

	// Register device
	device, err := h.manager.RegisterDevice(regReq)
	if err != nil {
		h.sendError(conn, req.ID, "REGISTRATION_FAILED", err.Error())
		return "", fmt.Errorf("failed to register device: %w", err)
	}

	deviceID := device.DeviceID()

	// Store WebSocket connection
	h.addDeviceSession(deviceID, conn)

	// Send registration response
	response := WebsocketResponse{
		ID:      req.ID,
		Type:    WSMessageTypeRegisterDeviceResponse,
		Success: true,
		Payload: nfc.DeviceRegistrationResponse{
			DeviceID:     deviceID,
			SessionToken: "",
			ServerInfo: nfc.ServerInfo{
				Version:      "1.0.0",
				SupportedNFC: []string{"mifare", "desfire", "type4", "ultralight"},
			},
		},
	}

	if err := conn.WriteJSON(response); err != nil {
		h.removeDeviceSession(deviceID)
		h.manager.UnregisterDevice(deviceID)
		return "", fmt.Errorf("failed to send registration response: %w", err)
	}

	log.Printf("[smartphone] Device registered: %s (%s)", device.String(), deviceID)

	return deviceID, nil
}

// handleTagScanned processes tag scan events from mobile app.
func (h *SmartphoneDeviceHandler) handleTagScanned(conn *websocket.Conn, req WebsocketRequest) {
	payloadBytes, err := json.Marshal(req.Payload)
	if err != nil {
		log.Printf("[smartphone] Failed to marshal tag data: %v", err)
		h.sendError(conn, req.ID, "INVALID_PAYLOAD", "Failed to process payload")
		return
	}

	var tagData nfc.SmartphoneTagData
	if err := json.Unmarshal(payloadBytes, &tagData); err != nil {
		log.Printf("[smartphone] Failed to parse tag data: %v", err)
		h.sendError(conn, req.ID, "INVALID_PAYLOAD", "Invalid tag data format")
		return
	}

	// Validate deviceID
	if err := h.validateDevice(tagData.DeviceID); err != nil {
		log.Printf("[smartphone] Device validation failed: %v", err)
		h.sendError(conn, req.ID, "INVALID_DEVICE", err.Error())
		return
	}

	// Send tag data to smartphone manager
	if err := h.manager.SendTagData(tagData.DeviceID, tagData); err != nil {
		log.Printf("[smartphone] Failed to send tag data: %v", err)
		h.sendError(conn, req.ID, "TAG_SEND_FAILED", err.Error())
		return
	}

	log.Printf("[smartphone] Tag scanned: device=%s, UID=%s, Type=%s", tagData.DeviceID, tagData.UID, tagData.Type)
}

// handleDeviceHeartbeat updates device last-seen timestamp.
func (h *SmartphoneDeviceHandler) handleDeviceHeartbeat(conn *websocket.Conn, req WebsocketRequest) {
	payloadBytes, err := json.Marshal(req.Payload)
	if err != nil {
		return
	}

	var heartbeat nfc.DeviceHeartbeat
	if err := json.Unmarshal(payloadBytes, &heartbeat); err != nil {
		return
	}

	if err := h.validateDevice(heartbeat.DeviceID); err != nil {
		return
	}

	h.manager.UpdateHeartbeat(heartbeat.DeviceID)
}

// handleDeviceDisconnect cleans up when device WebSocket closes.
func (h *SmartphoneDeviceHandler) handleDeviceDisconnect(deviceID string) {
	h.removeDeviceSession(deviceID)

	if h.manager != nil {
		h.manager.UnregisterDevice(deviceID)
	}

	log.Printf("[smartphone] Device disconnected: %s", deviceID)
}

// validateDevice checks if a device is registered.
func (h *SmartphoneDeviceHandler) validateDevice(deviceID string) error {
	if deviceID == "" {
		return fmt.Errorf("deviceID is required")
	}

	if h.manager == nil {
		return fmt.Errorf("smartphone manager not initialized")
	}

	_, exists := h.manager.GetDevice(deviceID)
	if !exists {
		return fmt.Errorf("device not registered: %s", deviceID)
	}

	return nil
}

// addDeviceSession stores a WebSocket connection for a device.
func (h *SmartphoneDeviceHandler) addDeviceSession(deviceID string, conn *websocket.Conn) {
	h.deviceSessionsMux.Lock()
	defer h.deviceSessionsMux.Unlock()

	h.deviceSessions[deviceID] = conn
	h.connToDeviceID[conn] = deviceID
}

// removeDeviceSession removes a WebSocket connection for a device.
func (h *SmartphoneDeviceHandler) removeDeviceSession(deviceID string) {
	h.deviceSessionsMux.Lock()
	defer h.deviceSessionsMux.Unlock()

	if conn, ok := h.deviceSessions[deviceID]; ok {
		delete(h.connToDeviceID, conn)
		delete(h.deviceSessions, deviceID)
	}
}

// SendToDevice sends a message to a specific device.
func (h *SmartphoneDeviceHandler) SendToDevice(deviceID string, message interface{}) error {
	h.deviceSessionsMux.RLock()
	conn, ok := h.deviceSessions[deviceID]
	h.deviceSessionsMux.RUnlock()

	if !ok {
		return fmt.Errorf("device not connected: %s", deviceID)
	}

	return conn.WriteJSON(message)
}

// sendError sends an error response to a device.
func (h *SmartphoneDeviceHandler) sendError(conn *websocket.Conn, requestID string, errorCode string, message string) {
	response := WebsocketResponse{
		ID:      requestID,
		Type:    WSMessageTypeError,
		Success: false,
		Error:   message,
		Payload: map[string]interface{}{
			"code": errorCode,
		},
	}

	if err := conn.WriteJSON(response); err != nil {
		log.Printf("[smartphone] Failed to send error response: %v", err)
	}
}

// IsDeviceConnection determines if a request is from a mobile device.
func IsDeviceConnection(r *http.Request) bool {
	// Check X-Device-Mode header
	if r.Header.Get("X-Device-Mode") == "true" {
		return true
	}

	// Check query parameter
	if r.URL.Query().Get("mode") == "device" {
		return true
	}

	return false
}
