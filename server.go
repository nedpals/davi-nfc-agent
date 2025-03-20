package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/clausecker/nfc/v2"
)

// DeviceStatus represents the connection status of the NFC device.
type DeviceStatus struct {
	Connected   bool   `json:"connected"`
	Message     string `json:"message,omitempty"`
	CardPresent bool   `json:"cardPresent"`
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
func recoverServer(reader *NFCReader, port int) {
	if r := recover(); r != nil {
		log.Printf("Server panic recovered: %v", r)
		log.Println("Restarting server in 5 seconds...")
		time.Sleep(5 * time.Second)
		startServer(reader, port)
	}
}

// runServer contains the main server logic
func startServer(reader *NFCReader, port int) {
	defer recoverServer(reader, port)
	defer gracefulShutdown(reader)

	version := nfc.Version()
	log.Printf("Starting NFC Agent using libnfc %s", version)
	log.Printf("Scanning for NFC devices...")

	if reader.hasDevice {
		reader.logDeviceInfo()
	} else {
		log.Printf("No NFC device connected, waiting for device...")
	}

	// Create a base context with the reader
	ctx := context.WithValue(context.Background(), readerContextKey, reader)

	// Set up HTTP routes with context
	http.DefaultServeMux = http.NewServeMux() // Reset mux for clean restart
	http.HandleFunc("/ws", enableCORS(func(w http.ResponseWriter, r *http.Request) {
		handleWebSocket(w, r.WithContext(ctx))
	}))
	http.HandleFunc("/", enableCORS(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("NFC Agent Server Running"))
	}))

	// Start the HTTP server in a goroutine
	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: http.DefaultServeMux,
	}

	go func() {
		log.Printf("Starting server on %s", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
			panic(err)
		}
	}()

	reader.Start()

	for data := range reader.Data() {
		if data.Err != nil {
			log.Printf("Error: %v", data.Err)
		} else {
			fmt.Printf("UID: %x\nDecoded text: %s\n", data.UID, data.Text)
		}
		broadcastToClients(data)
	}
}

// broadcastDeviceStatus sends the device status to all connected WebSocket clients.
func broadcastDeviceStatus(status DeviceStatus) {
	message := WebSocketMessage{
		Type:    "deviceStatus",
		Payload: status,
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
