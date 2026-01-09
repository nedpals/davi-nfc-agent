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
	"github.com/nedpals/davi-nfc-agent/nfc/multimanager"
	"github.com/nedpals/davi-nfc-agent/nfc/remotenfc"
)

const (
	DEFAULT_INPUT_PORT    = 9470
	DEFAULT_CONSUMER_PORT = 9471
)

var (
	// CLI flags
	devicePathFlag   string
	inputPortFlag    int
	consumerPortFlag int
	apiSecretFlag    string
)

func main() {
	// Command line flags
	flag.StringVar(&devicePathFlag, "device", "", "Path to NFC device (optional)")
	flag.IntVar(&inputPortFlag, "input-port", DEFAULT_INPUT_PORT, "Port for input server (devices, readers)")
	flag.IntVar(&consumerPortFlag, "consumer-port", DEFAULT_CONSUMER_PORT, "Port for consumer server (web clients)")
	flag.StringVar(&apiSecretFlag, "api-secret", "", "API secret for session handshake (optional)")
	flag.Parse()

	// Initialize smartphone manager
	smartphoneManager := remotenfc.NewManager(30 * time.Second)

	// Create multi-manager combining hardware and smartphone
	manager := multimanager.NewMultiManager(
		multimanager.ManagerEntry{Name: nfc.ManagerTypeHardware, Manager: nfc.NewManager()},
		multimanager.ManagerEntry{Name: nfc.ManagerTypeSmartphone, Manager: smartphoneManager},
	)

	// Create agent
	agent := NewAgent(manager)
	agent.InputPort = inputPortFlag
	agent.ConsumerPort = consumerPortFlag
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
