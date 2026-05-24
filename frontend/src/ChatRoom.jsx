import { useState, useEffect, useRef, useCallback } from 'react'

const WS_URL = import.meta.env.VITE_WS_URL
const INACTIVITY_MS = 10_000

export default function ChatRoom({ roomCode, playerNum, existingWs, addEvent, onEnd }) {
  const [messages, setMessages] = useState([])
  const [input, setInput] = useState('')
  const [status, setStatus] = useState('connecting') // online | inactive | offline | connecting
  const [partnerPresence, setPartnerPresence] = useState('absent')

  const wsRef = useRef(null)
  const inactivityTimerRef = useRef(null)
  const isUnmountedRef = useRef(false)
  const messagesEndRef = useRef(null)

  const playerName = `Player ${playerNum}`
  const partnerName = playerNum === 1 ? 'Player 2' : 'Player 1'

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages])

  const resetInactivityTimer = useCallback(() => {
    clearTimeout(inactivityTimerRef.current)
    inactivityTimerRef.current = setTimeout(() => {
      if (wsRef.current?.readyState === WebSocket.OPEN) {
        wsRef.current.close(1000, 'inactivity')
      }
    }, INACTIVITY_MS)
  }, [])

  const endSession = useCallback((reason) => {
    clearTimeout(inactivityTimerRef.current)
    if (wsRef.current) {
      wsRef.current.close(1000, 'user left')
      wsRef.current = null
    }
    onEnd(reason)
  }, [onEnd])

  const goOfflineOnUnload = useCallback(() => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.close(1000, 'offline')
    }
  }, [])

  const applyPartnerPresence = useCallback((presence) => {
    setPartnerPresence(presence)
  }, [])

  const attachHandlers = useCallback((ws) => {
    ws.onmessage = (e) => {
      if (isUnmountedRef.current) return
      try {
        const msg = JSON.parse(e.data)
        if (msg.kind === 'presence') {
          if (msg.player === partnerName) {
            applyPartnerPresence(msg.presence)
          }
          addEvent(msg.message, 'info')
          return
        }
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
      clearTimeout(inactivityTimerRef.current)
      if (isUnmountedRef.current) return

      if (e.code === 1008) {
        setStatus('offline')
        onEnd(e.reason || 'Could not join room — invalid or expired code')
        return
      }

      if (e.reason === 'user left' || e.reason === 'offline') {
        setStatus('offline')
        if (e.reason === 'user left') onEnd('You left the chat')
        return
      }

      if (e.reason === 'inactivity' || e.code === 1001) {
        setStatus('inactive')
        addEvent(`${playerName} inactive — focus the chat to come back online`, 'warn')
        return
      }

      setStatus('inactive')
      addEvent(`${playerName} connection closed — focus the chat to reconnect`, 'warn')
    }

    ws.onerror = () => addEvent(`${playerName} WebSocket error`, 'error')
  }, [playerName, partnerName, addEvent, onEnd, applyPartnerPresence])

  const connect = useCallback(() => {
    if (wsRef.current) return
    const ws = new WebSocket(`${WS_URL}/ws?room=${roomCode}&player=${playerNum}`)
    wsRef.current = ws

    ws.onopen = () => {
      if (isUnmountedRef.current) { ws.close(); return }
      setStatus('online')
      addEvent(`${playerName} online`, 'success')
      resetInactivityTimer()
    }

    attachHandlers(ws)
  }, [roomCode, playerNum, playerName, addEvent, resetInactivityTimer, attachHandlers])

  useEffect(() => {
    isUnmountedRef.current = false

    if (existingWs) {
      wsRef.current = existingWs
      if (existingWs.readyState === WebSocket.OPEN) {
        setStatus('online')
        resetInactivityTimer()
        if (playerNum === 2) setPartnerPresence('online')
      }
      attachHandlers(existingWs)
    } else {
      connect()
    }

    window.addEventListener('pagehide', goOfflineOnUnload)

    return () => {
      isUnmountedRef.current = true
      window.removeEventListener('pagehide', goOfflineOnUnload)
      clearTimeout(inactivityTimerRef.current)
    }
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  const handleChatActivity = useCallback(() => {
    if (!wsRef.current || wsRef.current.readyState !== WebSocket.OPEN) {
      addEvent(`${playerName} active — reconnecting`, 'info')
      connect()
      return
    }
    resetInactivityTimer()
  }, [playerName, addEvent, connect, resetInactivityTimer])

  const handleSend = useCallback(() => {
    const text = input.trim()
    if (!text) return
    if (!wsRef.current || wsRef.current.readyState !== WebSocket.OPEN) {
      addEvent(`${playerName} inactive — focus the chat to send`, 'warn')
      return
    }
    const msg = { from: playerName, text, ts: Date.now() }
    wsRef.current.send(JSON.stringify(msg))
    setMessages(prev => [...prev, { ...msg, own: true }])
    setInput('')
    resetInactivityTimer()
  }, [input, playerName, addEvent, resetInactivityTimer])

  const handleKeyDown = useCallback((e) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      handleSend()
    }
  }, [handleSend])

  const handleLeave = useCallback(() => {
    clearTimeout(inactivityTimerRef.current)
    if (wsRef.current) {
      wsRef.current.close(1000, 'user left')
      wsRef.current = null
    }
    onEnd('You left the chat')
  }, [onEnd])

  const statusLabel = {
    connecting: '◌ connecting…',
    online:     '● online',
    inactive:   '◐ inactive',
    offline:    '○ offline',
  }[status] ?? '○ offline'

  const partnerLabel = {
    absent:   'partner: waiting…',
    online:   'partner: ● online',
    inactive: 'partner: ◐ inactive',
    offline:  'partner: ○ offline',
  }[partnerPresence] ?? ''

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
        <span style={{ fontSize: '11px', color: 'var(--muted)' }}>{partnerLabel}</span>
        <button className="leave-btn" onClick={handleLeave}>leave</button>
      </div>

      {playerNum === 1 && partnerPresence === 'absent' && status === 'online' && (
        <div className="waiting-banner">
          Waiting for Player 2… messages you send now will be delivered when they join.
        </div>
      )}

      {partnerPresence === 'inactive' && status === 'online' && (
        <div className="waiting-banner">
          Your partner is inactive — messages will be delivered when they return.
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
          placeholder={status === 'inactive' ? 'Focus here to reconnect…' : 'Type a message…'}
          value={input}
          onChange={e => setInput(e.target.value)}
          onFocus={handleChatActivity}
          onKeyDown={e => { handleChatActivity(); handleKeyDown(e) }}
        />
        <button className="chatbox-send" onClick={handleSend}>send</button>
      </div>
    </div>
  )
}
