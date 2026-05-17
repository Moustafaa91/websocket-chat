package client

import (
	"backend/internal/message"
	"context"
	"encoding/json"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// How long a connection may be idle before the server closes it.
	inactivityTimeout = 10 * time.Second

	// The number of messages that can be queued in a client's Send channel before back-pressure kicks in.
	sendBufferSize = 32

	// The maximum time to wait for a WebSocket write to complete.
	writeWait = 5 * time.Second
)

type Client struct {
	Name string
	Send chan message.Message
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
		Send: make(chan message.Message, sendBufferSize),
	}
}

// ReadPump pumps inbound messages from the WebSocket to the Hub.
//
// One goroutine per connection. Exits when:
//   - the WebSocket is closed (clean or error)
//   - ctx is cancelled (server shutdown)
//   - the inactivity timer fires
//
// On exit it always calls hub.Unregister so the Hub can clean up.
func (c *Client) ReadPump(ctx context.Context, conn Conn, hub Sender, emitEvent func(level, msg string)) {
	defer func() {
		hub.Unregister(c.Name)
		conn.Close()
	}()

	// inactivityTimer fires if no message is received within the timeout window.
	// We use time.AfterFunc so the timer callback runs in its own goroutine and
	// simply closes the connection — ReadMessage will then return an error and
	// the pump exits naturally.
	timer := time.AfterFunc(inactivityTimeout, func() {
		emitEvent("warn", c.Name+" timed out (inactivity)")
		conn.Close()
	})
	defer timer.Stop()

	for {
		// Check for server shutdown before blocking on ReadMessage.
		select {
		case <-ctx.Done():
			return
		default:
		}

		_, raw, err := conn.ReadMessage()
		if err != nil {
			// Normal close (1000/1001) or unexpected network error —
			// either way, exit. The defer will unregister and close.
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				emitEvent("error", c.Name+" read error: "+err.Error())
			}
			return
		}

		// Reset the inactivity timer on every received message.
		// Per time.Timer docs: Stop before Reset; discard the channel if needed.
		if !timer.Stop() {
			// If AfterFunc fired, the function already ran (or is running).
			// We can't drain a channel here (AfterFunc doesn't use one),
			// so just reset — worst case the conn was already closed and
			// the next ReadMessage will return an error.
		}
		timer.Reset(inactivityTimeout)

		var m message.Message
		if err := json.Unmarshal(raw, &m); err != nil {
			emitEvent("error", c.Name+" sent invalid JSON: "+err.Error())
			continue
		}

		// Enforce sender identity — the client cannot spoof another user.
		m.From = c.Name

		// Derive recipient: the other fixed user.
		m.To = otherUser(c.Name)

		hub.Send(m)
	}
}

// WritePump pumps outbound messages from the Hub (via Send channel) to the WebSocket.
//
// One goroutine per connection. Exits when:
//   - c.Send is closed by the Hub (clean shutdown or unregister)
//   - a write error occurs
func (c *Client) WritePump(ctx context.Context, conn Conn, emitEvent func(level, msg string)) {
	defer conn.Close()

	for {
		select {
		case <-ctx.Done():
			// Server shutting down — send a close frame and exit.
			_ = conn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseGoingAway, "server shutdown"))
			return

		case m, ok := <-c.Send:
			if !ok {
				// Hub closed the channel — send a close frame.
				_ = conn.WriteMessage(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
				return
			}

			_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := conn.WriteJSON(m); err != nil {
				emitEvent("error", c.Name+" write error: "+err.Error())
				return
			}
			// Clear the deadline after a successful write.
			_ = conn.SetWriteDeadline(time.Time{})
		}
	}
}

// temporary helper for this demo
// TODO: replace with real user management
func otherUser(name string) string {
	if name == "alex" {
		return "bob"
	}
	return "alex"
}
