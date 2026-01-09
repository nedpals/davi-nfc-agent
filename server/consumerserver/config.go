package consumerserver

// Config holds configuration for the Consumer Server.
type Config struct {
	// Port is the HTTP/WebSocket port to listen on
	Port int

	// APISecret is the optional API secret for authentication
	APISecret string
}
