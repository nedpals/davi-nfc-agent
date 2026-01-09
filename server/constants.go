package server

// mDNS service discovery constants for legacy single-server mode
const (
	MDNSServiceType = "_nfc-agent._tcp"
	MDNSServiceName = "DAVI NFC Agent"
	MDNSDomain      = "local."
)

// mDNS service discovery constants for input server
const (
	MDNSInputServiceType = "_nfc-input._tcp"
	MDNSInputServiceName = "DAVI NFC Input"
)

// WebSocket message types for client-server communication
const (
	WSMessageTypeTagData       = "tagData"
	WSMessageTypeDeviceStatus  = "deviceStatus"
	WSMessageTypeWriteRequest  = "writeRequest"
	WSMessageTypeWriteResponse = "writeResponse"
	WSMessageTypeError         = "error"
)

// CORS configuration
const (
	CORSAllowOrigin  = "*"
	CORSAllowMethods = "GET, POST, OPTIONS"
	CORSAllowHeaders = "Content-Type, Authorization"
)
