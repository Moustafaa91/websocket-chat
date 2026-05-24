import { useState, useCallback, useRef, useEffect } from 'react'
import ChatRoom from './ChatRoom'
import EventLog from './EventLog'
import './App.css'

const getInitialTheme = () => {
  if (typeof window === 'undefined') return 'dark'
  const saved = window.localStorage.getItem('theme')
  if (saved === 'light' || saved === 'dark') return saved
  return window.matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark'
}

const API_URL = import.meta.env.VITE_API_URL

export default function App() {
  const [screen, setScreen]     = useState('home')   // 'home' | 'waiting' | 'joining' | 'chat'
  const [roomCode, setRoomCode] = useState('')
  const [playerNum, setPlayerNum] = useState(null)   // 1 | 2
  const [joinInput, setJoinInput] = useState('')
  const [joinError, setJoinError] = useState('')
  const [events, setEvents]     = useState([])
  const [theme, setTheme]       = useState(getInitialTheme)

  // Live WebSocket survives waiting/joining → chat transitions.
  const roomWsRef = useRef(null)
  // Prevents a second WS when React Strict Mode remounts waiting/joining screens.
  const wsConnectStartedRef = useRef(false)

  const counterRef = useRef(0)

  // Apply theme class to <html>
  useEffect(() => {
    const html = document.documentElement
    html.classList.toggle('theme-light', theme === 'light')
    html.classList.toggle('theme-dark', theme !== 'light')
    window.localStorage.setItem('theme', theme)
  }, [theme])

  const addEvent = useCallback((message, type = 'info') => {
    const id = counterRef.current++
    setEvents(prev => {
      const next = [...prev, { message, type, time: Date.now(), id }]
      return next.length > 50 ? next.slice(-50) : next
    })
  }, [])

  const toggleTheme = useCallback(() => {
    setTheme(prev => prev === 'dark' ? 'light' : 'dark')
  }, [])

  // ── Create flow ──────────────────────────────────────────────────────────────

  const handleCreate = useCallback(async () => {
    try {
      const res = await fetch(`${API_URL}/rooms`, { method: 'POST' })
      if (!res.ok) throw new Error('server error')
      const { code } = await res.json()
      setRoomCode(code)
      setPlayerNum(1)
      wsConnectStartedRef.current = false
      setScreen('waiting')
      addEvent(`Room ${code} created — share the code with your friend`, 'success')
    } catch (err) {
      addEvent('Could not create room: ' + err.message, 'error')
    }
  }, [addEvent])

  const handleCancelWaiting = useCallback(() => {
    if (roomWsRef.current) {
      roomWsRef.current.close(1000, 'cancelled')
      roomWsRef.current = null
    }
    wsConnectStartedRef.current = false
    setRoomCode('')
    setPlayerNum(null)
    setScreen('home')
    addEvent('Room cancelled', 'warn')
  }, [addEvent])

  // Called by WaitingScreen once Player 1's WS opens successfully.
  const handlePlayer1Ready = useCallback((ws, code) => {
    roomWsRef.current = ws
    setScreen('chat')
    addEvent(`Connected to room ${code} — waiting for Player 2`, 'info')
  }, [addEvent])

  // ── Join flow ────────────────────────────────────────────────────────────────

  const handleJoinSubmit = useCallback(() => {
    const code = joinInput.trim().toUpperCase()
    if (code.length !== 6) {
      setJoinError('Code must be 6 characters')
      return
    }
    setJoinError('')
    setRoomCode(code)
    setPlayerNum(2)
    wsConnectStartedRef.current = false
    setScreen('joining')
    addEvent(`Joining room ${code}…`, 'info')
  }, [joinInput, addEvent])

  const handleJoinSuccess = useCallback((ws, code) => {
    roomWsRef.current = ws
    setScreen('chat')
    addEvent(`Joined room ${code}`, 'success')
  }, [addEvent])

  const handleJoinFailure = useCallback((message) => {
    if (roomWsRef.current) {
      roomWsRef.current.close(1000, 'join failed')
      roomWsRef.current = null
    }
    wsConnectStartedRef.current = false
    setJoinError(message)
    setScreen('home')
    setRoomCode('')
    setPlayerNum(null)
    addEvent(message, 'error')
  }, [addEvent])

  // ── Chat ended ───────────────────────────────────────────────────────────────

  const handleChatEnd = useCallback((reason) => {
    roomWsRef.current = null
    wsConnectStartedRef.current = false
    addEvent(reason || 'Chat ended', 'warn')
    setScreen('home')
    setRoomCode('')
    setPlayerNum(null)
    setJoinInput('')
  }, [addEvent])

  // ── Render ───────────────────────────────────────────────────────────────────

  return (
    <div className="app">
      <header className="app-header">
        <div className="header-dot" />
        <h1>websocket<span>chat</span></h1>
        <p className="header-sub">real-time · go backend · websocket</p>
        <button className="theme-toggle" onClick={toggleTheme}>
          {theme === 'dark' ? 'Light mode' : 'Dark mode'}
        </button>
      </header>

      <main className="app-main app-main--split">
        <div className="app-content">
          {screen === 'home' && (
            <HomeScreen
              joinInput={joinInput}
              joinError={joinError}
              onJoinInputChange={e => setJoinInput(e.target.value)}
              onJoinSubmit={handleJoinSubmit}
              onCreate={handleCreate}
            />
          )}
          {screen === 'waiting' && (
            <WaitingScreen
              code={roomCode}
              connectStartedRef={wsConnectStartedRef}
              roomWsRef={roomWsRef}
              onCancel={handleCancelWaiting}
              onReady={handlePlayer1Ready}
              addEvent={addEvent}
            />
          )}
          {screen === 'joining' && (
            <JoiningScreen
              code={roomCode}
              connectStartedRef={wsConnectStartedRef}
              roomWsRef={roomWsRef}
              onSuccess={handleJoinSuccess}
              onFailure={handleJoinFailure}
            />
          )}
          {screen === 'chat' && (
            <ChatRoom
              roomCode={roomCode}
              playerNum={playerNum}
              existingWs={roomWsRef.current}
              addEvent={addEvent}
              onEnd={handleChatEnd}
            />
          )}
        </div>
        <EventLog events={events} />
      </main>
    </div>
  )
}

// ── HomeScreen ────────────────────────────────────────────────────────────────

function HomeScreen({ joinInput, joinError, onJoinInputChange, onJoinSubmit, onCreate }) {
  return (
    <div className="home-screen">
      <div className="home-card">
        <h2 className="home-title">Start a chat</h2>
        <p className="home-desc">
          Create a new room and share the code, or enter a code to join a friend's room.
        </p>

        <button className="btn btn--primary home-create" onClick={onCreate}>
          Create New Chat
        </button>

        <div className="home-divider"><span>or</span></div>

        <div className="home-join">
          <input
            className="chatbox-input"
            type="text"
            placeholder="Enter 6-character code"
            value={joinInput}
            onChange={onJoinInputChange}
            onKeyDown={e => e.key === 'Enter' && onJoinSubmit()}
            maxLength={6}
            style={{ textTransform: 'uppercase', letterSpacing: '0.15em' }}
          />
          <button className="btn btn--secondary" onClick={onJoinSubmit}>
            Join Chat
          </button>
          {joinError && <p className="home-error">{joinError}</p>}
        </div>
      </div>
    </div>
  )
}

// ── JoiningScreen ─────────────────────────────────────────────────────────────
// Opens Player 2's WebSocket and validates the room code before entering chat.

function JoiningScreen({ code, connectStartedRef, roomWsRef, onSuccess, onFailure }) {
  const firedRef = useRef(false)
  const WS_URL = import.meta.env.VITE_WS_URL

  useEffect(() => {
    if (roomWsRef.current?.readyState === WebSocket.OPEN) {
      onSuccess(roomWsRef.current, code)
      return
    }
    if (connectStartedRef.current) return
    connectStartedRef.current = true

    const ws = new WebSocket(`${WS_URL}/ws?room=${code}&player=2`)

    ws.onopen = () => {
      if (!firedRef.current) {
        firedRef.current = true
        onSuccess(ws, code)
      }
    }

    ws.onerror = () => {
      if (!firedRef.current) {
        connectStartedRef.current = false
        onFailure('Could not connect — check your connection and try again')
      }
    }

    ws.onclose = (e) => {
      if (!firedRef.current) {
        connectStartedRef.current = false
        const msg = e.reason || 'Invalid or expired room code'
        onFailure(msg)
      }
    }
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <div className="home-screen">
      <div className="home-card">
        <h2 className="home-title">Joining room</h2>
        <p className="home-desc">Connecting with code <span className="room-code-inline">{code}</span>…</p>
      </div>
    </div>
  )
}

// ── WaitingScreen ─────────────────────────────────────────────────────────────
// Opens Player 1's WebSocket and passes it up to App via onReady so it
// survives the transition to ChatRoom without closing.

function WaitingScreen({ code, connectStartedRef, roomWsRef, onCancel, onReady, addEvent }) {
  const firedRef = useRef(false)
  const WS_URL = import.meta.env.VITE_WS_URL

  useEffect(() => {
    if (roomWsRef.current?.readyState === WebSocket.OPEN) {
      onReady(roomWsRef.current, code)
      return
    }
    if (connectStartedRef.current) return
    connectStartedRef.current = true

    const ws = new WebSocket(`${WS_URL}/ws?room=${code}&player=1`)

    ws.onopen = () => {
      if (!firedRef.current) {
        firedRef.current = true
        onReady(ws, code)
      }
    }
    ws.onerror = () => addEvent('Connection error on room ' + code, 'error')
    ws.onclose = (e) => {
      if (e.code !== 1000) addEvent('Connection lost — code ' + e.code, 'error')
    }

    // No cleanup close here — App owns the WS from onReady onwards.
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <div className="home-screen">
      <div className="home-card">
        <h2 className="home-title">Waiting for Player 2</h2>
        <p className="home-desc">Share this code with your friend:</p>
        <div className="room-code">{code}</div>
        <p className="home-desc" style={{ fontSize: '12px', marginTop: '0.5rem' }}>
          Valid until someone joins or you cancel.
        </p>
        <button
          className="btn btn--secondary"
          style={{ marginTop: '1.5rem' }}
          onClick={onCancel}
        >
          Cancel
        </button>
      </div>
    </div>
  )
}
