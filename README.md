# websocket-chat

A real-time WebSocket chat POC demonstrating production-quality Go concurrency patterns.

Two players chat in a private room created by Player 1. Player 1 gets a six-character code to share; Player 2 enters that code to join. A Go backend manages room lifecycle, WebSocket connections, presence, inactivity timeouts, and in-memory message buffering.

Built to show what clean, idiomatic Go looks like under real network conditions.

**Source code:** [Moustafaa91/websocket-chat](https://github.com/Moustafaa91/websocket-chat)

**Live demo:** in progress

---

## What It Does

- Room-based chat: Player 1 creates a room and receives a code such as `ABC234`; Player 2 joins with that code.
- Clear waiting-room UX: Player 1 sees a large room code, a copy button, and a waiting message until Player 2 joins.
- Join validation: invalid or expired codes are rejected before chat starts, and the UI shows the server error.
- Real-time messaging between two players over WebSocket.
- Live presence updates for both players.
- In-memory buffering for messages sent while a partner is inactive.
- A live event log panel shows room and connection state changes in real time.
- Light and dark themes for the React UI.

### Presence States

| State | Meaning | Message behavior |
|---|---|---|
| Online | WebSocket is open and active. | Messages are delivered live. |
| Inactive | No chat activity for 10 seconds; WebSocket is closed, but the room remains available. | Messages are buffered and delivered when the player reconnects. |
| Offline | Player clicked Leave, closed the tab, or closed the browser. | Messages are not buffered. The player can rejoin later if the room still exists. |

The room stays open while at least one player is online or inactive. It is deleted only when both players are offline.

While Player 1 is on the waiting-code screen, the frontend sends a lightweight keepalive ping so the room does not become inactive just because Player 1 is waiting for Player 2.

### User Flow

1. Player 1 clicks **Create New Chat**.
2. Player 1 copies and shares the displayed six-character room code.
3. Player 2 enters the code and clicks **Join Chat**.
4. The room opens for both players when Player 2 joins.
5. Either player can click **Leave** to go offline.
6. If a player becomes inactive, their partner can still send messages; buffered messages are delivered when the inactive player focuses the chat input and reconnects.
7. Either player can return with the same code after going offline or inactive, as long as the room has not been deleted.

---

## Architecture

```text
Browser (React)
  Player 1 WS ----\
                   +----> Go HTTP Server
  Player 2 WS ----/             |
                                v
                         Hub goroutine
                         rooms map[code]*Room
                         per-player presence
                         buffered inactive messages
```

### HTTP API

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/rooms` | Reserve a new room code as an empty pending room. |
| `GET` | `/rooms/{code}?player=1\|2` | Validate that a room exists and the player slot can connect. |
| `GET` | `/ws?room=<code>&player=1\|2` | Open the WebSocket connection for Player 1 or Player 2. |
| `GET` | `/health` | Health check. |

Room codes are generated as three letters plus three digits, such as `KLM459`, using characters that avoid `I`/`O`/`0`/`1` confusion.

The Hub is a single goroutine that owns all shared state: the `rooms` map, message routing, presence, and buffering. There are no mutexes on the hot path; channels are the communication mechanism.

### WebSocket Close Reasons

| Reason | Hub action | Client UI |
|---|---|---|
| `inactivity` | Mark player inactive and keep the room available. | Stay in chat; reconnect on input focus. |
| `user left` / `offline` | Mark player offline and purge messages buffered for that player. | Leave chat screen; player can rejoin with the code if the room remains. |

---

## Project Structure

```text
websocket-chat/
├── backend/
│   ├── cmd/server/main.go       # entrypoint
│   └── internal/
│       ├── server/              # HTTP routes, CORS, WebSocket upgrade
│       ├── hub/                 # room lifecycle, routing, buffering
│       ├── client/              # read/write pumps, inactivity timer
│       ├── room/                # presence state, player slots
│       ├── message/
│       ├── event/
│       └── codegen/
└── frontend/
    └── src/
        ├── App.jsx
        ├── api/                 # REST + WebSocket helpers
        ├── hooks/               # theme, event log
        ├── screens/             # home, waiting, joining
        └── components/          # ChatRoom, EventLog
```

---

## Key Go Concepts Demonstrated

| Concept | Where |
|---|---|
| Hub pattern with channel-owned shared state | `backend/internal/hub/hub.go` |
| Room-scoped routing and lifecycle | `backend/internal/hub/`, `backend/internal/room/` |
| Read pump / write pump goroutine separation | `backend/internal/client/` |
| `time.Timer` usage for inactivity | `backend/internal/client/pump_read.go` |
| `context.Context` propagation and cancellation | `backend/cmd/server/main.go`, server/client packages |
| Graceful shutdown | `backend/cmd/server/main.go` |
| In-memory message buffering for inactive partners | `backend/internal/hub/messages.go` |

---

## Running Locally

### Backend

```bash
cd backend
go run ./cmd/server
```

The backend runs on `http://localhost:8080`.

### Frontend

Create `frontend/.env` or `frontend/.env.local`:

```env
VITE_API_URL=http://localhost:8080
VITE_WS_URL=ws://localhost:8080
```

Then run:

```bash
cd frontend
npm install
npm run dev
```

The frontend runs on `http://localhost:5173`.

Open two browser windows, or one normal window and one incognito window, to test Player 1 and Player 2 in the same room.

---

## Tests

```bash
cd backend
go test -race ./...
```

The backend test suite should pass cleanly under the race detector.

For the frontend:

```bash
cd frontend
npm run build
```

---

## Design Constraints

This is a focused POC, not a general chat platform. Intentional limitations:

- Two players per room.
- No authentication or user management.
- No database; all state is in memory and is lost on server restart.
- No permanent message history UI.
- Only undelivered messages for inactive partners are buffered.
- Rooms are deleted when both players are offline, not when one player is inactive.

---

## License

MIT
