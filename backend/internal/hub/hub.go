package hub

import (
	"backend/internal/client"
	"backend/internal/event"
	"backend/internal/message"
	"context"
	"fmt"
)

// Hub is the single goroutine that owns all shared state.
// All communication with the Hub happens through channels — never by
// touching its fields directly from outside.
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

// Register sends a client registration request to the Hub.
// Called from the HTTP handler goroutine (outside the Hub).
func (h *Hub) Register(c *client.Client) {
	h.register <- c
}

// Unregister sends a client unregistration request to the Hub.
// Called from the client's read pump goroutine (outside the Hub).
func (h *Hub) Unregister(name string) {
	h.unregister <- name
}

// Send delivers a message to the Hub for routing.
// Called from the client's read pump goroutine (outside the Hub).
func (h *Hub) Send(m message.Message) {
	h.messages <- m
}

// Run is the Hub's event loop. It must be called in its own goroutine.
// It exits cleanly when ctx is cancelled.
func (h *Hub) Run(ctx context.Context) {
	for {
		select {

		case <-ctx.Done():
			// Server is shutting down. Close every active client's Send
			// channel so their write pumps drain and exit cleanly.
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

// registerClient is called only from within Run — safe to touch hub fields directly.
func (h *Hub) registerClient(c *client.Client) {
	if existing, ok := h.clients[c.Name]; ok {
		// A stale entry exists (e.g. crash without clean unregister).
		// Close the old Send channel so the old write pump exits, then
		// replace with the fresh client.
		close(existing.Send)
		h.logEvent(event.LevelWarn, fmt.Sprintf("%s reconnected (replaced stale entry)", c.Name))
	}

	h.clients[c.Name] = c
	h.logEvent(event.LevelSuccess, fmt.Sprintf("%s connected", c.Name))

	// Flush any buffered messages accumulated while the client was away.
	if msgs, ok := h.pending[c.Name]; ok && len(msgs) > 0 {
		for _, m := range msgs {
			// Non-blocking send: if the Send buffer is somehow full, drop
			// rather than deadlock. The channel is buffered (see client.go).
			select {
			case c.Send <- m:
			default:
				// Log and drop — better than blocking the Hub goroutine.
				h.logEvent(event.LevelWarn, fmt.Sprintf("dropped buffered message for %s: send buffer full", c.Name))
			}
		}
		h.logEvent(event.LevelInfo, fmt.Sprintf("delivered %d buffered message(s) to %s", len(msgs), c.Name))
		delete(h.pending, c.Name)
	}
}

// unregisterClient is called only from within Run — safe to touch hub fields directly.
func (h *Hub) unregisterClient(name string) {
	c, ok := h.clients[name]
	if !ok {
		// Already gone (double-unregister is possible if both sides race).
		return
	}

	// Step 1: remove from the routing table FIRST.
	// Once deleted, no new messages will be directed to this client's Send
	// channel, so closing it below is safe.
	delete(h.clients, name)

	// Step 2: drain any messages already sitting in Send into pending.
	// These were queued before the disconnect was noticed — we must not
	// lose them.
drainLoop:
	for {
		select {
		case m, ok := <-c.Send:
			if !ok {
				break drainLoop
			}
			h.pending[name] = append(h.pending[name], m)
		default:
			break drainLoop
		}
	}

	// Step 3: close the channel to signal the write pump to exit.
	close(c.Send)

	h.logEvent(event.LevelWarn, fmt.Sprintf("%s disconnected", name))
}

// routeMessage is called only from within Run — safe to touch hub fields directly.
func (h *Hub) routeMessage(m message.Message) {
	recipient, ok := h.clients[m.To]
	if ok {
		// Recipient is online — deliver directly.
		select {
		case recipient.Send <- m:
		default:
			// Send buffer full — buffer the message for later delivery.
			h.logEvent(event.LevelWarn, fmt.Sprintf("send buffer full for %s, buffering message", m.To))
			h.pending[m.To] = append(h.pending[m.To], m)
		}
	} else {
		// Recipient is offline — buffer for delivery on reconnect.
		h.pending[m.To] = append(h.pending[m.To], m)
		h.logEvent(event.LevelInfo, fmt.Sprintf("buffered message for offline user %s", m.To))
	}
}

// logEvent emits a structured event on the Events channel.
// Non-blocking: if the consumer (main.go) is slow, we drop rather than
// blocking the Hub goroutine.
func (h *Hub) logEvent(level event.Level, msg string) {
	e := event.New(level, msg)
	select {
	case h.Events <- e:
	default:
	}
}
