# Extending NFC Support

This guide explains how to add support for new NFC readers or tag types to the davi-nfc-agent.

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                      MultiManager                            │
│  Aggregates multiple managers, routes device requests        │
├──────────────────┬──────────────────┬───────────────────────┤
│  HardwareManager │ SmartphoneManager│   YourManager         │
│  (libnfc/ACR122) │ (WebNFC/mobile)  │   (custom)            │
└────────┬─────────┴────────┬─────────┴──────────┬────────────┘
         │                  │                    │
         ▼                  ▼                    ▼
      Device             Device               Device
         │                  │                    │
         ▼                  ▼                    ▼
       Tag[]              Tag[]               Tag[]
```

### Core Interfaces

| Interface | Purpose |
|-----------|---------|
| `Manager` | Device discovery and connection |
| `Device` | Hardware communication |
| `Tag` | Tag operations (read/write/transceive) |

## Adding a New Device Type

### Step 1: Implement the Manager Interface

```go
package myreader

import "github.com/nedpals/davi-nfc-agent/nfc"

type MyManager struct {
    // Your connection state (USB, serial, network, etc.)
}

func NewManager() *MyManager {
    return &MyManager{}
}

// ListDevices returns available device identifiers
func (m *MyManager) ListDevices() ([]string, error) {
    // Enumerate connected devices
    // Return identifiers like "myreader:usb:001" or "myreader:192.168.1.100"
    return []string{"myreader:default"}, nil
}

// OpenDevice opens a device by its identifier
// The device should be fully initialized and ready to use when returned
func (m *MyManager) OpenDevice(deviceStr string) (nfc.Device, error) {
    // Parse deviceStr and connect to the hardware
    device := &MyDevice{
        connection: deviceStr,
    }

    // Perform any device-specific initialization here
    // The returned device should be ready to use immediately

    return device, nil
}
```

### Step 2: Implement the Device Interface

```go
type MyDevice struct {
    connection string
    // Your hardware handle (serial port, USB handle, socket, etc.)
}

func (d *MyDevice) Close() error {
    // Clean up resources
    return nil
}

func (d *MyDevice) String() string {
    return "My NFC Reader"
}

func (d *MyDevice) Connection() string {
    return d.connection
}

// DeviceType returns the device type identifier (implements DeviceInfoProvider)
func (d *MyDevice) DeviceType() string {
    return "myreader"
}

// SupportedTagTypes returns supported tag types (implements DeviceInfoProvider)
func (d *MyDevice) SupportedTagTypes() []string {
    return []string{"MIFARE Classic", "NTAG"}
}

func (d *MyDevice) Transceive(txData []byte) ([]byte, error) {
    // Send raw bytes to the reader and return response
    // This is for device-level commands, not tag communication
    return nil, nfc.NewNotSupportedError("Transceive")
}

func (d *MyDevice) GetTags() ([]nfc.Tag, error) {
    // Poll for tags on the reader
    // Return detected tags

    // Example: detect a tag and wrap it
    tagUID := "04A1B2C3D4E5F6"
    tagType := "MIFARE Classic 1K"

    tag := &MyTag{
        uid:     tagUID,
        tagType: tagType,
        device:  d,
    }

    return []nfc.Tag{tag}, nil
}
```

### Step 3: Implement the Tag Interface

```go
type MyTag struct {
    uid       string
    tagType   string
    device    *MyDevice
    connected bool
}

// --- TagIdentifier ---

func (t *MyTag) UID() string {
    return t.uid
}

func (t *MyTag) Type() string {
    return t.tagType
}

func (t *MyTag) NumericType() int {
    return 0 // Your type code
}

// --- TagCapabilityProvider (optional but recommended) ---

func (t *MyTag) Capabilities() nfc.TagCapabilities {
    return nfc.TagCapabilities{
        CanRead:       true,
        CanWrite:      true,
        CanTransceive: false,
        CanLock:       false,
        TagFamily:     "MIFARE Classic",
        Technology:    "ISO14443A",
        MemorySize:    1024,
        SupportsNDEF:  true,
    }
}

// --- TagConnection ---

func (t *MyTag) Connect() error {
    // Establish connection to tag (if needed)
    t.connected = true
    return nil
}

func (t *MyTag) Disconnect() error {
    t.connected = false
    return nil
}

// --- TagReader ---

func (t *MyTag) ReadData() ([]byte, error) {
    // Read NDEF data from the tag
    // Return raw NDEF bytes
    return nil, nil
}

// --- TagWriter ---

func (t *MyTag) WriteData(data []byte) error {
    // Write NDEF data to the tag
    return nfc.NewNotSupportedError("WriteData") // If not supported
}

// --- TagTransceiver ---

func (t *MyTag) Transceive(data []byte) ([]byte, error) {
    // Send raw command to tag and return response
    return nil, nfc.NewNotSupportedError("Transceive")
}

// --- TagLocker ---

func (t *MyTag) IsWritable() (bool, error) {
    return true, nil
}

func (t *MyTag) CanMakeReadOnly() (bool, error) {
    return false, nil
}

func (t *MyTag) MakeReadOnly() error {
    return nfc.NewNotSupportedError("MakeReadOnly")
}
```

### Step 4: Register with MultiManager

In your main.go or initialization code:

```go
import (
    "github.com/nedpals/davi-nfc-agent/nfc"
    "github.com/nedpals/davi-nfc-agent/nfc/multimanager"
    "myproject/myreader"
)

func main() {
    manager := multimanager.NewMultiManager(
        multimanager.ManagerEntry{Name: nfc.ManagerTypeHardware, Manager: nfc.NewManager()},
        multimanager.ManagerEntry{Name: "myreader", Manager: myreader.NewManager()},
    )

    // Use the manager...
}
```

## Capability-Based Implementation

You don't need to implement all methods if your device doesn't support them. Use capabilities to advertise what's supported:

```go
func (t *MyTag) Capabilities() nfc.TagCapabilities {
    return nfc.TagCapabilities{
        CanRead:       true,
        CanWrite:      false, // Read-only device
        CanTransceive: false,
        CanLock:       false,
    }
}

func (t *MyTag) WriteData(data []byte) error {
    // Return structured error for unsupported operations
    return nfc.NewNotSupportedError("WriteData")
}
```

Callers can check capabilities before calling methods:

```go
caps := nfc.GetTagCapabilities(tag)
if caps.CanWrite {
    tag.WriteData(data)
} else {
    log.Println("Tag does not support writing")
}
```

## Error Handling

Use the structured error types for consistent error handling:

```go
import "github.com/nedpals/davi-nfc-agent/nfc"

// For unsupported operations
return nfc.NewNotSupportedError("Transceive")

// For authentication failures
return nfc.NewAuthError("ReadData", tag.UID(), err)

// For read/write failures
return nfc.NewReadError("ReadData", err)
return nfc.NewWriteError("WriteData", err)

// For generic errors with context
return nfc.WrapError(nfc.ErrCodeReadFailed, "ReadSector", "failed to read sector 1", err)
```

Callers can handle errors programmatically:

```go
if nfc.IsNotSupportedError(err) {
    // Operation not supported, try alternative
}

if nfc.IsAuthError(err) {
    // Authentication failed, maybe try different key
}

code := nfc.GetErrorCode(err)
switch code {
case nfc.ErrCodeTagRemoved:
    // Tag was removed, retry
case nfc.ErrCodeReadFailed:
    // Read failed, handle error
}
```

## Optional: Server Integration

If your device needs WebSocket handlers (like smartphone NFC):

```go
import "github.com/nedpals/davi-nfc-agent/server"

// Implement server.ServerHandler
func (m *MyManager) Register(s server.HandlerServer) {
    s.HandleMessage("myreader:scan", m.handleScan)
}

// Implement server.ServerHandlerCloser for cleanup
func (m *MyManager) Close() {
    // Cleanup resources
}
```

## Testing Your Implementation

Create mock implementations for testing:

```go
func TestMyDevice(t *testing.T) {
    device := &MyDevice{connection: "test"}

    // Test capabilities
    caps := device.Capabilities()
    if !caps.CanPoll {
        t.Error("Expected CanPoll to be true")
    }

    // Test GetTags
    tags, err := device.GetTags()
    if err != nil {
        t.Errorf("GetTags failed: %v", err)
    }

    // Test tag capabilities
    for _, tag := range tags {
        tagCaps := nfc.GetTagCapabilities(tag)
        if !tagCaps.CanRead {
            t.Error("Expected tag to support reading")
        }
    }
}
```

## Examples

### Read-Only Network Device

A device that receives tag data over the network (read-only):

```go
type NetworkTag struct {
    uid     string
    tagType string
    data    []byte // Pre-loaded data
}

func (t *NetworkTag) Capabilities() nfc.TagCapabilities {
    return nfc.TagCapabilities{
        CanRead:       true,
        CanWrite:      false,
        CanTransceive: false,
        CanLock:       false,
        TagFamily:     t.tagType,
    }
}

func (t *NetworkTag) ReadData() ([]byte, error) {
    return t.data, nil
}

func (t *NetworkTag) WriteData(data []byte) error {
    return nfc.NewNotSupportedError("WriteData")
}
```

### Serial PN532 Reader

A device connected via serial port:

```go
type PN532Device struct {
    port   io.ReadWriteCloser
    conn   string
}

func (d *PN532Device) DeviceType() string {
    return "pn532-serial"
}

func (d *PN532Device) SupportedTagTypes() []string {
    return []string{"MIFARE Classic", "NTAG", "ISO14443-4"}
}

func (d *PN532Device) GetTags() ([]nfc.Tag, error) {
    // Send InListPassiveTarget command
    cmd := []byte{0xD4, 0x4A, 0x01, 0x00}
    resp, err := d.sendCommand(cmd)
    if err != nil {
        return nil, err
    }

    // Parse response and create tags
    // ...
}
```

## Interface Reference

### Required Methods

| Method | Interface | Required |
|--------|-----------|----------|
| `UID()` | TagIdentifier | Yes |
| `Type()` | TagIdentifier | Yes |
| `NumericType()` | TagIdentifier | Yes |
| `Connect()` | TagConnection | Yes (can be no-op) |
| `Disconnect()` | TagConnection | Yes (can be no-op) |
| `ReadData()` | TagReader | Yes |
| `WriteData()` | TagWriter | Yes (can return error) |
| `Transceive()` | TagTransceiver | Yes (can return error) |
| `IsWritable()` | TagLocker | Yes |
| `CanMakeReadOnly()` | TagLocker | Yes |
| `MakeReadOnly()` | TagLocker | Yes (can return error) |

### Optional Methods

| Method | Interface | Purpose |
|--------|-----------|---------|
| `Capabilities()` | TagCapabilityProvider | Runtime tag capability discovery |
| `DeviceType()` | DeviceInfoProvider | Device type identifier ("libnfc", "smartphone") |
| `SupportedTagTypes()` | DeviceInfoProvider | List of supported tag types |
| `SupportsEvents()` | DeviceEventEmitter | Whether device emits tag events |
| `IsHealthy()` | DeviceHealthChecker | Connection health validation |
| `Register()` | server.ServerHandler | WebSocket integration |
| `Close()` | server.ServerHandlerCloser | Cleanup on shutdown |

### DeviceInfoProvider Interface

Implement this interface to provide device metadata. Capabilities are built automatically from this:

```go
func (d *MyDevice) DeviceType() string {
    return "myreader"
}

func (d *MyDevice) SupportedTagTypes() []string {
    return []string{"MIFARE Classic", "NTAG"}
}
```

### DeviceEventEmitter Interface

For event-based devices (like smartphones) that receive tags via events rather than polling:

```go
func (d *MyDevice) SupportsEvents() bool {
    return true  // Tags arrive as events, not via polling
}
```

When `SupportsEvents()` returns true, `BuildDeviceCapabilities()` will automatically set:
- `CanPoll: false`
- `CanTransceive: false`
- `SupportsEvents: true`

### DeviceHealthChecker Interface

For devices that support connection health checking:

```go
func (d *MyDevice) IsHealthy() error {
    if !d.isConnected {
        return fmt.Errorf("device not connected")
    }
    return nil
}
```

The `DeviceManager` uses this interface to check device health before operations.
