package client

import (
	"backend/internal/event"
	"backend/internal/message"
)

const sendBufferSize = 64

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

// Client represents one player's WebSocket session.
type Client struct {
	Name string
	Room string
	Send chan Outbound
}

// Hub exposes the operations the read pump needs from the room hub.
type Hub interface {
	Send(m message.Message)
	SetInactive(name, code string)
	GoOffline(name, code string)
}

func NewClient(name, roomCode string) *Client {
	return &Client{
		Name: name,
		Room: roomCode,
		Send: make(chan Outbound, sendBufferSize),
	}
}
