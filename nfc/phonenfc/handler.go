package phonenfc

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/nedpals/davi-nfc-agent/server"
)

// deviceIDContextKey is the context key for storing device ID.
type deviceIDContextKey struct{}

// GetDeviceIDFromContext retrieves the device ID from context.
func GetDeviceIDFromContext(ctx context.Context) (string, bool) {
	deviceID, ok := ctx.Value(deviceIDContextKey{}).(string)
	return deviceID, ok
}

// WithDeviceID adds a device ID to the context.
func WithDeviceID(ctx context.Context, deviceID string) context.Context {
	return context.WithValue(ctx, deviceIDContextKey{}, deviceID)
}

// Handler handles all smartphone device WebSocket connections and management.
type Handler struct {
	manager           *Manager
	deviceSessions    map[string]*websocket.Conn // deviceID -> websocket conn
	deviceSessionsMux sync.RWMutex
	connToDeviceID    map[*websocket.Conn]string // reverse lookup: conn -> deviceID
	upgrader          websocket.Upgrader
}

// NewHandler creates a new smartphone handler.
func NewHandler(manager *Manager) *Handler {
	return &Handler{
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

// Register implements server.ServerHandler interface.
// Registers a custom WebSocket handler for device connections.
func (h *Handler) Register(s server.HandlerServer) {
	s.HandleWebSocket(IsDeviceConnection, func(w http.ResponseWriter, r *http.Request) bool {
		h.HandleWebSocket(w, r)
		return true
	})
}

// HandleWebSocket handles WebSocket connections from mobile devices.
// No authentication required - plug and play for seamless mobile integration.
func (h *Handler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
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

	var wsRequest server.WebsocketRequest
	if err := json.Unmarshal(message, &wsRequest); err != nil {
		log.Printf("[smartphone] Failed to parse registration message: %v", err)
		h.sendError(conn, "", "PARSE_ERROR", "Invalid message format")
		return
	}

	if wsRequest.Type != MessageTypeRegisterDevice {
		log.Printf("[smartphone] Expected '%s', got '%s'", MessageTypeRegisterDevice, wsRequest.Type)
		h.sendError(conn, wsRequest.ID, "INVALID_MESSAGE_TYPE", fmt.Sprintf("Expected '%s' message", MessageTypeRegisterDevice))
		return
	}

	// Handle device registration
	if err = h.handleRegisterDevice(r.Context(), conn, wsRequest); err != nil {
		log.Printf("[smartphone] Registration failed: %v", err)
		return
	}

	// Get deviceID from connection context
	deviceID = h.getDeviceIDFromConn(conn)
	if deviceID == "" {
		log.Printf("[smartphone] Failed to get deviceID after registration")
		h.sendError(conn, wsRequest.ID, "REGISTRATION_FAILED", "Failed to get device ID")
		return
	}

	// Handle device messages in loop
	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			break
		}

		if messageType == websocket.TextMessage {
			var wsRequest server.WebsocketRequest
			if err := json.Unmarshal(message, &wsRequest); err != nil {
				log.Printf("[smartphone] Failed to parse message: %v", err)
				h.sendError(conn, "", "PARSE_ERROR", "Invalid message format")
				continue
			}

			// Route message to appropriate handler
			ctx := WithDeviceID(r.Context(), deviceID)
			var handlerErr error

			switch wsRequest.Type {
			case MessageTypeTagScanned:
				handlerErr = h.handleTagScanned(ctx, conn, wsRequest)
			case MessageTypeDeviceHeartbeat:
				handlerErr = h.handleDeviceHeartbeat(ctx, conn, wsRequest)
			case MessageTypeWriteResponse:
				log.Printf("[smartphone] Write response received (not yet implemented)")
				continue
			default:
				log.Printf("[smartphone] Unknown message type: %s", wsRequest.Type)
				h.sendError(conn, wsRequest.ID, "UNKNOWN_TYPE", fmt.Sprintf("Unknown message type: %s", wsRequest.Type))
				continue
			}

			if handlerErr != nil {
				log.Printf("[smartphone] Handler error for message type '%s': %v", wsRequest.Type, handlerErr)
			}
		}
	}
}

// handleRegisterDevice processes a device registration request.
func (h *Handler) handleRegisterDevice(ctx context.Context, conn *websocket.Conn, req server.WebsocketRequest) error {
	// Parse DeviceRegistrationRequest from payload
	payloadBytes, err := json.Marshal(req.Payload)
	if err != nil {
		h.sendError(conn, req.ID, "INVALID_PAYLOAD", "Failed to process payload")
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	var regReq DeviceRegistrationRequest
	if err := json.Unmarshal(payloadBytes, &regReq); err != nil {
		h.sendError(conn, req.ID, "INVALID_PAYLOAD", "Invalid registration request format")
		return fmt.Errorf("failed to parse registration request: %w", err)
	}

	// Validate request
	if regReq.DeviceName == "" {
		h.sendError(conn, req.ID, "INVALID_REQUEST", "Device name is required")
		return fmt.Errorf("device name is required")
	}
	if regReq.Platform != "ios" && regReq.Platform != "android" {
		h.sendError(conn, req.ID, "INVALID_REQUEST", "Platform must be 'ios' or 'android'")
		return fmt.Errorf("invalid platform: %s", regReq.Platform)
	}

	// Register device
	device, err := h.manager.RegisterDevice(regReq)
	if err != nil {
		h.sendError(conn, req.ID, "REGISTRATION_FAILED", err.Error())
		return fmt.Errorf("failed to register device: %w", err)
	}

	deviceID := device.DeviceID()

	// Store WebSocket connection
	h.addDeviceSession(deviceID, conn)

	// Send registration response
	response := server.WebsocketResponse{
		ID:      req.ID,
		Type:    MessageTypeRegisterDeviceResponse,
		Success: true,
		Payload: DeviceRegistrationResponse{
			DeviceID:     deviceID,
			SessionToken: "",
			ServerInfo: ServerInfo{
				Version:      "1.0.0",
				SupportedNFC: []string{"mifare", "desfire", "type4", "ultralight"},
			},
		},
	}

	if err := conn.WriteJSON(response); err != nil {
		h.removeDeviceSession(deviceID)
		h.manager.UnregisterDevice(deviceID)
		return fmt.Errorf("failed to send registration response: %w", err)
	}

	log.Printf("[smartphone] Device registered: %s (%s)", device.String(), deviceID)

	return nil
}

// handleTagScanned processes a tag scan event from a mobile device.
func (h *Handler) handleTagScanned(ctx context.Context, conn *websocket.Conn, req server.WebsocketRequest) error {
	payloadBytes, err := json.Marshal(req.Payload)
	if err != nil {
		log.Printf("[smartphone] Failed to marshal tag data: %v", err)
		h.sendError(conn, req.ID, "INVALID_PAYLOAD", "Failed to process payload")
		return err
	}

	var tagData TagData
	if err := json.Unmarshal(payloadBytes, &tagData); err != nil {
		log.Printf("[smartphone] Failed to parse tag data: %v", err)
		h.sendError(conn, req.ID, "INVALID_PAYLOAD", "Invalid tag data format")
		return err
	}

	// Validate deviceID
	if err := h.validateDevice(tagData.DeviceID); err != nil {
		log.Printf("[smartphone] Device validation failed: %v", err)
		h.sendError(conn, req.ID, "INVALID_DEVICE", err.Error())
		return err
	}

	// Send tag data to smartphone manager
	if err := h.manager.SendTagData(tagData.DeviceID, tagData); err != nil {
		log.Printf("[smartphone] Failed to send tag data: %v", err)
		h.sendError(conn, req.ID, "TAG_SEND_FAILED", err.Error())
		return err
	}

	log.Printf("[smartphone] Tag scanned: device=%s, UID=%s, Type=%s", tagData.DeviceID, tagData.UID, tagData.Type)
	return nil
}

// handleDeviceHeartbeat processes a heartbeat from a mobile device.
func (h *Handler) handleDeviceHeartbeat(ctx context.Context, conn *websocket.Conn, req server.WebsocketRequest) error {
	payloadBytes, err := json.Marshal(req.Payload)
	if err != nil {
		return err
	}

	var heartbeat DeviceHeartbeat
	if err := json.Unmarshal(payloadBytes, &heartbeat); err != nil {
		return err
	}

	if err := h.validateDevice(heartbeat.DeviceID); err != nil {
		return err
	}

	h.manager.UpdateHeartbeat(heartbeat.DeviceID)
	return nil
}

// handleDeviceDisconnect cleans up when device WebSocket closes.
func (h *Handler) handleDeviceDisconnect(deviceID string) {
	h.removeDeviceSession(deviceID)

	if h.manager != nil {
		h.manager.UnregisterDevice(deviceID)
	}

	log.Printf("[smartphone] Device disconnected: %s", deviceID)
}

// validateDevice checks if a device is registered.
func (h *Handler) validateDevice(deviceID string) error {
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
func (h *Handler) addDeviceSession(deviceID string, conn *websocket.Conn) {
	h.deviceSessionsMux.Lock()
	defer h.deviceSessionsMux.Unlock()

	h.deviceSessions[deviceID] = conn
	h.connToDeviceID[conn] = deviceID
}

// getDeviceIDFromConn retrieves the device ID for a connection.
func (h *Handler) getDeviceIDFromConn(conn *websocket.Conn) string {
	h.deviceSessionsMux.RLock()
	defer h.deviceSessionsMux.RUnlock()

	return h.connToDeviceID[conn]
}

// removeDeviceSession removes a WebSocket connection for a device.
func (h *Handler) removeDeviceSession(deviceID string) {
	h.deviceSessionsMux.Lock()
	defer h.deviceSessionsMux.Unlock()

	if conn, ok := h.deviceSessions[deviceID]; ok {
		delete(h.connToDeviceID, conn)
		delete(h.deviceSessions, deviceID)
	}
}

// SendToDevice sends a message to a specific device.
func (h *Handler) SendToDevice(deviceID string, message interface{}) error {
	h.deviceSessionsMux.RLock()
	conn, ok := h.deviceSessions[deviceID]
	h.deviceSessionsMux.RUnlock()

	if !ok {
		return fmt.Errorf("device not connected: %s", deviceID)
	}

	return conn.WriteJSON(message)
}

// sendError sends an error response to a device.
func (h *Handler) sendError(conn *websocket.Conn, requestID string, errorCode string, message string) {
	response := server.WebsocketResponse{
		ID:      requestID,
		Type:    MessageTypeError,
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
