// Minimal API client for the bes-blind backend.
//
// Auth model (temporary):
// - The backend expects an authenticated user subject in header `X-User-Sub`
// - Real OIDC login/registration will be wired later; for now we store a "sub" in localStorage
//
// Anonymous users can still call endpoints that do not require auth.
//
// You can change the base URL with:
//   - VITE_API_BASE_URL (e.g. "http://localhost:8080")
// Default is "http://localhost:8080"

const DEFAULT_BASE_URL = 'http://localhost:8080'
const LS_USER_SUB_KEY = 'besblind.userSub'

export function getApiBaseUrl() {
  return (import.meta?.env?.VITE_API_BASE_URL || DEFAULT_BASE_URL).replace(/\/+$/, '')
}

export function getUserSub() {
  try {
    return (localStorage.getItem(LS_USER_SUB_KEY) || '').trim()
  } catch {
    return ''
  }
}

export function setUserSub(sub) {
  const v = (sub || '').trim()
  try {
    if (!v) localStorage.removeItem(LS_USER_SUB_KEY)
    else localStorage.setItem(LS_USER_SUB_KEY, v)
  } catch {
    // ignore
  }
  return v
}

export function clearUserSub() {
  return setUserSub('')
}

export class ApiError extends Error {
  constructor(message, { status = 0, payload = null, url = '', method = '' } = {}) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.payload = payload
    this.url = url
    this.method = method
  }
}

function buildHeaders({ auth = false, extraHeaders = {} } = {}) {
  const headers = {
    Accept: 'application/json',
    ...extraHeaders,
  }

  if (auth) {
    const sub = getUserSub()
    if (sub) headers['X-User-Sub'] = sub
  }

  return headers
}

async function parseJsonSafely(res) {
  const ct = (res.headers.get('content-type') || '').toLowerCase()
  if (!ct.includes('application/json')) return null
  try {
    return await res.json()
  } catch {
    return null
  }
}

async function request(path, { method = 'GET', auth = false, body, headers = {}, signal } = {}) {
  const base = getApiBaseUrl()
  const url = `${base}${path.startsWith('/') ? '' : '/'}${path}`

  const init = {
    method,
    headers: buildHeaders({ auth, extraHeaders: headers }),
    signal,
  }

  if (body !== undefined) {
    init.headers['Content-Type'] = 'application/json'
    init.body = JSON.stringify(body)
  }

  const res = await fetch(url, init)
  const payload = await parseJsonSafely(res)

  if (!res.ok) {
    const msg =
      (payload && (payload.error || payload.message)) ||
      `${method} ${path} failed with status ${res.status}`
    throw new ApiError(msg, { status: res.status, payload, url, method })
  }

  // Prefer JSON payload; if none, return null.
  return payload
}

// --------------------
// Public API methods
// --------------------

export const api = {
  // Rooms
  listRooms() {
    return request('/api/rooms')
  },

  createRoom({ name }) {
    return request('/api/rooms', { method: 'POST', auth: true, body: { name } })
  },

  getRoom(roomId) {
    return request(`/api/rooms/${encodeURIComponent(roomId)}`)
  },

  joinRoom(roomId, { nickname, pictureUrl } = {}) {
    return request(`/api/rooms/${encodeURIComponent(roomId)}/join`, {
      method: 'POST',
      auth: false, // anonymous allowed
      body: { nickname, pictureUrl },
    })
  },

  leaveRoom(roomId, { playerId }) {
    return request(`/api/rooms/${encodeURIComponent(roomId)}/leave`, {
      method: 'POST',
      auth: false,
      body: { playerId },
    })
  },

  // Owner controls
  kick(roomId, { playerId }) {
    return request(`/api/rooms/${encodeURIComponent(roomId)}/kick`, {
      method: 'POST',
      auth: true,
      body: { playerId },
    })
  },

  setScore(roomId, { playerId, score }) {
    return request(`/api/rooms/${encodeURIComponent(roomId)}/score/set`, {
      method: 'POST',
      auth: true,
      body: { playerId, score },
    })
  },

  addScore(roomId, { playerId, delta }) {
    return request(`/api/rooms/${encodeURIComponent(roomId)}/score/add`, {
      method: 'POST',
      auth: true,
      body: { playerId, delta },
    })
  },

  loadPlaylist(roomId, { playlistId }) {
    return request(`/api/rooms/${encodeURIComponent(roomId)}/playlist/load`, {
      method: 'POST',
      auth: true,
      body: { playlistId },
    })
  },

  setPlayback(roomId, { trackIndex, paused, positionMs } = {}) {
    return request(`/api/rooms/${encodeURIComponent(roomId)}/playback/set`, {
      method: 'POST',
      auth: true,
      body: { trackIndex, paused, positionMs },
    })
  },

  pause(roomId, { paused }) {
    return request(`/api/rooms/${encodeURIComponent(roomId)}/playback/pause`, {
      method: 'POST',
      auth: true,
      body: { paused },
    })
  },

  seek(roomId, { positionMs }) {
    return request(`/api/rooms/${encodeURIComponent(roomId)}/playback/seek`, {
      method: 'POST',
      auth: true,
      body: { positionMs },
    })
  },

  // Player actions
  buzz(roomId, { playerId }) {
    return request(`/api/rooms/${encodeURIComponent(roomId)}/buzz`, {
      method: 'POST',
      auth: false,
      body: { playerId },
    })
  },

  // Profile / account
  getMe() {
    return request('/api/me', { auth: true })
  },

  putMe({ nickname, pictureUrl }) {
    return request('/api/me', { method: 'PUT', auth: true, body: { nickname, pictureUrl } })
  },

  deleteMe() {
    return request('/api/me', { method: 'DELETE', auth: true })
  },

  // Playlists
  listMyPlaylists() {
    return request('/api/me/playlists', { auth: true })
  },

  createMyPlaylist({ name }) {
    return request('/api/me/playlists', { method: 'POST', auth: true, body: { name } })
  },

  patchMyPlaylist(playlistId, { name }) {
    return request(`/api/me/playlists/${encodeURIComponent(playlistId)}`, {
      method: 'PATCH',
      auth: true,
      body: { name },
    })
  },

  addMyPlaylistItem(playlistId, { title, youtubeUrl }) {
    return request(`/api/me/playlists/${encodeURIComponent(playlistId)}/items`, {
      method: 'POST',
      auth: true,
      body: { title, youtubeUrl },
    })
  },
}

// --------------------
// WebSocket helper
// --------------------
//
// Backend WS endpoint: GET /api/rooms/:roomId/ws
// Sends `room.snapshot` first, then incremental events.
//
// Note: Browser WebSocket cannot set custom headers reliably (no X-User-Sub),
// which is fine for read-only room events.
export function roomWebSocketUrl(roomId) {
  const base = getApiBaseUrl()
  const wsBase = base.replace(/^http:\/\//, 'ws://').replace(/^https:\/\//, 'wss://')
  return `${wsBase}/api/rooms/${encodeURIComponent(roomId)}/ws`
}
