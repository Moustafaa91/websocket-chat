package hub

import (
	"backend/internal/client"
	"backend/internal/message"
	"context"
	"testing"
	"time"
)

func newTestHub(t *testing.T) (*Hub, context.CancelFunc) {
	t.Helper()
	h := New()
	ctx, cancel := context.WithCancel(context.Background())
	go h.Run(ctx)
	return h, cancel
}

func makeClient(name string) *client.Client {
	return client.NewClient(name)
}

func drainMessages(ch <-chan client.Outbound, count int, timeout time.Duration) []message.Message {
	var out []message.Message
	deadline := time.After(timeout)
	for i := 0; i < count; i++ {
		select {
		case ob := <-ch:
			if ob.Kind == client.OutboundMessage {
				out = append(out, ob.Message)
			}
		case <-deadline:
			return out
		}
	}
	return out
}

func registerSync(t *testing.T, h *Hub, c *client.Client) {
	t.Helper()
	done := make(chan struct{})
	go func() { h.Register(c); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Register timed out")
	}
}

func unregisterSync(t *testing.T, h *Hub, name string) {
	t.Helper()
	done := make(chan struct{})
	go func() { h.Unregister(name); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Unregister timed out")
	}
}

func sendSync(t *testing.T, h *Hub, m message.Message) {
	t.Helper()
	done := make(chan struct{})
	go func() { h.Send(m); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Send timed out")
	}
}

func TestRegisterDeliversPendingMessages(t *testing.T) {
	h, cancel := newTestHub(t)
	defer cancel()

	bob := makeClient("bob")
	registerSync(t, h, bob)

	sendSync(t, h, message.Message{From: "bob", To: "alex", Text: "hello offline alex", TS: 1})
	time.Sleep(20 * time.Millisecond)

	alex := makeClient("alex")
	registerSync(t, h, alex)

	msgs := drainMessages(alex.Send, 1, time.Second)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 pending message on reconnect, got %d", len(msgs))
	}
	if msgs[0].Text != "hello offline alex" {
		t.Errorf("unexpected text: %q", msgs[0].Text)
	}

	unregisterSync(t, h, "bob")
}

func TestMessageRoutedToOnlineRecipient(t *testing.T) {
	h, cancel := newTestHub(t)
	defer cancel()

	alex := makeClient("alex")
	bob := makeClient("bob")
	registerSync(t, h, alex)
	registerSync(t, h, bob)

	sendSync(t, h, message.Message{From: "alex", To: "bob", Text: "hi bob", TS: 1})

	msgs := drainMessages(bob.Send, 1, time.Second)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message for bob, got %d", len(msgs))
	}
	if msgs[0].Text != "hi bob" {
		t.Errorf("unexpected text: %q", msgs[0].Text)
	}

	unexpected := drainMessages(alex.Send, 1, 50*time.Millisecond)
	if len(unexpected) != 0 {
		t.Errorf("alex should not receive a message, got %d", len(unexpected))
	}
}

func TestUnregisterBuffersDrainedMessages(t *testing.T) {
	h, cancel := newTestHub(t)
	defer cancel()

	alex := makeClient("alex")
	bob := makeClient("bob")
	registerSync(t, h, alex)
	registerSync(t, h, bob)

	sendSync(t, h, message.Message{From: "alex", To: "bob", Text: "see you later", TS: 1})
	time.Sleep(20 * time.Millisecond)

	unregisterSync(t, h, "bob")

	bob2 := makeClient("bob")
	registerSync(t, h, bob2)

	msgs := drainMessages(bob2.Send, 1, time.Second)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 buffered message on reconnect, got %d", len(msgs))
	}
	if msgs[0].Text != "see you later" {
		t.Errorf("unexpected text: %q", msgs[0].Text)
	}
}

func TestMessageToOfflineUserIsBuffered(t *testing.T) {
	h, cancel := newTestHub(t)
	defer cancel()

	bob := makeClient("bob")
	registerSync(t, h, bob)

	sendSync(t, h, message.Message{From: "bob", To: "alex", Text: "buffered", TS: 1})
	time.Sleep(20 * time.Millisecond)

	alex := makeClient("alex")
	registerSync(t, h, alex)

	msgs := drainMessages(alex.Send, 1, time.Second)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 buffered message, got %d", len(msgs))
	}
	if msgs[0].Text != "buffered" {
		t.Errorf("unexpected text: %q", msgs[0].Text)
	}
}

func TestContextCancellationClosesClientChannels(t *testing.T) {
	h, cancel := newTestHub(t)

	alex := makeClient("alex")
	registerSync(t, h, alex)

	cancel()

	select {
	case _, ok := <-alex.Send:
		if ok {
			t.Error("expected Send channel to be closed, got a value")
		}
	case <-time.After(time.Second):
		t.Fatal("Send channel not closed within 1s after context cancellation")
	}
}

func TestDoubleUnregisterIsIdempotent(t *testing.T) {
	h, cancel := newTestHub(t)
	defer cancel()

	alex := makeClient("alex")
	registerSync(t, h, alex)
	unregisterSync(t, h, "alex")
	unregisterSync(t, h, "alex")
}
