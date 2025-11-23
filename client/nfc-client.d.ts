/**
 * TypeScript type definitions for NFC Agent Client
 */

/**
 * Configuration options for NFCClient constructor
 */
export interface NFCClientOptions {
  /**
   * Optional API secret for authentication
   */
  apiSecret?: string;

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

  /**
   * Maximum reconnection attempts (0 = infinite)
   * @default 10
   */
  maxReconnectAttempts?: number;
}

/**
 * NDEF Record structure
 */
export interface NDEFRecord {
  /**
   * Type Name Format
   */
  tnf: number;

  /**
   * Record type (e.g., 'T' for text, 'U' for URI)
   */
  type: string;

  /**
   * Record ID (optional)
   */
  id?: string;

  /**
   * Text content (for text records)
   */
  text?: string;

  /**
   * URI content (for URI records)
   */
  uri?: string;

  /**
   * Raw payload bytes
   */
  payload: Uint8Array;
}

/**
 * Tag data event payload
 */
export interface TagData {
  /**
   * Card unique identifier (hex string)
   */
  uid: string;

  /**
   * Card type (e.g., 'MIFARE Classic 1K')
   */
  type: string;

  /**
   * NFC technology standard (e.g., 'ISO14443A')
   */
  technology: string;

  /**
   * Timestamp when card was scanned
   */
  scannedAt: Date | null;

  /**
   * Decoded text from card (convenience field)
   */
  text: string;

  /**
   * Message information (if available)
   */
  message: {
    type: 'ndef' | 'raw';
    records?: NDEFRecord[];
    data?: Uint8Array;
  } | null;

  /**
   * Array of NDEF records (if available)
   */
  ndefRecords: NDEFRecord[];

  /**
   * Error message (if any)
   */
  error: string | null;
}

/**
 * Device status event payload
 */
export interface DeviceStatus {
  /**
   * Whether NFC device is connected
   */
  connected: boolean;

  /**
   * Name of the NFC device
   */
  deviceName?: string;

  /**
   * Device message
   */
  message?: string;

  /**
   * Whether a card is currently present
   */
  cardPresent?: boolean;
}

/**
 * Error event payload
 */
export interface ErrorEvent {
  /**
   * Error object
   */
  error: Error;

  /**
   * Phase where error occurred
   */
  phase: 'connection' | 'websocket' | 'reconnection';
}

/**
 * Write request parameters
 */
export interface WriteRequest {
  /**
   * Text or URI content to write
   */
  text: string;

  /**
   * Index of record to update (0-based)
   * Use this to update a specific existing record
   */
  recordIndex?: number;

  /**
   * Record type
   * @default 'text'
   */
  recordType?: 'text' | 'uri';

  /**
   * Language code for text records
   * @default 'en'
   */
  language?: string;

  /**
   * Append new record instead of replacing
   * Set to true to safely add records without overwriting
   * @default false
   */
  append?: boolean;

  /**
   * Replace entire NDEF message (destructive)
   * Must be explicitly set to true to overwrite all existing data
   * @default false
   */
  replace?: boolean;
}

/**
 * Event handler function types
 */
export type TagDataHandler = (data: TagData) => void;
export type DeviceStatusHandler = (status: DeviceStatus) => void;
export type ConnectedHandler = () => void;
export type DisconnectedHandler = () => void;
export type ErrorHandler = (error: ErrorEvent) => void;

/**
 * Event name types
 */
export type EventName = 'tagData' | 'deviceStatus' | 'connected' | 'disconnected' | 'error';

/**
 * Event handler type map
 */
export interface EventHandlerMap {
  tagData: TagDataHandler;
  deviceStatus: DeviceStatusHandler;
  connected: ConnectedHandler;
  disconnected: DisconnectedHandler;
  error: ErrorHandler;
}

/**
 * NFC Agent Client
 *
 * A framework-agnostic JavaScript client for connecting to the NFC Agent server.
 * Supports session management, WebSocket communication, and NFC card operations.
 *
 * @example
 * ```typescript
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
 * ```
 */
export class NFCClient {
  /**
   * Creates a new NFC client instance
   *
   * @param serverUrl - Base URL of the NFC Agent server (e.g., 'http://localhost:18080')
   * @param options - Configuration options
   *
   * @example
   * ```typescript
   * const client = new NFCClient('http://localhost:18080', {
   *   apiSecret: 'my-secret-key',
   *   autoReconnect: true,
   *   reconnectDelay: 5000,
   *   maxReconnectAttempts: 5
   * });
   * ```
   */
  constructor(serverUrl: string, options?: NFCClientOptions);

  /**
   * Registers an event handler
   *
   * @param event - Event name
   * @param handler - Callback function
   *
   * @example
   * ```typescript
   * client.on('tagData', (data) => {
   *   console.log('Card UID:', data.uid);
   * });
   *
   * client.on('connected', () => {
   *   console.log('Connected to NFC Agent');
   * });
   * ```
   */
  on<K extends EventName>(event: K, handler: EventHandlerMap[K]): void;

  /**
   * Removes an event handler
   *
   * @param event - Event name
   * @param handler - Callback function to remove
   *
   * @example
   * ```typescript
   * const handler = (data) => console.log(data);
   * client.on('tagData', handler);
   * client.off('tagData', handler);
   * ```
   */
  off<K extends EventName>(event: K, handler: EventHandlerMap[K]): void;

  /**
   * Establishes WebSocket connection to the server
   *
   * Performs session handshake and establishes WebSocket connection.
   * Throws an error if connection fails.
   *
   * @returns Promise that resolves when connection is established
   * @throws {Error} If handshake or connection fails
   *
   * @example
   * ```typescript
   * try {
   *   await client.connect();
   *   console.log('Connected successfully');
   * } catch (err) {
   *   console.error('Connection failed:', err);
   * }
   * ```
   */
  connect(): Promise<void>;

  /**
   * Releases the session and disconnects
   *
   * Sends a release message to the server and closes the WebSocket connection.
   *
   * @returns Promise that resolves when disconnection is complete
   *
   * @example
   * ```typescript
   * await client.disconnect();
   * console.log('Disconnected');
   * ```
   */
  disconnect(): Promise<void>;

  /**
   * Writes text to an NFC card
   *
   * @param writeRequest - Write request parameters
   * @returns Promise that resolves when write is complete
   * @throws {Error} If not connected or write fails
   *
   * @example
   * ```typescript
   * // Replace entire card contents
   * await client.write({
   *   text: 'Hello, World!',
   *   replace: true
   * });
   *
   * // Append a new text record
   * await client.write({
   *   text: 'Additional text',
   *   append: true
   * });
   *
   * // Update record at index 0
   * await client.write({
   *   text: 'Updated text',
   *   recordIndex: 0
   * });
   *
   * // Write a URI record
   * await client.write({
   *   text: 'https://example.com',
   *   recordType: 'uri',
   *   replace: true
   * });
   * ```
   */
  write(writeRequest: WriteRequest): Promise<void>;

  /**
   * Gets the current connection status
   *
   * @returns True if connected to the server
   *
   * @example
   * ```typescript
   * if (client.isConnected()) {
   *   console.log('Client is connected');
   * }
   * ```
   */
  isConnected(): boolean;
}

export default NFCClient;
