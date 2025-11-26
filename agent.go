package main

import (
	"errors"
	"log"
	"os"
	"time"

	"github.com/nedpals/davi-nfc-agent/nfc"
	"github.com/nedpals/davi-nfc-agent/server"
)

type CardTypeFilter struct {
	MifareClassic1K  bool
	MifareClassic4K  bool
	MifareUltralight bool
	Desfire          bool
	Type4            bool
}

type CardTypeFilterName string

var (
	CardTypeMifareClassic1K  CardTypeFilterName = "MIFARE Classic 1K"
	CardTypeMifareClassic4K  CardTypeFilterName = "MIFARE Classic 4K"
	CardTypeMifareUltralight CardTypeFilterName = "MIFARE Ultralight"
	CardTypeDesfire          CardTypeFilterName = "DESFire"
	CardTypeType4            CardTypeFilterName = "Type4"
)

type Agent struct {
	Logger           *log.Logger
	Manager          nfc.Manager
	Reader           *nfc.NFCReader
	Server           *server.Server
	AllowedCardTypes CardTypeFilter
	APISecret        string
	ServerPort       int
}

func NewAgent(nfcManager nfc.Manager) *Agent {
	return &Agent{
		Logger:           log.New(os.Stderr, "[agent] ", log.LstdFlags),
		Manager:          nfcManager,
		AllowedCardTypes: CardTypeFilter{},
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

	nfcReader, err := nfc.NewNFCReader(devicePath, a.Manager, 5*time.Second)
	if err != nil {
		a.Logger.Printf("Error initializing NFC reader: %v", err)
		return err
	}

	a.Reader = nfcReader

	a.Server = server.New(server.Config{
		Reader:           a.Reader,
		Port:             a.ServerPort,
		SessionManager:   server.NewSessionManager(a.APISecret, 60*time.Second),
		AllowedCardTypes: a.allowedCardToMap(),
	})

	go a.Server.Start()
	return nil
}

func (a *Agent) Stop() {
	a.Logger.Println("Stopping agent...")

	if a.Server != nil {
		a.Server.Stop()
		a.Server = nil
	}

	if a.Reader != nil {
		a.Reader.Stop()
		a.Reader = nil
		a.Logger.Println("Agent stopped successfully")
	}
}

func (a *Agent) SetAllowCardType(cardType CardTypeFilterName, allow bool) {
	if allow {
		a.AllowCardType(cardType)
	} else {
		a.DisallowCardType(cardType)
	}
}

func (a *Agent) AllowAllCardTypes() {
	a.AllowedCardTypes.MifareClassic1K = true
	a.AllowedCardTypes.MifareClassic4K = true
	a.AllowedCardTypes.MifareUltralight = true
	a.AllowedCardTypes.Desfire = true
	a.AllowedCardTypes.Type4 = true
}

func (a *Agent) AllowedCardTypesLength() int {
	count := 0
	if a.AllowedCardTypes.MifareClassic1K {
		count++
	}
	if a.AllowedCardTypes.MifareClassic4K {
		count++
	}
	if a.AllowedCardTypes.MifareUltralight {
		count++
	}
	if a.AllowedCardTypes.Desfire {
		count++
	}
	if a.AllowedCardTypes.Type4 {
		count++
	}
	return count
}

func (a *Agent) AllowCardType(cardType CardTypeFilterName) {
	switch cardType {
	case CardTypeMifareClassic1K:
		a.AllowedCardTypes.MifareClassic1K = true
	case CardTypeMifareClassic4K:
		a.AllowedCardTypes.MifareClassic4K = true
	case CardTypeMifareUltralight:
		a.AllowedCardTypes.MifareUltralight = true
	case CardTypeDesfire:
		a.AllowedCardTypes.Desfire = true
	case CardTypeType4:
		a.AllowedCardTypes.Type4 = true
	}
}

func (a *Agent) DisallowCardType(cardType CardTypeFilterName) {
	switch cardType {
	case CardTypeMifareClassic1K:
		a.AllowedCardTypes.MifareClassic1K = false
	case CardTypeMifareClassic4K:
		a.AllowedCardTypes.MifareClassic4K = false
	case CardTypeMifareUltralight:
		a.AllowedCardTypes.MifareUltralight = false
	case CardTypeDesfire:
		a.AllowedCardTypes.Desfire = false
	case CardTypeType4:
		a.AllowedCardTypes.Type4 = false
	}
}

func (a *Agent) IsCardTypeAllowed(cardType CardTypeFilterName) bool {
	switch cardType {
	case CardTypeMifareClassic1K:
		return a.AllowedCardTypes.MifareClassic1K
	case CardTypeMifareClassic4K:
		return a.AllowedCardTypes.MifareClassic4K
	case CardTypeMifareUltralight:
		return a.AllowedCardTypes.MifareUltralight
	case CardTypeDesfire:
		return a.AllowedCardTypes.Desfire
	case CardTypeType4:
		return a.AllowedCardTypes.Type4
	default:
		return false
	}
}

func (a *Agent) allowedCardToMap() map[string]bool {
	allowed := make(map[string]bool)
	if a.AllowedCardTypes.MifareClassic1K {
		allowed[string(CardTypeMifareClassic1K)] = true
	}
	if a.AllowedCardTypes.MifareClassic4K {
		allowed[string(CardTypeMifareClassic4K)] = true
	}
	if a.AllowedCardTypes.MifareUltralight {
		allowed[string(CardTypeMifareUltralight)] = true
	}
	if a.AllowedCardTypes.Desfire {
		allowed[string(CardTypeDesfire)] = true
	}
	if a.AllowedCardTypes.Type4 {
		allowed[string(CardTypeType4)] = true
	}
	return allowed
}
