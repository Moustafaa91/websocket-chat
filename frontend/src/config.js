function trimTrailingSlash(value) {
  return value?.replace(/\/+$/, '')
}

function normalizeWsUrl(value) {
  const trimmed = trimTrailingSlash(value)
  if (!trimmed) return trimmed

  try {
    const url = new URL(trimmed)
    const isLocal =
      url.hostname === 'localhost' ||
      url.hostname === '127.0.0.1' ||
      url.hostname === '::1'

    if (url.protocol === 'ws:' && !isLocal) {
      url.protocol = 'wss:'
    }

    return trimTrailingSlash(url.toString())
  } catch {
    return trimmed
  }
}

export const API_URL = trimTrailingSlash(import.meta.env.VITE_API_URL)
export const WS_URL = normalizeWsUrl(import.meta.env.VITE_WS_URL)
