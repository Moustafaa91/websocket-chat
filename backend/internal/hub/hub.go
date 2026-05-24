package hub

import (
	"backend/internal/event"
	"backend/internal/room"
	"context"
)

// Hub owns all room state and processes mutations on a single goroutine.
// External callers interact only through the exported methods, which enqueue commands.
type Hub struct {
	rooms    map[string]*room.Room
	commands chan command
	Events   chan event.Event
}

func New() *Hub {
	return &Hub{
		rooms:    make(map[string]*room.Room),
		commands: make(chan command, 64),
		Events:   make(chan event.Event, 64),
	}
}

func (h *Hub) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			h.shutdown()
			return
		case cmd := <-h.commands:
			h.dispatch(cmd)
		}
	}
}

func (h *Hub) dispatch(cmd command) {
	switch cmd.kind {
	case cmdReserveCode:
		h.handleReserveCode(cmd)
	case cmdValidateCreate:
		h.handleValidateCreate(cmd)
	case cmdValidateJoin:
		h.handleValidateJoin(cmd)
	case cmdCreateRoom:
		h.handleCreateRoom(cmd)
	case cmdJoinRoom:
		h.handleJoinRoom(cmd)
	case cmdSetInactive:
		h.handleSetInactive(cmd)
	case cmdGoOffline:
		h.handleGoOffline(cmd)
	case cmdMessage:
		h.handleMessage(cmd.msg)
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
