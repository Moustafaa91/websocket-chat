package client

import (
	"backend/internal/event"
	"backend/internal/message"
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/gorilla/websocket"
)

const (
	inactivityTimeout = 10 * time.Second
	sendBufferSize    = 64
	writeWait         = 5 * time.Second
)

type OutboundKind int

const (
	OutboundMessage OutboundKind = iota
	OutboundEvent
)

type Outbound struct {
	Kind    OutboundKind
	Message message.Message
	Event   event.Event
}

type Client struct {
	Name string
	Room string
	Send chan Outbound
}

type RoomSender interface {
	Send(m message.Message)
	SetInactive(name, code string)
	GoOffline(name, code string)
}

type Conn interface {
	ReadMessage() (messageType int, p []byte, err error)
	WriteJSON(v any) error
	WriteMessage(messageType int, data []byte) error
	SetWriteDeadline(t time.Time) error
	Close() error
}

func NewClient(name, roomCode string) *Client {
	return &Client{
		Name: name,
		Room: roomCode,
		Send: make(chan Outbound, sendBufferSize),
	}
}

func (c *Client) ReadPump(ctx context.Context, conn Conn, hub RoomSender, emitEvent func(level, msg string)) {
	inactiveClose := false
	goOffline := false

	defer func() {
		switch {
		case inactiveClose:
			hub.SetInactive(c.Name, c.Room)
		case goOffline:
			hub.GoOffline(c.Name, c.Room)
		default:
			hub.GoOffline(c.Name, c.Room)
		}
		conn.Close()
	}()

	timer := time.AfterFunc(inactivityTimeout, func() {
		emitEvent("warn", c.Name+" timed out (inactivity)")
		inactiveClose = true
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
			var closeErr *websocket.CloseError
			if errors.As(err, &closeErr) {
				switch closeErr.Text {
				case "user left", "offline":
					goOffline = true
				case "inactivity":
					inactiveClose = true
				}
			} else if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
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

		if m.Ping {
			continue
		}

		m.From = c.Name
		m.Room = c.Room

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
					websocket.FormatCloseMessage(websocket.CloseGoingAway, "room closed"))
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
