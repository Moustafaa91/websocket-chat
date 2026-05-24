package hub

import (
	"backend/internal/codegen"
	"backend/internal/event"
	"backend/internal/room"
	"fmt"
)

func (h *Hub) ReserveCode() (string, error) {
	ch := make(chan reply, 1)
	h.commands <- command{kind: cmdReserveCode, reply: ch}
	r := <-ch
	return r.code, r.err
}

func (h *Hub) handleReserveCode(cmd command) {
	code := codegen.NewUnique(func(c string) bool {
		_, exists := h.rooms[c]
		return exists
	})
	h.rooms[code] = room.NewEmpty(code)
	h.logEvent(event.LevelInfo, code, fmt.Sprintf("room %s reserved", code))
	cmd.reply <- reply{code: code}
}
