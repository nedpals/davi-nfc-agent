# DAVI NFC Agent

A lightweight NFC card reader agent with WebSocket broadcasting capabilities. This agent reads NDEF formatted text from NFC tags and broadcasts the data to connected WebSocket clients in real-time.

## Features

- Read NDEF formatted text from NFC tags
- Write text to NFC tags via WebSocket API
- Real-time WebSocket broadcasting of tag data
- Automatic device reconnection and error recovery
- Cross-platform support (Linux, macOS, Windows)
- Detailed device status reporting

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
- `-api-secret`: API secret for session authentication (optional, recommended for untrusted environments)

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

## WebSocket API

### Session Management

The agent uses a session-based authentication system to prevent unauthorized access and session hijacking.

#### Acquiring a Session

Before connecting to the WebSocket, you must first acquire a session token via the `/handshake` endpoint:

```javascript
// Acquire session token
const response = await fetch('http://localhost:18080/handshake', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    secret: 'your-api-secret' // Optional, only required if -api-secret flag is set
  })
});

const { token } = await response.json();
```

**Session Features:**
- **Cryptographically secure tokens**: Sessions use 256-bit random tokens
- **Origin binding**: Sessions are bound to the origin that acquired them
- **IP binding**: Sessions are bound to the client's IP address
- **Auto-timeout**: Sessions expire after 60 seconds of inactivity
- **Single session limit**: Only one active session at a time

**Session Lifecycle:**
1. Client calls `/handshake` to acquire a session token
2. Token is bound to the client's origin and IP address
3. Client uses token to connect to WebSocket and make requests
4. Session automatically refreshes on activity
5. Session expires after 60 seconds of inactivity or when WebSocket disconnects

#### Using the Session Token

Pass the token when connecting to the WebSocket:

```javascript
// Connect with session token (via query parameter)
const socket = new WebSocket(`ws://localhost:18080/ws?token=${token}`);

// Or via header (if your WebSocket client supports it)
const socket = new WebSocket('ws://localhost:18080/ws', {
  headers: { 'X-Session-Token': token }
});
```

#### Releasing a Session

Sessions are automatically released when:
- The WebSocket connection closes
- The session times out (60 seconds of inactivity)
- A client explicitly releases via WebSocket message:

```javascript
socket.send(JSON.stringify({
  type: 'release'
}));
```

#### API Secret Protection

For additional security in untrusted environments, use the `-api-secret` flag:

```bash
./davi-nfc-agent -api-secret "your-secret-here"
```

When an API secret is configured:
- All handshake requests must include the correct secret
- Prevents unauthorized session acquisition

**Example with API secret:**
```bash
# Start agent with API secret
./davi-nfc-agent -api-secret "mySecretKey123"
```

```javascript
// Client must provide the secret
const response = await fetch('http://localhost:18080/handshake', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ secret: 'mySecretKey123' })
});
```

#### Complete Example (JavaScript)

```javascript
// Full session workflow
async function connectToNFC() {
  // 1. Acquire session
  const handshake = await fetch('http://localhost:18080/handshake', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ secret: 'mySecretKey123' }) // Optional
  });

  if (!handshake.ok) {
    throw new Error('Session already claimed or invalid secret');
  }

  const { token } = await handshake.json();

  // 2. Connect to WebSocket with token
  const socket = new WebSocket(`ws://localhost:18080/ws?token=${token}`);

  socket.onmessage = (event) => {
    const message = JSON.parse(event.data);
    console.log('Received:', message);
  };

  // 3. Session auto-releases on disconnect
  socket.onclose = () => {
    console.log('Session released');
  };

  return socket;
}
```

#### CLI Example (curl)

```bash
# 1. Acquire session token
TOKEN=$(curl -X POST http://localhost:18080/handshake \
  -H "Content-Type: application/json" \
  -d '{"secret":"mySecretKey123"}' \
  | jq -r '.token')

echo "Session token: $TOKEN"

# 2. Connect to WebSocket with token (using websocat or similar)
websocat "ws://localhost:18080/ws?token=$TOKEN"

# Or use with any WebSocket client that supports query parameters
```

### Connecting to the WebSocket

After acquiring a session token via `/handshake`, connect to the WebSocket:

```javascript
const socket = new WebSocket(`ws://localhost:18080/ws?token=${token}`);
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

### Messages to server

1. **Write Request**

The write API is designed to **prevent accidental data loss for cards with existing NDEF data**.

**Simple write** (for blank cards):
```json
{
  "type": "writeRequest",
  "payload": {
    "text": "Hello, NFC!"
  }
}
```

**Append new record** (safe - for cards with data):
```json
{
  "type": "writeRequest",
  "payload": {
    "text": "Additional info",
    "append": true
  }
}
```

**Update specific record** (safe - for cards with data):
```json
{
  "type": "writeRequest",
  "payload": {
    "text": "Updated text",
    "recordIndex": 0
  }
}
```

**Replace entire message** (explicit - for cards with data):
```json
{
  "type": "writeRequest",
  "payload": {
    "text": "New message",
    "replace": true
  }
}
```

**Payload Fields:**
- `text` (required): Text or URI content to write
- `append` (optional): Adds a new record without overwriting existing data
- `recordIndex` (optional): Index of record to update (0-based)
- `replace` (optional): Replaces entire NDEF message ⚠️ **DESTRUCTIVE**
- `recordType` (optional): Record type - `"text"` (default) or `"uri"`
- `language` (optional): ISO language code for text records (default: `"en"`)

**🛡️ Smart Safety Enforcement:**
- **Blank cards**: Simple `{"text": "..."}` works fine
- **Cards with existing NDEF**: Must specify `append`, `recordIndex`, or `replace` to prevent accidental overwrites

**Examples:**

**Write to blank card** (simple):
```json
{"type": "writeRequest", "payload": {"text": "Hello!"}}
```

**Append to card with data** (safe):
```json
{"type": "writeRequest", "payload": {"text": "Additional info", "append": true}}
```

**Update record on card with data** (safe):
```json
{"type": "writeRequest", "payload": {"text": "Updated", "recordIndex": 0}}
```

**Replace card with data** (explicit):
```json
{"type": "writeRequest", "payload": {"text": "New content", "replace": true}}
```

**Notes:**
- Blank/factory cards allow simple writes without operation mode
- Cards with existing NDEF require `append`, `recordIndex`, or `replace`
- `append` and `recordIndex` preserve existing records
- Supports all card types (Classic, DESFire, Ultralight)

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
