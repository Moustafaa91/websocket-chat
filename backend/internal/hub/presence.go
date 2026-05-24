package hub

import (
	"backend/internal/client"
	"backend/internal/event"
	"backend/internal/room"
	"fmt"
)

func (h *Hub) SetInactive(name, code string) {
	h.commands <- command{
		kind:   cmdSetInactive,
		client: &client.Client{Name: name},
		code:   code,
	}
}

func (h *Hub) GoOffline(name, code string) {
	h.commands <- command{
		kind:   cmdGoOffline,
		client: &client.Client{Name: name},
		code:   code,
	}
}

func (h *Hub) handleSetInactive(cmd command) {
	r, ok := h.rooms[cmd.code]
	if !ok {
		return
	}
	if r.PresenceOf(cmd.client.Name) == room.PresenceAbsent {
		return
	}

	r.ClearSlot(cmd.client.Name)
	r.SetPresence(cmd.client.Name, room.PresenceInactive)
	h.broadcastPresence(r, cmd.client.Name, room.PresenceInactive)
	h.logEvent(event.LevelInfo, cmd.code, fmt.Sprintf("%s is inactive in room %s", cmd.client.Name, cmd.code))
}

func (h *Hub) handleGoOffline(cmd command) {
	r, ok := h.rooms[cmd.code]
	if !ok {
		return
	}
	if r.PresenceOf(cmd.client.Name) == room.PresenceAbsent {
		return
	}

	r.ClearSlot(cmd.client.Name)
	r.SetPresence(cmd.client.Name, room.PresenceOffline)
	r.PurgePendingFor(cmd.client.Name)
	h.broadcastPresence(r, cmd.client.Name, room.PresenceOffline)
	h.logEvent(event.LevelInfo, cmd.code, fmt.Sprintf("%s is offline in room %s", cmd.client.Name, cmd.code))

	if r.ShouldDelete() {
		delete(h.rooms, cmd.code)
		h.logEvent(event.LevelInfo, cmd.code, fmt.Sprintf("room %s deleted (both players offline)", cmd.code))
	}
}
