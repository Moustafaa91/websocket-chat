package main

import (
	"backend/internal/client"
	"backend/internal/event"
	"backend/internal/hub"
	"context"
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

// For this POC, validUsers is the fixed set of users.
var validUsers = map[string]bool{
	"alex": true,
	"bob":  true,
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

	// eventFanout consumes Hub events and delivers them to all currently clients via their Send channels
	// WritePump is the sole writer to each WebSocket connection, so this never races.
	go eventFanout(ctx, h)

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", wsHandler(ctx, h))
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

// eventFanout reads events from the Hub and delivers them to every registered client's Send channel. Because WritePump is the sole writer to each conn,
// routing through Send eliminates the concurrent-write race entirely.
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

func wsHandler(ctx context.Context, h *hub.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userName := r.URL.Query().Get("user")
		if !validUsers[userName] {
			http.Error(w, "invalid user", http.StatusBadRequest)
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("websocket upgrade error for %s: %v", userName, err)
			return
		}

		c := client.NewClient(userName)

		emitEvent := func(level, msg string) {
			e := event.New(event.Level(level), msg)
			select {
			case h.Events <- e:
			default:
			}
		}

		h.Register(c)

		go c.WritePump(ctx, conn, emitEvent)
		c.ReadPump(ctx, conn, h, emitEvent)
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, `{"status":"ok"}`)
}
