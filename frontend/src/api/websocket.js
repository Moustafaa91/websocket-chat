import { WS_URL } from '../config'

/**
 * Opens a room WebSocket for the given player slot (1 or 2).
 * Resolves with the socket once the connection is open.
 */
export function connectRoom(code, player) {
  return new Promise((resolve, reject) => {
    const ws = new WebSocket(`${WS_URL}/ws?room=${encodeURIComponent(code)}&player=${player}`)
    let settled = false

    ws.onopen = () => {
      if (!settled) {
        settled = true
        resolve(ws)
      }
    }

    ws.onerror = () => {
      if (!settled) {
        settled = true
        reject(new Error('Could not connect - check your connection and try again'))
      }
    }

    ws.onclose = (e) => {
      if (!settled) {
        settled = true
        reject(new Error(e.reason || 'Connection closed before chat started'))
      }
    }
  })
}
