package event

import (
	"strconv"
	"time"
)

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
	Level   Level      `json:"level"`
	Message string     `json:"message"`
	Time    UnixMillis `json:"time"`
}

func New(level Level, message string) Event {
	return Event{
		Level:   level,
		Message: message,
		Time:    UnixMillis(time.Now()),
	}
}
