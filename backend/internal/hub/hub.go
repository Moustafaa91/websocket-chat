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
	kindReserveCode roomEventKind = iota
	kindCreateRoom
	kindJoinRoom
	kindSetInactive // idle — keep room, buffer messages
	kindGoOffline   // leave or closed tab — no buffering
	kindMessage
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

func (h *Hub) ReserveCode() (string, error) {
	reply := make(chan roomReply, 1)
	h.events <- roomEvent{kind: kindReserveCode, reply: reply}
	r := <-reply
	return r.code, r.err
}

func (h *Hub) CreateRoom(c *client.Client, code string) error {
	reply := make(chan roomReply, 1)
	h.events <- roomEvent{kind: kindCreateRoom, client: c, code: code, reply: reply}
	r := <-reply
	return r.err
}

func (h *Hub) JoinRoom(c *client.Client, code string) error {
	reply := make(chan roomReply, 1)
	h.events <- roomEvent{kind: kindJoinRoom, client: c, code: code, reply: reply}
	r := <-reply
	return r.err
}

func (h *Hub) SetInactive(name, code string) {
	h.events <- roomEvent{kind: kindSetInactive, client: &client.Client{Name: name}, code: code}
}

func (h *Hub) GoOffline(name, code string) {
	h.events <- roomEvent{kind: kindGoOffline, client: &client.Client{Name: name}, code: code}
}

func (h *Hub) Send(m message.Message) {
	h.events <- roomEvent{kind: kindMessage, msg: m}
}

func (h *Hub) BroadcastEvent(e event.Event) {
	if e.Room != "" {
		r, ok := h.rooms[e.Room]
		if !ok {
			return
		}
		h.sendEventToRoom(r, e)
		return
	}
	for _, r := range h.rooms {
		h.sendEventToRoom(r, e)
	}
}

func (h *Hub) sendEventToRoom(r *room.Room, e event.Event) {
	for _, p := range r.Players {
		if p != nil {
			select {
			case p.Send <- client.Outbound{Kind: client.OutboundEvent, Event: e}:
			default:
			}
		}
	}
}

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
			case kindSetInactive:
				h.handleSetInactive(e)
			case kindGoOffline:
				h.handleGoOffline(e)
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

func (h *Hub) handleReserveCode(e roomEvent) {
	code := codegen.NewUnique(func(c string) bool {
		_, exists := h.rooms[c]
		return exists
	})
	h.rooms[code] = room.NewEmpty(code)
	h.logEvent(event.LevelInfo, code, fmt.Sprintf("room %s reserved", code))
	e.reply <- roomReply{code: code}
}

func (h *Hub) handleCreateRoom(e roomEvent) {
	r, ok := h.rooms[e.code]
	if !ok {
		e.reply <- roomReply{err: fmt.Errorf("room %s not found — code may have expired", e.code)}
		return
	}

	idx := r.Index(e.client.Name)
	if idx != 0 {
		e.reply <- roomReply{err: fmt.Errorf("CreateRoom is only for Player 1")}
		return
	}

	if r.Players[idx] != nil {
		e.reply <- roomReply{err: fmt.Errorf("room %s already has Player 1 connected", e.code)}
		return
	}

	prev := r.Presence[idx]
	if prev == room.PresenceOffline {
		r.PurgePendingFor(e.client.Name)
	}

	r.Players[idx] = e.client
	r.SetPresence(e.client.Name, room.PresenceOnline)

	switch r.Status {
	case room.StatusPending:
		h.logEvent(event.LevelSuccess, e.code, fmt.Sprintf("Player 1 joined room %s — waiting for Player 2", e.code))
	case room.StatusActive:
		if prev == room.PresenceInactive {
			h.flushPendingFor(r, e.client)
			h.logEvent(event.LevelSuccess, e.code, fmt.Sprintf("Player 1 reconnected to room %s", e.code))
		} else if prev == room.PresenceOffline {
			h.logEvent(event.LevelSuccess, e.code, fmt.Sprintf("Player 1 rejoined room %s", e.code))
		}
	}

	h.broadcastPresence(r, e.client.Name, room.PresenceOnline)
	e.reply <- roomReply{}
}

func (h *Hub) handleJoinRoom(e roomEvent) {
	r, ok := h.rooms[e.code]
	if !ok {
		e.reply <- roomReply{err: fmt.Errorf("room %s not found — invalid or expired code", e.code)}
		return
	}

	idx := r.Index(e.client.Name)
	if idx != 1 {
		e.reply <- roomReply{err: fmt.Errorf("JoinRoom is only for Player 2")}
		return
	}

	if r.Players[idx] != nil {
		e.reply <- roomReply{err: fmt.Errorf("room %s already has Player 2 connected", e.code)}
		return
	}

	prev := r.Presence[idx]

	switch r.Status {
	case room.StatusPending:
		r.Join(e.client)
		h.logEvent(event.LevelSuccess, e.code, fmt.Sprintf("room %s is now active", e.code))
		h.flushPendingFor(r, e.client)
		h.broadcastPresence(r, "Player 2", room.PresenceOnline)
		e.reply <- roomReply{}
		return

	case room.StatusActive:
		switch prev {
		case room.PresenceOnline:
			e.reply <- roomReply{err: fmt.Errorf("room %s already has Player 2 connected", e.code)}
			return
		case room.PresenceOffline:
			r.PurgePendingFor(e.client.Name)
		}

		r.Players[idx] = e.client
		r.SetPresence(e.client.Name, room.PresenceOnline)

		if prev == room.PresenceInactive {
			h.flushPendingFor(r, e.client)
			h.logEvent(event.LevelSuccess, e.code, fmt.Sprintf("Player 2 reconnected to room %s", e.code))
		} else {
			h.logEvent(event.LevelSuccess, e.code, fmt.Sprintf("Player 2 rejoined room %s", e.code))
		}
		h.broadcastPresence(r, "Player 2", room.PresenceOnline)
		e.reply <- roomReply{}
		return

	}
}

func (h *Hub) handleSetInactive(e roomEvent) {
	r, ok := h.rooms[e.code]
	if !ok {
		return
	}

	if r.PresenceOf(e.client.Name) == room.PresenceAbsent {
		return
	}

	r.ClearSlot(e.client.Name)
	r.SetPresence(e.client.Name, room.PresenceInactive)
	h.broadcastPresence(r, e.client.Name, room.PresenceInactive)
	h.logEvent(event.LevelInfo, e.code, fmt.Sprintf("%s is inactive in room %s", e.client.Name, e.code))
}

func (h *Hub) handleGoOffline(e roomEvent) {
	r, ok := h.rooms[e.code]
	if !ok {
		return
	}

	if r.PresenceOf(e.client.Name) == room.PresenceAbsent {
		return
	}

	r.ClearSlot(e.client.Name)
	r.SetPresence(e.client.Name, room.PresenceOffline)
	r.PurgePendingFor(e.client.Name)
	h.broadcastPresence(r, e.client.Name, room.PresenceOffline)
	h.logEvent(event.LevelInfo, e.code, fmt.Sprintf("%s is offline in room %s", e.client.Name, e.code))

	if r.ShouldDelete() {
		delete(h.rooms, e.code)
		h.logEvent(event.LevelInfo, e.code, fmt.Sprintf("room %s deleted (both players offline)", e.code))
	}
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
		h.logEvent(event.LevelInfo, r.Code, fmt.Sprintf("delivered %d buffered message(s) to %s in room %s", delivered, c.Name, r.Code))
	}
}

func (h *Hub) broadcastPresence(r *room.Room, playerName string, p room.Presence) {
	e := event.NewPresence(r.Code, playerName, p.String())
	h.sendEventToRoom(r, e)
}

func (h *Hub) handleMessage(m message.Message) {
	r, ok := h.rooms[m.Room]
	if !ok {
		return
	}

	recipientName := room.Recipient(m.From)
	recipient := r.Other(m.From)

	if recipient != nil {
		select {
		case recipient.Send <- client.Outbound{Kind: client.OutboundMessage, Message: m}:
		default:
			r.Pending = append(r.Pending, m)
			h.logEvent(event.LevelWarn, m.Room, fmt.Sprintf("send buffer full in room %s", m.Room))
		}
		return
	}

	if r.CanBufferFor(recipientName) {
		r.Pending = append(r.Pending, m)
		h.logEvent(event.LevelInfo, m.Room, fmt.Sprintf("buffered message in room %s (%s unavailable)", m.Room, recipientName))
		return
	}

	h.logEvent(event.LevelInfo, m.Room, fmt.Sprintf("dropped message in room %s (%s is offline)", m.Room, recipientName))
}

func (h *Hub) logEvent(level event.Level, roomCode, msg string) {
	e := event.NewRoom(level, roomCode, msg)
	select {
	case h.Events <- e:
	default:
	}
}
