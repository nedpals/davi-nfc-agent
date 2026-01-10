// Package main provides an NFC card reader agent with WebSocket broadcasting capabilities.
// It supports reading NDEF formatted text from Mifare Classic tags and broadcasts the data
// to connected WebSocket clients.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"fyne.io/systray"

	"github.com/nedpals/davi-nfc-agent/buildinfo"
	"github.com/nedpals/davi-nfc-agent/nfc"
	"github.com/nedpals/davi-nfc-agent/nfc/multimanager"
	"github.com/nedpals/davi-nfc-agent/nfc/remotenfc"
	"github.com/nedpals/davi-nfc-agent/tls"
)

const (
	DEFAULT_INPUT_PORT     = 9470
	DEFAULT_CONSUMER_PORT  = 9471
	DEFAULT_BOOTSTRAP_PORT = 9472
)

var (
	// CLI flags
	versionFlag       bool
	devicePathFlag    string
	inputPortFlag     int
	consumerPortFlag  int
	bootstrapPortFlag int
	apiSecretFlag     string
	certFileFlag      string
	keyFileFlag       string
	autoTLSFlag       bool
	configDirFlag     string
)

func main() {
	// Command line flags
	flag.BoolVar(&versionFlag, "version", false, "Print version information and exit")
	flag.StringVar(&devicePathFlag, "device", "", "Path to NFC device (optional)")
	flag.IntVar(&inputPortFlag, "input-port", DEFAULT_INPUT_PORT, "Port for input server (devices, readers)")
	flag.IntVar(&consumerPortFlag, "consumer-port", DEFAULT_CONSUMER_PORT, "Port for consumer server (web clients)")
	flag.IntVar(&bootstrapPortFlag, "bootstrap-port", DEFAULT_BOOTSTRAP_PORT, "Port for CA bootstrap server (0 to disable)")
	flag.StringVar(&apiSecretFlag, "api-secret", "", "API secret for session handshake (optional)")
	flag.StringVar(&certFileFlag, "cert", "", "Path to TLS certificate file (enables HTTPS/WSS)")
	flag.StringVar(&keyFileFlag, "key", "", "Path to TLS private key file (enables HTTPS/WSS)")
	flag.BoolVar(&autoTLSFlag, "auto-tls", true, "Automatically generate and manage TLS certificates")
	flag.StringVar(&configDirFlag, "config-dir", "", "Config directory (default: platform-specific)")
	flag.Parse()

	// Handle --version flag
	if versionFlag {
		fmt.Println(buildinfo.BuildInfo())
		os.Exit(0)
	}

	log.Printf("Starting %s %s", buildinfo.Name, buildinfo.FullVersion())

	// Initialize auto-TLS if enabled (and no manual cert/key provided)
	var tlsMgr *tls.Manager
	if autoTLSFlag && certFileFlag == "" && keyFileFlag == "" {
		configDir := configDirFlag
		if configDir == "" {
			configDir = getDefaultConfigDir()
		}

		tlsMgr = tls.NewManager(configDir)
		certFile, keyFile, err := tlsMgr.EnsureCertificates()
		if err != nil {
			log.Printf("Warning: Auto-TLS failed: %v (running without TLS)", err)
			tlsMgr = nil
		} else {
			certFileFlag = certFile
			keyFileFlag = keyFile
		}
	}

	// Start CA bootstrap server if auto-TLS is enabled
	var bootstrapServer *tls.BootstrapServer
	if tlsMgr != nil && bootstrapPortFlag > 0 {
		bootstrapServer = tls.NewBootstrapServer(tlsMgr, bootstrapPortFlag)
		if err := bootstrapServer.Start(); err != nil {
			log.Printf("Warning: Failed to start bootstrap server: %v", err)
		}
	}

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
	agent.CertFile = certFileFlag
	agent.KeyFile = keyFileFlag
	agent.TLSManager = tlsMgr // For network change watching and cert regeneration

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		if bootstrapServer != nil {
			bootstrapServer.Stop()
		}
		systray.Quit()
	}()

	// Create and run systray app
	app := NewSystrayApp(agent, devicePathFlag, bootstrapPortFlag)
	app.Run()
}

// getDefaultConfigDir returns the platform-specific config directory.
func getDefaultConfigDir() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		// Fallback to home directory
		home, _ := os.UserHomeDir()
		configDir = filepath.Join(home, ".config")
	}
	return filepath.Join(configDir, buildinfo.DirName)
}
