// Package main provides an NFC card reader agent with WebSocket broadcasting capabilities.
// It supports reading NDEF formatted text from Mifare Classic tags and broadcasts the data
// to connected WebSocket clients.
package main

import (
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"fyne.io/systray"

	"github.com/nedpals/davi-nfc-agent/nfc"
	"github.com/nedpals/davi-nfc-agent/nfc/phonenfc"
)

const DEFAULT_PORT = 18080

var (
	// CLI flags
	devicePathFlag string
	portFlag       int
	apiSecretFlag  string
)

func main() {
	// Command line flags
	flag.StringVar(&devicePathFlag, "device", "", "Path to NFC device (optional)")
	flag.IntVar(&portFlag, "port", DEFAULT_PORT, "Port to listen on for the web interface")
	flag.StringVar(&apiSecretFlag, "api-secret", "", "API secret for session handshake (optional)")
	flag.Parse()

	// Initialize smartphone manager
	smartphoneManager := phonenfc.NewManager(30 * time.Second)

	// Create multi-manager combining hardware and smartphone
	manager := nfc.NewMultiManager(
		nfc.ManagerEntry{Name: nfc.ManagerTypeHardware, Manager: nfc.NewManager()},
		nfc.ManagerEntry{Name: nfc.ManagerTypeSmartphone, Manager: smartphoneManager},
	)

	// Create agent with explicit smartphone manager for dependency injection
	agent := NewAgent(manager, smartphoneManager)
	agent.ServerPort = portFlag
	agent.APISecret = apiSecretFlag

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		systray.Quit()
	}()

	// Create and run systray app
	app := NewSystrayApp(agent, devicePathFlag)
	app.Run()
}
