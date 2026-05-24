import { useState, useEffect, useRef, useCallback } from 'react'

const WS_URL = import.meta.env.VITE_WS_URL

const BACKOFF_BASE_MS = 1_000
const BACKOFF_MAX_MS = 30_000
const COLD_START_THRESHOLD_MS = 10_000

export default function ChatRoom({ roomCode, playerNum, existingWs, addEvent, onEnd }) {
  const [messages, setMessages]       = useState([])
  const [input, setInput]             = useState('')
  const [status, setStatus]           = useState('connecting')
  const [partnerOnline, setPartnerOnline] = useState(false)

  const wsRef               = useRef(null)
  const reconnectTimerRef   = useRef(null)
  const inactivityTimerRef  = useRef(null)
  const isUnmountedRef      = useRef(false)
  const retryDelayRef       = useRef(BACKOFF_BASE_MS)
  const retryStartRef       = useRef(null)
  const messagesEndRef      = useRef(null)

  const playerName = `Player ${playerNum}`

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages])

  // ── Backoff helpers ──────────────────────────────────────────────────────────

  const resetBackoff = useCallback(() => {
    retryDelayRef.current = BACKOFF_BASE_MS
    retryStartRef.current = null
  }, [])

  const scheduleReconnect = useCallback((connectFn) => {
    if (retryStartRef.current === null) retryStartRef.current = Date.now()
    const elapsed = Date.now() - retryStartRef.current
    setStatus(elapsed >= COLD_START_THRESHOLD_MS ? 'waking' : 'reconnecting')
    reconnectTimerRef.current = setTimeout(() => {
      if (!isUnmountedRef.current) connectFn()
    }, retryDelayRef.current)
    retryDelayRef.current = Math.min(retryDelayRef.current * 2, BACKOFF_MAX_MS)
  }, [])

  // ── Inactivity timer ─────────────────────────────────────────────────────────

  const resetInactivityTimer = useCallback(() => {
    clearTimeout(inactivityTimerRef.current)
    inactivityTimerRef.current = setTimeout(() => {
      if (wsRef.current?.readyState === WebSocket.OPEN) {
        wsRef.current.close(1000, 'inactivity')
      }
    }, 10_000)
  }, [])

  // ── WebSocket setup ──────────────────────────────────────────────────────────

  const endSession = useCallback((reason) => {
    clearTimeout(inactivityTimerRef.current)
    clearTimeout(reconnectTimerRef.current)
    if (wsRef.current) {
      wsRef.current.close(1000, reason)
      wsRef.current = null
    }
    onEnd(reason)
  }, [onEnd])

  // attachHandlers wires onmessage / onclose / onerror onto any WebSocket
  // instance — used for both the handed-off WS and fresh connections.
  const attachHandlers = useCallback((ws, connectFn) => {
    ws.onmessage = (e) => {
      if (isUnmountedRef.current) return
      try {
        const msg = JSON.parse(e.data)
        if (msg.level) {
          addEvent(msg.message, msg.level)
          if (msg.message.includes('Player 2 joined') || msg.message.includes('is back online')) {
            setPartnerOnline(true)
          }
          if (msg.message.includes('partner is idle')) {
            setPartnerOnline(false)
          }
          if (msg.message.includes('partner left')) {
            setPartnerOnline(false)
            endSession('Your partner left the chat')
          }
        } else {
          setMessages(prev => [...prev, msg])
        }
      } catch {
        addEvent(e.data, 'info')
      }
    }

    ws.onclose = (e) => {
      wsRef.current = null
      if (isUnmountedRef.current) return
      // 1008 = server rejected join/create (invalid or expired code).
      if (e.code === 1008) {
        setStatus('disconnected')
        onEnd(e.reason || 'Could not join room — invalid or expired code')
        return
      }
      // 1000 = clean close — only "user left" ends the session; inactivity stays in room.
      if (e.code === 1000) {
        setStatus('disconnected')
        if (e.reason === 'user left') return
        addEvent(`${playerName} idle — click the chat to reconnect`, 'warn')
        return
      }
      // 1001 = server-side close (inactivity timeout from backend) — reconnect.
      addEvent(`${playerName} connection lost (code ${e.code}) — retrying`, 'error')
      scheduleReconnect(connectFn)
    }

    ws.onerror = () => addEvent(`${playerName} WebSocket error`, 'error')
  }, [playerName, addEvent, scheduleReconnect, endSession, onEnd])

  const connect = useCallback(() => {
    if (wsRef.current) return
    const ws = new WebSocket(`${WS_URL}/ws?room=${roomCode}&player=${playerNum}`)
    wsRef.current = ws

    ws.onopen = () => {
      if (isUnmountedRef.current) { ws.close(); return }
      resetBackoff()
      setStatus('connected')
      addEvent(`${playerName} connected`, 'success')
      resetInactivityTimer()
    }

    attachHandlers(ws, connect)
  }, [roomCode, playerNum, playerName, addEvent, resetBackoff, resetInactivityTimer, attachHandlers])

  // ── Mount ────────────────────────────────────────────────────────────────────

  useEffect(() => {
    isUnmountedRef.current = false

    if (existingWs) {
      // Adopt the WebSocket opened in WaitingScreen / JoiningScreen.
      wsRef.current = existingWs
      if (existingWs.readyState === WebSocket.OPEN) {
        setStatus('connected')
        resetInactivityTimer()
        if (playerNum === 2) setPartnerOnline(true)
      }
      attachHandlers(existingWs, connect)
    } else {
      connect()
    }

    return () => {
      isUnmountedRef.current = true
      clearTimeout(inactivityTimerRef.current)
      clearTimeout(reconnectTimerRef.current)
      // Do not close the WebSocket here — React Strict Mode remounts this
      // component in dev, which would tear down the room. App closes on leave.
    }
  }, []) // eslint-disable-line react-hooks/exhaustive-deps
  // Intentionally empty deps — we only want this to run once on mount.

  // ── User interaction handlers ─────────────────────────────────────────────

  const handleFocus = useCallback(() => {
    clearTimeout(reconnectTimerRef.current)
    if (!wsRef.current || wsRef.current.readyState !== WebSocket.OPEN) {
      addEvent(`${playerName} became active — reconnecting`, 'info')
      connect()
    }
    resetInactivityTimer()
  }, [playerName, addEvent, connect, resetInactivityTimer])

  const handleSend = useCallback(() => {
    const text = input.trim()
    if (!text) return
    if (!wsRef.current || wsRef.current.readyState !== WebSocket.OPEN) {
      addEvent(`${playerName} tried to send but is disconnected — reconnecting`, 'warn')
      connect()
      return
    }
    const msg = { from: playerName, text, ts: Date.now() }
    wsRef.current.send(JSON.stringify(msg))
    setMessages(prev => [...prev, { ...msg, own: true }])
    setInput('')
    resetInactivityTimer()
  }, [input, playerName, addEvent, connect, resetInactivityTimer])

  const handleKeyDown = useCallback((e) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      handleSend()
    }
  }, [handleSend])

  const handleLeave = useCallback(() => {
    clearTimeout(inactivityTimerRef.current)
    clearTimeout(reconnectTimerRef.current)
    if (wsRef.current) {
      wsRef.current.close(1000, 'user left')
      wsRef.current = null
    }
    onEnd('You left the chat')
  }, [onEnd])

  // ── Render ───────────────────────────────────────────────────────────────────

  const statusLabel = {
    connecting:   '◌ connecting…',
    connected:    '● online',
    disconnected: '○ offline',
    reconnecting: '◌ reconnecting…',
    waking:       '◌ server waking up (~50s)…',
  }[status] ?? '○ offline'

  return (
    <div
      className={`chatbox chatbox--${status}`}
      style={{ maxWidth: '640px', width: '100%', margin: '0 auto' }}
    >
      <div className="chatbox-header">
        <span className="chatbox-user">{playerName}</span>
        <span style={{ fontSize: '11px', color: 'var(--muted)', letterSpacing: '0.05em' }}>
          room&nbsp;{roomCode}
        </span>
        <span className={`chatbox-status chatbox-status--${status}`}>{statusLabel}</span>
        <button className="leave-btn" onClick={handleLeave}>leave</button>
      </div>

      {playerNum === 1 && !partnerOnline && status === 'connected' && (
        <div className="waiting-banner">
          Waiting for Player 2… messages you send now will be delivered when they join.
        </div>
      )}

      <div className="chatbox-messages">
        {messages.length === 0 && (
          <p className="chatbox-empty">no messages yet</p>
        )}
        {messages.map((m, i) => (
          <div
            key={i}
            className={`message ${
              m.own || m.from === playerName ? 'message--own' : 'message--other'
            }`}
          >
            <span className="message-text">{m.text}</span>
          </div>
        ))}
        <div ref={messagesEndRef} />
      </div>

      <div className="chatbox-input-row">
        <input
          className="chatbox-input"
          type="text"
          placeholder="Type a message…"
          value={input}
          onChange={e => setInput(e.target.value)}
          onFocus={handleFocus}
          onKeyDown={handleKeyDown}
        />
        <button className="chatbox-send" onClick={handleSend}>send</button>
      </div>
    </div>
  )
}
