// Package main provides an NFC card reader agent with WebSocket broadcasting capabilities.
// It supports reading NDEF formatted text from Mifare Classic tags and broadcasts the data
// to connected WebSocket clients.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
)

var (
	upgrader = websocket.Upgrader{
		CheckOrigin: checkOrigin,
	}
	clients           = make(map[*websocket.Conn]bool)
	clientsMux        sync.RWMutex
	defaultPort       = 18080
	additionalOrigins string
)

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
	defer conn.Close()

	clientsMux.Lock()
	clients[conn] = true
	clientsMux.Unlock()

	// Send initial device status
	status := reader.getDeviceStatus()
	conn.WriteJSON(WebSocketMessage{
		Type:    "deviceStatus",
		Payload: status,
	})

	// Get last scanned data from cache
	uid, text := reader.cache.getLastScanned()
	if uid != "" {
		conn.WriteJSON(WebSocketMessage{
			Type: "tagData",
			Payload: map[string]interface{}{
				"uid":  uid,
				"text": text,
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

		if messageType == websocket.TextMessage {
			var wsMessage WebSocketMessage
			if err := json.Unmarshal(message, &wsMessage); err != nil {
				log.Printf("Failed to parse WebSocket message: %v", err)
				continue
			}

			switch wsMessage.Type {
			case "writeRequest":
				var writeReq WriteRequest
				payloadJSON, err := json.Marshal(wsMessage.Payload)
				if err != nil {
					log.Printf("Failed to marshal write payload: %v", err)
					continue
				}

				if err := json.Unmarshal(payloadJSON, &writeReq); err != nil {
					log.Printf("Failed to parse write request: %v", err)
					continue
				}

				// Get reader from context and attempt write
				reader := getReaderFromContext(r.Context())
				if reader == nil {
					conn.WriteJSON(WebSocketMessage{
						Type: "writeResponse",
						Payload: map[string]interface{}{
							"success": false,
							"error":   "NFC reader not available",
						},
					})
					continue
				}

				err = reader.WriteCardData(writeReq.Text)
				if err != nil {
					log.Println(err)
				}

				var errStr *string = nil
				if err != nil {
					errStr = new(string)
					*errStr = err.Error()
				}

				conn.WriteJSON(WebSocketMessage{
					Type: "writeResponse",
					Payload: map[string]interface{}{
						"success": err == nil,
						"error":   errStr,
					},
				})
			}
		}
	}
}

// broadcastToClients sends NFCData to all connected WebSocket clients.
// It handles client disconnections and removes disconnected clients from the pool.
func broadcastToClients(data NFCData) {
	var errStr *string = nil
	if data.Err != nil {
		errStr = new(string)
		*errStr = data.Err.Error()
	}

	message := WebSocketMessage{
		Type: "tagData",
		Payload: map[string]interface{}{
			"uid":  data.UID,
			"text": data.Text,
			"err":  errStr,
		},
	}

	clientsMux.RLock()
	for client := range clients {
		err := client.WriteJSON(message)
		if err != nil {
			log.Printf("WebSocket write error: %v", err)
			client.Close()
			delete(clients, client)
		}
	}
	clientsMux.RUnlock()
}

// getReaderFromContext retrieves the NFCReader instance from the context.
// Returns nil if no reader is found in the context.
func getReaderFromContext(ctx context.Context) *NFCReader {
	reader, _ := ctx.Value(readerContextKey).(*NFCReader)
	return reader
}

// WebSocketMessage represents a message sent to WebSocket clients.
type WebSocketMessage struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

// WriteRequest represents a request to write data to an NFC card
type WriteRequest struct {
	Text string `json:"text"`
}

// gracefulShutdown attempts to close all active connections and resources
func gracefulShutdown(reader *NFCReader) {
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

func main() {
	// Command line flags
	devicePath := flag.String("device", "", "Path to NFC device (optional)")
	port := flag.Int("port", defaultPort, "Port to listen on for the web interface")
	flag.Parse()

	// Regular mode - start NFC reader and web server
	reader, err := NewNFCReader(*devicePath)
	if err != nil {
		log.Fatalf("Error initializing NFC reader: %v", err)
	}
	defer reader.Close()

	// Start the web interface
	startServer(reader, *port)
}
