package server

import (
	"log"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/nedpals/davi-nfc-agent/nfc"
)

// WebsocketMessage represents a message sent to WebSocket clients.
type WebsocketMessage struct {
	Type    string `json:"type"`
	Payload any    `json:"payload"`
}

// SendErrorResponse sends an error response to a WebSocket connection.
func SendErrorResponse(conn *websocket.Conn, responseType string, errMsg string) error {
	return conn.WriteJSON(map[string]interface{}{
		"type": responseType,
		"payload": map[string]interface{}{
			"success": false,
			"error":   errMsg,
		},
	})
}

// SendSuccessResponse sends a success response to a WebSocket connection.
func SendSuccessResponse(conn *websocket.Conn, responseType string, payload interface{}) error {
	return conn.WriteJSON(map[string]interface{}{
		"type":    responseType,
		"payload": payload,
	})
}

// WebsocketClientManager manages WebSocket client connections and broadcasting.
type WebsocketClientManager struct {
	clients  map[*websocket.Conn]bool
	mu       sync.RWMutex
	lastCard *nfc.Card
	cardMu   sync.RWMutex
}

// NewClientManager creates a new ClientManager instance.
func NewClientManager() *WebsocketClientManager {
	return &WebsocketClientManager{
		clients: make(map[*websocket.Conn]bool),
	}
}

// Register adds a new client connection.
func (cm *WebsocketClientManager) Register(conn *websocket.Conn) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.clients[conn] = true
}

// Unregister removes a client connection.
func (cm *WebsocketClientManager) Unregister(conn *websocket.Conn) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	delete(cm.clients, conn)
}

// CloseAll closes all client connections.
func (cm *WebsocketClientManager) CloseAll() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	for client := range cm.clients {
		client.Close()
		delete(cm.clients, client)
	}
}

// GetLastCard returns the last broadcast card.
func (cm *WebsocketClientManager) GetLastCard() *nfc.Card {
	cm.cardMu.RLock()
	defer cm.cardMu.RUnlock()
	return cm.lastCard
}

// setLastCard sets the last broadcast card (internal use).
func (cm *WebsocketClientManager) setLastCard(card *nfc.Card) {
	cm.cardMu.Lock()
	defer cm.cardMu.Unlock()
	cm.lastCard = card
}

// broadcast sends a message to all connected clients.
func (cm *WebsocketClientManager) broadcast(message WebsocketMessage) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	for client := range cm.clients {
		err := client.WriteJSON(message)
		if err != nil {
			log.Printf("WebSocket write error: %v", err)
			client.Close()
			delete(cm.clients, client)
		}
	}
}

// BroadcastDeviceStatus sends the device status to all connected WebSocket clients.
func (cm *WebsocketClientManager) BroadcastDeviceStatus(status nfc.DeviceStatus) {
	message := WebsocketMessage{
		Type:    "deviceStatus",
		Payload: status,
	}
	cm.broadcast(message)
}

// BroadcastTagData sends NFCData to all connected WebSocket clients.
// It handles client disconnections and removes disconnected clients from the pool.
func (cm *WebsocketClientManager) BroadcastTagData(data nfc.NFCData) {
	var errStr *string = nil
	if data.Err != nil {
		errStr = new(string)
		*errStr = data.Err.Error()
	}

	var payload map[string]interface{}

	if data.Card != nil {
		// Update last broadcast card for systray display
		cm.setLastCard(data.Card)

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

	message := WebsocketMessage{
		Type:    "tagData",
		Payload: payload,
	}

	cm.broadcast(message)
}
