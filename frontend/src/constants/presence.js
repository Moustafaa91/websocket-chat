export const PRESENCE = {
  ABSENT: 'absent',
  ONLINE: 'online',
  INACTIVE: 'inactive',
  OFFLINE: 'offline',
}

export const CLOSE_REASON = {
  USER_LEFT: 'user left',
  OFFLINE: 'offline',
  INACTIVITY: 'inactivity',
}

export const INACTIVITY_MS = 10_000

export function playerName(num) {
  return `Player ${num}`
}

export function partnerName(num) {
  return num === 1 ? 'Player 2' : 'Player 1'
}
