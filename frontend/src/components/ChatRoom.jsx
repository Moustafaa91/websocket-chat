import { useState, useEffect, useRef, useCallback } from 'react'
import { connectRoom } from '../api/websocket'
import {
  CLOSE_REASON,
  INACTIVITY_MS,
  PRESENCE,
  partnerName,
  playerName,
} from '../constants/presence'

export default function ChatRoom({ roomCode, playerNum, existingWs, addEvent, onEnd }) {
  const [messages, setMessages] = useState([])
  const [input, setInput] = useState('')
  const [status, setStatus] = useState('connecting')
  const [partnerPresence, setPartnerPresence] = useState(PRESENCE.ABSENT)

  const wsRef = useRef(null)
  const inactivityTimerRef = useRef(null)
  const isUnmountedRef = useRef(false)
  const messagesEndRef = useRef(null)

  const name = playerName(playerNum)
  const partner = partnerName(playerNum)

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages])

  const resetInactivityTimer = useCallback(() => {
    clearTimeout(inactivityTimerRef.current)
    inactivityTimerRef.current = setTimeout(() => {
      if (wsRef.current?.readyState === WebSocket.OPEN) {
        wsRef.current.close(1000, CLOSE_REASON.INACTIVITY)
      }
    }, INACTIVITY_MS)
  }, [])

  const goOfflineOnUnload = useCallback(() => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.close(1000, CLOSE_REASON.OFFLINE)
    }
  }, [])

  const attachHandlers = useCallback((ws) => {
    ws.onmessage = (e) => {
      if (isUnmountedRef.current) return
      try {
        const msg = JSON.parse(e.data)
        if (msg.kind === 'presence') {
          if (msg.player === partner) {
            setPartnerPresence(msg.presence)
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
        setStatus(PRESENCE.OFFLINE)
        onEnd(e.reason || 'Could not join room - invalid or expired code')
        return
      }

      if (e.reason === CLOSE_REASON.USER_LEFT || e.reason === CLOSE_REASON.OFFLINE) {
        setStatus(PRESENCE.OFFLINE)
        if (e.reason === CLOSE_REASON.USER_LEFT) onEnd('You left the chat')
        return
      }

      if (e.reason === CLOSE_REASON.INACTIVITY || e.code === 1001) {
        setStatus(PRESENCE.INACTIVE)
        addEvent(`${name} inactive - focus the chat to come back online`, 'warn')
        return
      }

      setStatus(PRESENCE.INACTIVE)
      addEvent(`${name} connection closed - focus the chat to reconnect`, 'warn')
    }

    ws.onerror = () => addEvent(`${name} WebSocket error`, 'error')
  }, [name, partner, addEvent, onEnd])

  const connect = useCallback(() => {
    if (wsRef.current) return

    connectRoom(roomCode, playerNum)
      .then(ws => {
        if (isUnmountedRef.current) {
          ws.close()
          return
        }
        wsRef.current = ws
        setStatus(PRESENCE.ONLINE)
        addEvent(`${name} online`, 'success')
        resetInactivityTimer()
        attachHandlers(ws)
      })
      .catch(err => {
        addEvent(err.message, 'error')
        setStatus(PRESENCE.INACTIVE)
      })
  }, [roomCode, playerNum, name, addEvent, resetInactivityTimer, attachHandlers])

  useEffect(() => {
    isUnmountedRef.current = false

    if (existingWs) {
      wsRef.current = existingWs
      if (existingWs.readyState === WebSocket.OPEN) {
        setStatus(PRESENCE.ONLINE)
        resetInactivityTimer()
        setPartnerPresence(PRESENCE.ONLINE)
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
      addEvent(`${name} active - reconnecting`, 'info')
      connect()
      return
    }
    resetInactivityTimer()
  }, [name, addEvent, connect, resetInactivityTimer])

  const handleSend = useCallback(() => {
    const text = input.trim()
    if (!text) return
    if (!wsRef.current || wsRef.current.readyState !== WebSocket.OPEN) {
      addEvent(`${name} inactive - focus the chat to send`, 'warn')
      return
    }
    const msg = { from: name, text, ts: Date.now() }
    wsRef.current.send(JSON.stringify(msg))
    setMessages(prev => [...prev, { ...msg, own: true }])
    setInput('')
    resetInactivityTimer()
  }, [input, name, addEvent, resetInactivityTimer])

  const handleKeyDown = useCallback((e) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      handleSend()
    }
  }, [handleSend])

  const handleLeave = useCallback(() => {
    clearTimeout(inactivityTimerRef.current)
    if (wsRef.current) {
      wsRef.current.close(1000, CLOSE_REASON.USER_LEFT)
      wsRef.current = null
    }
    onEnd('You left the chat')
  }, [onEnd])

  const statusLabel = {
    connecting: 'connecting...',
    online: 'online',
    inactive: 'inactive',
    offline: 'offline',
  }[status] ?? 'offline'

  const partnerLabel = {
    [PRESENCE.ABSENT]: 'partner: waiting...',
    [PRESENCE.ONLINE]: 'partner: online',
    [PRESENCE.INACTIVE]: 'partner: inactive',
    [PRESENCE.OFFLINE]: 'partner: offline',
  }[partnerPresence] ?? ''

  return (
    <div className={`chatbox chatbox--${status}`}>
      <div className="chatbox-header">
        <span className="chatbox-user">{name}</span>
        <span className="chatbox-room">room&nbsp;{roomCode}</span>
        <span className={`chatbox-status chatbox-status--${status}`}>{statusLabel}</span>
        <span className="chatbox-partner">{partnerLabel}</span>
        <button type="button" className="leave-btn" onClick={handleLeave}>leave</button>
      </div>

      {playerNum === 1 && partnerPresence === PRESENCE.ABSENT && status === PRESENCE.ONLINE && (
        <div className="waiting-banner">
          Waiting for Player 2. Messages you send now will be delivered when they join.
        </div>
      )}

      {partnerPresence === PRESENCE.INACTIVE && status === PRESENCE.ONLINE && (
        <div className="waiting-banner">
          Your partner is inactive - messages will be delivered when they return.
        </div>
      )}

      <div className="chatbox-messages">
        {messages.length === 0 && <p className="chatbox-empty">no messages yet</p>}
        {messages.map((m, i) => (
          <div
            key={`${m.ts}-${i}`}
            className={`message ${m.own || m.from === name ? 'message--own' : 'message--other'}`}
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
          placeholder={status === PRESENCE.INACTIVE ? 'Focus here to reconnect...' : 'Type a message...'}
          value={input}
          onChange={e => setInput(e.target.value)}
          onFocus={handleChatActivity}
          onKeyDown={e => { handleChatActivity(); handleKeyDown(e) }}
        />
        <button type="button" className="chatbox-send" onClick={handleSend}>send</button>
      </div>
    </div>
  )
}
