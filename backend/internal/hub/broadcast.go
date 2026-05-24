package hub

import (
	"backend/internal/client"
	"backend/internal/event"
	"backend/internal/room"
)

func (h *Hub) BroadcastEvent(e event.Event) {
	if e.Room != "" {
		r, ok := h.rooms[e.Room]
		if !ok {
			return
		}
		h.deliverEvent(r, e)
		return
	}
	for _, r := range h.rooms {
		h.deliverEvent(r, e)
	}
}

func (h *Hub) deliverEvent(r *room.Room, e event.Event) {
	for _, p := range r.Players {
		if p == nil {
			continue
		}
		select {
		case p.Send <- client.Outbound{Kind: client.OutboundEvent, Event: e}:
		default:
		}
	}
}

func (h *Hub) broadcastPresence(r *room.Room, playerName string, p room.Presence) {
	e := event.NewPresence(r.Code, playerName, p.String())
	h.deliverEvent(r, e)
}

func (h *Hub) logEvent(level event.Level, roomCode, msg string) {
	e := event.NewRoom(level, roomCode, msg)
	select {
	case h.Events <- e:
	default:
	}
}
