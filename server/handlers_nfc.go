package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/gorilla/websocket"
	"github.com/nedpals/davi-nfc-agent/nfc"
)

// NFCHandler handles all NFC-related operations.
// It groups related NFC handler functions together for better organization.
type NFCHandler struct {
	reader           *nfc.NFCReader
	allowedCardTypes map[string]bool
}

// NewNFCHandler creates a new NFC handler.
func NewNFCHandler(reader *nfc.NFCReader, allowedCardTypes map[string]bool) *NFCHandler {
	return &NFCHandler{
		reader:           reader,
		allowedCardTypes: allowedCardTypes,
	}
}

// Register implements ServerHandler interface.
// It sets up message handlers and lifecycle in one place.
func (h *NFCHandler) Register(server HandlerServer) {
	// Register message handlers
	server.Handle(WSMessageTypeWriteRequest, h.handleWrite)
	
	// Register lifecycle - inline the start logic here
	server.StartLifecycle(func(ctx context.Context) {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case data := <-h.reader.Data():
					h.handleTagData(data, server)
				case statusUpdate := <-h.reader.StatusUpdates():
					server.BroadcastDeviceStatus(statusUpdate)
				}
			}
		}()
	})
}

// handleWrite processes a write request from a WebSocket client.
func (h *NFCHandler) handleWrite(ctx context.Context, conn *websocket.Conn, req WebsocketRequest) error {
	// Parse write request from payload
	payloadBytes, err := json.Marshal(req.Payload)
	if err != nil {
		log.Printf("Failed to marshal write request payload: %v", err)
		return h.sendError(conn, req.ID, "INVALID_PAYLOAD", "Invalid write request payload")
	}

	var writeReq WriteRequest
	if err := json.Unmarshal(payloadBytes, &writeReq); err != nil {
		log.Printf("Failed to parse write request: %v", err)
		return h.sendError(conn, req.ID, "INVALID_WRITE_REQUEST", "Failed to parse write request")
	}

	// Perform write operation
	err = HandleWriteRequest(h.reader, writeReq)
	if err != nil {
		log.Printf("Write operation failed: %v", err)
		return h.sendError(conn, req.ID, "WRITE_FAILED", err.Error())
	}

	// Send success response
	response := WebsocketResponse{
		ID:      req.ID,
		Type:    WSMessageTypeWriteResponse,
		Success: true,
		Payload: map[string]interface{}{
			"message": "Write operation completed successfully",
		},
	}

	if err := conn.WriteJSON(response); err != nil {
		log.Printf("Failed to send write response: %v", err)
		return err
	}

	return nil
}

// sendError sends an error response to the WebSocket client.
func (h *NFCHandler) sendError(conn *websocket.Conn, requestID string, errorCode string, message string) error {
	response := WebsocketResponse{
		ID:      requestID,
		Type:    WSMessageTypeError,
		Success: false,
		Error:   message,
		Payload: map[string]interface{}{
			"code": errorCode,
		},
	}

	return conn.WriteJSON(response)
}

// handleTagData processes incoming tag data, applies filters, and broadcasts results.
func (h *NFCHandler) handleTagData(data nfc.NFCData, server HandlerServer) {
	if data.Err != nil {
		log.Printf("Error: %v", data.Err)
		server.BroadcastTagData(data)
		return
	}

	if data.Card == nil {
		return
	}

	// Check card type filter
	if len(h.allowedCardTypes) > 0 && !h.allowedCardTypes[data.Card.Type] {
		log.Printf("Card type '%s' not in allowed list, ignoring", data.Card.Type)
		// Send error message to clients
		server.BroadcastTagData(nfc.NFCData{
			Card: nil,
			Err:  fmt.Errorf("card type '%s' not allowed by filter", data.Card.Type),
		})
		return
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

	server.BroadcastTagData(data)
}
