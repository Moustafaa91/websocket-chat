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

- **Three presence states per player:**

  - **Online** — WebSocket open; chat input focused or used within the last 10 seconds; messages delivered live.

  - **Inactive** — No chat activity for 10 seconds; WebSocket closed; room kept; messages **buffered** until they focus/type in the chat again.

  - **Offline** — Clicked *Leave*, closed the tab, or closed the browser; messages are **not** buffered; they can **re-join** later with the same room code.

- The room stays open while at least one player is **online** or **inactive**.

- The room is **deleted** only when **both** players are **offline**.

- A live **event log panel** shows backend state changes in real time.



### User flow



1. **Player 1** — Click *Create New Chat* → share the displayed room code.

2. **Player 2** — Enter the 6-character code and click *Join Chat*.

3. Both players chat. Either can click *Leave* (goes **offline**).

4. If a player is **inactive** (idle in chat), their partner can still send messages; they are delivered when the inactive player focuses the input and reconnects.

5. Either player can return with the same code after going **offline** or **inactive**.



---



## Architecture



```

Browser (React)

  Player 1 WS ──┐

                 ├──► Go HTTP Server

  Player 2 WS ──┘         │

                     Hub (goroutine)

                     rooms map[code]*Room

                     per-player presence (online / inactive / offline)

                     buffers messages for inactive partners only

```



**HTTP**



| Method | Path | Purpose |

|--------|------|---------|

| `POST` | `/rooms` | Reserve a new room code (empty pending room) |

| `GET` | `/rooms/{code}?player=1\|2` | Validate that a room exists and the player slot can connect |

| `GET` | `/ws?room=<code>&player=1\|2` | WebSocket — Player 1 creates the room; Player 2 joins it |

| `GET` | `/health` | Health check |



Room codes are generated as **3 letters + 3 digits** (e.g. `KLM459`), using characters that avoid `I`/`O`/`0`/`1` confusion.



The Hub is a single goroutine that owns all shared state (`rooms` map, message routing).

No mutexes on the hot path. Channels are the only communication mechanism.



**WebSocket close reasons**



| Reason | Hub action | Client UI |

|--------|------------|-----------|

| `inactivity` | Player → **inactive**, buffer messages | Stay in chat; reconnect on input focus |

| `user left` / `offline` | Player → **offline** | Leave chat screen (can re-join with code) |



---



## Project Structure



```

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

| Hub pattern — channel-only shared state | `internal/hub/hub.go` |

| Room-scoped routing and lifecycle | `internal/hub/hub.go`, `internal/room/room.go` |

| Read pump / write pump goroutine separation | `internal/client/client.go` |

| `time.Timer` and the correct reset pattern | `internal/client/client.go` |

| `context.Context` propagation and cancellation | everywhere |

| Graceful shutdown — draining in-flight messages | `cmd/server/main.go` |

| In-memory message buffering (inactive partners only) | `internal/hub/hub.go` |



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

- **One live WebSocket per player slot** — reconnect replaces the previous connection

- No database; all state is in memory and is lost on server restart

- No message history UI — only undelivered messages for **inactive** partners are buffered

- Room deleted when **both** players are offline — not when one goes inactive



---



## License



MIT

