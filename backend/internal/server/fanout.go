package server

import (
	"backend/internal/hub"
	"context"
)

// runEventFanout delivers hub log events to all connected players.
func runEventFanout(ctx context.Context, h *hub.Hub) {
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-h.Events:
			h.BroadcastEvent(e)
		}
	}
}
