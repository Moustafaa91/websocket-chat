package client

import "time"

// Conn abstracts the WebSocket connection for testability.
type Conn interface {
	ReadMessage() (messageType int, p []byte, err error)
	WriteJSON(v any) error
	WriteMessage(messageType int, data []byte) error
	SetWriteDeadline(t time.Time) error
	Close() error
}
