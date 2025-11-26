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

// NDEFRecordPayload represents an NDEF record in the broadcast payload.
// This structure is used when reading and broadcasting tag data to WebSocket clients.
// Field names are consistent with NDEFRecord for easier client-side handling.
type NDEFRecordPayload struct {
	Type     string `json:"type"`              // Record type: "text", "uri", etc. (human-readable)
	Content  string `json:"content,omitempty"` // Decoded content (text or URI)
	Language string `json:"language,omitempty"`// Language code for text records
	TNF      uint8  `json:"tnf"`               // Type Name Format (technical detail)
	ID       string `json:"id,omitempty"`      // Record ID (optional)
	Payload  []byte `json:"payload"`           // Raw payload data
}

// Config holds the server configuration
type Config struct {
	Reader           *nfc.NFCReader
	Port             int
	APISecret        string // Optional API secret for WebSocket connection
	AllowedCardTypes map[string]bool
}

// Server manages the HTTP and WebSocket server
type Server struct {
	config        Config
	httpServer    *http.Server
	ctx           context.Context
	cancel        context.CancelFunc
	lastCard      *nfc.Card
	cardMu        sync.RWMutex
	clients       map[*websocket.Conn]bool
	clientsMux    sync.RWMutex
	sessionActive bool       // Whether a WebSocket session is active
	sessionMux    sync.Mutex // Protects sessionActive
	upgrader      websocket.Upgrader
}

// New creates a new server instance
func New(config Config) *Server {
	return &Server{
		config:  config,
		clients: make(map[*websocket.Conn]bool),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins
			},
		},
	}
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
		Type:    "deviceStatus",
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
				// NDEF message - extract text and build records array
				text, _ = ndefMsg.GetText()

				records := make([]NDEFRecordPayload, 0, len(ndefMsg.Records()))
				for _, record := range ndefMsg.Records() {
					recordInfo := NDEFRecordPayload{
						TNF:     record.TNF,
						Payload: record.Payload,
					}

					// Add ID if present
					if len(record.ID) > 0 {
						recordInfo.ID = string(record.ID)
					}

					// Extract type-specific data and set Type + Content fields
					if recordText, ok := record.GetText(); ok {
						recordInfo.Type = "text"
						recordInfo.Content = recordText
						// TODO: Extract language from record if available
						recordInfo.Language = "en" // Default for now
					} else if recordURI, ok := record.GetURI(); ok {
						recordInfo.Type = "uri"
						recordInfo.Content = recordURI
					} else {
						// Unknown type - use raw type field
						recordInfo.Type = string(record.Type)
					}

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

	s.broadcast(&WebsocketMessage{
		Type:    "tagData",
		Payload: payload,
	})
}

// enableCORS is a middleware that adds CORS headers to responses
func enableCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Set CORS headers
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

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

	reader.Start()

	// Start data handling in a goroutine
	go func() {
		for {
			select {
			case <-s.ctx.Done():
				return
			case data := <-reader.Data():
				if data.Err != nil {
					log.Printf("Error: %v", data.Err)
				} else if data.Card != nil {
					// Check card type filter
					if len(s.config.AllowedCardTypes) > 0 && !s.config.AllowedCardTypes[data.Card.Type] {
						log.Printf("Card type '%s' not in allowed list, ignoring", data.Card.Type)
						// Send error message to clients
						s.BroadcastTagData(nfc.NFCData{
							Card: nil,
							Err:  fmt.Errorf("card type '%s' not allowed by filter", data.Card.Type),
						})
						continue
					}

					// Read message from card
					var text string
					if msg, err := data.Card.ReadMessage(); err == nil {
						if ndefMsg, ok := msg.(*nfc.NDEFMessage); ok {
							text, _ = ndefMsg.GetText()
							if text == "" {
								// Try URI if no text
								text, _ = ndefMsg.GetURI()
							}
						} else if textMsg, ok := msg.(*nfc.TextMessage); ok {
							text = textMsg.Text
						}
					}
					fmt.Printf("UID: %s\nDecoded text: %s\n", data.Card.UID, text)
				}
				s.BroadcastTagData(data)
			case statusUpdate := <-reader.StatusUpdates():
				s.BroadcastDeviceStatus(statusUpdate)
			}
		}
	}()

	// Block until shutdown is requested
	<-s.ctx.Done()
	log.Println("Server context cancelled, initiating shutdown...")

	return nil
}

// Stop stops the HTTP server gracefully
func (s *Server) Stop() {
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

// handleWebSocket upgrades HTTP connections to WebSocket connections and manages
// the client connection lifecycle
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Add CORS headers
	w.Header().Set("Access-Control-Allow-Origin", r.Header.Get("Origin"))
	w.Header().Set("Access-Control-Allow-Credentials", "true")

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
		"type":    "deviceStatus",
		"payload": status,
	})

	// Get last scanned data from cache
	uid := reader.GetLastScannedData()
	if uid != "" {
		conn.WriteJSON(map[string]interface{}{
			"type": "tagData",
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

			// Handle request based on type
			switch wsRequest.Type {
			case "writeRequest":
				s.handleWriteRequest(conn, reader, wsRequest)
			default:
				log.Printf("Unknown message type: %s", wsRequest.Type)
				s.sendErrorResponse(conn, wsRequest.ID, "UNKNOWN_TYPE", fmt.Sprintf("Unknown message type: %s", wsRequest.Type))
			}
		}
	}
}

// handleWriteRequest handles write requests from WebSocket clients
func (s *Server) handleWriteRequest(conn *websocket.Conn, reader *nfc.NFCReader, wsRequest WebsocketRequest) {
	// Parse write request from payload
	payloadBytes, err := json.Marshal(wsRequest.Payload)
	if err != nil {
		log.Printf("Failed to marshal write request payload: %v", err)
		s.sendErrorResponse(conn, wsRequest.ID, "INVALID_PAYLOAD", "Invalid write request payload")
		return
	}

	var writeReq WriteRequest
	if err := json.Unmarshal(payloadBytes, &writeReq); err != nil {
		log.Printf("Failed to parse write request: %v", err)
		s.sendErrorResponse(conn, wsRequest.ID, "INVALID_WRITE_REQUEST", "Failed to parse write request")
		return
	}

	// Perform write operation
	err = HandleWriteRequest(reader, writeReq)
	if err != nil {
		log.Printf("Write operation failed: %v", err)
		s.sendErrorResponse(conn, wsRequest.ID, "WRITE_FAILED", err.Error())
		return
	}

	// Send success response
	response := WebsocketResponse{
		ID:      wsRequest.ID,
		Type:    "writeResponse",
		Success: true,
		Payload: map[string]interface{}{
			"message": "Write operation completed successfully",
		},
	}

	if err := conn.WriteJSON(response); err != nil {
		log.Printf("Failed to send write response: %v", err)
	}
}

// sendErrorResponse sends a structured error response to a WebSocket client
func (s *Server) sendErrorResponse(conn *websocket.Conn, requestID string, errorCode string, message string) {
	response := WebsocketResponse{
		ID:      requestID,
		Type:    "error",
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
