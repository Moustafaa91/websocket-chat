package hub

import (
	"backend/internal/client"
	"backend/internal/event"
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
	for len(out) < count {
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

	unexpected := drainMessages(p1.Send, 1, 50*time.Millisecond)
	if len(unexpected) != 0 {
		t.Errorf("Player 1 should not receive echo, got %d", len(unexpected))
	}
}

func TestRoomMessageBufferedWhenPlayer2Absent(t *testing.T) {
	h, cancel := newTestHub(t)
	defer cancel()

	code, _ := setupRoom(t, h)

	h.Send(message.Message{From: "Player 1", To: "Player 2", Room: code, Text: "buffered"})
	time.Sleep(20 * time.Millisecond)

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

func TestGoOfflineDoesNotClosePartnerChannel(t *testing.T) {
	h, cancel := newTestHub(t)
	defer cancel()

	code, p1 := setupRoom(t, h)
	p2 := client.NewClient("Player 2", code)
	if err := h.JoinRoom(p2, code); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}

	h.GoOffline("Player 2", code)
	time.Sleep(20 * time.Millisecond)

	deadline := time.After(time.Second)
	gotOffline := false
	for !gotOffline {
		select {
		case ob := <-p1.Send:
			if ob.Kind == client.OutboundEvent &&
				ob.Event.Kind == event.KindPresence &&
				ob.Event.Player == "Player 2" &&
				ob.Event.Presence == "offline" {
				gotOffline = true
			}
		case <-deadline:
			t.Fatal("expected offline presence event for Player 2")
		}
	}

	select {
	case _, ok := <-p1.Send:
		if !ok {
			t.Fatal("partner channel closed unexpectedly")
		}
		t.Fatal("unexpected extra message on partner channel")
	default:
	}
}

func TestSetInactiveKeepsRoomAndBuffersMessages(t *testing.T) {
	h, cancel := newTestHub(t)
	defer cancel()

	code, _ := setupRoom(t, h)
	p2 := client.NewClient("Player 2", code)
	if err := h.JoinRoom(p2, code); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}

	h.SetInactive("Player 2", code)
	time.Sleep(20 * time.Millisecond)

	h.Send(message.Message{From: "Player 1", Room: code, Text: "while p2 inactive"})

	p2re := client.NewClient("Player 2", code)
	if err := h.JoinRoom(p2re, code); err != nil {
		t.Fatalf("JoinRoom reconnect: %v", err)
	}

	msgs := drainMessages(p2re.Send, 1, time.Second)
	if len(msgs) != 1 || msgs[0].Text != "while p2 inactive" {
		t.Fatalf("expected buffered message on reconnect, got %v", msgs)
	}
}

func TestBothInactiveKeepsRoomAndAllowsEitherPlayerToReturn(t *testing.T) {
	h, cancel := newTestHub(t)
	defer cancel()

	code, _ := setupRoom(t, h)
	p2 := client.NewClient("Player 2", code)
	if err := h.JoinRoom(p2, code); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}

	h.SetInactive("Player 1", code)
	h.SetInactive("Player 2", code)
	time.Sleep(20 * time.Millisecond)

	p1re := client.NewClient("Player 1", code)
	if err := h.CreateRoom(p1re, code); err != nil {
		t.Fatalf("CreateRoom reconnect after both inactive: %v", err)
	}

	h.Send(message.Message{From: "Player 1", Room: code, Text: "p2 can still receive this later"})

	p2re := client.NewClient("Player 2", code)
	if err := h.JoinRoom(p2re, code); err != nil {
		t.Fatalf("JoinRoom reconnect after both inactive: %v", err)
	}

	msgs := drainMessages(p2re.Send, 1, time.Second)
	if len(msgs) != 1 || msgs[0].Text != "p2 can still receive this later" {
		t.Fatalf("expected buffered message for Player 2 after both inactive, got %v", msgs)
	}
}

func TestOfflineDoesNotBufferMessages(t *testing.T) {
	h, cancel := newTestHub(t)
	defer cancel()

	code, _ := setupRoom(t, h)
	p2 := client.NewClient("Player 2", code)
	if err := h.JoinRoom(p2, code); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}

	h.GoOffline("Player 2", code)
	time.Sleep(20 * time.Millisecond)

	h.Send(message.Message{From: "Player 1", Room: code, Text: "should not be stored"})

	p2re := client.NewClient("Player 2", code)
	if err := h.JoinRoom(p2re, code); err != nil {
		t.Fatalf("JoinRoom rejoin: %v", err)
	}

	msgs := drainMessages(p2re.Send, 1, 200*time.Millisecond)
	if len(msgs) != 0 {
		t.Fatalf("expected no buffered messages after offline, got %v", msgs)
	}
}

func TestRoomDeletedWhenBothOffline(t *testing.T) {
	h, cancel := newTestHub(t)
	defer cancel()

	code, _ := setupRoom(t, h)
	h.GoOffline("Player 1", code)
	time.Sleep(20 * time.Millisecond)

	p2 := client.NewClient("Player 2", code)
	err := h.JoinRoom(p2, code)
	if err == nil {
		t.Fatal("expected room gone after both offline")
	}
}

func TestReconnectPlayer1AfterInactive(t *testing.T) {
	h, cancel := newTestHub(t)
	defer cancel()

	code, _ := setupRoom(t, h)
	p2 := client.NewClient("Player 2", code)
	if err := h.JoinRoom(p2, code); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}

	h.SetInactive("Player 1", code)
	time.Sleep(20 * time.Millisecond)

	h.Send(message.Message{From: "Player 2", Room: code, Text: "for inactive p1"})

	p1re := client.NewClient("Player 1", code)
	if err := h.CreateRoom(p1re, code); err != nil {
		t.Fatalf("CreateRoom reconnect: %v", err)
	}

	msgs := drainMessages(p1re.Send, 1, time.Second)
	if len(msgs) != 1 || msgs[0].Text != "for inactive p1" {
		t.Fatalf("expected buffered message for P1, got %v", msgs)
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
