/**
 * NFC Agent Client
 *
 * A framework-agnostic JavaScript client for connecting to the NFC Agent server.
 * Supports session management, WebSocket communication, and NFC card operations.
 *
 * @example
 * const client = new NFCClient('http://localhost:18080');
 *
 * client.on('tagData', (data) => {
 *   console.log('Card detected:', data.uid, data.text);
 * });
 *
 * client.on('deviceStatus', (status) => {
 *   console.log('Device connected:', status.connected);
 * });
 *
 * await client.connect();
 */
class NFCClient {
  /**
   * Creates a new NFC client instance
   * @param {string} serverUrl - Base URL of the NFC Agent server (e.g., 'http://localhost:18080')
   * @param {Object} options - Configuration options
   * @param {string} [options.apiSecret] - Optional API secret for authentication
   * @param {boolean} [options.autoReconnect=true] - Automatically reconnect on disconnect
   * @param {number} [options.reconnectDelay=3000] - Delay in ms before reconnecting
   * @param {number} [options.maxReconnectAttempts=10] - Maximum reconnection attempts (0 = infinite)
   */
  constructor(serverUrl, options = {}) {
    this.serverUrl = serverUrl.replace(/\/$/, ''); // Remove trailing slash
    this.apiSecret = options.apiSecret || '';
    this.autoReconnect = options.autoReconnect !== false;
    this.reconnectDelay = options.reconnectDelay || 3000;
    this.maxReconnectAttempts = options.maxReconnectAttempts || 10;

    this.ws = null;
    this.sessionToken = null;
    this.connected = false;
    this.reconnectAttempts = 0;
    this.intentionalDisconnect = false;

    // Event handlers
    this.eventHandlers = {
      tagData: [],
      deviceStatus: [],
      connected: [],
      disconnected: [],
      error: []
    };
  }

  /**
   * Registers an event handler
   * @param {string} event - Event name ('tagData', 'deviceStatus', 'connected', 'disconnected', 'error')
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
   * Performs session handshake with the server
   * @returns {Promise<string>} Session token
   * @throws {Error} If handshake fails
   */
  async handshake() {
    const response = await fetch(`${this.serverUrl}/handshake`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({
        secret: this.apiSecret
      })
    });

    if (!response.ok) {
      const error = await response.json().catch(() => ({}));
      throw new Error(error.error || `Handshake failed: ${response.statusText}`);
    }

    const data = await response.json();
    return data.token;
  }

  /**
   * Establishes WebSocket connection to the server
   * @returns {Promise<void>}
   * @throws {Error} If connection fails
   */
  async connect() {
    try {
      // Perform handshake to get session token
      this.sessionToken = await this.handshake();

      // Establish WebSocket connection
      const wsUrl = this.serverUrl.replace(/^http/, 'ws') + `/ws?token=${this.sessionToken}`;
      this.ws = new WebSocket(wsUrl);

      // Set up WebSocket event handlers
      this.ws.onopen = () => {
        this.connected = true;
        this.reconnectAttempts = 0;
        this._emit('connected', {});
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
        this._emit('disconnected', {});

        if (!this.intentionalDisconnect && this.autoReconnect) {
          this._attemptReconnect();
        }
      };

      // Wait for connection to establish
      await this._waitForConnection();
    } catch (err) {
      this._emit('error', { error: err, phase: 'connection' });
      throw err;
    }
  }

  /**
   * Waits for WebSocket connection to be established
   * @private
   * @returns {Promise<void>}
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
   * Handles incoming WebSocket messages
   * @private
   */
  _handleMessage(message) {
    const { type, payload } = message;

    switch (type) {
      case 'tagData':
        this._emit('tagData', this._parseTagData(payload));
        break;
      case 'deviceStatus':
        this._emit('deviceStatus', payload);
        break;
      case 'releaseResponse':
        // Session release acknowledgment
        break;
      default:
        console.warn('Unknown message type:', type);
    }
  }

  /**
   * Parses tag data payload
   * @private
   */
  _parseTagData(payload) {
    const tagData = {
      uid: payload.uid || '',
      type: payload.type || '',
      technology: payload.technology || '',
      scannedAt: payload.scannedAt ? new Date(payload.scannedAt) : null,
      text: payload.text || '',
      message: payload.message || null,
      error: payload.err || null,
      // Preserve raw payload for debugging
      _raw: payload
    };

    // Parse NDEF message if available
    if (tagData.message && tagData.message.type === 'ndef') {
      tagData.ndefRecords = tagData.message.records || [];
    }

    return tagData;
  }

  /**
   * Writes text to an NFC card
   * @param {Object} writeRequest - Write request parameters
   * @param {string} writeRequest.text - Text to write
   * @param {number} [writeRequest.recordIndex] - Index of record to update (0-based)
   * @param {string} [writeRequest.recordType='text'] - Record type ('text' or 'uri')
   * @param {string} [writeRequest.language='en'] - Language code for text records
   * @param {boolean} [writeRequest.append=false] - Append new record instead of replacing
   * @param {boolean} [writeRequest.replace=false] - Replace entire NDEF message (destructive)
   * @returns {Promise<void>}
   */
  async write(writeRequest) {
    if (!this.connected) {
      throw new Error('Not connected to server');
    }

    return new Promise((resolve, reject) => {
      const message = {
        type: 'writeRequest',
        payload: writeRequest
      };

      // Set up one-time response handler
      const responseHandler = (data) => {
        if (data.error) {
          reject(new Error(data.error));
        } else {
          resolve();
        }
      };

      // Send write request
      this.ws.send(JSON.stringify(message));

      // Note: Current server implementation doesn't send write responses
      // This is a placeholder for future implementation
      // For now, resolve immediately
      resolve();
    });
  }

  /**
   * Releases the session and disconnects
   * @returns {Promise<void>}
   */
  async disconnect() {
    this.intentionalDisconnect = true;

    if (this.connected && this.ws) {
      // Send release message
      this.ws.send(JSON.stringify({
        type: 'release'
      }));

      // Wait a bit for the message to be sent
      await new Promise(resolve => setTimeout(resolve, 100));

      // Close WebSocket connection
      this.ws.close();
    }

    this.sessionToken = null;
    this.connected = false;
    this.ws = null;
  }

  /**
   * Gets the current connection status
   * @returns {boolean} True if connected
   */
  isConnected() {
    return this.connected;
  }
}

// Export for different module systems
if (typeof module !== 'undefined' && module.exports) {
  module.exports = NFCClient;
}
if (typeof window !== 'undefined') {
  window.NFCClient = NFCClient;
}
