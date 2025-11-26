package nfc

import (
	"fmt"
	"log"
	"strings"
	"sync"
)

// MultiManager aggregates multiple Manager implementations.
type MultiManager struct {
	managers     map[string]Manager // managerName -> Manager instance
	managerOrder []string           // Ordered list of manager names (for fallback)
	mu           sync.RWMutex       // Protects managers map
}

// ManagerEntry represents a named manager for MultiManager initialization.
type ManagerEntry struct {
	Name    string
	Manager Manager
}

// NewMultiManager creates a new MultiManager with the given managers.
// Managers are tried in the order they are provided.
//
// Example:
//
//	mm := nfc.NewMultiManager(
//	    nfc.ManagerEntry{Name: "hardware", Manager: hardwareManager},
//	    nfc.ManagerEntry{Name: "smartphone", Manager: smartphoneManager},
//	)
func NewMultiManager(entries ...ManagerEntry) *MultiManager {
	mm := &MultiManager{
		managers:     make(map[string]Manager),
		managerOrder: []string{},
	}

	for _, entry := range entries {
		if entry.Name == "" || entry.Manager == nil {
			log.Printf("[multi] Skipping invalid manager entry: name=%s, manager=%v", entry.Name, entry.Manager)
			continue
		}

		if _, exists := mm.managers[entry.Name]; exists {
			log.Printf("[multi] Skipping duplicate manager: %s", entry.Name)
			continue
		}

		mm.managers[entry.Name] = entry.Manager
		mm.managerOrder = append(mm.managerOrder, entry.Name)
		log.Printf("[multi] Manager registered: %s", entry.Name)
	}

	return mm
}

// AddManager adds a manager with the given name (for dynamic registration).
// Managers are tried in the order they are added.
func (mm *MultiManager) AddManager(name string, manager Manager) error {
	if name == "" {
		return fmt.Errorf("manager name cannot be empty")
	}
	if manager == nil {
		return fmt.Errorf("manager cannot be nil")
	}

	mm.mu.Lock()
	defer mm.mu.Unlock()

	// Check if manager already exists
	if _, exists := mm.managers[name]; exists {
		return fmt.Errorf("manager with name '%s' already exists", name)
	}

	mm.managers[name] = manager
	mm.managerOrder = append(mm.managerOrder, name)

	log.Printf("[multi] Manager added: %s", name)

	return nil
}

// RemoveManager removes a manager by name.
func (mm *MultiManager) RemoveManager(name string) error {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	if _, exists := mm.managers[name]; !exists {
		return fmt.Errorf("manager not found: %s", name)
	}

	delete(mm.managers, name)

	// Remove from order list
	for i, n := range mm.managerOrder {
		if n == name {
			mm.managerOrder = append(mm.managerOrder[:i], mm.managerOrder[i+1:]...)
			break
		}
	}

	log.Printf("[multi] Manager removed: %s", name)

	return nil
}

// GetManager retrieves a specific manager by name.
func (mm *MultiManager) GetManager(name string) (Manager, bool) {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	manager, exists := mm.managers[name]
	return manager, exists
}

// OpenDevice opens a device using the appropriate manager.
// Device string format:
//   - "manager:deviceID" - explicit manager (e.g., "smartphone:abc123", "hardware:pn532")
//   - "deviceID" or "" - try all managers in order
func (mm *MultiManager) OpenDevice(deviceStr string) (Device, error) {
	mm.mu.RLock()
	managers := make(map[string]Manager)
	for k, v := range mm.managers {
		managers[k] = v
	}
	order := make([]string, len(mm.managerOrder))
	copy(order, mm.managerOrder)
	mm.mu.RUnlock()

	if len(managers) == 0 {
		return nil, fmt.Errorf("no managers registered")
	}

	// Check if device string has manager prefix
	parts := strings.SplitN(deviceStr, ":", 2)
	if len(parts) == 2 {
		// Has prefix - try specific manager
		managerName := parts[0]
		deviceID := parts[1]

		manager, exists := managers[managerName]
		if !exists {
			return nil, fmt.Errorf("manager not found: %s", managerName)
		}

		device, err := manager.OpenDevice(deviceID)
		if err != nil {
			return nil, fmt.Errorf("failed to open device '%s' with manager '%s': %w", deviceID, managerName, err)
		}

		return device, nil
	}

	// No prefix or empty string - try all managers in order
	var lastErr error
	for _, name := range order {
		manager := managers[name]
		device, err := manager.OpenDevice(deviceStr)
		if err == nil {
			// Success
			return device, nil
		}
		lastErr = err
	}

	// All managers failed
	if lastErr != nil {
		return nil, fmt.Errorf("all managers failed to open device '%s': %w", deviceStr, lastErr)
	}

	return nil, fmt.Errorf("no device found: %s", deviceStr)
}

// ListDevices aggregates device lists from all managers.
// Each device is prefixed with its manager name for disambiguation.
func (mm *MultiManager) ListDevices() ([]string, error) {
	mm.mu.RLock()
	managers := make(map[string]Manager)
	for k, v := range mm.managers {
		managers[k] = v
	}
	mm.mu.RUnlock()

	var allDevices []string
	var errors []string

	for name, manager := range managers {
		devices, err := manager.ListDevices()
		if err != nil {
			// Log warning but continue with other managers
			errMsg := fmt.Sprintf("manager '%s' failed to list devices: %v", name, err)
			log.Printf("[multi] %s", errMsg)
			errors = append(errors, errMsg)
			continue
		}

		// Prepend manager name to each device (if not already prefixed)
		for _, device := range devices {
			if !strings.Contains(device, ":") {
				// No prefix - add manager name
				allDevices = append(allDevices, fmt.Sprintf("%s:%s", name, device))
			} else {
				// Already prefixed (or contains colon) - keep as-is
				allDevices = append(allDevices, device)
			}
		}
	}

	// Return combined list (even if some managers had errors)
	return allDevices, nil
}

// GetManagerCount returns the number of registered managers.
func (mm *MultiManager) GetManagerCount() int {
	mm.mu.RLock()
	defer mm.mu.RUnlock()
	return len(mm.managers)
}

// GetManagerNames returns the names of all registered managers in order.
func (mm *MultiManager) GetManagerNames() []string {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	names := make([]string, len(mm.managerOrder))
	copy(names, mm.managerOrder)
	return names
}
