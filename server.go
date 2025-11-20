package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/nedpals/davi-nfc-agent/nfc"
)

type readerContextKeySymbol struct{}

var readerContextKey = readerContextKeySymbol{}

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
func recoverServer(reader *nfc.NFCReader, port int) { // Use nfc.NFCReader
	if r := recover(); r != nil {
		log.Printf("Server panic recovered: %v", r)
		log.Println("Restarting server in 5 seconds...")
		time.Sleep(5 * time.Second)
		startServer(reader, port)
	}
}

// broadcastDeviceStatus sends the device status to all connected WebSocket clients.
func broadcastDeviceStatus(status nfc.DeviceStatus) { // Use nfc.DeviceStatus
	message := WebSocketMessage{
		Type:    "deviceStatus",
		Payload: status,
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

var (
	currentServer *http.Server
	serverCtx     context.Context
	serverCancel  context.CancelFunc
)

func stopServer() {
	if currentServer != nil {
		if err := currentServer.Shutdown(context.Background()); err != nil {
			log.Printf("Server shutdown error: %v", err)
		}
		currentServer = nil
	}
	if serverCancel != nil {
		serverCancel()
	}
}

func startServer(reader *nfc.NFCReader, port int) { // Use nfc.NFCReader
	defer recoverServer(reader, port)

	// version := nfc.Version() // This was from github.com/clausecker/nfc/v2, direct usage removed
	// log.Printf("Starting NFC Agent using libnfc %s", version)
	log.Printf("Starting NFC Agent...") // Simplified log message
	log.Printf("Scanning for NFC devices...")

	// reader.hasDevice is not directly accessible. Use GetDeviceStatus()
	deviceStatus := reader.GetDeviceStatus()
	if deviceStatus.Connected {
		reader.LogDeviceInfo() // Use nfc.NFCReader method
	} else {
		log.Printf("No NFC device connected, waiting for device...")
	}

	// Create a base context with the reader
	ctx := context.WithValue(context.Background(), readerContextKey, reader)

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
		token := acquireSession(requestBody.Secret, origin, remoteAddr)
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

	// Configure WebSocket with permissive CORS policy
	http.HandleFunc("/ws", enableCORS(func(w http.ResponseWriter, r *http.Request) {
		// Set WebSocket upgrader to allow all origins
		upgrader.CheckOrigin = func(r *http.Request) bool {
			return true // Allow all origins
		}
		handleWebSocket(w, r.WithContext(ctx))
	}))

	http.HandleFunc("/", enableCORS(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("NFC Agent Server Running"))
	}))

	// Start the HTTP server in a goroutine
	serverCtx, serverCancel = context.WithCancel(context.Background())
	currentServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: http.DefaultServeMux,
	}

	go func() {
		log.Printf("Starting server on %s", currentServer.Addr)
		if err := currentServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
			panic(err)
		}
	}()

	reader.Start()

	// Start data handling in a goroutine
	go func() {
		for {
			select {
			case <-serverCtx.Done():
				return
			case data := <-reader.Data(): // reader.Data() returns chan nfc.NFCData

				if data.Err != nil {
					log.Printf("Error: %v", data.Err)
				} else if data.Card != nil {
					// Check card type filter
					if len(allowedCardTypes) > 0 && !allowedCardTypes[data.Card.Type] {
						log.Printf("Card type '%s' not in allowed list, ignoring", data.Card.Type)
						// Send error message to clients
						broadcastToClients(nfc.NFCData{
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
				broadcastToClients(data)
			case statusUpdate := <-reader.StatusUpdates(): // reader.StatusUpdates() returns chan nfc.DeviceStatus
				broadcastDeviceStatus(statusUpdate)
			}
		}
	}()

	// Block until shutdown is requested
	<-serverCtx.Done()
	log.Println("Server context cancelled, initiating shutdown...")

	// Perform graceful shutdown
	gracefulShutdown(reader)
}
