package client

import (
	"backend/internal/message"
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// ── Mock WebSocket ────────────────────────────────────────────────────────────

type mockConn struct {
	mu       sync.Mutex
	incoming [][]byte
	written  []interface{}
	closed   bool
	closeErr error
}

func (m *mockConn) ReadMessage() (int, []byte, error) {
	for {
		m.mu.Lock()
		if len(m.incoming) > 0 {
			raw := m.incoming[0]
			m.incoming = m.incoming[1:]
			m.mu.Unlock()
			return websocket.TextMessage, raw, nil
		}
		if m.closeErr != nil {
			err := m.closeErr
			m.mu.Unlock()
			return 0, nil, err
		}
		m.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
}

func (m *mockConn) WriteJSON(v any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return errors.New("write on closed connection")
	}
	m.written = append(m.written, v)
	return nil
}

func (m *mockConn) WriteMessage(_ int, _ []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func (m *mockConn) SetWriteDeadline(_ time.Time) error { return nil }

func (m *mockConn) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closeErr == nil {
		m.closeErr = errors.New("connection closed")
	}
	m.closed = true
	return nil
}

func (m *mockConn) getWritten() []interface{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]interface{}, len(m.written))
	copy(out, m.written)
	return out
}

// ── Mock Hub ──────────────────────────────────────────────────────────────────

type mockHub struct {
	mu   sync.Mutex
	sent []message.Message
	left []string // name+code pairs recorded as "name:code"
}

func (h *mockHub) Send(m message.Message) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sent = append(h.sent, m)
}

func (h *mockHub) SetInactive(name, code string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.left = append(h.left, "inactive:"+name+":"+code)
}

func (h *mockHub) GoOffline(name, code string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.left = append(h.left, "offline:"+name+":"+code)
}

func (h *mockHub) getSent() []message.Message {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]message.Message, len(h.sent))
	copy(out, h.sent)
	return out
}

func (h *mockHub) getLeft() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]string, len(h.left))
	copy(out, h.left)
	return out
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func nopEvent(_, _ string) {}

func marshalMsg(t *testing.T, m message.Message) []byte {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestReadPumpForwardsMessageToHub(t *testing.T) {
	conn := &mockConn{}
	hub := &mockHub{}
	c := NewClient("Player 1", "ABC123")

	conn.incoming = [][]byte{
		marshalMsg(t, message.Message{From: "Player 1", Text: "hello", TS: 1}),
	}
	conn.closeErr = errors.New("closed")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	c.ReadPump(ctx, conn, hub, nopEvent)

	sent := hub.getSent()
	if len(sent) != 1 {
		t.Fatalf("expected 1 message, got %d", len(sent))
	}
	if sent[0].From != "Player 1" {
		t.Errorf("expected From='Player 1', got %q", sent[0].From)
	}
	if sent[0].Room != "ABC123" {
		t.Errorf("expected Room='ABC123', got %q", sent[0].Room)
	}
	if sent[0].Text != "hello" {
		t.Errorf("unexpected text: %q", sent[0].Text)
	}
}

func TestReadPumpEnforcesRoomCode(t *testing.T) {
	conn := &mockConn{}
	hub := &mockHub{}
	c := NewClient("Player 1", "ABC123")

	// Client tries to spoof a different room code.
	conn.incoming = [][]byte{
		marshalMsg(t, message.Message{From: "Player 1", Room: "HACKED", Text: "spoof"}),
	}
	conn.closeErr = errors.New("closed")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	c.ReadPump(ctx, conn, hub, nopEvent)

	sent := hub.getSent()
	if len(sent) != 1 {
		t.Fatalf("expected 1 message, got %d", len(sent))
	}
	if sent[0].Room != "ABC123" {
		t.Errorf("expected Room enforced as 'ABC123', got %q", sent[0].Room)
	}
}

func TestReadPumpCallsLeaveRoomOnClose(t *testing.T) {
	conn := &mockConn{closeErr: errors.New("connection reset")}
	hub := &mockHub{}
	c := NewClient("Player 1", "ABC123")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	c.ReadPump(ctx, conn, hub, nopEvent)

	left := hub.getLeft()
	if len(left) != 1 || left[0] != "offline:Player 1:ABC123" {
		t.Errorf("expected GoOffline('Player 1', 'ABC123'), got %v", left)
	}
}

func TestReadPumpCallsGoOfflineOnUserLeft(t *testing.T) {
	conn := &mockConn{}
	hub := &mockHub{}
	c := NewClient("Player 1", "ABC123")

	conn.closeErr = &websocket.CloseError{Code: websocket.CloseNormalClosure, Text: "user left"}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	c.ReadPump(ctx, conn, hub, nopEvent)

	left := hub.getLeft()
	if len(left) != 1 || left[0] != "offline:Player 1:ABC123" {
		t.Errorf("expected GoOffline on user left, got %v", left)
	}
}

func TestReadPumpCallsSetInactiveOnInactivityClose(t *testing.T) {
	conn := &mockConn{}
	hub := &mockHub{}
	c := NewClient("Player 1", "ABC123")

	conn.closeErr = &websocket.CloseError{Code: websocket.CloseNormalClosure, Text: "inactivity"}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	c.ReadPump(ctx, conn, hub, nopEvent)

	left := hub.getLeft()
	if len(left) != 1 || left[0] != "inactive:Player 1:ABC123" {
		t.Errorf("expected SetInactive on inactivity, got %v", left)
	}
}

func TestWritePumpDeliversMessage(t *testing.T) {
	conn := &mockConn{}
	c := NewClient("Player 2", "ABC123")

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		c.Send <- Outbound{Kind: OutboundMessage, Message: message.Message{Text: "delivered"}}
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	c.WritePump(ctx, conn, nopEvent)

	if len(conn.getWritten()) != 1 {
		t.Fatalf("expected 1 write, got %d", len(conn.getWritten()))
	}
}

func TestWritePumpExitsOnChannelClose(t *testing.T) {
	conn := &mockConn{}
	c := NewClient("Player 1", "ABC123")
	done := make(chan struct{})

	go func() {
		c.WritePump(context.Background(), conn, nopEvent)
		close(done)
	}()

	close(c.Send)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("WritePump did not exit after Send closed")
	}
}

func TestWritePumpExitsOnContextCancel(t *testing.T) {
	conn := &mockConn{}
	c := NewClient("Player 1", "ABC123")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		c.WritePump(ctx, conn, nopEvent)
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("WritePump did not exit after context cancel")
	}
}
