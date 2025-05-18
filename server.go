package main

import (
	"context"
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
	defer gracefulShutdown(reader)

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
				} else {
					// UID is a string in nfc.NFCData, adjust printing if it was hex bytes before
					fmt.Printf("UID: %s\nDecoded text: %s\n", data.UID, data.Text)
				}
				broadcastToClients(data)
			case statusUpdate := <-reader.StatusUpdates(): // reader.StatusUpdates() returns chan nfc.DeviceStatus
				broadcastDeviceStatus(statusUpdate)
			}
		}
	}()
}
