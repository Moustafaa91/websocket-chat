import { API_URL } from '../config'

async function parseError(res) {
  try {
    const body = await res.json()
    return body.error || res.statusText
  } catch {
    return res.statusText || 'request failed'
  }
}

export async function createRoom() {
  const res = await fetch(`${API_URL}/rooms`, { method: 'POST' })
  if (!res.ok) {
    throw new Error(await parseError(res))
  }
  const { code } = await res.json()
  return code
}

export async function validateRoom(code, player) {
  const res = await fetch(`${API_URL}/rooms/${encodeURIComponent(code)}?player=${player}`)
  if (!res.ok) {
    throw new Error(await parseError(res))
  }
}
