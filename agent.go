package main

import (
	"errors"
	"log"
	"os"
	"time"

	"github.com/nedpals/davi-nfc-agent/nfc"
	"github.com/nedpals/davi-nfc-agent/server"
)

// GetAllCardTypeFilterNames returns all card type filter names from nfc package constants
func GetAllCardTypeFilterNames() []string {
	return nfc.GetAllCardTypes()
}

// GetCardTypeFilterDisplayName returns a user-friendly display name for a card type
func GetCardTypeFilterDisplayName(cardType string) string {
	return cardType
}

// GetCardTypeFilterTooltip returns a tooltip for a card type filter
func GetCardTypeFilterTooltip(cardType string) string {
	return "Allow " + cardType + " only"
}

type Agent struct {
	Logger           *log.Logger
	Manager          nfc.Manager     // NFC device manager (supports hardware and smartphone)
	Reader           *nfc.NFCReader
	Server           *server.Server
	AllowedCardTypes map[string]bool // Card type filter using map
	APISecret        string
	ServerPort       int
}

func NewAgent(nfcManager nfc.Manager) *Agent {
	return &Agent{
		Logger:           log.New(os.Stderr, "[agent] ", log.LstdFlags),
		Manager:          nfcManager,
		AllowedCardTypes: make(map[string]bool),
	}
}

func (a *Agent) Start(devicePath string) error {
	if a.Reader != nil {
		if devicePath == a.Reader.DevicePath() {
			a.Logger.Printf("NFC reader already running on device: %s", devicePath)
			return nil
		}
		return errors.New("agent is already running")
	}

	// Create NFC reader with manager (supports both hardware and smartphone devices)
	nfcReader, err := nfc.NewNFCReader(devicePath, a.Manager, 5*time.Second)
	if err != nil {
		a.Logger.Printf("Error initializing NFC reader: %v", err)
		return err
	}

	a.Reader = nfcReader

	// Create server
	a.Server = server.New(server.Config{
		Reader:           a.Reader,
		Port:             a.ServerPort,
		APISecret:        a.APISecret,
		AllowedCardTypes: a.AllowedCardTypes,
	})

	// Register handlers if Manager implements ServerHandler
	if handler, ok := a.Manager.(server.ServerHandler); ok {
		handler.Register(a.Server)
	}

	go a.Server.Start()
	return nil
}

func (a *Agent) Stop() {
	if a.Reader == nil && a.Server == nil {
		a.Logger.Println("Agent is not running")
		return
	}

	a.Logger.Println("Stopping agent...")

	if a.Server != nil {
		a.Server.Stop()
		a.Server = nil
	}

	if a.Reader != nil {
		a.Reader.Stop()
		a.Reader = nil
	}

	// Cleanup Manager if it has a Close method
	if closer, ok := a.Manager.(interface{ Close() }); ok {
		closer.Close()
	}

	a.Logger.Println("Agent stopped successfully")
}

func (a *Agent) SetAllowCardType(cardType string, allow bool) {
	if allow {
		a.AllowCardType(cardType)
	} else {
		a.DisallowCardType(cardType)
	}
}

func (a *Agent) AllowAllCardTypes() {
	for _, cardType := range nfc.GetAllCardTypes() {
		a.AllowedCardTypes[cardType] = true
	}
}

func (a *Agent) AllowedCardTypesLength() int {
	return len(a.AllowedCardTypes)
}

func (a *Agent) AllowCardType(cardType string) {
	a.AllowedCardTypes[cardType] = true
}

func (a *Agent) DisallowCardType(cardType string) {
	delete(a.AllowedCardTypes, cardType)
}

func (a *Agent) IsCardTypeAllowed(cardType string) bool {
	return a.AllowedCardTypes[cardType]
}
