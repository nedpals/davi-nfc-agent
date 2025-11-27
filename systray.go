package main

import (
	"fmt"
	"log"
	"net"
	"time"

	"fyne.io/systray"

	"github.com/nedpals/davi-nfc-agent/nfc"
)

// cardTypeFilterItem holds a menu item and its associated card type
type cardTypeFilterItem struct {
	menuItem *systray.MenuItem
	cardType string
}

// getLocalIPs returns a list of local IP addresses (excluding loopback)
func getLocalIPs() []string {
	var ips []string
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ips
	}

	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				ips = append(ips, ipNet.IP.String())
			}
		}
	}
	return ips
}

// SystrayApp manages the system tray interface for the NFC agent
type SystrayApp struct {
	agent         *Agent
	currentDevice string
	initialDevice string

	// Menu items
	mStatus         *systray.MenuItem
	mServerInfo     *systray.MenuItem
	mCardUID        *systray.MenuItem
	mCardType       *systray.MenuItem
	mStart          *systray.MenuItem
	mStop           *systray.MenuItem
	mDeviceMenu     *systray.MenuItem
	deviceMenuItems map[string]*systray.MenuItem

	// Mode menu items
	mModeMenu      *systray.MenuItem
	mReadWriteMode *systray.MenuItem
	mReadMode      *systray.MenuItem
	mWriteMode     *systray.MenuItem

	// Card filter menu items
	mCardFilterMenu *systray.MenuItem
	mFilterAll      *systray.MenuItem
	cardTypeFilters map[string]*cardTypeFilterItem // Maps card type to filter item
}

// NewSystrayApp creates a new systray application
func NewSystrayApp(agent *Agent, initialDevice string) *SystrayApp {
	return &SystrayApp{
		agent:           agent,
		initialDevice:   initialDevice,
		currentDevice:   initialDevice,
		deviceMenuItems: make(map[string]*systray.MenuItem),
		cardTypeFilters: make(map[string]*cardTypeFilterItem),
	}
}

// Run starts the systray application
func (s *SystrayApp) Run() {
	systray.Run(s.onReady, s.onExit)
}

// onReady is called when the systray is ready
func (s *SystrayApp) onReady() {
	s.setupUI()
	s.autoStartAgent()
	s.startCardInfoUpdater()
	s.startEventHandler()
}

// onExit is called when the systray is exiting
func (s *SystrayApp) onExit() {
	s.agent.Stop()
}

// setupUI initializes all menu items
func (s *SystrayApp) setupUI() {
	systray.SetIcon(iconData)
	systray.SetTooltip("NFC Card Reader Agent")

	// Status section
	s.mStatus = systray.AddMenuItem("Starting...", "Agent Status")
	s.mStatus.Disable()

	s.mServerInfo = systray.AddMenuItem("Server: Not running", "Server address and port")
	s.mServerInfo.Disable()

	systray.AddSeparator()

	// Card info section
	s.mCardUID = systray.AddMenuItem("Card UID: None", "Current card UID")
	s.mCardUID.Disable()

	s.mCardType = systray.AddMenuItem("Card Type: None", "Current card type")
	s.mCardType.Disable()

	systray.AddSeparator()

	// Device management section
	s.mDeviceMenu = systray.AddMenuItem("Device", "Select NFC Device")
	mRefreshDevices := s.mDeviceMenu.AddSubMenuItem("Refresh Devices", "Refresh device list")
	s.mDeviceMenu.AddSubMenuItemCheckbox("Auto-detect", "Auto-detect device", true)

	systray.AddSeparator()

	// Mode toggle section
	s.mModeMenu = systray.AddMenuItem("Mode: Read/Write", "Change operation mode")
	s.mReadWriteMode = s.mModeMenu.AddSubMenuItemCheckbox("Read/Write Mode", "Allow both read and write", true)
	s.mReadMode = s.mModeMenu.AddSubMenuItemCheckbox("Read Only Mode", "Only allow reading", false)
	s.mWriteMode = s.mModeMenu.AddSubMenuItemCheckbox("Write Only Mode", "Only allow writing", false)

	systray.AddSeparator()

	// Card type filtering section
	s.mCardFilterMenu = systray.AddMenuItem("Card Type Filter", "Filter cards by type")
	s.mFilterAll = s.mCardFilterMenu.AddSubMenuItemCheckbox("All Types", "Allow all card types", true)

	// Create card type filter menu items for each card type
	for _, cardType := range GetAllCardTypeFilterNames() {
		displayName := GetCardTypeFilterDisplayName(cardType)
		tooltip := GetCardTypeFilterTooltip(cardType)
		menuItem := s.mCardFilterMenu.AddSubMenuItemCheckbox(displayName, tooltip, false)
		s.cardTypeFilters[cardType] = &cardTypeFilterItem{
			menuItem: menuItem,
			cardType: cardType,
		}
	}

	systray.AddSeparator()

	// Agent control section
	s.mStart = systray.AddMenuItem("Start Agent", "Start the NFC agent")
	s.mStop = systray.AddMenuItem("Stop Agent", "Stop the NFC agent")
	s.mStart.Disable() // Disable start since we're auto-starting
	s.mStop.Disable()  // Will be enabled once agent starts

	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit", "Quit the application")

	// Start the event handler with the menu items
	go s.handleMenuEvents(mRefreshDevices, mQuit)
}

// autoStartAgent starts the agent automatically
func (s *SystrayApp) autoStartAgent() {
	go func() {
		if err := s.agent.Start(s.currentDevice); err == nil {
			s.updateStatus("Running")
			s.updateServerInfo()
			s.mStop.Enable()
		} else {
			s.updateStatus("Failed to Start")
			s.mStart.Enable()
		}
		s.updateDeviceList()
	}()
}

// startCardInfoUpdater starts a goroutine to update card information
func (s *SystrayApp) startCardInfoUpdater() {
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		lastUID := ""
		lastType := ""

		for range ticker.C {
			var card *nfc.Card
			if s.agent.Server != nil {
				card = s.agent.Server.GetLastCard()
			}

			uid, cardType := s.getCardInfo(card)

			if uid != lastUID {
				s.updateCardUID(uid)
				lastUID = uid
			}

			if cardType != lastType {
				s.updateCardType(cardType)
				lastType = cardType
			}
		}
	}()
}

// startEventHandler starts the main event handling loop
func (s *SystrayApp) startEventHandler() {
	// This will be called from handleMenuEvents
}

// handleMenuEvents processes all menu click events
func (s *SystrayApp) handleMenuEvents(mRefreshDevices, mQuit *systray.MenuItem) {
	for {
		select {
		case <-s.mStart.ClickedCh:
			s.handleStartAgent()
		case <-s.mStop.ClickedCh:
			s.handleStopAgent()
		case <-mRefreshDevices.ClickedCh:
			s.updateDeviceList()
		case <-s.mReadWriteMode.ClickedCh:
			s.handleModeSwitch(nfc.ModeReadWrite, "Read/Write")
		case <-s.mReadMode.ClickedCh:
			s.handleModeSwitch(nfc.ModeReadOnly, "Read Only")
		case <-s.mWriteMode.ClickedCh:
			s.handleModeSwitch(nfc.ModeWriteOnly, "Write Only")
		case <-s.mFilterAll.ClickedCh:
			s.handleFilterAll()
		case <-mQuit.ClickedCh:
			systray.Quit()
			return
		}

		// Handle card type filter selection
		s.handleCardFilterSelection()

		// Handle device selection
		s.handleDeviceSelection()
	}
}

// handleStartAgent starts the agent
func (s *SystrayApp) handleStartAgent() {
	if err := s.agent.Start(s.currentDevice); err == nil {
		s.updateStatus("Running")
		s.updateServerInfo()
		s.mStart.Disable()
		s.mStop.Enable()
	} else {
		s.updateStatus("Failed to Start")
	}
}

// handleStopAgent stops the agent
func (s *SystrayApp) handleStopAgent() {
	s.agent.Stop()
	s.updateStatus("Stopped")
	s.mServerInfo.SetTitle("Server: Not running")
	s.mStop.Disable()
	s.mStart.Enable()
}

// handleModeSwitch switches the reader mode
func (s *SystrayApp) handleModeSwitch(mode nfc.ReaderMode, modeName string) {
	if s.agent.Reader == nil {
		return
	}

	s.agent.Reader.SetMode(mode)
	s.mModeMenu.SetTitle("Mode: " + modeName)

	// Update checkboxes
	s.mReadWriteMode.Uncheck()
	s.mReadMode.Uncheck()
	s.mWriteMode.Uncheck()

	switch mode {
	case nfc.ModeReadWrite:
		s.mReadWriteMode.Check()
	case nfc.ModeReadOnly:
		s.mReadMode.Check()
	case nfc.ModeWriteOnly:
		s.mWriteMode.Check()
	}

	log.Printf("Switched to %s mode", modeName)
}

// handleFilterAll enables all card type filters
func (s *SystrayApp) handleFilterAll() {
	s.mFilterAll.Check()

	// Uncheck all individual filters
	for _, filter := range s.cardTypeFilters {
		filter.menuItem.Uncheck()
	}

	s.agent.AllowAllCardTypes()
}

// handleCardFilterSelection processes card type filter menu selections
func (s *SystrayApp) handleCardFilterSelection() {
	for _, filter := range s.cardTypeFilters {
		select {
		case <-filter.menuItem.ClickedCh:
			s.handleCardTypeToggle(filter)
		default:
			// No click event for this filter
		}
	}
}

// handleCardTypeToggle toggles a card type filter
func (s *SystrayApp) handleCardTypeToggle(filter *cardTypeFilterItem) {
	s.mFilterAll.Uncheck()

	// Toggle the card type
	s.agent.SetAllowCardType(filter.cardType, !filter.menuItem.Checked())

	// Update menu item
	if filter.menuItem.Checked() {
		filter.menuItem.Uncheck()
	} else {
		filter.menuItem.Check()
	}

	// If no filters active, revert to All
	if s.agent.AllowedCardTypesLength() == 0 {
		s.mFilterAll.Check()
	}
}

// handleDeviceSelection processes device menu selections
func (s *SystrayApp) handleDeviceSelection() {
	for deviceName, menuItem := range s.deviceMenuItems {
		select {
		case <-menuItem.ClickedCh:
			if s.currentDevice != deviceName {
				s.switchDevice(deviceName, menuItem)
			}
		default:
			// No click event for this menu item
		}
	}
}

// switchDevice switches to a different NFC device
func (s *SystrayApp) switchDevice(deviceName string, menuItem *systray.MenuItem) {
	// Uncheck all devices
	for _, item := range s.deviceMenuItems {
		item.Uncheck()
	}

	// Check selected device
	menuItem.Check()
	s.currentDevice = deviceName

	// Restart agent with new device
	if s.agent.Reader != nil {
		s.agent.Stop()
		if err := s.agent.Start(s.currentDevice); err == nil {
			s.updateStatus("Running")
			s.updateServerInfo()
			s.mStop.Enable()
			s.mStart.Disable()
		} else {
			s.updateStatus("Failed to Start")
			s.mServerInfo.SetTitle("Server: Not running")
			s.mStart.Enable()
			s.mStop.Disable()
		}
	}
}

// updateDeviceList refreshes the list of available devices
func (s *SystrayApp) updateDeviceList() {
	// Clear existing device menu items
	for _, item := range s.deviceMenuItems {
		item.Hide()
	}
	s.deviceMenuItems = make(map[string]*systray.MenuItem)

	// Get available devices
	devices, err := s.agent.Manager.ListDevices()
	if err != nil {
		log.Printf("Error listing devices: %v", err)
		return
	}

	// Add device menu items
	for _, device := range devices {
		deviceName := device
		isChecked := (s.currentDevice == deviceName) || (s.currentDevice == "" && len(s.deviceMenuItems) == 0)
		item := s.mDeviceMenu.AddSubMenuItemCheckbox(deviceName, "Select this device", isChecked)
		s.deviceMenuItems[deviceName] = item

		if isChecked && s.currentDevice == "" {
			s.currentDevice = deviceName
		}
	}
}

// updateStatus updates the status menu item and icon
func (s *SystrayApp) updateStatus(status string) {
	s.mStatus.SetTitle(status)
	
	// Update icon based on status
	switch status {
	case "Running":
		systray.SetIcon(iconDataConnected)
	case "Failed to Start":
		systray.SetIcon(iconDataError)
	case "Stopped":
		systray.SetIcon(iconDataStopped)
	default:
		// Starting or other states
		systray.SetIcon(iconData)
	}
}

// getCardInfo extracts UID and type from a card
func (s *SystrayApp) getCardInfo(card *nfc.Card) (uid, cardType string) {
	if card != nil {
		uid = card.UID
		cardType = card.Type
	}
	return
}

// updateCardUID updates the card UID display
func (s *SystrayApp) updateCardUID(uid string) {
	if uid == "" {
		s.mCardUID.SetTitle("Card UID: None")
	} else {
		s.mCardUID.SetTitle("Card UID: " + uid)
	}
}

// updateCardType updates the card type display
func (s *SystrayApp) updateCardType(cardType string) {
	if cardType == "" {
		s.mCardType.SetTitle("Card Type: None")
	} else {
		s.mCardType.SetTitle("Card Type: " + cardType)
	}
}

// updateServerInfo updates the server address and port display
func (s *SystrayApp) updateServerInfo() {
	if s.agent.Server == nil {
		s.mServerInfo.SetTitle("Server: Not running")
		return
	}

	port := s.agent.ServerPort
	if port == 0 {
		port = DEFAULT_PORT
	}

	ips := getLocalIPs()
	if len(ips) == 0 {
		s.mServerInfo.SetTitle(fmt.Sprintf("Server: localhost:%d", port))
		return
	}

	// Show the first IP address with port
	s.mServerInfo.SetTitle(fmt.Sprintf("Server: %s:%d", ips[0], port))
}
