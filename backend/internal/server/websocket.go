package server

import (
	"backend/internal/client"
	"backend/internal/event"
	"backend/internal/hub"
	"backend/internal/room"
	"context"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type wsHandler struct {
	ctx context.Context
	hub *hub.Hub
}

func (h wsHandler) serve(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("room")
	playerNum := r.URL.Query().Get("player")

	if code == "" || (playerNum != "1" && playerNum != "2") {
		writeError(w, http.StatusBadRequest, "missing or invalid room/player param")
		return
	}

	var validateErr error
	var playerName string
	if playerNum == "1" {
		playerName = room.Player1
		validateErr = h.hub.ValidateCreate(code)
	} else {
		playerName = room.Player2
		validateErr = h.hub.ValidateJoin(code)
	}
	if validateErr != nil {
		writeError(w, http.StatusNotFound, validateErr.Error())
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade error: %v", err)
		return
	}

	c := client.NewClient(playerName, code)
	emitEvent := func(level, msg string) {
		e := event.New(event.Level(level), msg)
		select {
		case h.hub.Events <- e:
		default:
		}
	}

	var registerErr error
	if playerNum == "1" {
		registerErr = h.hub.CreateRoom(c, code)
	} else {
		registerErr = h.hub.JoinRoom(c, code)
	}
	if registerErr != nil {
		log.Printf("room register error for %s: %v", code, registerErr)
		_ = conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.ClosePolicyViolation, registerErr.Error()))
		conn.Close()
		return
	}

	go c.WritePump(h.ctx, conn, emitEvent)
	c.ReadPump(h.ctx, conn, h.hub, emitEvent)
}
