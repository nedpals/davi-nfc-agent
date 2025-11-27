# Mobile App Integration Protocol

This document describes the WebSocket protocol for integrating mobile apps (iOS/Android) as NFC scanner devices with the davi-nfc-agent server.

## Overview

Mobile apps with native NFC support can register as NFC scanner devices and report scanned tags to the server. The server aggregates tags from both hardware NFC readers and smartphone devices, making them available to connected clients.

**Plug and Play**: No authentication or API secrets required. Simply connect and start scanning for a seamless integration experience.

## Auto-Discovery

The NFC agent automatically broadcasts its presence on the local network using **mDNS/Bonjour**, allowing mobile apps to discover and connect without manual configuration.

### Service Information

- **Service Type**: `_nfc-agent._tcp`
- **Service Name**: `DAVI NFC Agent`  
- **TXT Records**: `version`, `protocol`, `path`, `device_mode`

### iOS Discovery (NWBrowser)

```swift
import Network

let browser = NWBrowser(for: .bonjour(type: "_nfc-agent._tcp", domain: "local."), using: .tcp)
browser.browseResultsChangedHandler = { results, changes in
    for result in results {
        // Resolve to get host:port, then connect WebSocket
    }
}
browser.start(queue: .main)
```

### Android Discovery (NSD)

```kotlin
val nsdManager = context.getSystemService(Context.NSD_SERVICE) as NsdManager
nsdManager.discoverServices("_nfc-agent._tcp", NsdManager.PROTOCOL_DNS_SD, discoveryListener)
// Resolve found services to get host:port
```

See full implementation examples in the detailed documentation below.

## Manual Connection

If auto-discovery is unavailable, connect directly:

### WebSocket URL

```
ws://server:port/ws?mode=device
```

**Query Parameters:**
- `mode=device` - Required. Identifies this as a device connection (not a client)

**Alternative**: Set header `X-Device-Mode: true` instead of query parameter

## Message Format

All messages are JSON with the following structure:

```json
{
  "id": "unique-request-id",  // Optional for requests, echoed in responses
  "type": "messageType",
  "payload": { }
}
```

## Device Registration Flow

### 1. Register Device (Mobile → Server)

**Type:** `registerDevice`

**Payload:**
```json
{
  "deviceName": "John's iPhone 12 Pro",
  "platform": "ios",  // "ios" or "android"
  "appVersion": "1.0.0",
  "capabilities": {
    "canRead": true,
    "canWrite": true,  // Optional, for future use
    "nfcType": "isodep"  // NFC technology type
  },
  "metadata": {
    "osVersion": "iOS 16.0",
    "model": "iPhone14,2"
  }
}
```

**Response:** `registerDeviceResponse`

```json
{
  "id": "unique-request-id",
  "type": "registerDeviceResponse",
  "success": true,
  "payload": {
    "deviceID": "abc-123-def-456",  // Unique device identifier (UUID)
    "sessionToken": "",  // Reserved for future use
    "serverInfo": {
      "version": "1.0.0",
      "supportedNFC": ["mifare", "desfire", "type4", "ultralight"]
    }
  }
}
```

**Error Response:**
```json
{
  "id": "unique-request-id",
  "type": "error",
  "success": false,
  "error": "Error message",
  "payload": {
    "code": "ERROR_CODE"
  }
}
```

### 2. Send Tag Scan (Mobile → Server)

**Type:** `tagScanned`

**Payload:**
```json
{
  "deviceID": "abc-123-def-456",
  "uid": "04:AB:CD:EF:12:34:56",
  "technology": "ISO14443A",
  "type": "MIFARE Classic 1K",
  "scannedAt": "2025-11-27T10:30:00Z",
  "ndefMessage": {
    "records": [
      {
        "tnf": 1,
        "type": "VGV4dA==",  // base64("Text")
        "payload": "SGVsbG8gV29ybGQ=",  // base64 encoded
        "recordType": "text",
        "content": "Hello World",
        "language": "en"
      }
    ]
  },
  "rawData": "..."  // base64 encoded raw tag data (optional)
}
```

**Fields:**
- `deviceID` - Device identifier from registration
- `uid` - Tag UID in format "XX:XX:XX:XX..." (hex, colon-separated)
- `technology` - Tag technology: "ISO14443A", "ISO14443B", "ISO15693", etc.
- `type` - Human-readable tag type
- `scannedAt` - ISO 8601 timestamp
- `ndefMessage` - Optional NDEF message data
- `rawData` - Optional raw bytes from tag

### 3. Send Tag Removed (Mobile → Server)

**Type:** `tagRemoved`

Send this message when the NFC tag leaves the device's field.

**Payload:**
```json
{
  "deviceID": "abc-123-def-456",
  "uid": "04:AB:CD:EF:12:34:56",
  "removedAt": "2025-11-27T10:30:05Z"
}
```

**Fields:**
- `deviceID` - Device identifier from registration
- `uid` - Tag UID that was removed (hex, colon-separated)
- `removedAt` - ISO 8601 timestamp of removal

**No response required**

### 4. Heartbeat (Mobile → Server)

Send every 10 seconds to keep the device session alive.

**Type:** `deviceHeartbeat`

**Payload:**
```json
{
  "deviceID": "abc-123-def-456",
  "timestamp": "2025-11-27T10:30:00Z"
}
```

**No response required**

## Platform-Specific Implementation

### iOS (Core NFC)

#### Setup

1. Add `Near Field Communication Tag Reading` capability in Xcode
2. Add `NFCReaderUsageDescription` to Info.plist
3. Add NFC tag types to Info.plist:

```xml
<key>com.apple.developer.nfc.readersession.formats</key>
<array>
    <string>NDEF</string>
    <string>TAG</string>
</array>
```

#### Reading Tags

```swift
import CoreNFC

class NFCManager: NSObject, NFCNDEFReaderSessionDelegate {
    var session: NFCNDEFReaderSession?
    
    func startScanning() {
        session = NFCNDEFReaderSession(delegate: self, queue: nil, invalidateAfterFirstRead: false)
        session?.begin()
    }
    
    func readerSession(_ session: NFCNDEFReaderSession, didDetectNDEFs messages: [NFCNDEFMessage]) {
        for message in messages {
            // Convert to protocol format and send via WebSocket
            let tagData = convertToSmartphoneTagData(message)
            sendTagData(tagData)
        }
    }
    
    func readerSession(_ session: NFCNDEFReaderSession, didDetect tags: [NFCNDEFTag]) {
        guard let tag = tags.first else { return }
        
        session.connect(to: tag) { error in
            if error != nil { return }
            
            tag.readNDEF { message, error in
                if let message = message {
                    // Get UID
                    let uid = self.formatUID(tag.identifier)
                    
                    // Send tag data
                    let tagData = SmartphoneTagData(
                        deviceID: self.deviceID,
                        uid: uid,
                        technology: "ISO14443A",
                        type: self.getTagType(tag),
                        scannedAt: Date(),
                        ndefMessage: self.convertNDEFMessage(message)
                    )
                    
                    self.sendTagData(tagData)
                }
            }
        }
    }
}
```

#### UID Formatting

```swift
func formatUID(_ data: Data) -> String {
    return data.map { String(format: "%02X", $0) }.joined(separator: ":")
}
```

### Android

#### Setup

1. Add permission to AndroidManifest.xml:

```xml
<uses-permission android:name="android.permission.NFC" />
<uses-feature android:name="android.hardware.nfc" android:required="true" />
```

2. Add NFC intent filter:

```xml
<intent-filter>
    <action android:name="android.nfc.action.NDEF_DISCOVERED"/>
    <category android:name="android.intent.category.DEFAULT"/>
</intent-filter>
```

#### Reading Tags

```kotlin
class NFCManager(private val context: Context) {
    private var nfcAdapter: NfcAdapter? = null
    
    init {
        nfcAdapter = NfcAdapter.getDefaultAdapter(context)
    }
    
    fun enableForegroundDispatch(activity: Activity) {
        val intent = Intent(activity, activity.javaClass).apply {
            addFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP)
        }
        val pendingIntent = PendingIntent.getActivity(activity, 0, intent, PendingIntent.FLAG_MUTABLE)
        
        val filters = arrayOf(IntentFilter(NfcAdapter.ACTION_NDEF_DISCOVERED))
        
        nfcAdapter?.enableForegroundDispatch(activity, pendingIntent, filters, null)
    }
    
    fun handleIntent(intent: Intent) {
        when (intent.action) {
            NfcAdapter.ACTION_NDEF_DISCOVERED -> {
                val tag = intent.getParcelableExtra<Tag>(NfcAdapter.EXTRA_TAG)
                tag?.let { processTag(it) }
            }
        }
    }
    
    private fun processTag(tag: Tag) {
        val ndef = Ndef.get(tag)
        val uid = tag.id.toHexString(":")
        
        ndef?.connect()
        val ndefMessage = ndef?.ndefMessage
        ndef?.close()
        
        val tagData = SmartphoneTagData(
            deviceID = deviceID,
            uid = uid,
            technology = getTechnology(tag),
            type = getTagType(tag),
            scannedAt = Date(),
            ndefMessage = convertNDEFMessage(ndefMessage)
        )
        
        sendTagData(tagData)
    }
    
    private fun ByteArray.toHexString(separator: String = ""): String {
        return joinToString(separator) { "%02X".format(it) }
    }
    
    private fun getTechnology(tag: Tag): String {
        return when {
            tag.techList.contains("android.nfc.tech.IsoDep") -> "ISO14443A"
            tag.techList.contains("android.nfc.tech.NfcA") -> "ISO14443A"
            tag.techList.contains("android.nfc.tech.NfcB") -> "ISO14443B"
            tag.techList.contains("android.nfc.tech.NfcV") -> "ISO15693"
            else -> "Unknown"
        }
    }
}
```

## Error Codes

| Code | Description |
|------|-------------|
| `READ_ERROR` | Failed to read WebSocket message |
| `PARSE_ERROR` | Invalid JSON format |
| `INVALID_MESSAGE_TYPE` | Unknown or unexpected message type |
| `INVALID_PAYLOAD` | Invalid message payload structure |
| `INVALID_REQUEST` | Missing required fields or invalid values |
| `REGISTRATION_FAILED` | Device registration failed |
| `INVALID_DEVICE` | Device not registered or inactive |
| `TAG_SEND_FAILED` | Failed to process tag data |
| `UNKNOWN_TYPE` | Unknown message type |

## Best Practices

### Connection Management

1. **Automatic Reconnection**: Implement exponential backoff for reconnections
2. **Heartbeat**: Send heartbeat every 10 seconds
3. **Connection Health**: Monitor WebSocket connection state
4. **Graceful Shutdown**: Close WebSocket cleanly on app termination

### Tag Scanning

1. **UID Normalization**: Always format UIDs as colon-separated uppercase hex
2. **Timestamp**: Use ISO 8601 format for timestamps
3. **NDEF Parsing**: Include parsed NDEF data when available
4. **Error Handling**: Handle tag read errors gracefully

### Security

1. **Validation**: Validate server responses
2. **Timeout**: Implement request timeouts
3. **Rate Limiting**: Avoid flooding server with tags

### Testing

1. Test with various NFC tag types (MIFARE, DESFire, Type4, Ultralight)
2. Test connection loss and reconnection
3. Test with poor network conditions
4. Test rapid successive scans
5. Verify UID format correctness

## Example WebSocket Client (JavaScript/React Native)

```javascript
class NFCDeviceClient {
  constructor(serverUrl) {
    this.serverUrl = serverUrl;
    this.ws = null;
    this.deviceID = null;
    this.heartbeatInterval = null;
  }
  
  connect() {
    const url = `${this.serverUrl}?mode=device`;
    this.ws = new WebSocket(url);
    
    this.ws.onopen = () => {
      console.log('Connected to server');
      this.registerDevice();
    };
    
    this.ws.onmessage = (event) => {
      const message = JSON.parse(event.data);
      this.handleMessage(message);
    };
    
    this.ws.onerror = (error) => {
      console.error('WebSocket error:', error);
    };
    
    this.ws.onclose = () => {
      console.log('Disconnected from server');
      this.stopHeartbeat();
      // Implement reconnection logic
    };
  }
  
  registerDevice() {
    this.send({
      id: this.generateRequestID(),
      type: 'registerDevice',
      payload: {
        deviceName: 'My Phone',
        platform: 'ios',
        appVersion: '1.0.0',
        capabilities: {
          canRead: true,
          canWrite: false,
          nfcType: 'isodep'
        }
      }
    });
  }
  
  handleMessage(message) {
    if (message.type === 'registerDeviceResponse' && message.success) {
      this.deviceID = message.payload.deviceID;
      this.startHeartbeat();
    } else if (message.type === 'error') {
      console.error('Error:', message.error);
    }
  }
  
  sendTagData(tagData) {
    if (!this.deviceID) {
      console.error('Device not registered');
      return;
    }
    
    this.send({
      type: 'tagScanned',
      payload: {
        ...tagData,
        deviceID: this.deviceID
      }
    });
  }
  
  startHeartbeat() {
    this.heartbeatInterval = setInterval(() => {
      this.send({
        type: 'deviceHeartbeat',
        payload: {
          deviceID: this.deviceID,
          timestamp: new Date().toISOString()
        }
      });
    }, 10000);
  }
  
  stopHeartbeat() {
    if (this.heartbeatInterval) {
      clearInterval(this.heartbeatInterval);
      this.heartbeatInterval = null;
    }
  }
  
  send(message) {
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(message));
    }
  }
  
  generateRequestID() {
    return `req_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`;
  }
}
```

## Troubleshooting

### Device Not Registering

- Verify WebSocket URL and query parameters
- Ensure `mode=device` query parameter is set
- Check server logs for rejection reason
- Verify network connectivity to server

### Tags Not Appearing in Clients

- Verify device is registered (check `registerDeviceResponse`)
- Ensure heartbeat is being sent every 10 seconds
- Check tag UID format is correct (colon-separated hex)
- Verify device hasn't timed out (30s inactivity)

### Connection Drops

- Implement heartbeat mechanism
- Add automatic reconnection with exponential backoff
- Handle network transitions (WiFi → cellular)
- Monitor connection state actively

### NDEF Parsing Issues

- Verify TNF values are correct (0-7)
- Ensure base64 encoding for binary data
- Check record type matches TNF
- Validate payload format

## Support

For issues or questions:
- GitHub Issues: [davi-nfc-agent issues](https://github.com/nedpals/davi-nfc-agent/issues)
- Documentation: [README.md](../README.md)
