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

// WebsocketMessage represents a message sent to WebSocket clients.
type WebsocketMessage struct {
	Type    string `json:"type"`
	Payload any    `json:"payload"`
}

// Config holds the server configuration
type Config struct {
	Reader           *nfc.NFCReader
	Port             int
	SessionManager   *SessionManager
	AllowedCardTypes map[string]bool
}

// Server manages the HTTP and WebSocket server
type Server struct {
	config     Config
	httpServer *http.Server
	ctx        context.Context
	cancel     context.CancelFunc
	lastCard   *nfc.Card
	cardMu     sync.RWMutex
	clients    map[*websocket.Conn]bool
	clientsMux sync.RWMutex
	upgrader   websocket.Upgrader
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

	jsonMessage, err := json.Marshal(message)
	if err != nil {
		log.Printf("Failed to marshal WebSocket message: %v", err)
		return
	}

	for client := range s.clients {
		err := client.WriteJSON(jsonMessage)
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

	// Session handshake endpoint
	http.HandleFunc("/handshake", enableCORS(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Parse API secret from request body if provided
		var requestBody struct {
			Secret string `json:"secret,omitempty"`
		}
		json.NewDecoder(r.Body).Decode(&requestBody)

		// Get origin and remote address for session binding
		origin := r.Header.Get("Origin")
		remoteAddr := r.RemoteAddr

		// Attempt to acquire session with origin and IP binding
		token := s.config.SessionManager.Acquire(requestBody.Secret, origin, remoteAddr)
		if token == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": "Session already claimed or invalid secret",
			})
			return
		}

		// Return token
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"token": token,
		})
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

	// Validate session token
	token := r.URL.Query().Get("token")
	if token == "" {
		token = r.Header.Get("X-Session-Token")
	}

	// Get origin and remote address for validation
	origin := r.Header.Get("Origin")
	remoteAddr := r.RemoteAddr

	if !s.config.SessionManager.Validate(token, origin, remoteAddr) {
		log.Printf("Invalid or missing session token")
		http.Error(w, "Unauthorized: Invalid or missing session token", http.StatusUnauthorized)
		return
	}

	// Refresh session timeout on successful connection
	s.config.SessionManager.RefreshTimeout()

	reader := getReaderFromContext(r.Context())
	if reader == nil {
		log.Printf("No NFC reader in context")
		http.Error(w, "Server configuration error", http.StatusInternalServerError)
		return
	}

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	defer func() {
		conn.Close()
		// Release session when WebSocket disconnects
		s.config.SessionManager.Release()
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

		// Refresh session timeout on any incoming message
		s.config.SessionManager.RefreshTimeout()

		if messageType == websocket.TextMessage {
			var wsMessage map[string]interface{}
			if err := json.Unmarshal(message, &wsMessage); err != nil {
				log.Printf("Failed to parse WebSocket message: %v", err)
				continue
			}

			msgType, ok := wsMessage["type"].(string)
			if !ok {
				continue
			}

			switch msgType {
			case "writeRequest":
				// Write request handling is done in main.go
				// Forward the message through a channel or callback
				// For now, this is handled by the calling code
			case "release":
				// Client explicitly releases the session
				s.config.SessionManager.Release()
				conn.WriteJSON(map[string]interface{}{
					"type": "releaseResponse",
					"payload": map[string]interface{}{
						"success": true,
					},
				})
			}
		}
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
