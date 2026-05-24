import { useEffect, useRef } from 'react'
import { connectRoom } from '../api/websocket'

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

  useEffect(() => {
    if (roomWsRef.current?.readyState === WebSocket.OPEN) {
      onReady(roomWsRef.current, code)
      return
    }

    if (connectionRef.current) {
      connectionRef.current.cancelled = false
      if (connectionRef.current.ws?.readyState === WebSocket.OPEN && !firedRef.current) {
        firedRef.current = true
        onReady(connectionRef.current.ws, code)
      }
      return () => {
        connectionRef.current.cancelled = true
      }
    }

    const attempt = { cancelled: false, ws: null }
    connectionRef.current = attempt
    connectStartedRef.current = true

    connectRoom(code, 1)
      .then(ws => {
        attempt.ws = ws
        if (attempt.cancelled || firedRef.current) {
          ws.close(1000, 'cancelled')
          return
        }
        firedRef.current = true
        onReady(ws, code)
      })
      .catch(err => {
        if (!attempt.cancelled) {
          connectStartedRef.current = false
          connectionRef.current = null
          addEvent(err.message || 'Connection error on room ' + code, 'error')
        }
      })

    return () => {
      attempt.cancelled = true
    }
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <div className="home-screen">
      <div className="home-card">
        <h2 className="home-title">Waiting for Player 2</h2>
        <p className="home-desc">Share this code with your friend:</p>
        <div className="room-code">{code}</div>
        <p className="home-desc home-desc--hint">
          Valid until someone joins or you cancel.
        </p>
        <button
          className="btn btn--secondary"
          type="button"
          style={{ marginTop: '1.5rem' }}
          onClick={onCancel}
        >
          Cancel
        </button>
      </div>
    </div>
  )
}
