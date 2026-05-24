package server

import (
	"backend/internal/hub"
	"net/http"
)

type roomsHandler struct {
	hub *hub.Hub
}

func (h roomsHandler) create(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	code, err := h.hub.ReserveCode()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create room")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"code": code})
}

// lookup validates that a room exists and the requested player slot can connect.
// GET /rooms/{code}?player=1|2
func (h roomsHandler) lookup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	code := r.PathValue("code")
	if code == "" {
		writeError(w, http.StatusBadRequest, "room code required")
		return
	}

	player := r.URL.Query().Get("player")
	var err error
	switch player {
	case "1":
		err = h.hub.ValidateCreate(code)
	case "2":
		err = h.hub.ValidateJoin(code)
	default:
		writeError(w, http.StatusBadRequest, "player must be 1 or 2")
		return
	}

	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"code": code, "player": player})
}
