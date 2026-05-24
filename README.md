# websocket-chat

A real-time WebSocket chat POC demonstrating production-quality Go concurrency patterns.

Two players chat in a private room created by Player 1. Player 1 gets a six-character code to share; Player 2 enters that code to join. A Go backend manages room lifecycle, WebSocket connections, presence, inactivity timeouts, and in-memory message buffering.

[Live demo](https://go-websocket-chat.netlify.app/)

---

### Presence States

| State | Meaning | Message behavior |
|---|---|---|
| Online | WebSocket is open and active. | Messages are delivered live. |
| Inactive | No chat activity for 10 seconds; WebSocket is closed, but the room remains available. | Messages are buffered and delivered when the player reconnects. |
| Offline | Player clicked Leave, closed the tab, or closed the browser. | Messages are not buffered. The player can rejoin later if the room still exists. |

### User Flow

1. Player 1 clicks **Create New Chat**.
2. Player 1 copies and shares the displayed six-character room code.
3. Player 2 enters the code and clicks **Join Chat**.
4. The room opens for both players when Player 2 joins.
5. Either player can click **Leave** to go offline.
6. If a player becomes inactive, their partner can still send messages; buffered messages are delivered when the inactive player focuses the chat input and reconnects.
7. Either player can return with the same code after going offline or inactive, as long as the room has not been deleted.

---

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
For a hosted HTTPS backend, use `wss://` for the WebSocket URL.

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
