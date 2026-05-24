import { useEffect, useRef, useState } from 'react'
import { connectRoom } from '../api/websocket'
import { PRESENCE } from '../constants/presence'

function CopyIcon() {
  return (
    <svg
      aria-hidden="true"
      className="icon"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <rect width="14" height="14" x="8" y="8" rx="2" ry="2" />
      <path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2" />
    </svg>
  )
}

export default function WaitingScreen({
  code,
  connectStartedRef,
  roomWsRef,
  onCancel,
  onReady,
  addEvent,
}) {
  const firedRef = useRef(false)
  const connectionRef = useRef(null)
  const [copyState, setCopyState] = useState('idle')

  useEffect(() => {
    let reconnectTimer = null
    let keepAliveTimer = null

    function stopKeepAlive() {
      if (!keepAliveTimer) return
      window.clearInterval(keepAliveTimer)
      keepAliveTimer = null
    }

    function startKeepAlive(ws) {
      stopKeepAlive()
      keepAliveTimer = window.setInterval(() => {
        if (ws.readyState === WebSocket.OPEN) {
          ws.send(JSON.stringify({ ping: true }))
        }
      }, 5000)
    }

    function cleanupWaitingConnection() {
      if (connectionRef.current) {
        connectionRef.current.cancelled = true
      }
      if (reconnectTimer) window.clearTimeout(reconnectTimer)
      stopKeepAlive()
    }

    function openChat(ws) {
      firedRef.current = true
      if (reconnectTimer) window.clearTimeout(reconnectTimer)
      stopKeepAlive()
      onReady(ws, code)
    }

    function scheduleReconnect() {
      if (firedRef.current || reconnectTimer) return

      reconnectTimer = window.setTimeout(() => {
        reconnectTimer = null
        if (firedRef.current) return

        const attempt = { cancelled: false, ws: null }
        connectionRef.current = attempt
        connectStartedRef.current = true

        connectRoom(code, 1)
          .then(ws => {
            waitForPartner(ws, attempt, true)
          })
          .catch(err => {
            if (!attempt.cancelled) {
              connectStartedRef.current = false
              connectionRef.current = null
              roomWsRef.current = null
              addEvent(err.message || 'Connection error on room ' + code, 'error')
              scheduleReconnect()
            }
          })
      }, 1000)
    }

    function waitForPartner(ws, attempt, shouldLogConnection = false) {
      attempt.ws = ws
      roomWsRef.current = ws

      if (attempt.cancelled || firedRef.current) {
        ws.close(1000, 'cancelled')
        return
      }

      if (shouldLogConnection) {
        addEvent(`Connected to room ${code} - waiting for Player 2`, 'info')
      }

      startKeepAlive(ws)

      ws.onmessage = (e) => {
        if (attempt.cancelled || firedRef.current) return

        try {
          const msg = JSON.parse(e.data)
          const partnerJoined =
            msg.kind === 'presence' &&
            msg.player === 'Player 2' &&
            msg.presence === PRESENCE.ONLINE

          if (!partnerJoined) return

          openChat(ws)
        } catch {
          addEvent(e.data, 'info')
        }
      }

      ws.onclose = (e) => {
        if (attempt.cancelled || firedRef.current) return

        connectStartedRef.current = false
        connectionRef.current = null
        roomWsRef.current = null
        stopKeepAlive()
        addEvent(e.reason || 'Reconnecting while waiting for Player 2', 'warn')
        scheduleReconnect()
      }

      ws.onerror = () => {
        if (attempt.cancelled || firedRef.current) return

        connectStartedRef.current = false
        connectionRef.current = null
        roomWsRef.current = null
        stopKeepAlive()
        addEvent('WebSocket error while waiting for Player 2 - reconnecting', 'error')
        scheduleReconnect()
      }
    }

    if (roomWsRef.current?.readyState === WebSocket.OPEN) {
      const attempt = { cancelled: false, ws: roomWsRef.current }
      connectionRef.current = attempt
      waitForPartner(roomWsRef.current, attempt)
      return cleanupWaitingConnection
    }

    if (connectionRef.current) {
      connectionRef.current.cancelled = false
      if (connectionRef.current.ws?.readyState === WebSocket.OPEN) {
        waitForPartner(connectionRef.current.ws, connectionRef.current)
      }
      return cleanupWaitingConnection
    }

    const attempt = { cancelled: false, ws: null }
    connectionRef.current = attempt
    connectStartedRef.current = true

    connectRoom(code, 1)
      .then(ws => {
        waitForPartner(ws, attempt, true)
      })
      .catch(err => {
        if (!attempt.cancelled) {
          connectStartedRef.current = false
          connectionRef.current = null
          roomWsRef.current = null
          addEvent(err.message || 'Connection error on room ' + code, 'error')
        }
      })

    return cleanupWaitingConnection
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  async function handleCopy() {
    try {
      if (navigator.clipboard?.writeText) {
        await navigator.clipboard.writeText(code)
      } else {
        const textarea = document.createElement('textarea')
        textarea.value = code
        textarea.setAttribute('readonly', '')
        textarea.style.position = 'absolute'
        textarea.style.left = '-9999px'
        document.body.appendChild(textarea)
        textarea.select()
        document.execCommand('copy')
        document.body.removeChild(textarea)
      }

      setCopyState('copied')
      addEvent(`Room code ${code} copied`, 'success')
      window.setTimeout(() => setCopyState('idle'), 1800)
    } catch {
      setCopyState('failed')
      addEvent('Could not copy room code', 'error')
      window.setTimeout(() => setCopyState('idle'), 1800)
    }
  }

  return (
    <div className="home-screen">
      <div className="home-card home-card--wide">
        <div className="state-eyebrow">
          <span className="status-pulse" />
          Room ready
        </div>
        <h2 className="home-title">Waiting for your second partner</h2>
        <p className="home-desc">
          Share this code with the other participant. The chat will start as soon as they join.
        </p>

        <div className="room-code-panel" aria-label={`Room code ${code}`}>
          <div>
            <span className="room-code-label">Join code</span>
            <div className="room-code">{code}</div>
          </div>
          <button
            className="copy-button"
            type="button"
            onClick={handleCopy}
            aria-label="Copy room code"
            title="Copy room code"
          >
            <CopyIcon />
            <span>{copyState === 'copied' ? 'Copied' : copyState === 'failed' ? 'Failed' : 'Copy'}</span>
          </button>
        </div>

        <div className="waiting-message" role="status" aria-live="polite">
          <span className="waiting-spinner" />
          <span>Waiting until the second partner joins the room...</span>
        </div>

        <p className="home-desc home-desc--hint">
          Keep this page open. You can cancel if you need to create a different room.
        </p>
        <button
          className="btn btn--secondary"
          type="button"
          onClick={onCancel}
        >
          Cancel
        </button>
      </div>
    </div>
  )
}
