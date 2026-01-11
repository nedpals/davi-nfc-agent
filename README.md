# Davi NFC Agent

A lightweight NFC card reader agent with WebSocket broadcasting capabilities. Reads and writes NDEF formatted data from NFC tags and broadcasts to connected clients in real-time. This is for use for the NFC-related functionality integrated into the [Davi](https://davi.social) platform.

## Features

- **Multiple Device Support**: Hardware NFC readers and remote devices simultaneously
- **Remote NFC Devices**: Smartphones, browsers with WebNFC, or any device that can connect to the API
- **Read/Write NDEF**: Text and URI records
- **Real-time WebSocket**: Instant tag data broadcasting
- **Cross-platform**: Linux, macOS, Windows
- **System Tray UI**: Device management and status

## Supported Devices

**Hardware Readers**: ACR122U, ACR1252U, and other PC/SC-compatible readers

**Remote Devices**: Any NFC-capable device that connects via the [Device Server API](docs/api.md#device-server-api), including:
- Smartphones (iPhone 7+/iOS 13+, Android 4.4+)
- Browsers with WebNFC (Chrome on Android)
- Custom hardware or IoT devices

**Card Types**: MIFARE Classic, DESFire, Ultralight, ISO14443-4 Type 4A (experimental)

## Quick Start

Download pre-built binaries from [releases](https://github.com/dotside-studios/davi-nfc-agent/releases), or build from source:

```bash
git clone https://github.com/dotside-studios/davi-nfc-agent.git
cd davi-nfc-agent
go build .
./davi-nfc-agent
```

See the [Installation Guide](docs/installation.md) for platform-specific setup and troubleshooting.

### Command-line Options

```bash
./davi-nfc-agent                    # System tray mode (default)
./davi-nfc-agent -cli               # CLI mode
./davi-nfc-agent -client-port 8080  # Custom client port
./davi-nfc-agent -device pn532_uart:/dev/ttyUSB0  # Specific device
./davi-nfc-agent -api-secret mysecret  # API authentication
```

## Usage Examples

The agent runs two servers:
- **Device Server** (port 9470): Connects NFC readers and smartphones
- **Client Server** (port 9471): Serves client applications

### JavaScript / TypeScript

Use the included [client library](docs/javascript-client.md) for browser or Node.js applications.

```javascript
const client = new NFCClient('http://localhost:9471');

client.on('tagData', (data) => {
  console.log('Card:', data.uid, data.text);
});

await client.connect();

// Write to a card
await client.write({
  records: [{ type: 'text', content: 'Hello, NFC!' }]
});
```

### Android (Kotlin)

Connect to the Client Server via WebSocket using OkHttp or similar.

```kotlin
val client = OkHttpClient()
val request = Request.Builder()
    .url("ws://192.168.1.100:9471/ws")
    .build()

val listener = object : WebSocketListener() {
    override fun onMessage(webSocket: WebSocket, text: String) {
        val msg = JSONObject(text)
        if (msg.getString("type") == "tagData") {
            val payload = msg.getJSONObject("payload")
            Log.d("NFC", "Card UID: ${payload.getString("uid")}")
        }
    }
}

client.newWebSocket(request, listener)
```

See [API Reference](docs/api.md) for the full WebSocket protocol.

### Use Your Phone as an NFC Reader

Connect your smartphone to the Device Server using the [NFCDeviceClient](docs/javascript-client.md#nfcdeviceclient-device-input).

```javascript
const device = new NFCDeviceClient('ws://192.168.1.100:9470');

device.on('registered', ({ deviceID }) => {
  console.log('Registered as:', deviceID);
});

await device.connect();

// Start scanning with WebNFC (Chrome on Android)
if (NFCDeviceClient.isWebNFCSupported()) {
  await device.startNFCScanning();
}
```

### Raw WebSocket

Connect directly without a client library. See [API Reference](docs/api.md) for all message types.

```javascript
const ws = new WebSocket('ws://localhost:9471/ws');

ws.onmessage = (event) => {
  const msg = JSON.parse(event.data);
  if (msg.type === 'tagData') {
    console.log('Card UID:', msg.payload.uid);
  }
};

// Write request
ws.send(JSON.stringify({
  type: 'writeRequest',
  payload: {
    records: [{ type: 'text', content: 'Hello!' }]
  }
}));
```

## Extending

The agent's modular NFC layer supports adding custom readers and tag types beyond the built-in PC/SC and smartphone support. See [Extending NFC Support](docs/extending-nfc-support.md) to integrate your own hardware or protocols.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup, cross-compilation, and guidelines.

## License

[MIT License](LICENSE)

<hr />

Copyright © 2025-2026 Ned Palacios and Dotside Studios. All rights reserved.