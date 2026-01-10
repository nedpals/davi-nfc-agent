package main

import (
	"errors"
	"log"
	"os"
	"time"

	"github.com/nedpals/davi-nfc-agent/nfc"
	"github.com/nedpals/davi-nfc-agent/nfc/remotenfc"
	"github.com/nedpals/davi-nfc-agent/server"
	"github.com/nedpals/davi-nfc-agent/server/consumerserver"
	"github.com/nedpals/davi-nfc-agent/server/inputserver"
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
	Manager          nfc.Manager // NFC device manager (supports hardware and smartphone)
	Reader           *nfc.NFCReader
	AllowedCardTypes map[string]bool // Card type filter using map
	APISecret        string

	// Two-server architecture
	Bridge         *server.ServerBridge
	InputServer    *inputserver.Server
	ConsumerServer *consumerserver.Server
	InputPort      int // Default: 9470
	ConsumerPort   int // Default: 9471

	// TLS configuration (optional, shared by both servers)
	CertFile string // Path to TLS certificate file
	KeyFile  string // Path to TLS private key file
}

func NewAgent(nfcManager nfc.Manager) *Agent {
	return &Agent{
		Logger:           log.New(os.Stderr, "[agent] ", log.LstdFlags),
		Manager:          nfcManager,
		AllowedCardTypes: make(map[string]bool),
		InputPort:        9470,
		ConsumerPort:     9471,
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

	// Create bridge for inter-server communication
	a.Bridge = server.NewServerBridge()

	// Get device manager - try direct cast first, then check MultiManager
	var deviceManager *remotenfc.Manager
	if pm, ok := a.Manager.(*remotenfc.Manager); ok {
		deviceManager = pm
	} else if mm, ok := a.Manager.(interface{ GetManager(string) (nfc.Manager, bool) }); ok {
		// MultiManager - get the smartphone manager
		if mgr, exists := mm.GetManager(nfc.ManagerTypeSmartphone); exists {
			if pm, ok := mgr.(*remotenfc.Manager); ok {
				deviceManager = pm
			}
		}
	}

	// Create input server (handles devices via WebSocket, hardware readers)
	a.InputServer = inputserver.New(inputserver.Config{
		Reader:           a.Reader,
		DeviceManager:    deviceManager,
		Port:             a.InputPort,
		APISecret:        a.APISecret,
		AllowedCardTypes: a.AllowedCardTypes,
		CertFile:         a.CertFile,
		KeyFile:          a.KeyFile,
	}, a.Bridge)

	// Create consumer server (handles client connections)
	a.ConsumerServer = consumerserver.New(consumerserver.Config{
		Port:      a.ConsumerPort,
		APISecret: a.APISecret,
		CertFile:  a.CertFile,
		KeyFile:   a.KeyFile,
	}, a.Bridge)

	// Start both servers
	go a.InputServer.Start()
	go a.ConsumerServer.Start()

	a.Logger.Printf("Input server on port %d, Consumer server on port %d", a.InputPort, a.ConsumerPort)
	return nil
}

func (a *Agent) Stop() {
	if a.Reader == nil && a.InputServer == nil {
		a.Logger.Println("Agent is not running")
		return
	}

	a.Logger.Println("Stopping agent...")

	if a.ConsumerServer != nil {
		a.ConsumerServer.Stop()
		a.ConsumerServer = nil
	}

	if a.InputServer != nil {
		a.InputServer.Stop()
		a.InputServer = nil
	}

	if a.Bridge != nil {
		a.Bridge.Close()
		a.Bridge = nil
	}

	if a.Reader != nil {
		a.Reader.Stop()
		a.Reader = nil
	}

	// Cleanup Manager if it's a remotenfc.Manager
	if pm, ok := a.Manager.(*remotenfc.Manager); ok {
		pm.Close()
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
