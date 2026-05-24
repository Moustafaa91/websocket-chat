package hub

import (
	"backend/internal/client"
	"backend/internal/event"
	"backend/internal/message"
	"backend/internal/room"
	"fmt"
)

func (h *Hub) Send(m message.Message) {
	h.commands <- command{kind: cmdMessage, msg: m}
}

func (h *Hub) handleMessage(m message.Message) {
	r, ok := h.rooms[m.Room]
	if !ok {
		return
	}

	recipientName := room.Partner(m.From)
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

func (h *Hub) flushPendingFor(r *room.Room, c *client.Client) {
	var kept []message.Message
	delivered := 0
	for _, m := range r.Pending {
		if room.Partner(m.From) == c.Name {
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
