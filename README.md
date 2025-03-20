# DAVI NFC Agent

A lightweight NFC card reader agent with WebSocket broadcasting capabilities. This agent reads NDEF formatted text from NFC tags and broadcasts the data to connected WebSocket clients in real-time.

## Features

- Read NDEF formatted text from NFC tags
- Write text to NFC tags via WebSocket API
- Real-time WebSocket broadcasting of tag data
- Automatic device reconnection and error recovery
- Cross-platform support (Linux, macOS)
- Detailed device status reporting

## Limitations

- Currently only supports Mifare Classic 1K and 4K tags
- Other NFC tag types (NTAG, DESFire, Ultralight, etc.) are not supported
- Reading and writing is limited to NDEF text records only

## Requirements

- libnfc (v1.8.0 or later)
- libfreefare (v0.4.0 or later)

## Installation

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

To build manually for a specific platform:

```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=1 go build -o davi-nfc-agent-linux-amd64 .
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
