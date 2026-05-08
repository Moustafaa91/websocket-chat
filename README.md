# websocket-chat

A real-time WebSocket chat POC demonstrating production-quality Go concurrency patterns.

Two users, **Alex** and **Bob**  are chatting through a Go backend that manages WebSocket lifecycles,
inactivity timeouts, and in-memory message buffering.
Built to show what clean, idiomatic Go looks like under real network conditions.

**Live demo:** .. in progress

---

## What It Does

- Real-time messaging between two fixed parties over WebSocket.
- Each connection has a **10-second inactivity timeout**.
- The frontend **reconnects automatically** when you become active again.
- Messages sent while a user was away are **buffered and delivered on reconnect**.
- A live **event log panel** shows every backend state change in real time.

---

## Architecture

```
Browser (React)
  Alex WS ──┐
             ├──► Go HTTP Server
  Bob WS  ──┘         │
                   Hub (goroutine)
                   routes messages
                   manages inactivity timers
                   buffers undelivered messages
```

The Hub is a single goroutine that owns all shared state.
No mutexes on the hot path. Channels are the only communication mechanism.

---

## Project Structure

```
websocket-chat/
├── backend/
│   ├── cmd/
│   │   └── server/
│   │       └── main.go          # Entry point, wiring, graceful shutdown
│   ├── internal/
│   │   ├── hub/
│   │   │   ├── hub.go           # Central message router and state owner
│   │   │   └── hub_test.go
│   │   ├── client/
│   │   │   ├── client.go        # Per-user WebSocket lifecycle (read pump / write pump)
│   │   │   └── client_test.go
│   │   └── event/
│   │       └── event.go         # Structured log event types
│   ├── go.mod
│   └── go.sum
└── frontend/
    ├── public/
    ├── src/
    │   ├── App.jsx              # Root layout
    │   ├── ChatBox.jsx          # Per-user chat panel with auto-reconnect
    │   └── EventLog.jsx         # Live backend log panel
    ├── index.html
    ├── package.json
    └── vite.config.js
```

---

## Key Go Concepts Demonstrated

| Concept | Where |
|---|---|
| Hub pattern — channel-only shared state | `internal/hub/hub.go` |
| Read pump / write pump goroutine separation | `internal/client/client.go` |
| `time.Timer` and the correct reset pattern | `internal/client/client.go` |
| `context.Context` propagation and cancellation | everywhere |
| Graceful shutdown — draining in-flight messages | `cmd/server/main.go` |
| In-memory message buffering with delivery guarantee | `internal/hub/hub.go` |

---

## Running Locally

### Backend

```bash
cd backend
go run ./cmd/server
```

Runs on `http://localhost:8080`.

### Frontend

```bash
cd frontend
npm install
npm run dev
```

Runs on `http://localhost:5173`.

---

## Tests

```bash
cd backend
go test -race ./...
```

The `-race` flag is non-negotiable. All tests must pass clean under the race detector.

---

## Design Constraints

This is a focused POC, not a general chat platform. Intentional limitations:

- Two fixed users only (Alex and Bob) — no authentication, no user management
- No database, all state is in memory and is lost on restart
- No message history UI — only undelivered messages from the current session are buffered (maybe a future enhancement)

---

## License

MIT
