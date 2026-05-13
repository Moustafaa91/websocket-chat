import { useState, useEffect, useRef, useCallback } from 'react'

const WS_URL = import.meta.env.VITE_WS_URL

const BACKOFF_BASE_MS = 1_000
const BACKOFF_MAX_MS = 30_000
const COLD_START_THRESHOLD_MS = 10_000 // after this long retrying, show cold-start message

export default function ChatBox({ user, addEvent }) {
  const [messages, setMessages] = useState([])
  const [input, setInput] = useState('')
  const [status, setStatus] = useState('disconnected') // 'connected' | 'disconnected' | 'reconnecting' | 'waking'
  const wsRef = useRef(null)
  const inactivityTimerRef = useRef(null)
  const reconnectTimerRef = useRef(null)
  const isUnmountedRef = useRef(false)
  const retryDelayRef = useRef(BACKOFF_BASE_MS)   // current backoff delay, persists across renders
  const retryStartRef = useRef(null)               // timestamp of first retry attempt in current backoff sequence
  const messagesEndRef = useRef(null)

  const scrollToBottom = () => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }

  useEffect(() => {
    scrollToBottom()
  }, [messages])

  const resetBackoff = useCallback(() => {
    retryDelayRef.current = BACKOFF_BASE_MS
    retryStartRef.current = null
  }, [])

  const scheduleReconnect = useCallback((connectFn) => {
    // Record when the first retry in this sequence started
    if (retryStartRef.current === null) {
      retryStartRef.current = Date.now()
    }

    const elapsed = Date.now() - retryStartRef.current
    const isLikelyColdStart = elapsed >= COLD_START_THRESHOLD_MS

    if (isLikelyColdStart) {
      setStatus('waking')
    } else {
      setStatus('reconnecting')
    }

    reconnectTimerRef.current = setTimeout(() => {
      if (!isUnmountedRef.current) connectFn()
    }, retryDelayRef.current)

    // Advance backoff for next attempt
    retryDelayRef.current = Math.min(retryDelayRef.current * 2, BACKOFF_MAX_MS)
  }, [])

  const disconnect = useCallback((reason = 'inactivity') => {
    clearTimeout(inactivityTimerRef.current)
    clearTimeout(reconnectTimerRef.current)
    resetBackoff()

    if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN) {
      wsRef.current.close(1000, reason)
    }
    wsRef.current = null

    if (!isUnmountedRef.current) {
      setStatus('disconnected')
      addEvent(`${user} disconnected — ${reason}`, 'warn')
    }
  }, [user, addEvent, resetBackoff])

  const connect = useCallback(() => {
    if (wsRef.current) return

    const ws = new WebSocket(`${WS_URL}/ws?user=${user.toLowerCase()}`)
    wsRef.current = ws

    ws.onopen = () => {
      if (isUnmountedRef.current) { ws.close(); return }
      resetBackoff()
      setStatus('connected')
      addEvent(`${user} connected`, 'success')
    }

    ws.onmessage = (e) => {
      if (isUnmountedRef.current) return
      try {
        const msg = JSON.parse(e.data)
        // Distinguish chat messages from log events by shape:
        // chat messages have a 'text' field; log events have a 'level' field.
        if (msg.level) {
          addEvent(msg.message, msg.level)
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

      // code 1000 = clean close (inactivity timeout, unmount, or explicit disconnect).
      // Do NOT auto-reconnect. Wait for explicit user action.
      if (e.code === 1000) {
        setStatus('disconnected')
        return
      }

      // Unexpected loss — retry with exponential backoff.
      addEvent(`${user} connection lost (code ${e.code}) — retrying`, 'error')
      scheduleReconnect(connect)
    }

    ws.onerror = () => {
      // onerror always fires before onclose — just log it.
      // The retry logic lives in onclose to avoid double-scheduling.
      addEvent(`${user} WebSocket error`, 'error')
    }
  }, [user, addEvent, resetBackoff, scheduleReconnect])

  // On mount: connect immediately.
  useEffect(() => {
    isUnmountedRef.current = false
    connect()
    return () => {
      isUnmountedRef.current = true
      clearTimeout(inactivityTimerRef.current)
      clearTimeout(reconnectTimerRef.current)
      if (wsRef.current) wsRef.current.close(1000, 'unmount')
    }
  }, [connect])

  const resetInactivityTimer = useCallback(() => {
    clearTimeout(inactivityTimerRef.current)
    inactivityTimerRef.current = setTimeout(() => {
      disconnect('inactivity')
    }, 10_000)
  }, [disconnect])

  const handleFocus = useCallback(() => {
    // User is present — attempt immediately regardless of backoff wait.
    // If the attempt fails, backoff resumes from its current delay (not reset).
    clearTimeout(reconnectTimerRef.current)

    if (!wsRef.current || wsRef.current.readyState !== WebSocket.OPEN) {
      addEvent(`${user} became active — reconnecting`, 'info')
      connect()
    }
    resetInactivityTimer()
  }, [user, addEvent, connect, resetInactivityTimer])

  const handleChange = useCallback((e) => {
    setInput(e.target.value)
    resetInactivityTimer()
  }, [resetInactivityTimer])

  const handleSend = useCallback(() => {
    const text = input.trim()
    if (!text) return

    if (!wsRef.current || wsRef.current.readyState !== WebSocket.OPEN) {
      addEvent(`${user} tried to send but is disconnected — reconnecting`, 'warn')
      connect()
      return
    }

    const msg = { from: user.toLowerCase(), text, ts: Date.now() }
    wsRef.current.send(JSON.stringify(msg))
    setMessages(prev => [...prev, { ...msg, own: true }])
    setInput('')
    resetInactivityTimer()
    addEvent(`${user} sent: "${text}"`, 'info')
  }, [input, user, addEvent, connect, resetInactivityTimer])

  const handleKeyDown = useCallback((e) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      handleSend()
    }
  }, [handleSend])

  const statusLabel = {
    connected: '● online',
    disconnected: '○ offline',
    reconnecting: '◌ reconnecting…',
    waking: '◌ server waking up (~50s)…',
  }[status]

  return (
    <div className={`chatbox chatbox--${status}`}>
      <div className="chatbox-header">
        <span className="chatbox-user">{user}</span>
        <span className={`chatbox-status chatbox-status--${status}`}>{statusLabel}</span>
      </div>

      <div className="chatbox-messages">
        {messages.length === 0 && (
          <p className="chatbox-empty">no messages yet</p>
        )}
        {messages.map((m, i) => (
          <div
            key={i}
            className={`message ${m.own || m.from === user.toLowerCase() ? 'message--own' : 'message--other'}`}
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
          placeholder={`${user} says…`}
          value={input}
          onChange={handleChange}
          onFocus={handleFocus}
          onKeyDown={handleKeyDown}
        />
        <button className="chatbox-send" onClick={handleSend}>
          send
        </button>
      </div>
    </div>
  )
}
