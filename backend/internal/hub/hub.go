package hub

import (
	"backend/internal/client"
	"backend/internal/event"
	"backend/internal/message"
	"context"
)

type hub struct {
	clients    map[string]*client.Client
	pending    map[string][]message.Message
	register   chan *client.Client
	unregister chan string
	messages   chan message.Message
	events     chan event.Event
}

func (h *hub) run(ctx context.Context) {

}

func (h *hub) logEvent(level event.Level, message string) {
	e := event.New(level, message)
	h.events <- e
}

func (h *hub) registerClient(c *client.Client) {

}

func (h *hub) unregisterClient(name string) {

}

func (h *hub) sendMessage(m message.Message) {

}
