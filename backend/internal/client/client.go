package client

import "backend/internal/message"

type Client struct {
	Name string
	Send chan message.Message
}
