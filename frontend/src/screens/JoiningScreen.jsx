import { useEffect, useRef } from 'react'
import { connectRoom } from '../api/websocket'

export default function JoiningScreen({
  code,
  connectStartedRef,
  onSuccess,
  onFailure,
}) {
  const firedRef = useRef(false)
  const connectionRef = useRef(null)

  useEffect(() => {
    function cleanupConnection() {
      if (connectionRef.current) {
        connectionRef.current.cancelled = true
      }
    }

    if (connectionRef.current) {
      connectionRef.current.cancelled = false
      if (connectionRef.current.ws?.readyState === WebSocket.OPEN && !firedRef.current) {
        firedRef.current = true
        onSuccess(connectionRef.current.ws, code)
      }
      return cleanupConnection
    }

    if (connectStartedRef.current) return
    connectStartedRef.current = true

    const attempt = { cancelled: false, ws: null }
    connectionRef.current = attempt

    connectRoom(code, 2)
      .then(ws => {
        attempt.ws = ws
        if (attempt.cancelled || firedRef.current) {
          ws.close(1000, 'cancelled')
          return
        }
        firedRef.current = true
        onSuccess(ws, code)
      })
      .catch(err => {
        if (!attempt.cancelled && !firedRef.current) {
          connectStartedRef.current = false
          connectionRef.current = null
          onFailure(err.message)
        }
      })

    return cleanupConnection
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <div className="home-screen">
      <div className="home-card">
        <div className="state-eyebrow">
          <span className="status-pulse" />
          Connecting
        </div>
        <h2 className="home-title">Joining room</h2>
        <p className="home-desc">
          Establishing the WebSocket session for room <span className="room-code-inline">{code}</span>.
        </p>
        <div className="waiting-message" role="status" aria-live="polite">
          <span className="waiting-spinner" />
          <span>Checking the room and connecting you...</span>
        </div>
      </div>
    </div>
  )
}
