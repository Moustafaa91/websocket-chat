package hub

import (
	"backend/internal/client"
	"backend/internal/event"
	"backend/internal/room"
	"fmt"
)

func (h *Hub) ValidateCreate(code string) error {
	ch := make(chan reply, 1)
	h.commands <- command{kind: cmdValidateCreate, code: code, reply: ch}
	return (<-ch).err
}

func (h *Hub) ValidateJoin(code string) error {
	ch := make(chan reply, 1)
	h.commands <- command{kind: cmdValidateJoin, code: code, reply: ch}
	return (<-ch).err
}

func (h *Hub) CreateRoom(c *client.Client, code string) error {
	ch := make(chan reply, 1)
	h.commands <- command{kind: cmdCreateRoom, client: c, code: code, reply: ch}
	return (<-ch).err
}

func (h *Hub) JoinRoom(c *client.Client, code string) error {
	ch := make(chan reply, 1)
	h.commands <- command{kind: cmdJoinRoom, client: c, code: code, reply: ch}
	return (<-ch).err
}

func (h *Hub) handleValidateCreate(cmd command) {
	cmd.reply <- reply{err: h.checkCreate(cmd.code, nil)}
}

func (h *Hub) handleValidateJoin(cmd command) {
	cmd.reply <- reply{err: h.checkJoin(cmd.code, nil)}
}

func (h *Hub) handleCreateRoom(cmd command) {
	if err := h.checkCreate(cmd.code, cmd.client); err != nil {
		cmd.reply <- reply{err: err}
		return
	}

	r := h.rooms[cmd.code]
	idx := r.Slot(cmd.client.Name)
	prev := r.Presence[idx]

	if prev == room.PresenceOffline {
		r.PurgePendingFor(cmd.client.Name)
	}

	r.Players[idx] = cmd.client
	r.SetPresence(cmd.client.Name, room.PresenceOnline)

	switch r.Status {
	case room.StatusPending:
		h.logEvent(event.LevelSuccess, cmd.code, fmt.Sprintf("Player 1 joined room %s — waiting for Player 2", cmd.code))
	case room.StatusActive:
		if prev == room.PresenceInactive {
			h.flushPendingFor(r, cmd.client)
			h.logEvent(event.LevelSuccess, cmd.code, fmt.Sprintf("Player 1 reconnected to room %s", cmd.code))
		} else if prev == room.PresenceOffline {
			h.logEvent(event.LevelSuccess, cmd.code, fmt.Sprintf("Player 1 rejoined room %s", cmd.code))
		}
	}

	h.broadcastPresence(r, cmd.client.Name, room.PresenceOnline)
	cmd.reply <- reply{}
}

func (h *Hub) handleJoinRoom(cmd command) {
	if err := h.checkJoin(cmd.code, cmd.client); err != nil {
		cmd.reply <- reply{err: err}
		return
	}

	r := h.rooms[cmd.code]
	idx := r.Slot(cmd.client.Name)
	prev := r.Presence[idx]

	switch r.Status {
	case room.StatusPending:
		r.Join(cmd.client)
		h.logEvent(event.LevelSuccess, cmd.code, fmt.Sprintf("room %s is now active", cmd.code))
		h.flushPendingFor(r, cmd.client)
		h.broadcastPresence(r, room.Player2, room.PresenceOnline)
		cmd.reply <- reply{}
		return

	case room.StatusActive:
		if prev == room.PresenceOffline {
			r.PurgePendingFor(cmd.client.Name)
		}

		r.Players[idx] = cmd.client
		r.SetPresence(cmd.client.Name, room.PresenceOnline)

		if prev == room.PresenceInactive {
			h.flushPendingFor(r, cmd.client)
			h.logEvent(event.LevelSuccess, cmd.code, fmt.Sprintf("Player 2 reconnected to room %s", cmd.code))
		} else {
			h.logEvent(event.LevelSuccess, cmd.code, fmt.Sprintf("Player 2 rejoined room %s", cmd.code))
		}
		h.broadcastPresence(r, room.Player2, room.PresenceOnline)
		cmd.reply <- reply{}
	}
}

func (h *Hub) checkCreate(code string, c *client.Client) error {
	r, ok := h.rooms[code]
	if !ok {
		return fmt.Errorf("room %s not found — code may have expired", code)
	}
	if c == nil {
		return nil
	}
	if r.Slot(c.Name) != 0 {
		return fmt.Errorf("create is only for Player 1")
	}
	if r.Players[0] != nil {
		return fmt.Errorf("room %s already has Player 1 connected", code)
	}
	return nil
}

func (h *Hub) checkJoin(code string, c *client.Client) error {
	r, ok := h.rooms[code]
	if !ok {
		return fmt.Errorf("room %s not found — invalid or expired code", code)
	}
	if c == nil {
		return nil
	}
	if r.Slot(c.Name) != 1 {
		return fmt.Errorf("join is only for Player 2")
	}
	if r.Players[1] != nil {
		return fmt.Errorf("room %s already has Player 2 connected", code)
	}
	if r.Status == room.StatusActive && r.Presence[1] == room.PresenceOnline {
		return fmt.Errorf("room %s already has Player 2 connected", code)
	}
	return nil
}
