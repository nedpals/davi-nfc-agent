/**
 * TypeScript type definitions for NFC Device Client
 */

/**
 * Configuration options for NFCDeviceClient constructor
 */
export interface NFCDeviceClientOptions {
  /**
   * Device name for registration
   * @default 'Web NFC Device'
   */
  deviceName?: string;

  /**
   * Platform identifier
   * @default 'web'
   */
  platform?: string;

  /**
   * Application version
   * @default '1.0.0'
   */
  appVersion?: string;

  /**
   * Device can read NFC tags
   * @default true
   */
  canRead?: boolean;

  /**
   * Device can write NFC tags
   * @default false
   */
  canWrite?: boolean;

  /**
   * Automatically send heartbeats
   * @default true
   */
  autoHeartbeat?: boolean;

  /**
   * Heartbeat interval in milliseconds
   * @default 30000
   */
  heartbeatInterval?: number;

  /**
   * Automatically reconnect on disconnect
   * @default true
   */
  autoReconnect?: boolean;

  /**
   * Delay in milliseconds before reconnecting
   * @default 3000
   */
  reconnectDelay?: number;
}

/**
 * Server information received during registration
 */
export interface ServerInfo {
  /**
   * Server version
   */
  version: string;

  /**
   * Supported NFC types
   */
  supportedNFC: string[];
}

/**
 * Registration event payload
 */
export interface RegisteredEvent {
  /**
   * Assigned device ID
   */
  deviceID: string;

  /**
   * Server information
   */
  serverInfo: ServerInfo;
}

/**
 * NDEF record in protocol format
 */
export interface NDEFRecordProtocol {
  /**
   * Type Name Format
   */
  typeNameFormat?: string;

  /**
   * Record type
   */
  type: string;

  /**
   * Record ID
   */
  id?: string;

  /**
   * Text content (for text records)
   */
  text?: string;

  /**
   * Language code (for text records)
   */
  language?: string;

  /**
   * URI (for URI records)
   */
  uri?: string;

  /**
   * Raw data (base64 encoded)
   */
  rawData?: string;
}

/**
 * NDEF message in protocol format
 */
export interface NDEFMessageProtocol {
  /**
   * Message type
   */
  type: 'ndef' | 'raw';

  /**
   * NDEF records
   */
  records: NDEFRecordProtocol[];
}

/**
 * Write request received from server
 */
export interface WriteRequestEvent {
  /**
   * Unique request ID for correlation
   */
  requestID: string;

  /**
   * Target device ID
   */
  deviceID: string;

  /**
   * NDEF message to write
   */
  ndefMessage: NDEFMessageProtocol | null;
}

/**
 * Tag data for scan events
 */
export interface DeviceTagData {
  /**
   * Tag UID (hex format)
   */
  uid: string;

  /**
   * NFC technology (e.g., 'ISO14443A', 'NFC')
   */
  technology?: string;

  /**
   * Tag type (e.g., 'MIFARE Classic 1K', 'NDEF')
   */
  type?: string;

  /**
   * Answer to Reset (if applicable)
   */
  atr?: string;

  /**
   * Timestamp of scan (ISO format)
   */
  scannedAt?: string;

  /**
   * NDEF message data
   */
  ndefMessage?: NDEFMessageProtocol | null;

  /**
   * Raw tag data (base64 encoded)
   */
  rawData?: string | null;

  /**
   * Source of the scan ('webnfc' or 'manual')
   */
  source?: 'webnfc' | 'manual';
}

/**
 * NFC reading event from WebNFC
 */
export interface NFCReadingEvent extends DeviceTagData {
  /**
   * Source is always 'webnfc' for this event
   */
  source: 'webnfc';
}

/**
 * NFC reading error event
 */
export interface NFCReadingErrorEvent {
  /**
   * Error object
   */
  error: Error | DOMException;
}

/**
 * Error event payload
 */
export interface DeviceErrorEvent {
  /**
   * Error object
   */
  error: Error;

  /**
   * Error code (if structured error)
   */
  code?: string;

  /**
   * Phase where error occurred
   */
  phase?: 'connection' | 'websocket' | 'registration' | 'reconnection';
}

/**
 * Event handler function types
 */
export type RegisteredHandler = (event: RegisteredEvent) => void;
export type WriteRequestHandler = (event: WriteRequestEvent) => void;
export type NFCReadingHandler = (event: NFCReadingEvent) => void;
export type NFCReadingErrorHandler = (event: NFCReadingErrorEvent) => void;
export type DeviceConnectedHandler = () => void;
export type DeviceDisconnectedHandler = () => void;
export type DeviceErrorHandler = (error: DeviceErrorEvent) => void;

/**
 * Event name types
 */
export type DeviceEventName = 'registered' | 'writeRequest' | 'nfcReading' | 'nfcReadingError' | 'connected' | 'disconnected' | 'error';

/**
 * Event handler type map
 */
export interface DeviceEventHandlerMap {
  registered: RegisteredHandler;
  writeRequest: WriteRequestHandler;
  nfcReading: NFCReadingHandler;
  nfcReadingError: NFCReadingErrorHandler;
  connected: DeviceConnectedHandler;
  disconnected: DeviceDisconnectedHandler;
  error: DeviceErrorHandler;
}

/**
 * NFC Device Client
 *
 * A JavaScript client for connecting to the NFC Agent InputServer as a device.
 * Supports WebNFC integration for real browser-based NFC scanning.
 *
 * @example
 * ```typescript
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
 *   client.respondToWrite(requestID, true);
 * });
 *
 * await client.connect();
 *
 * if (NFCDeviceClient.isWebNFCSupported()) {
 *   await client.startNFCScanning();
 * }
 * ```
 */
export class NFCDeviceClient {
  /**
   * Creates a new NFC Device client instance
   *
   * @param serverUrl - Base URL of the InputServer (e.g., 'ws://localhost:9470')
   * @param options - Configuration options
   */
  constructor(serverUrl: string, options?: NFCDeviceClientOptions);

  /**
   * Check if WebNFC is supported in the current browser
   * @returns True if WebNFC is supported
   */
  static isWebNFCSupported(): boolean;

  /**
   * Registers an event handler
   *
   * @param event - Event name
   * @param handler - Callback function
   */
  on<K extends DeviceEventName>(event: K, handler: DeviceEventHandlerMap[K]): void;

  /**
   * Removes an event handler
   *
   * @param event - Event name
   * @param handler - Callback function to remove
   */
  off<K extends DeviceEventName>(event: K, handler: DeviceEventHandlerMap[K]): void;

  /**
   * Establishes WebSocket connection and registers as a device
   *
   * @returns Promise that resolves when connected and registered
   * @throws {Error} If connection or registration fails
   */
  connect(): Promise<void>;

  /**
   * Disconnect from the server
   */
  disconnect(): Promise<void>;

  /**
   * Start WebNFC scanning. Tags detected will be automatically sent to the server.
   *
   * @returns Promise that resolves when scanning starts
   * @throws {Error} If WebNFC is not supported or permission denied
   */
  startNFCScanning(): Promise<void>;

  /**
   * Stop WebNFC scanning
   */
  stopNFCScanning(): void;

  /**
   * Check if currently scanning with WebNFC
   * @returns True if scanning
   */
  isNFCScanning(): boolean;

  /**
   * Manually send a tag scan event to the server
   *
   * @param tagData - Tag data to send
   * @returns Promise that resolves when sent
   * @throws {Error} If not connected or registered
   */
  scanTag(tagData: DeviceTagData): Promise<void>;

  /**
   * Send a tag removed event to the server
   *
   * @param uid - UID of the removed tag
   * @returns Promise that resolves when sent
   * @throws {Error} If not connected or registered
   */
  removeTag(uid: string): Promise<void>;

  /**
   * Respond to a write request from the server
   *
   * @param requestID - The request ID from the write request
   * @param success - Whether the write was successful
   * @param error - Error message if unsuccessful
   */
  respondToWrite(requestID: string, success: boolean, error?: string): Promise<void>;

  /**
   * Get the assigned device ID
   * @returns Device ID or null if not registered
   */
  getDeviceID(): string | null;

  /**
   * Get server info received during registration
   * @returns Server info or null if not registered
   */
  getServerInfo(): ServerInfo | null;

  /**
   * Check if connected and registered with the server
   * @returns True if connected and registered
   */
  isConnected(): boolean;
}

export default NFCDeviceClient;
