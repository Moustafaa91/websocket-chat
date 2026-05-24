package main

import (
	"backend/internal/client"
	"backend/internal/event"
	"backend/internal/hub"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

const (
	shutdownTimeout = 10 * time.Second
	defaultPort     = "8080"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	h := hub.New()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go h.Run(ctx)
	go eventFanout(ctx, h)

	mux := http.NewServeMux()
	mux.HandleFunc("/rooms", corsMiddleware(roomsHandler(h)))
	mux.HandleFunc("/ws", corsMiddleware(wsHandler(ctx, h)))
	mux.HandleFunc("/health", healthHandler)

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("server listening on :%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen error: %v", err)
		}
	}()

	<-quit
	log.Println("shutdown signal received")
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("server shutdown error: %v", err)
	}
	log.Println("server stopped")
}

// roomsHandler handles POST /rooms.
// Reserves a room code and returns it. The frontend then opens a WebSocket
// with ?room=<code>&player=1 to activate the room.
func roomsHandler(h *hub.Hub) http.HandlerFunc {
	type response struct {
		Code string `json:"code"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		code, err := h.ReserveCode()
		if err != nil {
			http.Error(w, "could not create room", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response{Code: code})
	}
}

// wsHandler handles GET /ws?room=<code>&player=1|2
func wsHandler(ctx context.Context, h *hub.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("room")
		playerNum := r.URL.Query().Get("player")

		if code == "" || (playerNum != "1" && playerNum != "2") {
			http.Error(w, "missing or invalid room/player param", http.StatusBadRequest)
			return
		}

		name := "Player " + playerNum

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("websocket upgrade error: %v", err)
			return
		}

		c := client.NewClient(name, code)

		emitEvent := func(level, msg string) {
			e := event.New(event.Level(level), msg)
			select {
			case h.Events <- e:
			default:
			}
		}

		// Register with the Hub before starting pumps.
		if playerNum == "1" {
			if err := h.CreateRoom(c, code); err != nil {
				log.Printf("CreateRoom error for %s: %v", code, err)
				_ = conn.WriteMessage(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.ClosePolicyViolation, err.Error()))
				conn.Close()
				return
			}
		} else {
			if err := h.JoinRoom(c, code); err != nil {
				log.Printf("JoinRoom error for %s: %v", code, err)
				_ = conn.WriteMessage(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.ClosePolicyViolation, err.Error()))
				conn.Close()
				return
			}
		}

		go c.WritePump(ctx, conn, emitEvent)
		c.ReadPump(ctx, conn, h, emitEvent)
	}
}

// eventFanout delivers Hub events to all connected players.
func eventFanout(ctx context.Context, h *hub.Hub) {
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-h.Events:
			h.BroadcastEvent(e)
		}
	}
}

func corsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, `{"status":"ok"}`)
}
