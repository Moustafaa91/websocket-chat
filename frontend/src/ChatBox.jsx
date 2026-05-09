import { useState, useEffect, useRef, useCallback } from 'react'

const WS_URL = import.meta.env.VITE_WS_URL
const RECONNECT_DELAY_MS = 500

export default function ChatBox({ user, addEvent }) {
  const [messages, setMessages] = useState([])
  const [input, setInput] = useState('')
  const [status, setStatus] = useState('disconnected') // 'connected' | 'disconnected' | 'reconnecting'

  const wsRef = useRef(null)
  const inactivityTimerRef = useRef(null)
  const reconnectTimerRef = useRef(null)
  const isUnmountedRef = useRef(false)
  const messagesEndRef = useRef(null)

  const scrollToBottom = () => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }

  useEffect(() => {
    scrollToBottom()
  }, [messages])

  const disconnect = useCallback((reason = 'inactivity') => {
    clearTimeout(inactivityTimerRef.current)
    if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN) {
      wsRef.current.close(1000, reason)
    }
    wsRef.current = null
    if (!isUnmountedRef.current) {
      setStatus('disconnected')
      addEvent(`${user} disconnected — ${reason}`, 'warn')
    }
  }, [user, addEvent])

  const connect = useCallback(() => {
    if (wsRef.current) return

    const ws = new WebSocket(`${WS_URL}/ws?user=${user.toLowerCase()}`)
    wsRef.current = ws

    ws.onopen = () => {
      if (isUnmountedRef.current) { ws.close(); return }
      setStatus('connected')
      addEvent(`${user} connected`, 'success')
    }

    ws.onmessage = (e) => {
      if (isUnmountedRef.current) return
      try {
        const msg = JSON.parse(e.data)
        setMessages(prev => [...prev, msg])
      } catch {
        // log events arrive as plain text (type: 'log')
        addEvent(e.data, 'info')
      }
    }

    ws.onclose = (e) => {
      wsRef.current = null
      if (isUnmountedRef.current) return
      setStatus('disconnected')
      // code 1000 = clean close (inactivity). Anything else = unexpected.
      if (e.code !== 1000) {
        addEvent(`${user} connection lost (code ${e.code}) — will retry`, 'error')
        setStatus('reconnecting')
        reconnectTimerRef.current = setTimeout(() => {
          if (!isUnmountedRef.current) connect()
        }, RECONNECT_DELAY_MS)
      }
    }

    ws.onerror = () => {
      addEvent(`${user} WebSocket error`, 'error')
    }
  }, [user, addEvent])

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
    // The server closes the socket after 10 s of inactivity.
    // We mirror that locally so the UI reflects reality before the server fires.
    inactivityTimerRef.current = setTimeout(() => {
      disconnect('inactivity')
    }, 10_000)
  }, [disconnect])

  const handleFocus = useCallback(() => {
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
