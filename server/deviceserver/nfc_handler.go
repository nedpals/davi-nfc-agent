package deviceserver

import (
	"context"
	"fmt"
	"log"

	"github.com/dotside-studios/davi-nfc-agent/nfc"
	"github.com/dotside-studios/davi-nfc-agent/server"
)

// NFCHandler handles NFC reader operations for the device server.
// It reads from the NFC reader and sends data through the bridge.
type NFCHandler struct {
	reader           *nfc.NFCReader
	allowedCardTypes map[string]bool
	bridge           *server.ServerBridge
}

// NewNFCHandler creates a new NFC handler for the device server.
func NewNFCHandler(reader *nfc.NFCReader, allowedCardTypes map[string]bool, bridge *server.ServerBridge) *NFCHandler {
	return &NFCHandler{
		reader:           reader,
		allowedCardTypes: allowedCardTypes,
		bridge:           bridge,
	}
}

// Register sets up message handlers and lifecycle for the device server.
func (h *NFCHandler) Register(s *Server) {
	// Register lifecycle - listen for tag data from reader
	s.StartLifecycle(func(ctx context.Context) {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case data := <-h.reader.Data():
					h.handleTagData(data, s)
				case statusUpdate := <-h.reader.StatusUpdates():
					s.BroadcastDeviceStatus(statusUpdate)
				}
			}
		}()
	})
}

// handleTagData processes incoming tag data, applies filters, and broadcasts results.
func (h *NFCHandler) handleTagData(data nfc.NFCData, s *Server) {
	if data.Err != nil {
		log.Printf("Error: %v", data.Err)
		s.BroadcastTagData(data)
		return
	}

	if data.Card == nil {
		return
	}

	// Check card type filter
	if len(h.allowedCardTypes) > 0 && !h.allowedCardTypes[data.Card.Type] {
		log.Printf("Card type '%s' not in allowed list, ignoring", data.Card.Type)
		// Send error message to consumers
		s.BroadcastTagData(nfc.NFCData{
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

	s.BroadcastTagData(data)
}
