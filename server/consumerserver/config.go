package consumerserver

// Config holds configuration for the Consumer Server.
type Config struct {
	// Port is the HTTP/WebSocket port to listen on
	Port int

	// APISecret is the optional API secret for authentication
	APISecret string

	// TLS configuration (optional)
	CertFile string // Path to TLS certificate file
	KeyFile  string // Path to TLS private key file
}

// TLSEnabled returns true if TLS is configured.
func (c Config) TLSEnabled() bool {
	return c.CertFile != "" && c.KeyFile != ""
}
