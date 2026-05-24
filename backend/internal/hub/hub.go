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
	kindDisconnect                       // idle / connection lost — keep room
	kindLeaveRoom                        // explicit leave — close room
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

// Disconnect clears a player's slot but keeps the room (inactivity / reconnect).
func (h *Hub) Disconnect(name, code string) {
	h.events <- roomEvent{kind: kindDisconnect, client: &client.Client{Name: name}, code: code}
}

// LeaveRoom permanently closes the room. Fire and forget.
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
			case kindDisconnect:
				h.handleDisconnect(e)
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

	switch r.Status {
	case room.StatusActive:
		if r.Players[0] != nil {
			e.reply <- roomReply{err: fmt.Errorf("room %s already has Player 1 connected", e.code)}
			return
		}
		r.Players[0] = e.client
		h.flushPendingFor(r, e.client)
		h.logEvent(event.LevelSuccess, fmt.Sprintf("Player 1 reconnected to room %s", e.code))
		if r.Players[1] != nil {
			h.notifyPartnerBack(r, "Player 1")
		}
		e.reply <- roomReply{}
		return

	case room.StatusPending:
		r.Players[0] = e.client
		h.logEvent(event.LevelSuccess, fmt.Sprintf("Player 1 joined room %s — waiting for Player 2", e.code))
		e.reply <- roomReply{}
		return

	default:
		e.reply <- roomReply{err: fmt.Errorf("room %s is closed", e.code)}
	}
}

func (h *Hub) handleJoinRoom(e roomEvent) {
	r, ok := h.rooms[e.code]
	if !ok {
		e.reply <- roomReply{err: fmt.Errorf("room %s not found — invalid or expired code", e.code)}
		return
	}

	switch r.Status {
	case room.StatusActive:
		if r.Players[1] != nil {
			e.reply <- roomReply{err: fmt.Errorf("room %s already has Player 2 connected", e.code)}
			return
		}
		r.Players[1] = e.client
		h.flushPendingFor(r, e.client)
		h.logEvent(event.LevelSuccess, fmt.Sprintf("Player 2 reconnected to room %s", e.code))
		if r.Players[0] != nil {
			h.notifyPartnerBack(r, "Player 2")
		}
		e.reply <- roomReply{}
		return

	case room.StatusPending:
		r.Join(e.client)
		h.logEvent(event.LevelSuccess, fmt.Sprintf("room %s is now active", e.code))
		h.flushPendingFor(r, e.client)

		if r.Players[0] != nil {
			notify := event.New(event.LevelSuccess, "Player 2 joined — you can start chatting!")
			select {
			case r.Players[0].Send <- client.Outbound{Kind: client.OutboundEvent, Event: notify}:
			default:
			}
		}
		e.reply <- roomReply{}
		return

	default:
		e.reply <- roomReply{err: fmt.Errorf("room %s is closed", e.code)}
	}
}

func (h *Hub) handleDisconnect(e roomEvent) {
	r, ok := h.rooms[e.code]
	if !ok {
		return
	}

	other := r.Other(e.client.Name)
	r.Remove(e.client.Name)

	if other != nil {
		notify := event.New(event.LevelWarn, "Your partner is idle — messages will be delivered when they return")
		select {
		case other.Send <- client.Outbound{Kind: client.OutboundEvent, Event: notify}:
		default:
		}
	}

	h.logEvent(event.LevelInfo, fmt.Sprintf("%s disconnected from room %s (idle)", e.client.Name, e.code))
}

func (h *Hub) flushPendingFor(r *room.Room, c *client.Client) {
	var kept []message.Message
	delivered := 0
	for _, m := range r.Pending {
		if room.Recipient(m.From) == c.Name {
			select {
			case c.Send <- client.Outbound{Kind: client.OutboundMessage, Message: m}:
				delivered++
			default:
				kept = append(kept, m)
			}
		} else {
			kept = append(kept, m)
		}
	}
	r.Pending = kept
	if delivered > 0 {
		h.logEvent(event.LevelInfo, fmt.Sprintf("delivered %d buffered message(s) to %s in room %s", delivered, c.Name, r.Code))
	}
}

func (h *Hub) notifyPartnerBack(r *room.Room, back string) {
	other := r.Other(back)
	if other == nil {
		return
	}
	notify := event.New(event.LevelSuccess, back+" is back online")
	select {
	case other.Send <- client.Outbound{Kind: client.OutboundEvent, Event: notify}:
	default:
	}
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
