package hub

import (
	"backend/internal/client"
	"backend/internal/message"
)

type commandKind int

const (
	cmdReserveCode commandKind = iota
	cmdValidateCreate
	cmdValidateJoin
	cmdCreateRoom
	cmdJoinRoom
	cmdSetInactive
	cmdGoOffline
	cmdMessage
)

type command struct {
	kind   commandKind
	client *client.Client
	code   string
	msg    message.Message
	reply  chan reply
}

type reply struct {
	code string
	err  error
}
