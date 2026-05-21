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

// setupRoom reserves a code and creates Player 1 in the Hub.
// Returns the code and Player 1's client.
func setupRoom(t *testing.T, h *Hub) (string, *client.Client) {
	t.Helper()

	code, err := h.ReserveCode()
	if err != nil {
		t.Fatalf("ReserveCode: %v", err)
	}

	p1 := client.NewClient("Player 1", code)
	if err := h.CreateRoom(p1, code); err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	return code, p1
}

func TestRoomMessageDeliveredWhenBothOnline(t *testing.T) {
	h, cancel := newTestHub(t)
	defer cancel()

	code, p1 := setupRoom(t, h)
	p2 := client.NewClient("Player 2", code)
	if err := h.JoinRoom(p2, code); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}

	h.Send(message.Message{From: "Player 1", To: "Player 2", Room: code, Text: "hello"})

	msgs := drainMessages(p2.Send, 1, time.Second)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message for Player 2, got %d", len(msgs))
	}
	if msgs[0].Text != "hello" {
		t.Errorf("unexpected text: %q", msgs[0].Text)
	}

	// p1 should not receive their own message
	unexpected := drainMessages(p1.Send, 1, 50*time.Millisecond)
	if len(unexpected) != 0 {
		t.Errorf("Player 1 should not receive echo, got %d", len(unexpected))
	}
}

func TestRoomMessageBufferedWhenPlayer2Offline(t *testing.T) {
	h, cancel := newTestHub(t)
	defer cancel()

	code, _ := setupRoom(t, h)

	// Player 2 has not joined yet — message should be buffered.
	h.Send(message.Message{From: "Player 1", To: "Player 2", Room: code, Text: "buffered"})
	time.Sleep(20 * time.Millisecond)

	// Now Player 2 joins — should receive the buffered message.
	p2 := client.NewClient("Player 2", code)
	if err := h.JoinRoom(p2, code); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}

	msgs := drainMessages(p2.Send, 1, time.Second)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 buffered message, got %d", len(msgs))
	}
	if msgs[0].Text != "buffered" {
		t.Errorf("unexpected text: %q", msgs[0].Text)
	}
}

func TestJoinInvalidCodeReturnsError(t *testing.T) {
	h, cancel := newTestHub(t)
	defer cancel()

	p2 := client.NewClient("Player 2", "NOEXIST")
	err := h.JoinRoom(p2, "NOEXIST")
	if err == nil {
		t.Fatal("expected error joining non-existent room, got nil")
	}
}

func TestLeaveRoomClosesPartnerChannel(t *testing.T) {
	h, cancel := newTestHub(t)
	defer cancel()

	code, p1 := setupRoom(t, h)
	p2 := client.NewClient("Player 2", code)
	if err := h.JoinRoom(p2, code); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}

	// Player 2 leaves — Player 1's Send should be closed.
	h.LeaveRoom("Player 2", code)

	select {
	case _, ok := <-p1.Send:
		if ok {
			// Could be the "partner left" event — drain until closed.
			select {
			case _, ok2 := <-p1.Send:
				if !ok2 {
					return // closed — correct
				}
			case <-time.After(time.Second):
				t.Fatal("p1.Send not closed after partner left")
			}
		}
		// ok == false means closed directly — correct
	case <-time.After(time.Second):
		t.Fatal("p1.Send not closed within 1s after partner left")
	}
}

func TestContextCancellationClosesAllChannels(t *testing.T) {
	h, cancel := newTestHub(t)

	code, p1 := setupRoom(t, h)
	p2 := client.NewClient("Player 2", code)
	if err := h.JoinRoom(p2, code); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}

	cancel()

	for _, c := range []*client.Client{p1, p2} {
		select {
		case _, ok := <-c.Send:
			if ok {
				// Drain one more in case it was the partner-left event.
				select {
				case _, ok2 := <-c.Send:
					if !ok2 {
						continue
					}
				case <-time.After(time.Second):
					t.Fatalf("%s Send not closed after shutdown", c.Name)
				}
			}
		case <-time.After(time.Second):
			t.Fatalf("%s Send not closed within 1s after context cancel", c.Name)
		}
	}
}

func TestReserveCodeIsUnique(t *testing.T) {
	h, cancel := newTestHub(t)
	defer cancel()

	seen := map[string]bool{}
	for i := 0; i < 20; i++ {
		code, err := h.ReserveCode()
		if err != nil {
			t.Fatalf("ReserveCode %d: %v", i, err)
		}
		if seen[code] {
			t.Fatalf("duplicate code generated: %s", code)
		}
		seen[code] = true
	}
}
