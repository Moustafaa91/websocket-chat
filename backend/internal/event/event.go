package event

import (
	"strconv"
	"time"
)

const KindPresence = "presence"

type Level string

const (
	LevelInfo    Level = "info"
	LevelWarn    Level = "warn"
	LevelError   Level = "error"
	LevelSuccess Level = "success"
)

type UnixMillis time.Time

func (u UnixMillis) MarshalJSON() ([]byte, error) {
	ms := time.Time(u).UnixMilli()
	return []byte(strconv.FormatInt(ms, 10)), nil
}

type Event struct {
	Level    Level      `json:"level"`
	Message  string     `json:"message"`
	Time     UnixMillis `json:"time"`
	Room     string     `json:"room,omitempty"`
	Kind     string     `json:"kind,omitempty"`
	Player   string     `json:"player,omitempty"`
	Presence string     `json:"presence,omitempty"`
}

func New(level Level, message string) Event {
	return Event{
		Level:   level,
		Message: message,
		Time:    UnixMillis(time.Now()),
	}
}

func NewRoom(level Level, room, message string) Event {
	e := New(level, message)
	e.Room = room
	return e
}

func NewPresence(room, player, presence string) Event {
	return Event{
		Kind:     KindPresence,
		Room:     room,
		Player:   player,
		Presence: presence,
		Level:    LevelInfo,
		Message:  player + " is " + presence,
		Time:     UnixMillis(time.Now()),
	}
}
