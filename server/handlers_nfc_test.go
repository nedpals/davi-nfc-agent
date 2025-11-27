package server

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/nedpals/davi-nfc-agent/nfc"
)

// mockHandlerServer is a mock implementation of HandlerServer for testing
type mockHandlerServer struct {
	tagDataCalls      []nfc.NFCData
	deviceStatusCalls []nfc.DeviceStatus
	handlers          map[string]HandlerFunc
	wsHandlers        []mockWSHandlerEntry
	lifecycleStarters []func(ctx context.Context)
	mu                sync.Mutex
}

type mockWSHandlerEntry struct {
	matcher func(r *http.Request) bool
	handler WebSocketHandlerFunc
}

func newMockHandlerServer() *mockHandlerServer {
	return &mockHandlerServer{
		handlers: make(map[string]HandlerFunc),
	}
}

func (m *mockHandlerServer) Handle(messageType string, handler HandlerFunc) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handlers[messageType] = handler
	return nil
}

func (m *mockHandlerServer) HandleWebSocket(matcher func(r *http.Request) bool, handler WebSocketHandlerFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.wsHandlers = append(m.wsHandlers, mockWSHandlerEntry{
		matcher: matcher,
		handler: handler,
	})
}

func (m *mockHandlerServer) StartLifecycle(start func(ctx context.Context)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lifecycleStarters = append(m.lifecycleStarters, start)
}

func (m *mockHandlerServer) BroadcastTagData(data nfc.NFCData) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tagDataCalls = append(m.tagDataCalls, data)
}

func (m *mockHandlerServer) BroadcastDeviceStatus(status nfc.DeviceStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deviceStatusCalls = append(m.deviceStatusCalls, status)
}

func (m *mockHandlerServer) GetTagDataCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.tagDataCalls)
}

func (m *mockHandlerServer) GetDeviceStatusCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.deviceStatusCalls)
}

func (m *mockHandlerServer) GetLastTagData() *nfc.NFCData {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.tagDataCalls) == 0 {
		return nil
	}
	return &m.tagDataCalls[len(m.tagDataCalls)-1]
}

func (m *mockHandlerServer) HasHandler(messageType string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.handlers[messageType]
	return ok
}

func (m *mockHandlerServer) GetLifecycleStarterCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.lifecycleStarters)
}

func TestNewNFCHandler(t *testing.T) {
	// Create mock NFC reader
	manager := nfc.NewMockManager()
	reader, err := nfc.NewNFCReader("mock://test", manager, 5*time.Second)
	if err != nil {
		t.Fatalf("failed to create NFC reader: %v", err)
	}
	defer reader.Stop()

	allowedCardTypes := map[string]bool{"NTAG213": true}

	handler := NewNFCHandler(reader, allowedCardTypes)
	if handler == nil {
		t.Fatal("NewNFCHandler returned nil")
	}
	if handler.reader != reader {
		t.Fatal("handler reader is not the same as input reader")
	}
	if len(handler.allowedCardTypes) != 1 {
		t.Fatalf("expected 1 allowed card type, got %d", len(handler.allowedCardTypes))
	}
}

func TestNFCHandler_Register(t *testing.T) {
	// Create mock NFC reader
	manager := nfc.NewMockManager()
	reader, err := nfc.NewNFCReader("mock://test", manager, 5*time.Second)
	if err != nil {
		t.Fatalf("failed to create NFC reader: %v", err)
	}
	defer reader.Stop()

	handler := NewNFCHandler(reader, nil)
	mockServer := newMockHandlerServer()

	// Register with mock server
	handler.Register(mockServer)

	// Verify writeRequest handler is registered
	if !mockServer.HasHandler(WSMessageTypeWriteRequest) {
		t.Fatal("writeRequest handler not registered")
	}

	// Verify lifecycle was registered
	if mockServer.GetLifecycleStarterCount() != 1 {
		t.Fatalf("expected 1 lifecycle starter, got %d", mockServer.GetLifecycleStarterCount())
	}
}

func TestNFCHandler_Integration(t *testing.T) {
	// Create mock NFC reader
	manager := nfc.NewMockManager()
	reader, err := nfc.NewNFCReader("mock://test", manager, 5*time.Second)
	if err != nil {
		t.Fatalf("failed to create NFC reader: %v", err)
	}
	defer reader.Stop()

	handler := NewNFCHandler(reader, nil)
	mockServer := newMockHandlerServer()

	// Register with mock server
	handler.Register(mockServer)

	t.Run("all routes registered correctly", func(t *testing.T) {
		// Verify writeRequest handler is registered and functional
		if !mockServer.HasHandler(WSMessageTypeWriteRequest) {
			t.Fatal("writeRequest handler not registered")
		}
	})
}

func TestNFCHandler_Lifecycle(t *testing.T) {
	t.Run("register lifecycle function", func(t *testing.T) {
		manager := nfc.NewMockManager()
		reader, err := nfc.NewNFCReader("mock://test", manager, 5*time.Second)
		if err != nil {
			t.Fatalf("failed to create NFC reader: %v", err)
		}
		defer reader.Stop()

		handler := NewNFCHandler(reader, nil)
		mockServer := newMockHandlerServer()

		// Register should create lifecycle function
		handler.Register(mockServer)

		if mockServer.GetLifecycleStarterCount() != 1 {
			t.Fatalf("expected 1 lifecycle starter, got %d", mockServer.GetLifecycleStarterCount())
		}
	})

	t.Run("context cancellation stops handler", func(t *testing.T) {
		manager := nfc.NewMockManager()
		reader, err := nfc.NewNFCReader("mock://test", manager, 5*time.Second)
		if err != nil {
			t.Fatalf("failed to create NFC reader: %v", err)
		}
		defer reader.Stop()

		handler := NewNFCHandler(reader, nil)
		mockServer := newMockHandlerServer()

		ctx, cancel := context.WithCancel(context.Background())

		// Register and get lifecycle starter
		handler.Register(mockServer)

		// Start the lifecycle
		mockServer.mu.Lock()
		starters := mockServer.lifecycleStarters
		mockServer.mu.Unlock()

		if len(starters) != 1 {
			t.Fatalf("expected 1 lifecycle starter, got %d", len(starters))
		}

		// Run the lifecycle starter
		starters[0](ctx)

		// Cancel immediately
		cancel()

		// Give goroutine time to exit
		time.Sleep(10 * time.Millisecond)

		// If we get here without hanging, the test passes
	})
}

func TestNFCHandler_handleTagData(t *testing.T) {
	t.Run("handle tag data with error", func(t *testing.T) {
		manager := nfc.NewMockManager()
		reader, err := nfc.NewNFCReader("mock://test", manager, 5*time.Second)
		if err != nil {
			t.Fatalf("failed to create NFC reader: %v", err)
		}
		defer reader.Stop()

		mockServer := newMockHandlerServer()
		handler := NewNFCHandler(reader, nil)

		// Handle data with error
		data := nfc.NFCData{
			Card: nil,
			Err:  fmt.Errorf("test error"),
		}

		handler.handleTagData(data, mockServer)

		// Verify error was broadcast
		if mockServer.GetTagDataCallCount() != 1 {
			t.Fatalf("expected 1 broadcast call, got %d", mockServer.GetTagDataCallCount())
		}

		lastData := mockServer.GetLastTagData()
		if lastData.Err == nil {
			t.Fatal("expected error to be broadcast")
		}
		if lastData.Err.Error() != "test error" {
			t.Fatalf("expected error 'test error', got '%v'", lastData.Err)
		}
	})

	t.Run("handle tag data with nil card", func(t *testing.T) {
		manager := nfc.NewMockManager()
		reader, err := nfc.NewNFCReader("mock://test", manager, 5*time.Second)
		if err != nil {
			t.Fatalf("failed to create NFC reader: %v", err)
		}
		defer reader.Stop()

		mockServer := newMockHandlerServer()
		handler := NewNFCHandler(reader, nil)

		// Handle data with nil card (no error)
		data := nfc.NFCData{
			Card: nil,
			Err:  nil,
		}

		handler.handleTagData(data, mockServer)

		// Should not broadcast anything for nil card
		if mockServer.GetTagDataCallCount() != 0 {
			t.Fatalf("expected 0 broadcast calls, got %d", mockServer.GetTagDataCallCount())
		}
	})

	t.Run("handle tag data with allowed card type", func(t *testing.T) {
		manager := nfc.NewMockManager()
		reader, err := nfc.NewNFCReader("mock://test", manager, 5*time.Second)
		if err != nil {
			t.Fatalf("failed to create NFC reader: %v", err)
		}
		defer reader.Stop()

		mockServer := newMockHandlerServer()
		allowedCardTypes := map[string]bool{"Mock Tag": true} // MockTag type() returns "Mock Tag" with a space
		handler := NewNFCHandler(reader, allowedCardTypes)

		// Create a mock tag and card
		mockTag := nfc.NewMockTag("04:23:45:67:89:AB")
		card := nfc.NewCard(mockTag)
		
		data := nfc.NFCData{
			Card: card,
			Err:  nil,
		}

		handler.handleTagData(data, mockServer)
		
		// Should broadcast the card data
		if mockServer.GetTagDataCallCount() != 1 {
			t.Fatalf("expected 1 broadcast call, got %d", mockServer.GetTagDataCallCount())
		}

		lastData := mockServer.GetLastTagData()
		if lastData.Card == nil {
			t.Fatal("expected card to be broadcast")
		}
		if lastData.Card.UID != data.Card.UID {
			t.Fatalf("expected UID '%s', got '%s'", data.Card.UID, lastData.Card.UID)
		}
	})

	t.Run("handle tag data with disallowed card type", func(t *testing.T) {
		manager := nfc.NewMockManager()
		reader, err := nfc.NewNFCReader("mock://test", manager, 5*time.Second)
		if err != nil {
			t.Fatalf("failed to create NFC reader: %v", err)
		}
		defer reader.Stop()

		mockServer := newMockHandlerServer()
		allowedCardTypes := map[string]bool{"NTAG213": true} // Only allow NTAG213, not MockTag
		handler := NewNFCHandler(reader, allowedCardTypes)

		// Create a mock tag and card (MockTag type, not NTAG213)
		mockTag := nfc.NewMockTag("04:23:45:67:89:CD")
		card := nfc.NewCard(mockTag)
		
		data := nfc.NFCData{
			Card: card,
			Err:  nil,
		}

		handler.handleTagData(data, mockServer)
		
		// Should broadcast error for disallowed card type
		if mockServer.GetTagDataCallCount() != 1 {
			t.Fatalf("expected 1 broadcast call, got %d", mockServer.GetTagDataCallCount())
		}

		lastData := mockServer.GetLastTagData()
		if lastData.Card != nil {
			t.Fatal("expected card to be nil for disallowed type")
		}
		if lastData.Err == nil {
			t.Fatal("expected error to be broadcast for disallowed type")
		}
	})

	t.Run("handle tag data with no filter (allow all)", func(t *testing.T) {
		manager := nfc.NewMockManager()
		reader, err := nfc.NewNFCReader("mock://test", manager, 5*time.Second)
		if err != nil {
			t.Fatalf("failed to create NFC reader: %v", err)
		}
		defer reader.Stop()

		mockServer := newMockHandlerServer()
		handler := NewNFCHandler(reader, nil) // No filter

		// Create a mock tag and card
		mockTag := nfc.NewMockTag("04:23:45:67:89:EF")
		card := nfc.NewCard(mockTag)
		
		data := nfc.NFCData{
			Card: card,
			Err:  nil,
		}

		handler.handleTagData(data, mockServer)
		
		// Should broadcast the card data (no filter)
		if mockServer.GetTagDataCallCount() != 1 {
			t.Fatalf("expected 1 broadcast call, got %d", mockServer.GetTagDataCallCount())
		}

		lastData := mockServer.GetLastTagData()
		if lastData.Card == nil {
			t.Fatal("expected card to be broadcast")
		}
	})
}
