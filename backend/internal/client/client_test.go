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

// ── Mock WebSocket connection ─────────────────────────────────────────────────

// mockConn implements the Conn interface for testing.
// It allows tests to inject messages and inspect writes without a real network.
type mockConn struct {
	mu       sync.Mutex
	incoming [][]byte      // messages ReadMessage returns, in order
	written  []interface{} // values passed to WriteJSON
	closed   bool
	closeErr error // error ReadMessage returns after incoming is exhausted
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

func (m *mockConn) WriteMessage(messageType int, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func (m *mockConn) SetWriteDeadline(t time.Time) error { return nil }

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

// ── Mock Hub (Sender interface) ───────────────────────────────────────────────

type mockHub struct {
	mu           sync.Mutex
	sent         []message.Message
	unregistered []string
}

func (h *mockHub) Send(m message.Message) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sent = append(h.sent, m)
}

func (h *mockHub) Unregister(name string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.unregistered = append(h.unregistered, name)
}

func (h *mockHub) getSent() []message.Message {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]message.Message, len(h.sent))
	copy(out, h.sent)
	return out
}

func (h *mockHub) getUnregistered() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]string, len(h.unregistered))
	copy(out, h.unregistered)
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

// TestReadPumpForwardsMessageToHub verifies that a valid incoming message is
// decoded and forwarded to the Hub with the correct From and To fields.
func TestReadPumpForwardsMessageToHub(t *testing.T) {
	conn := &mockConn{}
	hub := &mockHub{}
	c := NewClient("alex")

	// Inject one valid message then close the connection.
	conn.incoming = [][]byte{
		marshalMsg(t, message.Message{From: "alex", To: "bob", Text: "hello", TS: 1}),
	}
	conn.closeErr = errors.New("connection closed")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	c.ReadPump(ctx, conn, hub, nopEvent)

	sent := hub.getSent()
	if len(sent) != 1 {
		t.Fatalf("expected 1 message sent to hub, got %d", len(sent))
	}
	if sent[0].From != "alex" {
		t.Errorf("From field should be enforced as 'alex', got %q", sent[0].From)
	}
	if sent[0].To != "bob" {
		t.Errorf("To should be 'bob', got %q", sent[0].To)
	}
	if sent[0].Text != "hello" {
		t.Errorf("unexpected text: %q", sent[0].Text)
	}
}

// TestReadPumpEnforcesSenderIdentity verifies that even if the client sends a
// spoofed From field, the Hub always sees the real user's name.
func TestReadPumpEnforcesSenderIdentity(t *testing.T) {
	conn := &mockConn{}
	hub := &mockHub{}
	c := NewClient("bob")

	// Client tries to pretend to be alex.
	conn.incoming = [][]byte{
		marshalMsg(t, message.Message{From: "alex", To: "bob", Text: "spoofed", TS: 1}),
	}
	conn.closeErr = errors.New("eof")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	c.ReadPump(ctx, conn, hub, nopEvent)

	sent := hub.getSent()
	if len(sent) != 1 {
		t.Fatalf("expected 1 message, got %d", len(sent))
	}
	if sent[0].From != "bob" {
		t.Errorf("expected From to be enforced as 'bob', got %q", sent[0].From)
	}
}

// TestReadPumpCallsUnregisterOnClose verifies that Unregister is always called
// when the connection closes, regardless of reason.
func TestReadPumpCallsUnregisterOnClose(t *testing.T) {
	conn := &mockConn{
		closeErr: errors.New("connection reset"),
	}
	hub := &mockHub{}
	c := NewClient("alex")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	c.ReadPump(ctx, conn, hub, nopEvent)

	unregistered := hub.getUnregistered()
	if len(unregistered) != 1 || unregistered[0] != "alex" {
		t.Errorf("expected Unregister('alex'), got %v", unregistered)
	}
}

// TestWritePumpDeliversMessage verifies that a message placed on c.Send is
// written to the WebSocket connection.
func TestWritePumpDeliversMessage(t *testing.T) {
	conn := &mockConn{}
	c := NewClient("bob")

	ctx, cancel := context.WithCancel(context.Background())

	// Send one message then close the Send channel to terminate the write pump.
	go func() {
		c.Send <- message.Message{From: "alex", To: "bob", Text: "delivered", TS: 1}
		// Give the pump time to write it, then cancel.
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	c.WritePump(ctx, conn, nopEvent)

	written := conn.getWritten()
	if len(written) != 1 {
		t.Fatalf("expected 1 message written to conn, got %d", len(written))
	}
}

// TestWritePumpExitsOnChannelClose verifies that closing c.Send causes the
// write pump to exit cleanly (no goroutine leak).
func TestWritePumpExitsOnChannelClose(t *testing.T) {
	conn := &mockConn{}
	c := NewClient("alex")

	ctx := context.Background()
	done := make(chan struct{})

	go func() {
		c.WritePump(ctx, conn, nopEvent)
		close(done)
	}()

	// Close the Send channel — write pump should exit.
	close(c.Send)

	select {
	case <-done:
		// Correct — pump exited.
	case <-time.After(time.Second):
		t.Fatal("WritePump did not exit after Send channel was closed")
	}
}

// TestWritePumpExitsOnContextCancel verifies that cancelling the context
// causes the write pump to exit cleanly.
func TestWritePumpExitsOnContextCancel(t *testing.T) {
	conn := &mockConn{}
	c := NewClient("alex")

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		c.WritePump(ctx, conn, nopEvent)
		close(done)
	}()

	cancel()

	select {
	case <-done:
		// Correct — pump exited.
	case <-time.After(time.Second):
		t.Fatal("WritePump did not exit after context cancellation")
	}
}

// TestOtherUser verifies the two-user routing helper.
func TestOtherUser(t *testing.T) {
	if got := otherUser("alex"); got != "bob" {
		t.Errorf("otherUser('alex') = %q, want 'bob'", got)
	}
	if got := otherUser("bob"); got != "alex" {
		t.Errorf("otherUser('bob') = %q, want 'alex'", got)
	}
}
