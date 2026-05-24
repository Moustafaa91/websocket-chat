package server

import (
	"backend/internal/hub"
	"context"
	"fmt"
	"log"
	"net/http"
	"time"
)

const shutdownTimeout = 10 * time.Second

// Server wires HTTP routes to the room hub.
type Server struct {
	hub    *hub.Hub
	http   *http.Server
	cancel context.CancelFunc
}

// New starts the hub goroutines and returns a configured Server.
func New(ctx context.Context, port string) *Server {
	h := hub.New()
	runCtx, cancel := context.WithCancel(ctx)

	go h.Run(runCtx)
	go runEventFanout(runCtx, h)

	rooms := roomsHandler{hub: h}
	ws := wsHandler{ctx: runCtx, hub: h}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /rooms", rooms.create)
	mux.HandleFunc("GET /rooms/{code}", rooms.lookup)
	mux.HandleFunc("GET /ws", ws.serve)
	mux.HandleFunc("GET /health", health)

	return &Server{
		hub:    h,
		cancel: cancel,
		http: &http.Server{
			Addr:    ":" + port,
			Handler: cors(mux),
		},
	}
}

// ListenAndServe blocks until the server stops or returns a non-ErrServerClosed error.
func (s *Server) ListenAndServe() error {
	log.Printf("server listening on %s", s.http.Addr)
	err := s.http.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

// Shutdown stops accepting connections and drains in-flight requests.
func (s *Server) Shutdown(ctx context.Context) error {
	s.cancel()
	if err := s.http.Shutdown(ctx); err != nil {
		return fmt.Errorf("http shutdown: %w", err)
	}
	return nil
}

// ShutdownTimeout is the grace period for in-flight HTTP requests.
func ShutdownTimeout() time.Duration {
	return shutdownTimeout
}
