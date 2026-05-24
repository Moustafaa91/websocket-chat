package client

import (
	"backend/internal/message"
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

const inactivityTimeout = 10 * time.Second

type closeMode int32

const (
	closeModeOffline closeMode = iota
	closeModeInactive
)

func (c *Client) ReadPump(ctx context.Context, conn Conn, hub Hub, emitEvent func(level, msg string)) {
	var mode atomic.Int32
	mode.Store(int32(closeModeOffline))

	defer func() {
		switch closeMode(mode.Load()) {
		case closeModeInactive:
			hub.SetInactive(c.Name, c.Room)
		default:
			hub.GoOffline(c.Name, c.Room)
		}
		conn.Close()
	}()

	timer := time.AfterFunc(inactivityTimeout, func() {
		emitEvent("warn", c.Name+" timed out (inactivity)")
		mode.Store(int32(closeModeInactive))
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
					mode.Store(int32(closeModeOffline))
				case "inactivity":
					mode.Store(int32(closeModeInactive))
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
