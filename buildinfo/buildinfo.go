// Package buildinfo contains application metadata that can be set at build time.
//
// For release builds, use ldflags to set the version:
//
//	go build -ldflags "-X github.com/nedpals/davi-nfc-agent/buildinfo.Version=1.0.0"
//
// Or set multiple values:
//
//	go build -ldflags "\
//	  -X github.com/nedpals/davi-nfc-agent/buildinfo.Version=1.0.0 \
//	  -X github.com/nedpals/davi-nfc-agent/buildinfo.Commit=$(git rev-parse --short HEAD) \
//	  -X github.com/nedpals/davi-nfc-agent/buildinfo.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
package buildinfo

import (
	"fmt"
	"runtime"
)

// Application metadata - can be overridden at build time via ldflags
var (
	// Name is the technical application name
	Name = "davi-nfc-agent"

	// ConfigDirName is the name of the config directory within user config paths
	DirName = "davi-nfc-agent"

	// DisplayName is the user-friendly name (used for UI, mDNS, titles)
	DisplayName = "Davi NFC Agent"

	// Description is a short description of the application
	Description = "NFC card reader agent with WebSocket broadcasting"

	// Version is the semantic version (set via ldflags for releases)
	Version = "dev"

	// Commit is the git commit hash (set via ldflags)
	Commit = ""

	// BuildTime is the build timestamp (set via ldflags)
	BuildTime = ""
)

// FullVersion returns the version string with optional commit info.
// Examples:
//   - "dev" (development build)
//   - "1.0.0" (release build)
//   - "1.0.0 (abc1234)" (release build with commit)
func FullVersion() string {
	if Commit != "" {
		return fmt.Sprintf("%s (%s)", Version, Commit)
	}
	return Version
}

// UserAgent returns a user agent string for HTTP requests.
// Example: "davi-nfc-agent/1.0.0"
func UserAgent() string {
	return fmt.Sprintf("%s/%s", Name, Version)
}

// BuildInfo returns a multi-line string with full build information.
func BuildInfo() string {
	info := fmt.Sprintf("%s %s\n", Name, FullVersion())
	info += fmt.Sprintf("  %s\n", Description)
	info += fmt.Sprintf("  Go: %s\n", runtime.Version())
	info += fmt.Sprintf("  OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH)
	if BuildTime != "" {
		info += fmt.Sprintf("\n  Built: %s", BuildTime)
	}
	return info
}

// IsDev returns true if this is a development build.
func IsDev() bool {
	return Version == "dev"
}
