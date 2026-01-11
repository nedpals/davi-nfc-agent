#!/bin/bash
set -e

# Unified build script for all platforms
# Usage: ./scripts/build.sh [os] [arch]
#   os: linux, darwin, windows (defaults to current OS)
#   arch: amd64, arm64 (defaults to current arch)

# Parse arguments or use defaults
TARGET_OS="${1:-$(go env GOOS)}"
TARGET_ARCH="${2:-$(go env GOARCH)}"

# Build info (can be overridden via environment variables)
BUILD_VERSION="${BUILD_VERSION:-dev}"
BUILD_COMMIT="${BUILD_COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo 'unknown')}"
BUILD_TIME="${BUILD_TIME:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"

# Generate ldflags for buildinfo package
PKG="github.com/dotside-studios/davi-nfc-agent/buildinfo"
LDFLAGS="-X $PKG.Version=$BUILD_VERSION -X $PKG.Commit=$BUILD_COMMIT -X $PKG.BuildTime=$BUILD_TIME"

# Determine binary name
if [ "$TARGET_OS" = "windows" ]; then
    BINARY_NAME="davi-nfc-agent-$TARGET_OS-$TARGET_ARCH.exe"
else
    BINARY_NAME="davi-nfc-agent-$TARGET_OS-$TARGET_ARCH"
fi

echo "=== Building $BINARY_NAME ==="
echo "  Version: $BUILD_VERSION"
echo "  Commit: $BUILD_COMMIT"
echo "  Build Time: $BUILD_TIME"

GOOS=$TARGET_OS GOARCH=$TARGET_ARCH go build -ldflags="$LDFLAGS" -o "$BINARY_NAME" .

echo "✓ Built: $BINARY_NAME"
ls -lh "$BINARY_NAME"
