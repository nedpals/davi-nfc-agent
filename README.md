# DAVI NFC Agent

A lightweight NFC card reader agent with WebSocket broadcasting capabilities. This agent reads NDEF formatted text from NFC tags and broadcasts the data to connected WebSocket clients in real-time.

## Features

- Read NDEF formatted text from NFC tags
- Write text to NFC tags via WebSocket API
- Real-time WebSocket broadcasting of tag data
- Automatic device reconnection and error recovery
- Cross-platform support (Linux, macOS, Windows)
- Detailed device status reporting

## Limitations

- Currently only supports Mifare Classic 1K and 4K tags
- Other NFC tag types (NTAG, DESFire, Ultralight, etc.) are not supported
- Reading and writing is limited to NDEF text records only

## Requirements

- libnfc (v1.8.0 or later)
- libfreefare (v0.4.0 or later)
- libusb (v1.0 or later)

## Installation

### Installing Dependencies

#### Linux (Debian/Ubuntu)

For Debian-based distributions, it's recommended to install dependencies from the package manager:

```bash
# Install libnfc and its dependencies
sudo apt update
sudo apt install -y libnfc-dev libfreefare-dev libusb-1.0-0-dev
```

This is the preferred method as it ensures compatibility with your system's libraries and avoids compilation issues.

#### Building Dependencies from Source

If your package manager has outdated versions or you need specific features, you can build the dependencies from source:

```bash
# Download and build libnfc
wget https://github.com/nfc-tools/libnfc/releases/download/libnfc-1.8.0/libnfc-1.8.0.tar.bz2
tar xjf libnfc-1.8.0.tar.bz2
cd libnfc-1.8.0
autoreconf -vis
./configure --prefix=/usr --sysconfdir=/etc
make
sudo make install
cd ..

# Download and build libfreefare
wget https://github.com/nfc-tools/libfreefare/releases/download/libfreefare-0.4.0/libfreefare-0.4.0.tar.bz2
tar xjf libfreefare-0.4.0.tar.bz2
cd libfreefare-0.4.0
autoreconf -vis
./configure --prefix=/usr --sysconfdir=/etc
make
sudo make install
cd ..
```

#### Other Systems

The project includes a helper script to install libusb, which is required for libnfc:

```bash
# Install libusb dependency
chmod +x scripts/install-libusb.sh
./scripts/install-libusb.sh
```

This script automatically detects your operating system and installs libusb using the appropriate package manager (apt, yum, dnf, pacman, or brew).

#### Windows Dependencies

For Windows, install the dependencies using the following steps:

```bash
# Install required tools using Chocolatey
choco install mingw
choco install msys2

# Launch MSYS2 and install dependencies
pacman -S mingw-w64-x86_64-toolchain autoconf automake libtool mingw-w64-x86_64-libusb
```

### Building from source

```bash
# Clone the repository
git clone https://github.com/nedpals/davi-nfc-agent.git
cd davi-nfc-agent

# Build the project
go build -o davi-nfc-agent .
```

### Pre-built binaries

Pre-built binaries for various platforms are available in the [releases](https://github.com/nedpals/davi-nfc-agent/releases) section.

## Usage

```bash
# Start the agent with default settings
./davi-nfc-agent

# Specify a particular NFC device
./davi-nfc-agent -device pn532_uart:/dev/ttyUSB0

# Change the WebSocket server port
./davi-nfc-agent -port 8080
```

### Command-line options

- `-device`: Path to NFC device (optional, autodetects if not specified)
- `-port`: Port to listen on for WebSocket connections (default: 18080)

## WebSocket API

### Connecting to the WebSocket

```javascript
const socket = new WebSocket('ws://localhost:18080/ws');
```

### Messages from server

1. **Device Status**
```json
{
  "type": "deviceStatus",
  "payload": {
    "connected": true,
    "message": "Device connected",
    "cardPresent": false
  }
}
```

2. **Tag Data**
```json
{
  "type": "tagData",
  "payload": {
    "uid": "a1b2c3d4",
    "text": "Hello, NFC!",
    "err": null
  }
}
```

### Messages to server

1. **Write Request**
```json
{
  "type": "writeRequest",
  "payload": {
    "text": "Hello, NFC!"
  }
}
```

2. **Write Response**
```json
{
  "type": "writeResponse",
  "payload": {
    "success": true,
    "error": null
  }
}
```

## Supported NFC Readers

This application supports any NFC reader compatible with libnfc, including:

- ACR122U
- PN532-based readers
- SCL3711

## Building for Different Platforms

The GitHub Actions workflow builds the application for multiple platforms:
- Linux (amd64, arm64)
- macOS (amd64, arm64)
- Windows (amd64, arm64)

To build manually for a specific platform:

```bash
# For Linux
GOOS=linux GOARCH=amd64 CGO_ENABLED=1 go build -o davi-nfc-agent-linux-amd64 .

# For macOS
GOOS=darwin GOARCH=amd64 CGO_ENABLED=1 go build -o davi-nfc-agent-darwin-amd64 .

# For Windows
GOOS=windows GOARCH=amd64 CGO_ENABLED=1 go build -o davi-nfc-agent-windows-amd64.exe .
```

## Troubleshooting

### Common Issues

1. **"No NFC devices found"**
   - Ensure your NFC reader is properly connected
   - Check that libnfc is installed and configured correctly
   - Verify user permissions for accessing USB/serial devices

2. **"Failed to connect to device"**  
   - Try unplugging and reconnecting your NFC reader
   - Restart the application
   - Check for conflicting applications using the NFC reader

## Development

Requirements:
- Go 1.21 or later
- libnfc development libraries
- libfreefare development libraries

## License

[MIT License](LICENSE)
