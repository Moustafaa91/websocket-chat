# websocket-chat

A real-time WebSocket chat POC demonstrating production-quality Go concurrency patterns.

Two players chat in a **private room** created by Player 1. Player 1 gets a six-character code to share; Player 2 enters that code to join. A Go backend manages room lifecycle, WebSocket connections, inactivity timeouts, and in-memory message buffering.

Built to show what clean, idiomatic Go looks like under real network conditions.

**Live demo:** .. in progress

---

## What It Does

- **Room-based chat** — Player 1 creates a room and receives a code (e.g. `ABC234`); Player 2 joins with that code.
- **Join validation** — Invalid or expired codes are rejected before chat starts; the UI shows the server error.
- Real-time messaging between the two players over WebSocket.
- Each connection has a **10-second inactivity timeout** — the WebSocket closes, but the **room stays open**.
- The frontend **reconnects when you focus the chat** after going idle; both players remain in the room UI.
- Messages sent while a partner is idle are **buffered and delivered when they reconnect**.
- A live **event log panel** shows every backend state change in real time.

### User flow

1. **Player 1** — Click *Create New Chat* → share the displayed room code.
2. **Player 2** — Enter the 6-character code and click *Join Chat*.
3. Both players chat in the same room until one leaves or times out.

---

## Architecture

```
Browser (React)
  Player 1 WS ──┐
                 ├──► Go HTTP Server
  Player 2 WS ──┘         │
                     Hub (goroutine)
                     rooms map[code]*Room
                     routes messages per room
                     buffers undelivered messages
```

**HTTP**

| Method | Path | Purpose |
|--------|------|---------|
| `POST` | `/rooms` | Reserve a new room code (empty pending room) |
| `GET` | `/ws?room=<code>&player=1\|2` | WebSocket — Player 1 creates the room; Player 2 joins it |
| `GET` | `/health` | Health check |

Room codes are generated as **3 letters + 3 digits** (e.g. `KLM459`), using characters that avoid `I`/`O`/`0`/`1` confusion.

The Hub is a single goroutine that owns all shared state (`rooms` map, message routing).
No mutexes on the hot path. Channels are the only communication mechanism.

---

## Project Structure

```
websocket-chat/
├── backend/
│   ├── cmd/
│   │   └── server/
│   │       └── main.go          # Entry point, /rooms, /ws, graceful shutdown
│   ├── internal/
│   │   ├── hub/
│   │   │   ├── hub.go           # Room lifecycle, message routing, buffering
│   │   │   └── hub_test.go
│   │   ├── client/
│   │   │   ├── client.go        # Per-connection WebSocket (read pump / write pump)
│   │   │   └── client_test.go
│   │   ├── room/
│   │   │   └── room.go          # Room state (pending / active / closed)
│   │   ├── codegen/
│   │   │   └── codegen.go       # Unique 6-character room codes
│   │   ├── message/
│   │   │   └── message.go       # Chat message payload
│   │   └── event/
│   │       └── event.go         # Structured log event types
│   ├── go.mod
│   └── go.sum
└── frontend/
    ├── public/
    ├── src/
    │   ├── App.jsx              # Home, waiting, joining, and chat screens
    │   ├── ChatRoom.jsx         # Chat UI with auto-reconnect
    │   ├── EventLog.jsx         # Live backend log panel
    │   └── App.css
    ├── index.html
    ├── package.json
    └── vite.config.js
```

---

## Key Go Concepts Demonstrated

| Concept | Where |
|---|---|
| Hub pattern — channel-only shared state | `internal/hub/hub.go` |
| Room-scoped routing and lifecycle | `internal/hub/hub.go`, `internal/room/room.go` |
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

Create `frontend/.env` (or `.env.local`):

```env
VITE_API_URL=http://localhost:8080
VITE_WS_URL=ws://localhost:8080
```

```bash
cd frontend
npm install
npm run dev
```

Runs on `http://localhost:5173`.

Open two browser windows (or one normal + one incognito) to test Player 1 and Player 2 in the same room.

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

- **Two players per room** — no authentication, no user management
- **One active session per room** — a second Player 2 cannot join an active room
- No database; all state is in memory and is lost on restart
- No message history UI — only undelivered messages from the current session are buffered
- Room codes expire only on explicit *leave*, partner permanent leave, or server restart — not on inactivity

---

## License

MIT
