/**
 * NFC Device Client
 *
 * A JavaScript client for connecting to the NFC Agent InputServer as a device.
 * Supports WebNFC integration for real browser-based NFC scanning.
 *
 * @example
 * const client = new NFCDeviceClient('ws://localhost:9470', {
 *   deviceName: 'Browser NFC Device',
 *   platform: 'web'
 * });
 *
 * client.on('registered', ({ deviceID }) => {
 *   console.log('Registered as device:', deviceID);
 * });
 *
 * client.on('writeRequest', ({ requestID, ndefMessage }) => {
 *   console.log('Write request received:', requestID);
 *   // Handle write request...
 *   client.respondToWrite(requestID, true);
 * });
 *
 * await client.connect();
 *
 * // Use WebNFC if available
 * if (NFCDeviceClient.isWebNFCSupported()) {
 *   await client.startNFCScanning();
 * }
 */
class NFCDeviceClient {
  /**
   * Creates a new NFC Device client instance
   * @param {string} serverUrl - Base URL of the InputServer (e.g., 'ws://localhost:9470')
   * @param {Object} options - Configuration options
   * @param {string} [options.deviceName='Web NFC Device'] - Device name for registration
   * @param {string} [options.platform='web'] - Platform identifier
   * @param {string} [options.appVersion='1.0.0'] - Application version
   * @param {boolean} [options.canRead=true] - Device can read NFC tags
   * @param {boolean} [options.canWrite=false] - Device can write NFC tags
   * @param {boolean} [options.autoHeartbeat=true] - Automatically send heartbeats
   * @param {number} [options.heartbeatInterval=30000] - Heartbeat interval in ms
   * @param {boolean} [options.autoReconnect=true] - Automatically reconnect on disconnect
   * @param {number} [options.reconnectDelay=3000] - Delay in ms before reconnecting
   */
  constructor(serverUrl, options = {}) {
    this.serverUrl = serverUrl.replace(/\/$/, '');
    this.deviceName = options.deviceName || 'Web NFC Device';
    this.platform = options.platform || 'web';
    this.appVersion = options.appVersion || '1.0.0';
    this.canRead = options.canRead !== false;
    this.canWrite = options.canWrite || false;
    this.autoHeartbeat = options.autoHeartbeat !== false;
    this.heartbeatInterval = options.heartbeatInterval || 30000;
    this.autoReconnect = options.autoReconnect !== false;
    this.reconnectDelay = options.reconnectDelay || 3000;

    this.ws = null;
    this.deviceID = null;
    this.serverInfo = null;
    this.connected = false;
    this.intentionalDisconnect = false;
    this.reconnectAttempts = 0;
    this.maxReconnectAttempts = 10;

    // Heartbeat timer
    this._heartbeatTimer = null;

    // WebNFC state
    this._nfcReader = null;
    this._nfcAbortController = null;
    this._nfcScanning = false;
    this._lastScannedUID = null;

    // Request tracking
    this._pendingRequests = {};
    this._requestIdCounter = 0;

    // Event handlers
    this.eventHandlers = {
      registered: [],
      writeRequest: [],
      nfcReading: [],
      nfcReadingError: [],
      connected: [],
      disconnected: [],
      error: []
    };
  }

  /**
   * Check if WebNFC is supported in the current browser
   * @returns {boolean} True if WebNFC is supported
   */
  static isWebNFCSupported() {
    return 'NDEFReader' in window;
  }

  /**
   * Registers an event handler
   * @param {string} event - Event name
   * @param {Function} handler - Callback function
   */
  on(event, handler) {
    if (this.eventHandlers[event]) {
      this.eventHandlers[event].push(handler);
    } else {
      console.warn(`Unknown event type: ${event}`);
    }
  }

  /**
   * Removes an event handler
   * @param {string} event - Event name
   * @param {Function} handler - Callback function to remove
   */
  off(event, handler) {
    if (this.eventHandlers[event]) {
      this.eventHandlers[event] = this.eventHandlers[event].filter(h => h !== handler);
    }
  }

  /**
   * Emits an event to all registered handlers
   * @private
   */
  _emit(event, data) {
    if (this.eventHandlers[event]) {
      this.eventHandlers[event].forEach(handler => {
        try {
          handler(data);
        } catch (err) {
          console.error(`Error in ${event} handler:`, err);
        }
      });
    }
  }

  /**
   * Establishes WebSocket connection and registers as a device
   * @returns {Promise<void>}
   */
  async connect() {
    try {
      // Build WebSocket URL with device mode
      let wsUrl = this.serverUrl.replace(/^http/, 'ws');
      if (!wsUrl.includes('/ws')) {
        wsUrl += '/ws';
      }
      wsUrl += '?mode=device';

      this.ws = new WebSocket(wsUrl);

      this.ws.onopen = async () => {
        this.connected = true;
        this.reconnectAttempts = 0;
        this._emit('connected', {});

        // Auto-register after connection
        try {
          await this._register();
        } catch (err) {
          this._emit('error', { error: err, phase: 'registration' });
        }
      };

      this.ws.onmessage = (event) => {
        try {
          const message = JSON.parse(event.data);
          this._handleMessage(message);
        } catch (err) {
          console.error('Failed to parse WebSocket message:', err);
        }
      };

      this.ws.onerror = (error) => {
        this._emit('error', { error, phase: 'websocket' });
      };

      this.ws.onclose = () => {
        this.connected = false;
        this.deviceID = null;
        this._stopHeartbeat();
        this._emit('disconnected', {});

        if (!this.intentionalDisconnect && this.autoReconnect) {
          this._attemptReconnect();
        }
      };

      await this._waitForConnection();
    } catch (err) {
      this._emit('error', { error: err, phase: 'connection' });
      throw err;
    }
  }

  /**
   * Waits for WebSocket connection to be established
   * @private
   */
  _waitForConnection() {
    return new Promise((resolve, reject) => {
      const timeout = setTimeout(() => {
        reject(new Error('Connection timeout'));
      }, 10000);

      const checkConnection = () => {
        if (this.ws.readyState === WebSocket.OPEN) {
          clearTimeout(timeout);
          resolve();
        } else if (this.ws.readyState === WebSocket.CLOSED || this.ws.readyState === WebSocket.CLOSING) {
          clearTimeout(timeout);
          reject(new Error('Connection failed'));
        } else {
          setTimeout(checkConnection, 100);
        }
      };

      checkConnection();
    });
  }

  /**
   * Attempts to reconnect to the server
   * @private
   */
  async _attemptReconnect() {
    if (this.maxReconnectAttempts > 0 && this.reconnectAttempts >= this.maxReconnectAttempts) {
      this._emit('error', {
        error: new Error('Max reconnection attempts reached'),
        phase: 'reconnection'
      });
      return;
    }

    this.reconnectAttempts++;
    console.log(`Reconnecting... (attempt ${this.reconnectAttempts})`);

    setTimeout(async () => {
      try {
        await this.connect();
      } catch (err) {
        console.error('Reconnection failed:', err);
      }
    }, this.reconnectDelay);
  }

  /**
   * Registers the device with the server
   * @private
   */
  async _register() {
    return new Promise((resolve, reject) => {
      const requestId = this._generateRequestId();
      this._pendingRequests[requestId] = { resolve, reject };

      const timeout = setTimeout(() => {
        delete this._pendingRequests[requestId];
        reject(new Error('Registration timeout'));
      }, 10000);

      this._pendingRequests[requestId] = {
        resolve: (data) => {
          clearTimeout(timeout);
          resolve(data);
        },
        reject: (err) => {
          clearTimeout(timeout);
          reject(err);
        }
      };

      const message = {
        id: requestId,
        type: 'registerDevice',
        payload: {
          deviceName: this.deviceName,
          platform: this.platform,
          appVersion: this.appVersion,
          capabilities: {
            canRead: this.canRead,
            canWrite: this.canWrite,
            nfcType: 'webnfc'
          },
          metadata: {
            userAgent: typeof navigator !== 'undefined' ? navigator.userAgent : 'unknown'
          }
        }
      };

      this.ws.send(JSON.stringify(message));
    });
  }

  /**
   * Handles incoming WebSocket messages
   * @private
   */
  _handleMessage(message) {
    const { id, type, payload, success, error } = message;

    // Handle registration response
    if (type === 'registerDeviceResponse') {
      if (id && this._pendingRequests[id]) {
        const { resolve, reject } = this._pendingRequests[id];
        delete this._pendingRequests[id];

        if (success) {
          this.deviceID = payload.deviceID;
          this.serverInfo = payload.serverInfo;
          this._startHeartbeat();
          this._emit('registered', { deviceID: this.deviceID, serverInfo: this.serverInfo });
          resolve(payload);
        } else {
          reject(new Error(error || 'Registration failed'));
        }
      }
      return;
    }

    // Handle responses with ID
    if (id && this._pendingRequests[id]) {
      const { resolve, reject } = this._pendingRequests[id];
      delete this._pendingRequests[id];

      if (success) {
        resolve(payload);
      } else {
        reject(new Error(error || 'Request failed'));
      }
      return;
    }

    // Handle server-initiated messages
    switch (type) {
      case 'deviceWriteRequest':
        this._emit('writeRequest', {
          requestID: payload.requestID,
          deviceID: payload.deviceID,
          ndefMessage: payload.ndefMessage
        });
        break;
      case 'error':
        this._emit('error', { error: new Error(error), code: payload?.code });
        break;
      default:
        console.warn('Unknown message type:', type);
    }
  }

  /**
   * Generates a unique request ID
   * @private
   */
  _generateRequestId() {
    return `dev_${++this._requestIdCounter}_${Date.now()}`;
  }

  /**
   * Starts the heartbeat timer
   * @private
   */
  _startHeartbeat() {
    if (!this.autoHeartbeat) return;

    this._stopHeartbeat();
    this._heartbeatTimer = setInterval(() => {
      if (this.connected && this.deviceID) {
        this._sendHeartbeat();
      }
    }, this.heartbeatInterval);
  }

  /**
   * Stops the heartbeat timer
   * @private
   */
  _stopHeartbeat() {
    if (this._heartbeatTimer) {
      clearInterval(this._heartbeatTimer);
      this._heartbeatTimer = null;
    }
  }

  /**
   * Sends a heartbeat to the server
   * @private
   */
  _sendHeartbeat() {
    const message = {
      type: 'deviceHeartbeat',
      payload: {
        deviceID: this.deviceID,
        timestamp: new Date().toISOString()
      }
    };
    this.ws.send(JSON.stringify(message));
  }

  /**
   * Start WebNFC scanning. Tags detected will be automatically sent to the server.
   * @returns {Promise<void>}
   * @throws {Error} If WebNFC is not supported or permission denied
   */
  async startNFCScanning() {
    if (!NFCDeviceClient.isWebNFCSupported()) {
      throw new Error('WebNFC is not supported in this browser');
    }

    if (this._nfcScanning) {
      return; // Already scanning
    }

    try {
      this._nfcReader = new NDEFReader();
      this._nfcAbortController = new AbortController();

      this._nfcReader.onreading = (event) => {
        this._handleNFCReading(event);
      };

      this._nfcReader.onreadingerror = (error) => {
        this._emit('nfcReadingError', { error });
      };

      await this._nfcReader.scan({ signal: this._nfcAbortController.signal });
      this._nfcScanning = true;
    } catch (err) {
      this._nfcScanning = false;
      throw err;
    }
  }

  /**
   * Stop WebNFC scanning
   */
  stopNFCScanning() {
    if (this._nfcAbortController) {
      this._nfcAbortController.abort();
      this._nfcAbortController = null;
    }
    this._nfcReader = null;
    this._nfcScanning = false;
    this._lastScannedUID = null;
  }

  /**
   * Check if currently scanning with WebNFC
   * @returns {boolean}
   */
  isNFCScanning() {
    return this._nfcScanning;
  }

  /**
   * Infers card type from UID length and format
   * WebNFC doesn't expose card type directly, so we use heuristics
   * @private
   * @param {string} uid - The card UID (colon-separated hex)
   * @returns {string} Inferred card type
   */
  _inferCardType(uid) {
    if (!uid) return 'Unknown';

    // Count UID bytes (format: "XX:XX:XX:XX" or "XX-XX-XX-XX" or "XXXXXXXX")
    const cleanUID = uid.replace(/[:-]/g, '');
    const uidLength = cleanUID.length / 2; // Each byte is 2 hex chars

    // Infer based on UID length
    // See: https://www.nxp.com/docs/en/application-note/AN10927.pdf
    switch (uidLength) {
      case 4:
        // 4-byte UID: Typically Mifare Classic 1K/4K, Mifare Mini
        return 'Mifare Classic';
      case 7:
        // 7-byte UID: Mifare Ultralight, NTAG, Mifare DESFire, Mifare Plus
        // Without more info, we can't distinguish between these
        return 'Mifare Ultralight/NTAG';
      case 10:
        // 10-byte UID: Rare, some special Mifare cards
        return 'Mifare (10-byte UID)';
      default:
        return 'NDEF';
    }
  }

  /**
   * Handles WebNFC reading event
   * @private
   */
  _handleNFCReading(event) {
    const uid = event.serialNumber || '';

    // Convert NDEF message to our format
    const ndefMessage = this._convertNDEFMessage(event.message);

    // Infer card type from UID
    const cardType = this._inferCardType(uid);

    const tagData = {
      uid: uid,
      technology: 'NFC-A', // WebNFC typically works with NFC-A (ISO14443A)
      type: cardType,
      scannedAt: new Date().toISOString(),
      ndefMessage: ndefMessage,
      source: 'webnfc'
    };

    // Emit local event
    this._emit('nfcReading', tagData);

    // Send to server if connected
    if (this.connected && this.deviceID) {
      this.scanTag(tagData);
    }

    this._lastScannedUID = uid;
  }

  /**
   * Converts WebNFC NDEFMessage to protocol format
   * @private
   */
  _convertNDEFMessage(ndefMessage) {
    if (!ndefMessage || !ndefMessage.records) {
      return null;
    }

    const records = [];
    for (const record of ndefMessage.records) {
      const convertedRecord = {
        recordType: record.recordType,
        language: record.lang || 'en'
      };

      // Handle different record types
      if (record.recordType === 'text') {
        const decoder = new TextDecoder(record.encoding || 'utf-8');
        convertedRecord.content = decoder.decode(record.data);
      } else if (record.recordType === 'url') {
        convertedRecord.recordType = 'uri';
        const decoder = new TextDecoder();
        convertedRecord.content = decoder.decode(record.data);
      } else if (record.data) {
        // For other types, store raw data as base64
        const bytes = new Uint8Array(record.data);
        convertedRecord.payload = this._arrayBufferToBase64(bytes);
      }

      records.push(convertedRecord);
    }

    return {
      records: records
    };
  }

  /**
   * Convert ArrayBuffer to base64
   * @private
   */
  _arrayBufferToBase64(buffer) {
    let binary = '';
    const bytes = new Uint8Array(buffer);
    for (let i = 0; i < bytes.byteLength; i++) {
      binary += String.fromCharCode(bytes[i]);
    }
    return btoa(binary);
  }

  /**
   * Manually send a tag scan event to the server
   * @param {Object} tagData - Tag data to send
   * @param {string} tagData.uid - Tag UID
   * @param {string} [tagData.technology='ISO14443A'] - NFC technology
   * @param {string} [tagData.type='Unknown'] - Tag type
   * @param {Object} [tagData.ndefMessage] - NDEF message
   * @returns {Promise<void>}
   */
  async scanTag(tagData) {
    if (!this.connected) {
      throw new Error('Not connected to server');
    }

    if (!this.deviceID) {
      throw new Error('Not registered with server');
    }

    const message = {
      type: 'tagScanned',
      payload: {
        deviceID: this.deviceID,
        uid: tagData.uid,
        technology: tagData.technology || 'ISO14443A',
        type: tagData.type || 'Unknown',
        atr: tagData.atr || '',
        scannedAt: tagData.scannedAt || new Date().toISOString(),
        ndefMessage: tagData.ndefMessage || null,
        rawData: tagData.rawData || null
      }
    };

    this.ws.send(JSON.stringify(message));
  }

  /**
   * Send a tag removed event to the server
   * @param {string} uid - UID of the removed tag
   * @returns {Promise<void>}
   */
  async removeTag(uid) {
    if (!this.connected) {
      throw new Error('Not connected to server');
    }

    if (!this.deviceID) {
      throw new Error('Not registered with server');
    }

    const message = {
      type: 'tagRemoved',
      payload: {
        deviceID: this.deviceID,
        uid: uid,
        removedAt: new Date().toISOString()
      }
    };

    this.ws.send(JSON.stringify(message));
    this._lastScannedUID = null;
  }

  /**
   * Respond to a write request from the server
   * @param {string} requestID - The request ID from the write request
   * @param {boolean} success - Whether the write was successful
   * @param {string} [error] - Error message if unsuccessful
   */
  async respondToWrite(requestID, success, error = '') {
    if (!this.connected) {
      throw new Error('Not connected to server');
    }

    const message = {
      type: 'deviceWriteResponse',
      payload: {
        requestID: requestID,
        success: success,
        error: error
      }
    };

    this.ws.send(JSON.stringify(message));
  }

  /**
   * Get the assigned device ID
   * @returns {string|null}
   */
  getDeviceID() {
    return this.deviceID;
  }

  /**
   * Get server info received during registration
   * @returns {Object|null}
   */
  getServerInfo() {
    return this.serverInfo;
  }

  /**
   * Check if connected to the server
   * @returns {boolean}
   */
  isConnected() {
    return this.connected && this.deviceID !== null;
  }

  /**
   * Disconnect from the server
   */
  async disconnect() {
    this.intentionalDisconnect = true;

    this.stopNFCScanning();
    this._stopHeartbeat();

    if (this.ws) {
      this.ws.close();
    }

    this.connected = false;
    this.deviceID = null;
    this.ws = null;
  }
}

// Export for different module systems
if (typeof module !== 'undefined' && module.exports) {
  module.exports = NFCDeviceClient;
}
if (typeof window !== 'undefined') {
  window.NFCDeviceClient = NFCDeviceClient;
}
