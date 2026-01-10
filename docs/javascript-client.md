# JavaScript Client Library

A framework-agnostic JavaScript client for integrating with the NFC Agent.

## Installation

Copy the client files to your project:

```bash
cp client/nfc-client.js your-project/
cp client/nfc-client.d.ts your-project/  # For TypeScript
```

Or include directly in HTML:

```html
<script src="nfc-client.js"></script>
```

## Quick Start

```javascript
// Create client instance
const client = new NFCClient('http://localhost:9471', {
  apiSecret: 'your-secret',  // Optional
  autoReconnect: true        // Auto-reconnect on disconnect
});

// Listen for tag scans
client.on('tagData', (data) => {
  console.log('Card UID:', data.uid);
  console.log('Card Type:', data.type);
  console.log('Text:', data.text);
});

// Listen for device status
client.on('deviceStatus', (status) => {
  console.log('Device connected:', status.connected);
});

// Connect to server
await client.connect();

// Write to a card
await client.write({
  records: [
    { type: 'text', content: 'Hello, NFC!' }
  ]
});

// Disconnect when done
await client.disconnect();
```

## API Reference

### Constructor

```javascript
new NFCClient(serverUrl, options?)
```

| Parameter | Type | Description |
|-----------|------|-------------|
| `serverUrl` | string | Base URL of the NFC Agent server |
| `options.apiSecret` | string | Optional API secret for authentication |
| `options.autoReconnect` | boolean | Auto-reconnect on disconnect (default: true) |

### Methods

#### `connect()`

Connect to the WebSocket server. First connection wins.

```javascript
await client.connect();
```

#### `disconnect()`

Disconnect from the server. Releases session automatically.

```javascript
await client.disconnect();
```

#### `write(request)`

Write NDEF data to a card.

```javascript
await client.write({
  records: [
    { type: 'text', content: 'Hello!', language: 'en' },
    { type: 'uri', content: 'https://example.com' }
  ]
});
```

#### `isConnected()`

Check if WebSocket is connected.

```javascript
if (client.isConnected()) {
  // ...
}
```

#### `healthCheck()`

Perform REST API health check.

```javascript
const health = await client.healthCheck();
```

### Events

| Event | Payload | Description |
|-------|---------|-------------|
| `tagData` | Tag data object | Tag was scanned |
| `deviceStatus` | Status object | Device status changed |
| `connected` | - | WebSocket connected |
| `disconnected` | - | WebSocket disconnected |
| `error` | Error object | Error occurred |

```javascript
client.on('tagData', (data) => { /* ... */ });
client.on('deviceStatus', (status) => { /* ... */ });
client.on('connected', () => { /* ... */ });
client.on('disconnected', () => { /* ... */ });
client.on('error', (err) => { /* ... */ });
```

## Examples

### Simple Tag Reader

```javascript
const client = new NFCClient('http://localhost:9471');

client.on('tagData', (data) => {
  document.getElementById('uid').textContent = data.uid;
  document.getElementById('text').textContent = data.text;
});

await client.connect();
```

### Write to Card

```javascript
const client = new NFCClient('http://localhost:9471');
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

### Append to Existing Data

```javascript
const client = new NFCClient('http://localhost:9471');
await client.connect();

client.on('tagData', async (data) => {
  if (!data.message) return;

  // Extract existing records
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

### With Error Handling

```javascript
const client = new NFCClient('http://localhost:9471');

client.on('error', (err) => {
  console.error('NFC Error:', err);
});

client.on('disconnected', () => {
  console.log('Disconnected - will auto-reconnect');
});

try {
  await client.connect();
  await client.write({
    records: [{ type: 'text', content: 'Hello!' }]
  });
} catch (err) {
  console.error('Failed:', err);
}
```

## TypeScript Support

TypeScript definitions are provided in `nfc-client.d.ts`. Import types:

```typescript
import { NFCClient, TagData, DeviceStatus, WriteRequest } from './nfc-client';

const client = new NFCClient('http://localhost:9471');

client.on('tagData', (data: TagData) => {
  console.log(data.uid);
});
```

See `client/nfc-client.d.ts` for full type definitions.

---

# NFCDeviceClient (Device Input)

Use `NFCDeviceClient` to connect to the **Device Server** (port 9470) as an NFC device. This allows browsers with WebNFC support to act as NFC readers.

## Installation

```bash
cp client/nfc-device-client.js your-project/
cp client/nfc-device-client.d.ts your-project/  # For TypeScript
```

Or include directly in HTML:

```html
<script src="nfc-device-client.js"></script>
```

## Quick Start

```javascript
const client = new NFCDeviceClient('ws://localhost:9470', {
  deviceName: 'Browser NFC Reader',
  platform: 'web'
});

// Listen for registration
client.on('registered', ({ deviceID }) => {
  console.log('Registered as device:', deviceID);
});

// Listen for write requests from server
client.on('writeRequest', ({ requestID, ndefMessage }) => {
  console.log('Write request:', ndefMessage);
  // Handle write, then respond
  client.respondToWrite(requestID, true);
});

// Connect to server
await client.connect();

// Start WebNFC scanning (if supported)
if (NFCDeviceClient.isWebNFCSupported()) {
  await client.startNFCScanning();
}
```

## API Reference

### Constructor

```javascript
new NFCDeviceClient(serverUrl, options?)
```

| Parameter | Type | Description |
|-----------|------|-------------|
| `serverUrl` | string | Device Server URL (e.g., `ws://localhost:9470`) |
| `options.deviceName` | string | Device name for registration (default: `'Web NFC Device'`) |
| `options.platform` | string | Platform identifier (default: `'web'`) |
| `options.appVersion` | string | App version (default: `'1.0.0'`) |
| `options.canRead` | boolean | Device can read tags (default: `true`) |
| `options.canWrite` | boolean | Device can write tags (default: `false`) |
| `options.autoHeartbeat` | boolean | Send heartbeats automatically (default: `true`) |
| `options.heartbeatInterval` | number | Heartbeat interval in ms (default: `30000`) |
| `options.autoReconnect` | boolean | Auto-reconnect on disconnect (default: `true`) |

### Static Methods

#### `NFCDeviceClient.isWebNFCSupported()`

Check if WebNFC is available in the current browser.

```javascript
if (NFCDeviceClient.isWebNFCSupported()) {
  // Can use startNFCScanning()
}
```

### Methods

#### `connect()`

Connect to the Device Server and register as a device.

```javascript
await client.connect();
```

#### `disconnect()`

Disconnect from the server.

```javascript
await client.disconnect();
```

#### `startNFCScanning()`

Start WebNFC scanning. Detected tags are automatically sent to the server.

```javascript
await client.startNFCScanning();
```

#### `stopNFCScanning()`

Stop WebNFC scanning.

```javascript
client.stopNFCScanning();
```

#### `isNFCScanning()`

Check if currently scanning.

```javascript
if (client.isNFCScanning()) {
  // Currently scanning
}
```

#### `scanTag(tagData)`

Manually send a tag scan event (for non-WebNFC sources).

```javascript
await client.scanTag({
  uid: '04A1B2C3D4E5F6',
  type: 'MIFARE Classic',
  ndefMessage: { records: [...] }
});
```

#### `removeTag(uid)`

Notify server that a tag was removed.

```javascript
await client.removeTag('04A1B2C3D4E5F6');
```

#### `respondToWrite(requestID, success, error?)`

Respond to a write request from the server.

```javascript
client.respondToWrite(requestID, true);
// or on failure:
client.respondToWrite(requestID, false, 'Write failed');
```

#### `getDeviceID()`

Get assigned device ID after registration.

```javascript
const deviceID = client.getDeviceID();
```

#### `isConnected()`

Check if connected and registered.

```javascript
if (client.isConnected()) {
  // Ready to send/receive
}
```

### Events

| Event | Payload | Description |
|-------|---------|-------------|
| `registered` | `{ deviceID, serverInfo }` | Successfully registered with server |
| `writeRequest` | `{ requestID, deviceID, ndefMessage }` | Server requests a write operation |
| `nfcReading` | Tag data object | WebNFC detected a tag |
| `nfcReadingError` | `{ error }` | WebNFC reading error |
| `connected` | - | WebSocket connected |
| `disconnected` | - | WebSocket disconnected |
| `error` | `{ error, phase }` | Error occurred |

```javascript
client.on('registered', ({ deviceID }) => { /* ... */ });
client.on('writeRequest', ({ requestID, ndefMessage }) => { /* ... */ });
client.on('nfcReading', (tagData) => { /* ... */ });
client.on('error', ({ error, phase }) => { /* ... */ });
```

## Examples

### WebNFC Browser Reader

```javascript
const client = new NFCDeviceClient('ws://localhost:9470', {
  deviceName: 'Chrome NFC Reader',
  canWrite: true
});

client.on('registered', ({ deviceID }) => {
  console.log('Device ID:', deviceID);
  document.getElementById('status').textContent = 'Connected';
});

client.on('nfcReading', (tagData) => {
  console.log('Scanned:', tagData.uid);
  document.getElementById('lastTag').textContent = tagData.uid;
});

client.on('writeRequest', async ({ requestID, ndefMessage }) => {
  try {
    // Write using WebNFC
    const writer = new NDEFReader();
    await writer.write(ndefMessage);
    client.respondToWrite(requestID, true);
  } catch (err) {
    client.respondToWrite(requestID, false, err.message);
  }
});

await client.connect();

if (NFCDeviceClient.isWebNFCSupported()) {
  await client.startNFCScanning();
} else {
  alert('WebNFC not supported in this browser');
}
```

### Manual Tag Input (No WebNFC)

```javascript
const client = new NFCDeviceClient('ws://localhost:9470', {
  deviceName: 'Manual Input Device'
});

await client.connect();

// Simulate tag scan from other source
document.getElementById('scanBtn').onclick = async () => {
  const uid = document.getElementById('uidInput').value;
  await client.scanTag({
    uid: uid,
    type: 'Manual',
    scannedAt: new Date().toISOString()
  });
};
```

## TypeScript Support

TypeScript definitions are provided in `nfc-device-client.d.ts`:

```typescript
import { NFCDeviceClient, DeviceTagData, WriteRequestEvent } from './nfc-device-client';

const client = new NFCDeviceClient('ws://localhost:9470');

client.on('writeRequest', (event: WriteRequestEvent) => {
  console.log(event.requestID);
});
```

See `client/nfc-device-client.d.ts` for full type definitions.
