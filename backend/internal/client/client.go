package client

import (
	"backend/internal/event"
	"backend/internal/message"
	"context"
	"encoding/json"
	"time"

	"github.com/gorilla/websocket"
)

const (
	inactivityTimeout = 10 * time.Second
	sendBufferSize    = 32
	writeWait         = 5 * time.Second
)

// OutboundKind distinguishes what is inside an Outbound envelope.
type OutboundKind int

const (
	OutboundMessage OutboundKind = iota
	OutboundEvent
)

// Outbound is a tagged union that carries either a chat message or a log event.
// WritePump is the only goroutine that reads from Send and writes to the WebSocket, both types flow through the same channel to preserve that guarantee.
type Outbound struct {
	Kind    OutboundKind
	Message message.Message
	Event   event.Event
}

type Client struct {
	Name string
	Send chan Outbound
}

type Sender interface {
	Send(m message.Message)
	Unregister(name string)
}

type Conn interface {
	ReadMessage() (messageType int, p []byte, err error)
	WriteJSON(v any) error
	WriteMessage(messageType int, data []byte) error
	SetWriteDeadline(t time.Time) error
	Close() error
}

func NewClient(name string) *Client {
	return &Client{
		Name: name,
		Send: make(chan Outbound, sendBufferSize),
	}
}

func (c *Client) ReadPump(ctx context.Context, conn Conn, hub Sender, emitEvent func(level, msg string)) {
	defer func() {
		hub.Unregister(c.Name)
		conn.Close()
	}()

	timer := time.AfterFunc(inactivityTimeout, func() {
		emitEvent("warn", c.Name+" timed out (inactivity)")
		conn.Close()
	})
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		_, raw, err := conn.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				emitEvent("error", c.Name+" read error: "+err.Error())
			}
			return
		}

		if !timer.Stop() {
		}
		timer.Reset(inactivityTimeout)

		var m message.Message
		if err := json.Unmarshal(raw, &m); err != nil {
			emitEvent("error", c.Name+" sent invalid JSON: "+err.Error())
			continue
		}

		m.From = c.Name
		m.To = otherUser(c.Name)

		hub.Send(m)
	}
}

func (c *Client) WritePump(ctx context.Context, conn Conn, emitEvent func(level, msg string)) {
	defer conn.Close()

	for {
		select {
		case <-ctx.Done():
			_ = conn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseGoingAway, "server shutdown"))
			return

		case outbound, ok := <-c.Send:
			if !ok {
				_ = conn.WriteMessage(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseGoingAway, "server shutdown — inactivity"))
				return
			}

			_ = conn.SetWriteDeadline(time.Now().Add(writeWait))

			var err error
			switch outbound.Kind {
			case OutboundMessage:
				err = conn.WriteJSON(outbound.Message)
			case OutboundEvent:
				err = conn.WriteJSON(outbound.Event)
			}

			if err != nil {
				emitEvent("error", c.Name+" write error: "+err.Error())
				return
			}
			_ = conn.SetWriteDeadline(time.Time{})
		}
	}
}

func otherUser(name string) string {
	if name == "alex" {
		return "bob"
	}
	return "alex"
}
