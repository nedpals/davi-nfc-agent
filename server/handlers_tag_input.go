package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/nedpals/davi-nfc-agent/nfc"
	"github.com/nedpals/davi-nfc-agent/protocol"
)

// handleTagInput handles POST /api/v1/tag requests for injecting tag data.
func (s *Server) handleTagInput(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Parse request body
	var req protocol.TagInputRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendTagInputError(w, http.StatusBadRequest, protocol.ErrCodeInvalidRequest,
			"Failed to parse request body: "+err.Error())
		return
	}

	// Validate and normalize UID
	normalizedUID, err := protocol.ParseUID(req.UID)
	if err != nil {
		s.sendTagInputError(w, http.StatusBadRequest, protocol.ErrCodeInvalidUID, err.Error())
		return
	}

	// Create internal tag from request
	tag, err := s.convertTagInputToTag(req, normalizedUID)
	if err != nil {
		s.sendTagInputError(w, http.StatusBadRequest, protocol.ErrCodeInvalidNDEF, err.Error())
		return
	}

	// Create Card and broadcast
	card := nfc.NewCard(tag)
	s.BroadcastTagData(nfc.NFCData{Card: card, Err: nil})

	source := req.Source
	if source == "" {
		source = "http-api"
	}
	log.Printf("[http-api] Tag input received: UID=%s, Type=%s, Source=%s",
		normalizedUID, req.Type, source)

	// Send success response
	response := protocol.TagInputResponse{
		Success: true,
		Message: "Tag data broadcast to all clients",
		UID:     normalizedUID,
	}
	json.NewEncoder(w).Encode(response)
}

// convertTagInputToTag converts a TagInputRequest to an internal nfc.Tag.
func (s *Server) convertTagInputToTag(req protocol.TagInputRequest, normalizedUID string) (nfc.Tag, error) {
	// Determine tag type
	tagType := req.Type
	if tagType == "" {
		tagType = "Unknown"
	}

	// Determine technology
	technology := req.Technology
	if technology == "" {
		technology = protocol.InferTechnology(tagType)
	}

	// Determine timestamp
	scannedAt := time.Now()
	if req.ScannedAt != nil {
		scannedAt = *req.ScannedAt
	}

	// Determine source
	source := req.Source
	if source == "" {
		source = "http-api"
	}

	// Convert NDEF message if present
	var ndefMsg *nfc.NDEFMessage
	var ndefData []byte
	if req.Message != nil && len(req.Message.Records) > 0 {
		var err error
		ndefMsg, err = nfc.ConvertNDEFInput(req.Message)
		if err != nil {
			return nil, err
		}
		ndefData, err = ndefMsg.Encode()
		if err != nil {
			return nil, fmt.Errorf("failed to encode NDEF message: %w", err)
		}
	}

	// Create HTTPInputTag
	tag := &HTTPInputTag{
		uid:        normalizedUID,
		tagType:    tagType,
		technology: technology,
		ndefData:   ndefData,
		ndefMsg:    ndefMsg,
		scannedAt:  scannedAt,
		source:     source,
	}

	return tag, nil
}

// sendTagInputError sends an error response for tag input endpoint.
func (s *Server) sendTagInputError(w http.ResponseWriter, statusCode int, errorCode, message string) {
	w.WriteHeader(statusCode)
	response := protocol.TagInputResponse{
		Success:   false,
		Error:     message,
		ErrorCode: errorCode,
	}
	json.NewEncoder(w).Encode(response)
}
