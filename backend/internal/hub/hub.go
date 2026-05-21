package hub

import (
	"backend/internal/client"
	"backend/internal/codegen"
	"backend/internal/event"
	"backend/internal/message"
	"backend/internal/room"
	"context"
	"fmt"
)

type roomEventKind int

const (
	kindReserveCode roomEventKind = iota // POST /rooms — reserve a code, no client yet
	kindCreateRoom                       // Player 1 WebSocket connects
	kindJoinRoom                         // Player 2 WebSocket connects
	kindLeaveRoom                        // a player disconnects
	kindMessage                          // chat message
)

type roomEvent struct {
	kind   roomEventKind
	client *client.Client
	code   string
	msg    message.Message
	reply  chan roomReply
}

type roomReply struct {
	code string
	err  error
}

type Hub struct {
	rooms  map[string]*room.Room
	events chan roomEvent
	Events chan event.Event
}

func New() *Hub {
	return &Hub{
		rooms:  make(map[string]*room.Room),
		events: make(chan roomEvent, 64),
		Events: make(chan event.Event, 64),
	}
}

// ── Public API ────────────────────────────────────────────────────────────────

// ReserveCode generates a room code and stores an empty pending room.
// Called from POST /rooms before the WebSocket connection is established.
func (h *Hub) ReserveCode() (string, error) {
	reply := make(chan roomReply, 1)
	h.events <- roomEvent{kind: kindReserveCode, reply: reply}
	r := <-reply
	return r.code, r.err
}

// CreateRoom registers Player 1 into a previously reserved room.
func (h *Hub) CreateRoom(c *client.Client, code string) error {
	reply := make(chan roomReply, 1)
	h.events <- roomEvent{kind: kindCreateRoom, client: c, code: code, reply: reply}
	r := <-reply
	return r.err
}

// JoinRoom registers Player 2 into an existing pending room.
func (h *Hub) JoinRoom(c *client.Client, code string) error {
	reply := make(chan roomReply, 1)
	h.events <- roomEvent{kind: kindJoinRoom, client: c, code: code, reply: reply}
	r := <-reply
	return r.err
}

// LeaveRoom removes a client from their room. Fire and forget.
func (h *Hub) LeaveRoom(name, code string) {
	h.events <- roomEvent{kind: kindLeaveRoom, client: &client.Client{Name: name}, code: code}
}

// Send routes a chat message. Fire and forget.
func (h *Hub) Send(m message.Message) {
	h.events <- roomEvent{kind: kindMessage, msg: m}
}

// BroadcastEventAll delivers an event to every connected player across all rooms.
// Called from eventFanout — reads h.rooms outside Run, intentional narrow exception.
func (h *Hub) BroadcastEventAll(e event.Event) {
	for _, r := range h.rooms {
		for _, p := range r.Players {
			if p != nil {
				select {
				case p.Send <- client.Outbound{Kind: client.OutboundEvent, Event: e}:
				default:
				}
			}
		}
	}
}

// ── Event loop ────────────────────────────────────────────────────────────────

func (h *Hub) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			h.shutdown()
			return
		case e := <-h.events:
			switch e.kind {
			case kindReserveCode:
				h.handleReserveCode(e)
			case kindCreateRoom:
				h.handleCreateRoom(e)
			case kindJoinRoom:
				h.handleJoinRoom(e)
			case kindLeaveRoom:
				h.handleLeave(e)
			case kindMessage:
				h.handleMessage(e.msg)
			}
		}
	}
}

func (h *Hub) shutdown() {
	for _, r := range h.rooms {
		for _, p := range r.Players {
			if p != nil {
				close(p.Send)
			}
		}
	}
}

// ── Handlers (called only from Run) ──────────────────────────────────────────

func (h *Hub) handleReserveCode(e roomEvent) {
	code := codegen.NewUnique(func(c string) bool {
		_, exists := h.rooms[c]
		return exists
	})
	// Store an empty room — Player 1 slot filled when WebSocket connects.
	h.rooms[code] = room.NewEmpty(code)
	h.logEvent(event.LevelInfo, fmt.Sprintf("room %s reserved", code))
	e.reply <- roomReply{code: code}
}

func (h *Hub) handleCreateRoom(e roomEvent) {
	r, ok := h.rooms[e.code]
	if !ok {
		e.reply <- roomReply{err: fmt.Errorf("room %s not found — code may have expired", e.code)}
		return
	}
	if r.Status != room.StatusPending {
		e.reply <- roomReply{err: fmt.Errorf("room %s is no longer available", e.code)}
		return
	}
	r.Players[0] = e.client
	h.logEvent(event.LevelSuccess, fmt.Sprintf("Player 1 joined room %s — waiting for Player 2", e.code))
	e.reply <- roomReply{}
}

func (h *Hub) handleJoinRoom(e roomEvent) {
	r, ok := h.rooms[e.code]
	if !ok {
		e.reply <- roomReply{err: fmt.Errorf("room %s not found — invalid or expired code", e.code)}
		return
	}
	if r.Status != room.StatusPending {
		e.reply <- roomReply{err: fmt.Errorf("room %s is already full or closed", e.code)}
		return
	}

	r.Join(e.client)
	h.logEvent(event.LevelSuccess, fmt.Sprintf("room %s is now active", e.code))

	// Flush messages buffered before Player 2 arrived.
	for _, m := range r.Pending {
		select {
		case e.client.Send <- client.Outbound{Kind: client.OutboundMessage, Message: m}:
		default:
		}
	}
	if len(r.Pending) > 0 {
		h.logEvent(event.LevelInfo, fmt.Sprintf("delivered %d buffered message(s) in room %s", len(r.Pending), e.code))
		r.Pending = nil
	}

	// Notify Player 1 their partner arrived.
	if r.Players[0] != nil {
		notify := event.New(event.LevelSuccess, "Player 2 joined — you can start chatting!")
		select {
		case r.Players[0].Send <- client.Outbound{Kind: client.OutboundEvent, Event: notify}:
		default:
		}
	}

	e.reply <- roomReply{}
}

func (h *Hub) handleLeave(e roomEvent) {
	r, ok := h.rooms[e.code]
	if !ok {
		return
	}

	other := r.Other(e.client.Name)
	if other != nil {
		notify := event.New(event.LevelWarn, "Your partner left the chat. The room is now closed.")
		select {
		case other.Send <- client.Outbound{Kind: client.OutboundEvent, Event: notify}:
		default:
		}
		close(other.Send)
	}

	r.Remove(e.client.Name)
	r.Status = room.StatusClosed
	delete(h.rooms, e.code)

	h.logEvent(event.LevelWarn, fmt.Sprintf("room %s closed", e.code))
}

func (h *Hub) handleMessage(m message.Message) {
	r, ok := h.rooms[m.Room]
	if !ok {
		return
	}

	recipient := r.Other(m.From)
	if recipient != nil {
		select {
		case recipient.Send <- client.Outbound{Kind: client.OutboundMessage, Message: m}:
		default:
			r.Pending = append(r.Pending, m)
			h.logEvent(event.LevelWarn, fmt.Sprintf("send buffer full in room %s", m.Room))
		}
	} else {
		r.Pending = append(r.Pending, m)
		h.logEvent(event.LevelInfo, fmt.Sprintf("buffered message in room %s (partner offline)", m.Room))
	}
}

func (h *Hub) logEvent(level event.Level, msg string) {
	e := event.New(level, msg)
	select {
	case h.Events <- e:
	default:
	}
}
