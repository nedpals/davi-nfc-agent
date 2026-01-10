package server

import "github.com/nedpals/davi-nfc-agent/buildinfo"

// mDNS service discovery constants for legacy single-server mode
var (
	MDNSServiceType = "_nfc-agent._tcp"
	MDNSServiceName = buildinfo.DisplayName
	MDNSDomain      = "local."
)

// mDNS service discovery constants for input server
var (
	MDNSInputServiceType = "_nfc-input._tcp"
	MDNSInputServiceName = buildinfo.DisplayName + " Input"
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
