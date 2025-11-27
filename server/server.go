// Package server provides HTTP and WebSocket server infrastructure for the NFC agent.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/grandcat/zeroconf"
	"github.com/nedpals/davi-nfc-agent/nfc"
)

type readerContextKeySymbol struct{}

var readerContextKey = readerContextKeySymbol{}

// WebsocketMessage represents a message sent to/from WebSocket clients.
type WebsocketMessage struct {
	ID      string `json:"id,omitempty"` // Request ID for correlation
	Type    string `json:"type"`
	Payload any    `json:"payload"`
}

// WebsocketRequest represents an incoming request from WebSocket clients.
type WebsocketRequest struct {
	ID      string                 `json:"id,omitempty"` // Client-generated request ID
	Type    string                 `json:"type"`
	Payload map[string]interface{} `json:"payload,omitempty"`
}

// WebsocketResponse represents a response to a WebSocket request.
type WebsocketResponse struct {
	ID      string `json:"id,omitempty"` // Same as request ID
	Type    string `json:"type"`         // Response type (e.g., "writeResponse")
	Success bool   `json:"success"`      // Whether operation succeeded
	Payload any    `json:"payload,omitempty"`
	Error   string `json:"error,omitempty"` // Error message if failed
}


// Config holds the server configuration
type Config struct {
	Reader            *nfc.NFCReader
	Port              int
	APISecret         string // Optional API secret for WebSocket connection
	AllowedCardTypes  map[string]bool
	SmartphoneHandler *SmartphoneHandler // Smartphone device handler (optional)
}

// Server manages the HTTP and WebSocket server
type Server struct {
	config        Config
	httpServer    *http.Server
	ctx           context.Context
	cancel        context.CancelFunc
	lastCard      *nfc.Card
	cardMu        sync.RWMutex
	
	// Client WebSocket management
	clients       map[*websocket.Conn]bool
	clientsMux    sync.RWMutex
	sessionActive bool       // Whether a WebSocket session is active
	sessionMux    sync.Mutex // Protects sessionActive
	upgrader      websocket.Upgrader
	
	// Handler registry (unified for both client and device connections)
	handlerRegistry *HandlerRegistry
	
	// mDNS service for auto-discovery
	mdnsServer *zeroconf.Server
}

// New creates a new server instance
func New(config Config) *Server {
	s := &Server{
		config:  config,
		clients: make(map[*websocket.Conn]bool),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins
			},
		},
		handlerRegistry: NewHandlerRegistry(),
	}

	// Register NFC reader handlers
	if config.Reader != nil {
		nfcHandler := NewNFCHandler(config.Reader, config.AllowedCardTypes)
		nfcHandler.Register(s)
	}
	
	// Register smartphone handler if present
	if config.SmartphoneHandler != nil {
		config.SmartphoneHandler.Register(s)
	}

	return s
}

// Handle implements HandlerServer interface.
func (s *Server) Handle(messageType string, handler HandlerFunc) error {
	return s.handlerRegistry.Handle(messageType, handler)
}

// StartLifecycle implements HandlerServer interface.
func (s *Server) StartLifecycle(start func(ctx context.Context)) {
	s.handlerRegistry.RegisterLifecycle(start)
}

// GetLastCard returns the last broadcast card
func (s *Server) GetLastCard() *nfc.Card {
	s.cardMu.RLock()
	defer s.cardMu.RUnlock()
	return s.lastCard
}

// setLastCard sets the last broadcast card (internal use)
func (s *Server) setLastCard(card *nfc.Card) {
	s.cardMu.Lock()
	defer s.cardMu.Unlock()
	s.lastCard = card
}

// broadcast sends a message to all connected clients
func (s *Server) broadcast(message *WebsocketMessage) {
	s.clientsMux.Lock()
	defer s.clientsMux.Unlock()

	for client := range s.clients {
		err := client.WriteJSON(message)
		if err != nil {
			log.Printf("WebSocket write error: %v", err)
			client.Close()
			delete(s.clients, client)
		}
	}
}

// BroadcastDeviceStatus sends the device status to all connected WebSocket clients
func (s *Server) BroadcastDeviceStatus(status nfc.DeviceStatus) {
	s.broadcast(&WebsocketMessage{
		Type:    WSMessageTypeDeviceStatus,
		Payload: status,
	})
}

// BroadcastTagData sends NFCData to all connected WebSocket clients
func (s *Server) BroadcastTagData(data nfc.NFCData) {
	var errStr *string = nil
	if data.Err != nil {
		errStr = new(string)
		*errStr = data.Err.Error()
	}

	var payload map[string]interface{}

	if data.Card != nil {
		// Update last broadcast card for systray display
		s.setLastCard(data.Card)

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
				// NDEF message - use ToJSONMap for structured conversion
				text, _ = ndefMsg.GetText()
				messageInfo = ndefMsg.ToJSONMap()
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

	s.broadcast(&WebsocketMessage{
		Type:    WSMessageTypeTagData,
		Payload: payload,
	})
}

// enableCORS is a middleware that adds CORS headers to responses
func enableCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Set CORS headers
		w.Header().Set("Access-Control-Allow-Origin", CORSAllowOrigin)
		w.Header().Set("Access-Control-Allow-Methods", CORSAllowMethods)
		w.Header().Set("Access-Control-Allow-Headers", CORSAllowHeaders)

		// Handle preflight OPTIONS requests
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Call the next handler
		next(w, r)
	}
}

// recoverServer handles panic recovery and server restart
func (s *Server) recoverServer() {
	if r := recover(); r != nil {
		log.Printf("Server panic recovered: %v", r)
		log.Println("Restarting server in 5 seconds...")
		time.Sleep(5 * time.Second)
		s.Start()
	}
}

// Start starts the HTTP server and begins handling requests
func (s *Server) Start() error {
	defer s.recoverServer()

	log.Printf("Starting NFC Agent...")
	log.Printf("Scanning for NFC devices...")

	reader := s.config.Reader

	// Check device status
	deviceStatus := reader.GetDeviceStatus()
	if deviceStatus.Connected {
		reader.LogDeviceInfo()
	} else {
		log.Printf("No NFC device connected, waiting for device...")
	}

	// Create a base context with the reader
	baseCtx := context.WithValue(context.Background(), readerContextKey, reader)

	// Set up HTTP routes with context
	http.DefaultServeMux = http.NewServeMux() // Reset mux for clean restart

	// API v1 routes
	apiV1 := "/api/v1"

	http.HandleFunc(apiV1+"/health", enableCORS(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.handleHealthCheck(w, r)
	}))

	// Configure WebSocket endpoint
	http.HandleFunc("/ws", enableCORS(func(w http.ResponseWriter, r *http.Request) {
		s.handleWebSocket(w, r.WithContext(baseCtx))
	}))

	http.HandleFunc("/", enableCORS(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("NFC Agent Server Running"))
	}))

	// Start the HTTP server in a goroutine
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.config.Port),
		Handler: http.DefaultServeMux,
	}

	go func() {
		log.Printf("Starting server on %s", s.httpServer.Addr)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
			panic(err)
		}
	}()

	// Register mDNS service for auto-discovery
	if err := s.startMDNS(); err != nil {
		log.Printf("Warning: Failed to start mDNS service: %v", err)
		log.Printf("Auto-discovery will not be available, but server will continue normally")
	}

	reader.Start()

	// Start lifecycle handlers (NFCHandler will start its data processing loop)
	s.handlerRegistry.StartLifecycleHandlers(s.ctx)

	// Block until shutdown is requested
	<-s.ctx.Done()
	log.Println("Server context cancelled, initiating shutdown...")

	return nil
}

// Stop stops the HTTP server gracefully
func (s *Server) Stop() {
	// Shutdown mDNS service
	if s.mdnsServer != nil {
		s.mdnsServer.Shutdown()
		s.mdnsServer = nil
		log.Printf("mDNS service stopped")
	}

	if s.httpServer != nil {
		if err := s.httpServer.Shutdown(context.Background()); err != nil {
			log.Printf("Server shutdown error: %v", err)
		}
		s.httpServer = nil
	}
	if s.cancel != nil {
		s.cancel()
	}
}

// startMDNS registers the NFC agent as an mDNS service for auto-discovery
func (s *Server) startMDNS() error {
	// Service type: _nfc-agent._tcp
	// This allows mobile apps to discover NFC agents on the local network
	serviceName := MDNSServiceName
	serviceType := MDNSServiceType
	domain := MDNSDomain
	port := s.config.Port

	// Additional text records for service info
	txtRecords := []string{
		"version=1.0",
		"protocol=websocket",
		"path=/ws",
		"device_mode=?mode=device",
	}

	server, err := zeroconf.Register(serviceName, serviceType, domain, port, txtRecords, nil)
	if err != nil {
		return fmt.Errorf("failed to register mDNS service: %w", err)
	}

	s.mdnsServer = server
	log.Printf("mDNS service registered: %s on port %d", serviceName, port)
	log.Printf("Mobile apps can now auto-discover this agent on the local network")

	return nil
}

// handleWebSocket upgrades HTTP connections to WebSocket connections and manages
// the client connection lifecycle
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Add CORS headers
	w.Header().Set("Access-Control-Allow-Origin", r.Header.Get("Origin"))
	w.Header().Set("Access-Control-Allow-Credentials", "true")

	// Determine if this is a device or client connection
	if s.config.SmartphoneHandler != nil && IsDeviceConnection(r) {
		s.config.SmartphoneHandler.HandleWebSocket(w, r)
		return
	}

	// Check if session is already active (first come, first served)
	s.sessionMux.Lock()
	if s.sessionActive {
		s.sessionMux.Unlock()
		log.Printf("WebSocket connection rejected: session already claimed")
		http.Error(w, "Session already claimed by another client", http.StatusConflict)
		return
	}
	s.sessionActive = true
	s.sessionMux.Unlock()

	// Validate optional API secret if configured
	if s.config.APISecret != "" {
		secret := r.URL.Query().Get("secret")
		if secret != s.config.APISecret {
			s.sessionMux.Lock()
			s.sessionActive = false
			s.sessionMux.Unlock()
			log.Printf("WebSocket connection rejected: invalid API secret")
			http.Error(w, "Unauthorized: Invalid API secret", http.StatusUnauthorized)
			return
		}
	}

	reader := getReaderFromContext(r.Context())
	if reader == nil {
		s.sessionMux.Lock()
		s.sessionActive = false
		s.sessionMux.Unlock()
		log.Printf("No NFC reader in context")
		http.Error(w, "Server configuration error", http.StatusInternalServerError)
		return
	}

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.sessionMux.Lock()
		s.sessionActive = false
		s.sessionMux.Unlock()
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	log.Printf("WebSocket connected from %s", r.RemoteAddr)

	defer func() {
		conn.Close()
		// Release session when WebSocket disconnects
		s.sessionMux.Lock()
		s.sessionActive = false
		s.sessionMux.Unlock()
		log.Printf("WebSocket disconnected, session released")
	}()

	s.clientsMux.Lock()
	s.clients[conn] = true
	s.clientsMux.Unlock()

	// Send initial device status
	status := reader.GetDeviceStatus()
	conn.WriteJSON(map[string]interface{}{
		"type":    WSMessageTypeDeviceStatus,
		"payload": status,
	})

	// Get last scanned data from cache
	uid := reader.GetLastScannedData()
	if uid != "" {
		conn.WriteJSON(map[string]interface{}{
			"type": WSMessageTypeTagData,
			"payload": map[string]interface{}{
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
			s.clientsMux.Lock()
			delete(s.clients, conn)
			s.clientsMux.Unlock()
			break
		}

		if messageType == websocket.TextMessage {
			var wsRequest WebsocketRequest
			if err := json.Unmarshal(message, &wsRequest); err != nil {
				log.Printf("Failed to parse WebSocket message: %v", err)
				s.sendErrorResponse(conn, "", "PARSE_ERROR", "Invalid message format")
				continue
			}

			// Get handler from registry
			handler, ok := s.handlerRegistry.Get(wsRequest.Type)
			if !ok {
				log.Printf("Unknown message type: %s", wsRequest.Type)
				s.sendErrorResponse(conn, wsRequest.ID, "UNKNOWN_TYPE", fmt.Sprintf("Unknown message type: %s", wsRequest.Type))
				continue
			}

			// Call handler
			if err := handler(r.Context(), conn, wsRequest); err != nil {
				log.Printf("Handler error for message type '%s': %v", wsRequest.Type, err)
				// Error already sent by handler, just log it
			}
		}
	}
}

// sendErrorResponse sends a structured error response to a WebSocket client
func (s *Server) sendErrorResponse(conn *websocket.Conn, requestID string, errorCode string, message string) {
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
		log.Printf("Failed to send error response: %v", err)
	}
}

// getReaderFromContext retrieves the NFCReader instance from the context
func getReaderFromContext(ctx context.Context) *nfc.NFCReader {
	reader, _ := ctx.Value(readerContextKey).(*nfc.NFCReader)
	return reader
}

// GetReaderFromContext is exported for use by other packages
func GetReaderFromContext(ctx context.Context) *nfc.NFCReader {
	return getReaderFromContext(ctx)
}

// handleHealthCheck provides a health check endpoint (GET /api/v1/health)
func (s *Server) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "ok",
		"timestamp": time.Now().Format("2006-01-02T15:04:05Z07:00"),
	})
}
