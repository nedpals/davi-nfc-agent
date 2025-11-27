package phonenfc

import "time"

// Device timing constants
const (
	DeviceTimeout     = 30 * time.Second        // Device inactivity timeout
	HeartbeatInterval = 10 * time.Second        // Expected heartbeat frequency
	TagChannelBuffer  = 10                      // Tag channel buffer size
	GetTagsTimeout    = 500 * time.Millisecond  // GetTags blocking timeout
	CleanupInterval   = 15 * time.Second        // Cleanup check interval
)

// WebSocket message types for smartphone device communication
const (
	MessageTypeRegisterDevice         = "registerDevice"
	MessageTypeRegisterDeviceResponse = "registerDeviceResponse"
	MessageTypeTagScanned             = "tagScanned"
	MessageTypeTagRemoved             = "tagRemoved"
	MessageTypeDeviceHeartbeat        = "deviceHeartbeat"
	MessageTypeWriteResponse          = "writeResponse"
	MessageTypeError                  = "error"
)
