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
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

const (
	shutdownTimeout = 10 * time.Second
	defaultPort     = "8080"
)

var upgrader = websocket.Upgrader{
	// Allow all origins for this POC.
	// In production: check r.Header.Get("Origin") against an allowlist.
	CheckOrigin: func(r *http.Request) bool { return true },
}

// For this POC, validUsers is the fixed set of users.
var validUsers = map[string]bool{
	"alex": true,
	"bob":  true,
}

type eventBroadcaster struct {
	mu          sync.Mutex
	subscribers map[*websocket.Conn]struct{}
}

func newEventBroadcaster() *eventBroadcaster {
	return &eventBroadcaster{
		subscribers: make(map[*websocket.Conn]struct{}),
	}
}

func (b *eventBroadcaster) subscribe(conn *websocket.Conn) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subscribers[conn] = struct{}{}
}

func (b *eventBroadcaster) unsubscribe(conn *websocket.Conn) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.subscribers, conn)
}

func (b *eventBroadcaster) broadcast(e event.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for conn := range b.subscribers {
		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := conn.WriteJSON(e); err != nil {
			delete(b.subscribers, conn)
		}
		_ = conn.SetWriteDeadline(time.Time{})
	}
}

func (b *eventBroadcaster) run(ctx context.Context, h *hub.Hub) {
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-h.Events:
			b.broadcast(e)
		}
	}
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	h := hub.New()
	broadcaster := newEventBroadcaster()

	// ctx controls the lifetime of all goroutines.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go h.Run(ctx)
	go broadcaster.run(ctx, h)

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", wsHandler(ctx, h, broadcaster))
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

// wsHandler upgrades HTTP to WebSocket, registers the client with the Hub,
// and starts the read and write pumps.
func wsHandler(ctx context.Context, h *hub.Hub, broadcaster *eventBroadcaster) http.HandlerFunc {
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

		// Subscribe before pumps start so no events are missed.
		broadcaster.subscribe(conn)
		defer broadcaster.unsubscribe(conn)

		c := client.NewClient(userName)

		emitEvent := func(level, msg string) {
			e := event.New(event.Level(level), msg)
			select {
			case h.Events <- e:
			default:
				// Events channel full
				// drop rather than block.
			}
		}

		// Register before starting WritePump so pending messages have a
		// consumer ready when the Hub flushes them.
		h.Register(c)

		go c.WritePump(ctx, conn, emitEvent)

		// ReadPump blocks until the connection closes.
		// The deferred broadcaster.unsubscribe runs after it returns.
		c.ReadPump(ctx, conn, h, emitEvent)
	}
}

// healthHandler is a liveness probe for the deployment in Render.com.
func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, `{"status":"ok"}`)
}
