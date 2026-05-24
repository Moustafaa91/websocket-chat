package room

import (
	"backend/internal/client"
	"backend/internal/message"
)

type Status int

const (
	StatusPending Status = iota
	StatusActive
)

// Presence is per-player connection state within a room.
type Presence int

const (
	PresenceAbsent Presence = iota
	PresenceOnline
	PresenceInactive // idle timeout — WS closed, messages buffered
	PresenceOffline  // left or closed tab — no buffering
)

func (p Presence) String() string {
	switch p {
	case PresenceOnline:
		return "online"
	case PresenceInactive:
		return "inactive"
	case PresenceOffline:
		return "offline"
	default:
		return "absent"
	}
}

type Room struct {
	Code     string
	Status   Status
	Players  [2]*client.Client
	Presence [2]Presence
	Pending  []message.Message
}

func NewEmpty(code string) *Room {
	return &Room{
		Code:     code,
		Status:   StatusPending,
		Presence: [2]Presence{PresenceAbsent, PresenceAbsent},
	}
}

func (r *Room) Join(joiner *client.Client) {
	r.Players[1] = joiner
	r.Presence[1] = PresenceOnline
	r.Status = StatusActive
}

func (r *Room) Slot(name string) int {
	return Slot(name)
}

func (r *Room) Other(name string) *client.Client {
	if r.Players[0] != nil && r.Players[0].Name == name {
		return r.Players[1]
	}
	if r.Players[1] != nil && r.Players[1].Name == name {
		return r.Players[0]
	}
	return nil
}

func (r *Room) PresenceOf(name string) Presence {
	return r.Presence[r.Slot(name)]
}

func (r *Room) SetPresence(name string, p Presence) {
	r.Presence[r.Slot(name)] = p
}

func (r *Room) ClearSlot(name string) {
	r.Players[r.Slot(name)] = nil
}

// ShouldDelete reports whether the room can be removed from memory.
func (r *Room) ShouldDelete() bool {
	for _, p := range r.Presence {
		if p == PresenceOnline || p == PresenceInactive {
			return false
		}
	}
	return true
}

func (r *Room) PurgePendingFor(recipient string) {
	kept := r.Pending[:0]
	for _, m := range r.Pending {
		if Partner(m.From) != recipient {
			kept = append(kept, m)
		}
	}
	r.Pending = kept
}

func (r *Room) CanBufferFor(recipient string) bool {
	switch r.PresenceOf(recipient) {
	case PresenceOnline:
		return false
	case PresenceInactive, PresenceAbsent:
		return true
	default:
		return false
	}
}
