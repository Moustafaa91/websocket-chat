package room

import (
	"backend/internal/client"
	"backend/internal/message"
)

type Status int

const (
	StatusPending Status = iota
	StatusActive
	StatusClosed
)

type Room struct {
	Code    string
	Status  Status
	Players [2]*client.Client // [0] = Player 1 (creator), [1] = Player 2 (joiner)
	Pending []message.Message
}

// NewEmpty creates a reserved room with no players yet.
// Player 1 is assigned when their WebSocket connects.
func NewEmpty(code string) *Room {
	return &Room{
		Code:   code,
		Status: StatusPending,
	}
}

// Join assigns Player 2 and activates the room.
func (r *Room) Join(joiner *client.Client) {
	r.Players[1] = joiner
	r.Status = StatusActive
}

// Other returns the other player given one player's name. Returns nil if
// the partner has not connected yet or has already left.
func (r *Room) Other(name string) *client.Client {
	if r.Players[0] != nil && r.Players[0].Name == name {
		return r.Players[1]
	}
	if r.Players[1] != nil && r.Players[1].Name == name {
		return r.Players[0]
	}
	return nil
}

// Remove clears the slot for the named player.
func (r *Room) Remove(name string) {
	for i, p := range r.Players {
		if p != nil && p.Name == name {
			r.Players[i] = nil
		}
	}
}
