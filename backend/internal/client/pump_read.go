package client

import (
	"backend/internal/message"
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/gorilla/websocket"
)

const inactivityTimeout = 10 * time.Second

func (c *Client) ReadPump(ctx context.Context, conn Conn, hub Hub, emitEvent func(level, msg string)) {
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
