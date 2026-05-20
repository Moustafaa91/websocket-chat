package hub

import (
	"backend/internal/client"
	"backend/internal/message"
	"context"
	"testing"
	"time"
)

// newTestHub creates a Hub and starts its Run loop in the background.
// The returned cancel func stops the Hub; call it in defer.
func newTestHub(t *testing.T) (*Hub, context.CancelFunc) {
	t.Helper()
	h := New()
	ctx, cancel := context.WithCancel(context.Background())
	go h.Run(ctx)
	return h, cancel
}

// makeClient creates a client with a named user and a buffered Send channel.
func makeClient(name string) *client.Client {
	return client.NewClient(name)
}

// drain reads all messages from ch within a short deadline.
// Returns whatever was buffered; returns nil if nothing arrives.
func drain(ch <-chan message.Message, count int, timeout time.Duration) []message.Message {
	var out []message.Message
	deadline := time.After(timeout)
	for i := 0; i < count; i++ {
		select {
		case m := <-ch:
			out = append(out, m)
		case <-deadline:
			return out
		}
	}
	return out
}

// sendToHub delivers a value on a channel, failing the test if the Hub does
// not consume it within the timeout (indicates deadlock or logic error).
func registerSync(t *testing.T, h *Hub, c *client.Client) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		h.Register(c)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Register timed out — Hub goroutine not running?")
	}
}

func unregisterSync(t *testing.T, h *Hub, name string) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		h.Unregister(name)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Unregister timed out — Hub goroutine not running?")
	}
}

func sendSync(t *testing.T, h *Hub, m message.Message) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		h.Send(m)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Send timed out — Hub goroutine not running?")
	}
}

// TestRegisterDeliversPendingMessages verifies that messages buffered while a
// client was offline are flushed to the client's Send channel on reconnect.
func TestRegisterDeliversPendingMessages(t *testing.T) {
	h, cancel := newTestHub(t)
	defer cancel()

	alex := makeClient("alex")
	bob := makeClient("bob")

	// Register Bob so he can receive messages.
	registerSync(t, h, bob)

	// Send a message to Alex while Alex is offline.
	sendSync(t, h, message.Message{From: "bob", To: "alex", Text: "hello offline alex", TS: 1})

	// Give the Hub a moment to buffer the message.
	time.Sleep(20 * time.Millisecond)

	// Now Alex connects.
	registerSync(t, h, alex)

	// The Hub should flush the buffered message into alex.Send.
	msgs := drain(alex.Send, 1, time.Second)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 pending message delivered on reconnect, got %d", len(msgs))
	}
	if msgs[0].Text != "hello offline alex" {
		t.Errorf("unexpected message text: %q", msgs[0].Text)
	}

	// Clean up.
	unregisterSync(t, h, "bob")
}

// TestMessageRoutedToOnlineRecipient verifies direct delivery when both users
// are connected.
func TestMessageRoutedToOnlineRecipient(t *testing.T) {
	h, cancel := newTestHub(t)
	defer cancel()

	alex := makeClient("alex")
	bob := makeClient("bob")

	registerSync(t, h, alex)
	registerSync(t, h, bob)

	sendSync(t, h, message.Message{From: "alex", To: "bob", Text: "hi bob", TS: 1})

	msgs := drain(bob.Send, 1, time.Second)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message delivered to bob, got %d", len(msgs))
	}
	if msgs[0].Text != "hi bob" {
		t.Errorf("unexpected message text: %q", msgs[0].Text)
	}

	// Nothing should arrive in alex's Send.
	unexpected := drain(alex.Send, 1, 50*time.Millisecond)
	if len(unexpected) != 0 {
		t.Errorf("alex should not have received a message, got %d", len(unexpected))
	}
}

// TestUnregisterBuffersDrainedMessages verifies that messages already queued
// in a client's Send channel at disconnect time are moved to pending.
func TestUnregisterBuffersDrainedMessages(t *testing.T) {
	h, cancel := newTestHub(t)
	defer cancel()

	alex := makeClient("alex")
	bob := makeClient("bob")

	registerSync(t, h, alex)
	registerSync(t, h, bob)

	// Send a message to Bob while he's registered — it lands in bob.Send.
	sendSync(t, h, message.Message{From: "alex", To: "bob", Text: "see you later", TS: 1})

	// Give the Hub time to route it.
	time.Sleep(20 * time.Millisecond)

	// Unregister Bob without draining his Send — the Hub should move the
	// message from bob.Send into h.pending["bob"].
	unregisterSync(t, h, "bob")

	// Bob reconnects.
	bob2 := makeClient("bob")
	registerSync(t, h, bob2)

	msgs := drain(bob2.Send, 1, time.Second)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 buffered message delivered to bob on reconnect, got %d", len(msgs))
	}
	if msgs[0].Text != "see you later" {
		t.Errorf("unexpected message text: %q", msgs[0].Text)
	}
}

// TestMessageToOfflineUserIsBuffered verifies that a message sent to a
// disconnected user is stored and not lost.
func TestMessageToOfflineUserIsBuffered(t *testing.T) {
	h, cancel := newTestHub(t)
	defer cancel()

	bob := makeClient("bob")
	registerSync(t, h, bob)

	// Alex is not registered — message to alex should be buffered.
	sendSync(t, h, message.Message{From: "bob", To: "alex", Text: "buffered", TS: 1})

	// Give the Hub time to process.
	time.Sleep(20 * time.Millisecond)

	// Now Alex connects — should receive the buffered message.
	alex := makeClient("alex")
	registerSync(t, h, alex)

	msgs := drain(alex.Send, 1, time.Second)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 buffered message, got %d", len(msgs))
	}
	if msgs[0].Text != "buffered" {
		t.Errorf("unexpected message: %q", msgs[0].Text)
	}
}

// TestContextCancellationClosesClientChannels verifies that cancelling the
// Hub's context causes all active client Send channels to be closed, allowing
// write pumps to exit cleanly.
func TestContextCancellationClosesClientChannels(t *testing.T) {
	h, cancel := newTestHub(t)

	alex := makeClient("alex")
	registerSync(t, h, alex)

	// Cancel the context — Hub should close alex.Send.
	cancel()

	// Wait for the channel to be closed.
	select {
	case _, ok := <-alex.Send:
		if ok {
			t.Error("expected Send channel to be closed, got a value instead")
		}
		// ok == false means channel closed — correct.
	case <-time.After(time.Second):
		t.Fatal("Send channel was not closed within 1s after context cancellation")
	}
}

// TestDoubleUnregisterIsIdempotent verifies that unregistering a client that
// is already gone does not panic or deadlock.
func TestDoubleUnregisterIsIdempotent(t *testing.T) {
	h, cancel := newTestHub(t)
	defer cancel()

	alex := makeClient("alex")
	registerSync(t, h, alex)
	unregisterSync(t, h, "alex")

	// Second unregister should be a no-op.
	unregisterSync(t, h, "alex")
}
