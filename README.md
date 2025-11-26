# DAVI NFC Agent

A lightweight NFC card reader agent with WebSocket broadcasting capabilities. This agent reads NDEF formatted text from NFC tags and broadcasts the data to connected WebSocket clients in real-time. **Now supports smartphones as NFC readers!**

## Features

- **Multiple Device Support**: Use hardware NFC readers AND smartphones simultaneously
- **Smartphone NFC Scanning**: iOS and Android devices can act as NFC readers
- Read NDEF formatted text from NFC tags
- Write text to NFC tags via WebSocket API
- Real-time WebSocket broadcasting of tag data
- Automatic device reconnection and error recovery
- Cross-platform support (Linux, macOS, Windows)
- Detailed device status reporting

## Supported Devices

### Hardware NFC Readers
- PN532-based USB readers
- ACR122U readers
- Other libnfc-compatible devices

### Smartphone Devices
- **iOS**: iPhone 7 and later (iOS 13+) with Core NFC support
- **Android**: Devices with NFC hardware (Android 4.4+)

## Supported Card Types

- **MIFARE Classic** (1K and 4K variants)
- **MIFARE DESFire** (EV1, EV2, EV3)
- **MIFARE Ultralight** (including Ultralight C)
- **ISO14443-4 Type 4A** tags

All card types support reading and writing NDEF formatted messages, including text and URI records.

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

### Starting the server

```bash
# Start the agent with default settings
./davi-nfc-agent

# Specify a particular NFC device
./davi-nfc-agent -device pn532_uart:/dev/ttyUSB0

# Change the WebSocket server port
./davi-nfc-agent -port 8080
```

**Command-line options:**
- `-device`: Path to NFC device (optional, autodetects if not specified)
- `-port`: Port to listen on for WebSocket connections (default: 18080)
- `-cli`: Run in CLI mode instead of system tray mode (default: system tray)
- `-api-secret`: API secret for authentication (optional, recommended for untrusted environments)

### System Tray Mode

By default, the agent runs in system tray mode with a graphical interface:

- **Device Management**: Select and switch between NFC devices
- **Mode Toggle**: Switch between read and write modes
- **Card Filters**: Filter by card type (Classic, Ultralight, DESFire, Type 4)
- **Real-time Status**: View connected card UID and type

To run in CLI mode without system tray:
```bash
./davi-nfc-agent -cli
```

### Using Smartphones as NFC Readers

Smartphones with native NFC support can connect as additional NFC reading devices:

#### Quick Start

1. **Start the agent**:
   ```bash
   ./davi-nfc-agent
   ```

2. **Mobile app auto-discovers the agent** via mDNS/Bonjour (service type: `_nfc-agent._tcp`)

3. **Or connect manually** to the WebSocket endpoint:
   ```
   ws://localhost:18080/ws?mode=device
   ```

4. **Register and scan** - that's it!

#### Supported Platforms

**iOS (iPhone 7+, iOS 13+)**
- Uses Core NFC framework
- Supports NDEF and ISO14443 tag reading
- Requires app with NFC capability enabled

**Android (4.4+)**
- Uses Android NFC API
- Supports various tag technologies
- Requires NFC permission in manifest

#### Integration

See **[Mobile App Integration Protocol](docs/MOBILE_APP_PROTOCOL.md)** for detailed integration instructions including:
- WebSocket protocol specification
- Platform-specific implementation guides (iOS/Android)
- NDEF message format
- Example code snippets
- Troubleshooting guide

## API Overview

The NFC Agent provides two simple API interfaces:

1. **WebSocket API** (`/ws`) - Real-time tag scanning and write operations (primary interface)
2. **REST API** (`/api/v1/*`) - Status queries

**Simple session management:** First WebSocket connection wins (automatic lock). Disconnect to release.

---

## WebSocket API

### Connecting

Simply connect to `/ws` - first connection wins:

```javascript
const ws = new WebSocket('ws://localhost:18080/ws');
```

**With optional API secret:**
```javascript
const ws = new WebSocket('ws://localhost:18080/ws?secret=your-secret');
```

**Session behavior:**
- ✅ First connection claims the session (automatic lock)
- ✅ Session released automatically on disconnect
- ❌ Subsequent connections rejected with `409 Conflict` until first disconnects
- 🔐 Optional API secret for untrusted environments (via query param)

### Messages from Server

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

When a card is detected and read, the server broadcasts structured data:

```json
{
  "type": "tagData",
  "payload": {
    "uid": "04A1B2C3D4E5F6",
    "type": "MIFARE Classic 1K",
    "technology": "ISO14443A",
    "scannedAt": "2024-10-06T12:34:56Z",
    "message": {
      "type": "ndef",
      "records": [
        {
          "tnf": 1,
          "type": "T",
          "text": "Hello, NFC!",
          "payload": [72, 101, 108, 108, 111, 44, 32, 78, 70, 67, 33]
        }
      ]
    },
    "text": "Hello, NFC!",
    "err": null
  }
}
```

**Payload Fields:**
- `uid`: Card unique identifier (hex string)
- `type`: Card type
  - `"MIFARE Classic 1K"` / `"MIFARE Classic 4K"`
  - `"MIFARE DESFire"` (EV1/EV2/EV3)
  - `"MIFARE Ultralight"` / `"MIFARE Ultralight C"`
  - `"ISO14443-4 Type 4A"`
- `technology`: NFC technology standard (`"ISO14443A"`, `"ISO14443B"`, etc.)
- `scannedAt`: ISO 8601 timestamp when card was detected
- `message`: Structured message data (when available)
  - **For NDEF messages** (`type: "ndef"`):
    - `records`: Array of NDEF records
      - `tnf`: Type Name Format (0x01 = Well Known)
      - `type`: Record type (`"T"` = Text, `"U"` = URI)
      - `text`: Decoded text (for Text records)
      - `uri`: Decoded URI (for URI records)
      - `id`: Record ID (optional)
      - `payload`: Raw payload bytes
  - **For raw text** (`type: "raw"`):
    - `data`: Raw byte array
- `text`: Quick access to first text record (for convenience)
- `err`: Error message string if read failed, `null` on success

---

## REST API

Simple HTTP endpoints for status checks.

### Base URL
```
http://localhost:18080/api/v1
```

### Endpoints

#### Health Check (GET `/api/v1/health`)

Simple health check for monitoring.

```bash
curl http://localhost:18080/api/v1/health
```

---

### Messages to Server

All client messages should include an optional `id` field for request/response correlation.

#### 1. Write Request

**Simplified API:** Always overwrites the entire NDEF message.  
**To append:** Read current data first, modify it in-memory, then write back the complete message.

This follows the same approach as most NFC tools (NFC Tools, TagWriter, etc.).

**Write single text record:**
```json
{
  "id": "req_1_1234567890",
  "type": "writeRequest",
  "payload": {
    "records": [
      {
        "type": "text",
        "content": "Hello, NFC!",
        "language": "en"
      }
    ]
  }
}
```

**Write multiple records:**
```json
{
  "id": "req_2_1234567890",
  "type": "writeRequest",
  "payload": {
    "records": [
      {
        "type": "text",
        "content": "Hello, NFC!",
        "language": "en"
      },
      {
        "type": "uri",
        "content": "https://example.com"
      }
    ]
  }
}
```

**Append records (read-modify-write pattern):**
```javascript
// 1. Read current tag data
const currentData = await client.getLastTag();

// 2. Extract existing records
const existingRecords = currentData.message.records.map(r => ({
  type: r.type === 'T' ? 'text' : 'uri',
  content: r.text || r.uri,
  language: r.language || 'en'
}));

// 3. Write back with new record appended
socket.send(JSON.stringify({
  id: 'req_3_1234567890',
  type: 'writeRequest',
  payload: {
    records: [
      ...existingRecords,
      { type: 'text', content: 'New record' }
    ]
  }
}));
```

**Payload Fields:**
- `records` (array, required): Array of NDEF records to write
  - `type` (string, required): Record type - `"text"` or `"uri"`
  - `content` (string, required): Text or URI content
  - `language` (string, optional): ISO language code for text records (default: `"en"`)

**Notes:**
- ⚠️ **Always performs complete overwrite** - no append/update modes
- To preserve existing data, read first then write complete message
- Cleaner and more predictable than server-side merging
- Matches behavior of popular NFC tools

#### 2. Write Response

Server responds with success or error:

**Success:**
```json
{
  "id": "req_1_1234567890",
  "type": "writeResponse",
  "success": true,
  "payload": {
    "message": "Write operation completed successfully"
  }
}
```

**Error:**
```json
{
  "id": "req_1_1234567890",
  "type": "error",
  "success": false,
  "error": "Write failed: card removed",
  "payload": {
    "code": "WRITE_FAILED"
  }
}
```


---

## JavaScript Client Library

A framework-agnostic JavaScript client is provided for easy integration.

### Installation

```bash
# Copy the client files to your project
cp client/nfc-client.js your-project/
cp client/nfc-client.d.ts your-project/  # For TypeScript
```

Or include directly in HTML:

```html
<script src="nfc-client.js"></script>
```

### Quick Start

```javascript
// Create client instance
const client = new NFCClient('http://localhost:18080', {
  apiSecret: 'your-secret',  // Optional - for API secret protection
  autoReconnect: true        // Auto-reconnect on disconnect
});

// Listen for tag scans
client.on('tagData', (data) => {
  console.log('Card UID:', data.uid);
  console.log('Card Type:', data.type);
  console.log('Text:', data.text);
  console.log('Records:', data.message?.records);
});

// Listen for device status
client.on('deviceStatus', (status) => {
  console.log('Device connected:', status.connected);
});

// Connect to server (first connection wins)
await client.connect();

// Write to a card
await client.write({
  records: [
    { type: 'text', content: 'Hello, NFC!' },
    { type: 'uri', content: 'https://example.com' }
  ]
});

// Use REST API methods (no WebSocket needed)
const status = await client.getStatus();
const lastTag = await client.getLastTag();

// Disconnect when done (releases session automatically)
await client.disconnect();
```

### API Methods

**WebSocket Methods:**
- `connect()` - Connect to server (first connection wins)
- `disconnect()` - Disconnect (releases session automatically)
- `write(request)` - Write NDEF data to card
- `isConnected()` - Check connection status

**REST API Methods:**
- `healthCheck()` - Perform health check

**Events:**
- `tagData` - Tag scanned
- `deviceStatus` - Device status changed
- `connected` - WebSocket connected
- `disconnected` - WebSocket disconnected
- `error` - Error occurred

See `client/nfc-client.d.ts` for full TypeScript definitions.

---

## Complete Examples

### Example 1: Simple Tag Reader

```javascript
const client = new NFCClient('http://localhost:18080');

client.on('tagData', (data) => {
  document.getElementById('uid').textContent = data.uid;
  document.getElementById('text').textContent = data.text;
});

await client.connect();
```

### Example 2: Write to Card

```javascript
const client = new NFCClient('http://localhost:18080');

await client.connect();

// Write single text record
await client.write({
  records: [{ type: 'text', content: 'Hello, NFC!' }]
});

// Write multiple records
await client.write({
  records: [
    { type: 'text', content: 'Welcome!' },
    { type: 'uri', content: 'https://example.com' }
  ]
});
```

### Example 3: Append to Existing Data

```javascript
const client = new NFCClient('http://localhost:18080');

await client.connect();

// Wait for tag scan
client.on('tagData', async (data) => {
  if (!data.message) return;

  // Extract existing records (structure is consistent!)
  const existingRecords = data.message.records.map(r => ({
    type: r.type,
    content: r.content,
    language: r.language
  }));

  // Append new record
  await client.write({
    records: [
      ...existingRecords,
      { type: 'text', content: 'Appended record' }
    ]
  });
});
```

---

## Supported NFC Readers

This application supports any NFC reader compatible with libnfc, including:

- ACR122U
- PN532-based readers
- SCL3711

## Building for Different Platforms

### Quick Start

The project includes build scripts that handle all dependencies and cross-compilation:

```bash
# Build for your current platform (auto-detected)
./scripts/build-unix.sh

# Cross-compile for Linux
./scripts/build-unix.sh linux amd64
./scripts/build-unix.sh linux arm64

# Cross-compile for macOS
./scripts/build-unix.sh darwin amd64
./scripts/build-unix.sh darwin arm64

# Cross-compile for Windows (from Linux)
./scripts/build-windows.sh amd64
```

### Prerequisites

For cross-compilation, you'll need:
- **Go 1.21+**
- **Zig 0.11.0** (for cross-compilation)
- **autotools**: autoconf, automake, libtool
- **pkg-config**
- **wget**

**Install Zig:**
```bash
# macOS
brew install zig

# Linux
wget https://ziglang.org/download/0.11.0/zig-linux-x86_64-0.11.0.tar.xz
tar xf zig-linux-x86_64-0.11.0.tar.xz
sudo mv zig-linux-x86_64-0.11.0 /usr/local/zig
export PATH="/usr/local/zig:$PATH"
```

### What the Scripts Do

The build scripts automatically:
1. Download and compile all C dependencies (libusb, libnfc, libfreefare, OpenSSL)
2. Apply platform-specific patches
3. Configure cross-compilation toolchains
4. Build the Go binary with proper CGO flags
5. Install dependencies to `~/cross-build/[os]-[arch]/`

### Platform Support

| Platform | Script | Architectures |
|----------|--------|---------------|
| Linux | `build-unix.sh` | amd64, arm64 |
| macOS | `build-unix.sh` | amd64, arm64 |
| Windows | `build-windows.sh` | amd64 |

### Examples

**Build for current platform:**
```bash
./scripts/build-unix.sh
# Output: davi-nfc-agent-darwin-arm64 (or linux-amd64, etc.)
```

**Cross-compile from macOS to Linux ARM64:**
```bash
./scripts/build-unix.sh linux arm64
# Takes ~10-15 minutes (first time, then cached)
# Output: davi-nfc-agent-linux-arm64
```

**Build all platforms:**
```bash
# macOS/Linux builds
for os in linux darwin; do
  for arch in amd64 arm64; do
    ./scripts/build-unix.sh $os $arch
  done
done

# Windows (from Linux)
./scripts/build-windows.sh amd64
```

### Build Artifacts

- **Binary**: Created in current directory
  - `davi-nfc-agent-linux-amd64`
  - `davi-nfc-agent-darwin-arm64`
  - `davi-nfc-agent-windows-amd64.exe`

- **Dependencies**: Cached in `~/cross-build/`
  - `~/cross-build/linux-amd64/lib/` (reusable)
  - `~/cross-build/darwin-arm64/lib/`
  - `~/cross-build/windows-amd64/lib/`

### CI/CD

The GitHub Actions workflow (`.github/workflows/build-v2.yml`) uses these scripts to automatically build for all platforms and create releases on pushes to master.

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

## Architecture

The agent consists of three main components:

- **NFC Layer**: Modular abstraction over libnfc/libfreefare for reading and writing NFC tags
  - See [nfc/README.md](nfc/README.md) for detailed technical documentation
- **WebSocket Server**: Real-time broadcasting of tag data to connected clients
- **System Tray**: Optional GUI for device and mode management

## Development

Requirements:
- Go 1.21 or later
- libnfc development libraries
- libfreefare development libraries

## License

[MIT License](LICENSE)
