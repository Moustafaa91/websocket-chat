package client

import (
	"context"
	"time"

	"github.com/gorilla/websocket"
)

const writeWait = 5 * time.Second

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
