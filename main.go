// Package main provides an NFC card reader agent with WebSocket broadcasting capabilities.
// It supports reading NDEF formatted text from Mifare Classic tags and broadcasts the data
// to connected WebSocket clients.
package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"fyne.io/systray"

	"github.com/nedpals/davi-nfc-agent/nfc"
)

const DEFAULT_PORT = 18080

var (
	// CLI flags
	devicePathFlag string
	portFlag       int
	systrayFlag    bool
	apiSecretFlag  string

	// Global state
	agent         *Agent
	currentDevice string
)

func onReady() {
	systray.SetIcon(iconData)
	systray.SetTitle("NFC Agent")
	systray.SetTooltip("NFC Card Reader Agent")

	// Status section
	mStatus := systray.AddMenuItem("Starting...", "Agent Status")
	mStatus.Disable()

	mConnection := systray.AddMenuItem("Connection: Disconnected", "Connection Status")
	mConnection.Disable()

	mMode := systray.AddMenuItem("Mode: Read/Write", "Current Mode")
	mMode.Disable()

	systray.AddSeparator()

	// Card info section
	mCardUID := systray.AddMenuItem("Card UID: None", "Current card UID")
	mCardUID.Disable()

	mCardType := systray.AddMenuItem("Card Type: None", "Current card type")
	mCardType.Disable()

	systray.AddSeparator()

	// Device management section
	mDeviceMenu := systray.AddMenuItem("Device", "Select NFC Device")
	mRefreshDevices := mDeviceMenu.AddSubMenuItem("Refresh Devices", "Refresh device list")
	mDeviceMenu.AddSubMenuItemCheckbox("Auto-detect", "Auto-detect device", true)

	systray.AddSeparator()

	// Mode toggle section
	mModeMenu := systray.AddMenuItem("Switch Mode", "Change operation mode")
	mReadWriteMode := mModeMenu.AddSubMenuItemCheckbox("Read/Write Mode", "Allow both read and write", true)
	mReadMode := mModeMenu.AddSubMenuItemCheckbox("Read Only Mode", "Only allow reading", false)
	mWriteMode := mModeMenu.AddSubMenuItemCheckbox("Write Only Mode", "Only allow writing", false)

	systray.AddSeparator()

	// Card type filtering section
	mCardFilterMenu := systray.AddMenuItem("Card Type Filter", "Filter cards by type")
	mFilterAll := mCardFilterMenu.AddSubMenuItemCheckbox("All Types", "Allow all card types", true)
	mFilterMifare := mCardFilterMenu.AddSubMenuItemCheckbox("MIFARE Classic", "Allow MIFARE Classic only", false)
	mFilterMifareUltra := mCardFilterMenu.AddSubMenuItemCheckbox("MIFARE Ultralight", "Allow MIFARE Ultralight only", false)
	mFilterDESFire := mCardFilterMenu.AddSubMenuItemCheckbox("DESFire", "Allow DESFire only", false)
	mFilterType4 := mCardFilterMenu.AddSubMenuItemCheckbox("Type 4", "Allow Type 4 only", false)

	systray.AddSeparator()

	// Agent control section
	mStart := systray.AddMenuItem("Start Agent", "Start the NFC agent")
	mStop := systray.AddMenuItem("Stop Agent", "Stop the NFC agent")
	mStart.Disable() // Disable start since we're auto-starting
	mStop.Disable()  // Will be enabled once agent starts

	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit", "Quit the application")

	// Initialize device list
	deviceMenuItems := make(map[string]*systray.MenuItem)
	updateDeviceList := func() {
		// Clear existing device menu items
		for _, item := range deviceMenuItems {
			item.Hide()
		}
		deviceMenuItems = make(map[string]*systray.MenuItem)

		// Get available devices
		devices, err := agent.Manager.ListDevices()
		if err != nil {
			log.Printf("Error listing devices: %v", err)
			return
		}

		// Add device menu items
		for _, device := range devices {
			deviceName := device
			isChecked := (currentDevice == deviceName) || (currentDevice == "" && len(deviceMenuItems) == 0)
			item := mDeviceMenu.AddSubMenuItemCheckbox(deviceName, "Select this device", isChecked)
			deviceMenuItems[deviceName] = item

			if isChecked && currentDevice == "" {
				currentDevice = deviceName
			}
		}
	}

	// Auto-start the agent
	go func() {
		if err := agent.Start(currentDevice); err == nil {
			mStatus.SetTitle("Running")
			mConnection.SetTitle("Connection: Connected")
			if currentDevice != "" {
				mConnection.SetTitle("Connection: Connected (" + currentDevice + ")")
			}
			mStop.Enable()
		} else {
			mStatus.SetTitle("Failed to Start")
			mConnection.SetTitle("Connection: Failed")
			mStart.Enable()
		}
		updateDeviceList()
	}()

	// Goroutine to update card info display
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		lastUID := ""
		lastType := ""

		for range ticker.C {
			var card *nfc.Card
			if agent.Server != nil {
				card = agent.Server.GetLastCard()
			}
			var uid, cardType string
			if card != nil {
				uid = card.UID
				cardType = card.Type
			}

			if uid != lastUID {
				if uid == "" {
					mCardUID.SetTitle("Card UID: None")
				} else {
					mCardUID.SetTitle("Card UID: " + uid)
				}
				lastUID = uid
			}

			if cardType != lastType {
				if cardType == "" {
					mCardType.SetTitle("Card Type: None")
				} else {
					mCardType.SetTitle("Card Type: " + cardType)
				}
				lastType = cardType
			}
		}
	}()

	go func() {
		for {
			select {
			case <-mStart.ClickedCh:
				if err := agent.Start(currentDevice); err == nil {
					mStatus.SetTitle("Running")
					mConnection.SetTitle("Connection: Connected")
					if currentDevice != "" {
						mConnection.SetTitle("Connection: Connected (" + currentDevice + ")")
					}
					mStart.Disable()
					mStop.Enable()
				} else {
					mStatus.SetTitle("Failed to Start")
					mConnection.SetTitle("Connection: Failed")
				}
			case <-mStop.ClickedCh:
				agent.Stop()
				mStatus.SetTitle("Stopped")
				mConnection.SetTitle("Connection: Disconnected")
				mStop.Disable()
				mStart.Enable()
			case <-mRefreshDevices.ClickedCh:
				updateDeviceList()
			case <-mReadWriteMode.ClickedCh:
				if !mReadWriteMode.Checked() && agent.Reader != nil {
					agent.Reader.SetMode(nfc.ModeReadWrite)
					mMode.SetTitle("Mode: Read/Write")
					mReadWriteMode.Check()
					mReadMode.Uncheck()
					mWriteMode.Uncheck()
					log.Println("Switched to read/write mode")
				}
			case <-mReadMode.ClickedCh:
				if !mReadMode.Checked() && agent.Reader != nil {
					agent.Reader.SetMode(nfc.ModeReadOnly)
					mMode.SetTitle("Mode: Read Only")
					mReadMode.Check()
					mReadWriteMode.Uncheck()
					mWriteMode.Uncheck()
					log.Println("Switched to read-only mode")
				}
			case <-mWriteMode.ClickedCh:
				if !mWriteMode.Checked() && agent.Reader != nil {
					agent.Reader.SetMode(nfc.ModeWriteOnly)
					mMode.SetTitle("Mode: Write Only")
					mWriteMode.Check()
					mReadWriteMode.Uncheck()
					mReadMode.Uncheck()
					log.Println("Switched to write-only mode")
				}
			case <-mFilterAll.ClickedCh:
				mFilterAll.Check()
				mFilterMifare.Uncheck()
				mFilterMifareUltra.Uncheck()
				mFilterDESFire.Uncheck()
				mFilterType4.Uncheck()
				agent.AllowAllCardTypes()
			case <-mFilterMifare.ClickedCh:
				mFilterAll.Uncheck()

				agent.SetAllowCardType(CardTypeMifareClassic1K, !mFilterMifare.Checked())
				agent.SetAllowCardType(CardTypeMifareClassic4K, !mFilterMifare.Checked())

				if mFilterMifare.Checked() {
					mFilterMifare.Uncheck()
				} else {
					mFilterMifare.Check()
				}

				// Check if no filters active, then revert to All
				if agent.AllowedCardTypesLength() == 0 {
					mFilterAll.Check()
				}
			case <-mFilterMifareUltra.ClickedCh:
				mFilterAll.Uncheck()

				agent.SetAllowCardType(CardTypeMifareUltralight, !mFilterMifareUltra.Checked())

				if mFilterMifareUltra.Checked() {
					mFilterMifareUltra.Uncheck()
				} else {
					mFilterMifareUltra.Check()
				}

				// Check if no filters active, then revert to All
				if agent.AllowedCardTypesLength() == 0 {
					mFilterAll.Check()
				}
			case <-mFilterDESFire.ClickedCh:
				mFilterAll.Uncheck()

				agent.SetAllowCardType(CardTypeDesfire, !mFilterDESFire.Checked())

				if mFilterDESFire.Checked() {
					mFilterDESFire.Uncheck()
				} else {
					mFilterDESFire.Check()
				}
				// Check if no filters active, then revert to All
				if agent.AllowedCardTypesLength() == 0 {
					mFilterAll.Check()
				}
			case <-mFilterType4.ClickedCh:
				mFilterAll.Uncheck()

				agent.SetAllowCardType(CardTypeType4, !mFilterType4.Checked())

				if mFilterType4.Checked() {
					mFilterType4.Uncheck()
				} else {
					mFilterType4.Check()
				}

				// Check if no filters active, then revert to All
				if agent.AllowedCardTypesLength() == 0 {
					mFilterAll.Check()
				}
			case <-mQuit.ClickedCh:
				systray.Quit()
				return
			}

			// Handle device selection
			for deviceName, menuItem := range deviceMenuItems {
				select {
				case <-menuItem.ClickedCh:
					if currentDevice != deviceName {
						// Uncheck all devices
						for _, item := range deviceMenuItems {
							item.Uncheck()
						}
						// Check selected device
						menuItem.Check()
						currentDevice = deviceName

						// Restart agent with new device
						if agent.Reader != nil {
							agent.Stop()
							if err := agent.Start(currentDevice); err == nil {
								mStatus.SetTitle("Running")
								mConnection.SetTitle("Connection: Connected (" + currentDevice + ")")
								mStop.Enable()
								mStart.Disable()
							} else {
								mStatus.SetTitle("Failed to Start")
								mConnection.SetTitle("Connection: Failed")
								mStart.Enable()
								mStop.Disable()
							}
						}
					}
				default:
					// No click event for this menu item
				}
			}
		}
	}()
}

func onExit() {
	agent.Stop()
}

func main() {
	// Command line flags
	flag.StringVar(&devicePathFlag, "device", "", "Path to NFC device (optional)")
	flag.IntVar(&portFlag, "port", DEFAULT_PORT, "Port to listen on for the web interface")
	flag.BoolVar(&systrayFlag, "cli", false, "Run in CLI mode (default: system tray mode)")
	flag.StringVar(&apiSecretFlag, "api-secret", "", "API secret for session handshake (optional)")
	flag.Parse()

	agent = NewAgent(nfc.NewManager())

	agent.ServerPort = portFlag
	agent.APISecret = apiSecretFlag

	// Run in CLI mode only if explicitly requested
	if systrayFlag {
		if err := agent.Start(devicePathFlag); err != nil {
			log.Fatalf("Failed to start agent: %v", err)
		}
		defer agent.Stop()

		// Set up signal handling for graceful shutdown
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

		// Wait for shutdown signal
		<-sigChan
		log.Println("Shutdown signal received, stopping server...")
	} else {
		// Default systray mode
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

		go func() {
			<-sigChan
			systray.Quit()
		}()

		systray.Run(onReady, onExit)
	}
}
