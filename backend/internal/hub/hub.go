package hub

import (
	"backend/internal/client"
	"backend/internal/event"
	"backend/internal/message"
	"context"
	"fmt"
)

// Hub is the single goroutine that owns all shared state.
// All communication with the Hub happens through channels never by touching its fields directly from outside.
type Hub struct {
	clients    map[string]*client.Client
	pending    map[string][]message.Message
	register   chan *client.Client
	unregister chan string
	messages   chan message.Message
	Events     chan event.Event
}

func New() *Hub {
	return &Hub{
		clients:    make(map[string]*client.Client),
		pending:    make(map[string][]message.Message),
		register:   make(chan *client.Client),
		unregister: make(chan string),
		messages:   make(chan message.Message),
		Events:     make(chan event.Event, 64),
	}
}

func (h *Hub) Register(c *client.Client) {
	h.register <- c
}

func (h *Hub) Unregister(name string) {
	h.unregister <- name
}

func (h *Hub) Send(m message.Message) {
	h.messages <- m
}

func (h *Hub) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			for name, c := range h.clients {
				close(c.Send)
				delete(h.clients, name)
			}
			return
		case c := <-h.register:
			h.registerClient(c)
		case name := <-h.unregister:
			h.unregisterClient(name)
		case m := <-h.messages:
			h.routeMessage(m)
		}
	}
}

func (h *Hub) registerClient(c *client.Client) {
	if existing, ok := h.clients[c.Name]; ok {
		close(existing.Send)
		h.logEvent(event.LevelWarn, fmt.Sprintf("%s reconnected (replaced stale entry)", c.Name))
	}

	h.clients[c.Name] = c
	h.logEvent(event.LevelSuccess, fmt.Sprintf("%s connected", c.Name))

	if msgs, ok := h.pending[c.Name]; ok && len(msgs) > 0 {
		for _, m := range msgs {
			select {
			case c.Send <- client.Outbound{Kind: client.OutboundMessage, Message: m}:
			default:
				h.logEvent(event.LevelWarn, fmt.Sprintf("dropped buffered message for %s: send buffer full", c.Name))
			}
		}
		h.logEvent(event.LevelInfo, fmt.Sprintf("delivered %d buffered message(s) to %s", len(msgs), c.Name))
		delete(h.pending, c.Name)
	}
}

func (h *Hub) unregisterClient(name string) {
	c, ok := h.clients[name]
	if !ok {
		return
	}

	delete(h.clients, name)

drainLoop:
	for {
		select {
		case outbound, ok := <-c.Send:
			if !ok {
				break drainLoop
			}
			// Only drain chat messages into pending — events are ephemeral.
			if outbound.Kind == client.OutboundMessage {
				h.pending[name] = append(h.pending[name], outbound.Message)
			}
		default:
			break drainLoop
		}
	}

	close(c.Send)
	h.logEvent(event.LevelWarn, fmt.Sprintf("%s disconnected", name))
}

func (h *Hub) routeMessage(m message.Message) {
	recipient, ok := h.clients[m.To]
	if ok {
		select {
		case recipient.Send <- client.Outbound{Kind: client.OutboundMessage, Message: m}:
		default:
			h.logEvent(event.LevelWarn, fmt.Sprintf("send buffer full for %s, buffering message", m.To))
			h.pending[m.To] = append(h.pending[m.To], m)
		}
	} else {
		h.pending[m.To] = append(h.pending[m.To], m)
		h.logEvent(event.LevelInfo, fmt.Sprintf("buffered message for offline user %s", m.To))
	}
}

func (h *Hub) logEvent(level event.Level, msg string) {
	e := event.New(level, msg)
	select {
	case h.Events <- e:
	default:
	}
}

// BroadcastEvent delivers a log event to every currently connected client by routing it through each client's Send channel.
// Called from the eventFanout goroutine in main.go.
// Safe to call from outside the Hub's Run goroutine because it only reads h.clients and it is the only caller outside Run that does so.
func (h *Hub) BroadcastEvent(e event.Event) {
	for _, c := range h.clients {
		select {
		case c.Send <- client.Outbound{Kind: client.OutboundEvent, Event: e}:
		default:
			// Client's send buffer is full, drop the event rather than block.
		}
	}
}
