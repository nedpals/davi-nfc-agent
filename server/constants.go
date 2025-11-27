package server

// mDNS service discovery constants
const (
	MDNSServiceType = "_nfc-agent._tcp"
	MDNSServiceName = "DAVI NFC Agent"
	MDNSDomain      = "local."
)

// WebSocket message types for client-server communication
const (
	WSMessageTypeTagData       = "tagData"
	WSMessageTypeDeviceStatus  = "deviceStatus"
	WSMessageTypeWriteRequest  = "writeRequest"
	WSMessageTypeWriteResponse = "writeResponse"
	WSMessageTypeError         = "error"
)

// WebSocket message types for smartphone device communication
const (
	WSMessageTypeRegisterDevice         = "registerDevice"
	WSMessageTypeRegisterDeviceResponse = "registerDeviceResponse"
	WSMessageTypeTagScanned             = "tagScanned"
	WSMessageTypeDeviceHeartbeat        = "deviceHeartbeat"
)

// CORS configuration
const (
	CORSAllowOrigin  = "*"
	CORSAllowMethods = "GET, POST, OPTIONS"
	CORSAllowHeaders = "Content-Type, Authorization"
)
