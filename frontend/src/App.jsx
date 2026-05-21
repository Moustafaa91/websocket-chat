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
  const [screen, setScreen]     = useState('home')   // 'home' | 'waiting' | 'chat'
  const [roomCode, setRoomCode] = useState('')
  const [playerNum, setPlayerNum] = useState(null)   // 1 | 2
  const [joinInput, setJoinInput] = useState('')
  const [joinError, setJoinError] = useState('')
  const [events, setEvents]     = useState([])
  const [theme, setTheme]       = useState(getInitialTheme)

  // Player 1's WebSocket is hoisted here so it survives the
  // waiting → chat screen transition without closing.
  const p1WsRef = useRef(null)

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
      setScreen('waiting')
      addEvent(`Room ${code} created — share the code with your friend`, 'success')
    } catch (err) {
      addEvent('Could not create room: ' + err.message, 'error')
    }
  }, [addEvent])

  const handleCancelWaiting = useCallback(() => {
    // Close Player 1's WebSocket if it was opened.
    if (p1WsRef.current) {
      p1WsRef.current.close(1000, 'cancelled')
      p1WsRef.current = null
    }
    setRoomCode('')
    setPlayerNum(null)
    setScreen('home')
    addEvent('Room cancelled', 'warn')
  }, [addEvent])

  // Called by WaitingScreen once Player 1's WS opens successfully.
  const handlePlayer1Ready = useCallback((ws, code) => {
    p1WsRef.current = ws
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
    setScreen('chat')
    addEvent(`Joining room ${code}…`, 'info')
  }, [joinInput, addEvent])

  // ── Chat ended ───────────────────────────────────────────────────────────────

  const handleChatEnd = useCallback((reason) => {
    p1WsRef.current = null  // ChatRoom will have closed it
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
              onCancel={handleCancelWaiting}
              onReady={handlePlayer1Ready}
              addEvent={addEvent}
            />
          )}
          {screen === 'chat' && (
            <ChatRoom
              roomCode={roomCode}
              playerNum={playerNum}
              existingWs={playerNum === 1 ? p1WsRef.current : null}
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

// ── WaitingScreen ─────────────────────────────────────────────────────────────
// Opens Player 1's WebSocket and passes it up to App via onReady so it
// survives the transition to ChatRoom without closing.

function WaitingScreen({ code, onCancel, onReady, addEvent }) {
  const wsRef = useRef(null)
  const firedRef = useRef(false)
  const WS_URL = import.meta.env.VITE_WS_URL

  useEffect(() => {
    const ws = new WebSocket(`${WS_URL}/ws?room=${code}&player=1`)
    wsRef.current = ws

    ws.onopen = () => {
      if (!firedRef.current) {
        firedRef.current = true
        // Hand the live WebSocket up to App before this component unmounts.
        onReady(ws, code)
      }
    }
    ws.onerror = () => addEvent('Connection error on room ' + code, 'error')
    ws.onclose = (e) => {
      if (e.code !== 1000) addEvent('Connection lost — code ' + e.code, 'error')
    }

    // No cleanup close here — App owns the WS from onReady onwards.
    // If the user cancels, App.handleCancelWaiting closes it explicitly.
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
