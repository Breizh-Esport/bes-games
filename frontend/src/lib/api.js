// Minimal API client for the bes-games backend.
//
// Auth model (temporary):
// - The backend expects an authenticated user subject in header `X-User-Sub`
// - Real OIDC login/registration will be wired later; for now we store a "sub" in localStorage
//
// Anonymous users can still call endpoints that do not require auth.
//
// You can change the base URL with:
//   - VITE_API_BASE_URL (e.g. 'http://localhost:8080')
// Default is 'http://localhost:8080'

const DEFAULT_BASE_URL = "http://localhost:8080";
const LS_USER_SUB_KEY = "besgames.userSub";
const LS_GUEST_SUB_KEY = "besgames.guestSub";

export function getApiBaseUrl() {
  return (import.meta?.env?.VITE_API_BASE_URL || DEFAULT_BASE_URL).replace(
    /\/+$/,
    "",
  );
}

export function getUserSub() {
  try {
    return (localStorage.getItem(LS_USER_SUB_KEY) || "").trim();
  } catch {
    return "";
  }
}

function getOrCreateGuestSub() {
  try {
    let v = (localStorage.getItem(LS_GUEST_SUB_KEY) || "").trim();
    if (!v) {
      const suffix =
        typeof crypto !== "undefined" && crypto.randomUUID
          ? crypto.randomUUID()
          : Math.random().toString(36).slice(2);
      v = `guest_${suffix}`;
      localStorage.setItem(LS_GUEST_SUB_KEY, v);
    }
    return v;
  } catch {
    return "";
  }
}

export function setUserSub(sub) {
  const v = (sub || "").trim();
  try {
    if (!v) localStorage.removeItem(LS_USER_SUB_KEY);
    else localStorage.setItem(LS_USER_SUB_KEY, v);
  } catch {
    // ignore
  }
  return v;
}

export function clearUserSub() {
  return setUserSub("");
}

export class ApiError extends Error {
  constructor(
    message,
    { status = 0, payload = null, url = "", method = "" } = {},
  ) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.payload = payload;
    this.url = url;
    this.method = method;
  }
}

function buildHeaders({ auth = false, extraHeaders = {} } = {}) {
  const headers = {
    Accept: "application/json",
    ...extraHeaders,
  };

  if (auth) {
    const sub = getUserSub();
    if (sub) headers["X-User-Sub"] = sub;
  }

  return headers;
}

async function parseJsonSafely(res) {
  const ct = (res.headers.get("content-type") || "").toLowerCase();
  if (!ct.includes("application/json")) return null;
  try {
    return await res.json();
  } catch {
    return null;
  }
}

async function request(
  path,
  { method = "GET", auth = false, body, headers = {}, signal } = {},
) {
  const base = getApiBaseUrl();
  const url = `${base}${path.startsWith("/") ? "" : "/"}${path}`;

  const init = {
    method,
    headers: buildHeaders({ auth, extraHeaders: headers }),
    signal,
  };

  if (body !== undefined) {
    init.headers["Content-Type"] = "application/json";
    init.body = JSON.stringify(body);
  }

  const res = await fetch(url, init);
  const payload = await parseJsonSafely(res);

  if (!res.ok) {
    const msg =
      (payload && (payload.error || payload.message)) ||
      `${method} ${path} failed with status ${res.status}`;
    throw new ApiError(msg, { status: res.status, payload, url, method });
  }

  return payload;
}

function gamePrefix(gameId) {
  const id = (gameId || "").trim();
  if (!id) throw new Error("gameId is required");
  return `/api/games/${encodeURIComponent(id)}`;
}

// --------------------
// Public API methods
// --------------------

export const api = {
  // Games
  listGames() {
    return request("/api/games");
  },

  // Rooms (per-game)
  listRooms(gameId) {
    return request(`${gamePrefix(gameId)}/rooms`);
  },

  createRoom(gameId, { name, playlistId, visibility, password }) {
    return request(`${gamePrefix(gameId)}/rooms`, {
      method: "POST",
      auth: true,
      body: { name, playlistId, visibility, password },
    });
  },

  getRoom(gameId, roomId) {
    return request(`${gamePrefix(gameId)}/rooms/${encodeURIComponent(roomId)}`);
  },

  joinRoom(gameId, roomId, { nickname, pictureUrl, password } = {}) {
    const userSub = getUserSub();
    const headers = {};
    if (!userSub) {
      const guestSub = getOrCreateGuestSub();
      if (guestSub) headers["X-User-Sub"] = guestSub;
    }
    return request(
      `${gamePrefix(gameId)}/rooms/${encodeURIComponent(roomId)}/join`,
      {
        method: "POST",
        auth: true, // use auth sub when set; otherwise attach a guest sub
        headers,
        body: { nickname, pictureUrl, password },
      },
    );
  },

  leaveRoom(gameId, roomId, { playerId }) {
    return request(
      `${gamePrefix(gameId)}/rooms/${encodeURIComponent(roomId)}/leave`,
      {
        method: "POST",
        auth: false,
        body: { playerId },
      },
    );
  },

  // Owner controls
  kick(gameId, roomId, { playerId }) {
    return request(
      `${gamePrefix(gameId)}/rooms/${encodeURIComponent(roomId)}/kick`,
      {
        method: "POST",
        auth: true,
        body: { playerId },
      },
    );
  },

  setScore(gameId, roomId, { playerId, score }) {
    return request(
      `${gamePrefix(gameId)}/rooms/${encodeURIComponent(roomId)}/score/set`,
      {
        method: "POST",
        auth: true,
        body: { playerId, score },
      },
    );
  },

  addScore(gameId, roomId, { playerId, delta }) {
    return request(
      `${gamePrefix(gameId)}/rooms/${encodeURIComponent(roomId)}/score/add`,
      {
        method: "POST",
        auth: true,
        body: { playerId, delta },
      },
    );
  },

  loadPlaylist(gameId, roomId, { playlistId }) {
    return request(
      `${gamePrefix(gameId)}/rooms/${encodeURIComponent(roomId)}/playlist/load`,
      {
        method: "POST",
        auth: true,
        body: { playlistId },
      },
    );
  },

  setPlayback(gameId, roomId, { trackIndex, paused, positionMs } = {}) {
    return request(
      `${gamePrefix(gameId)}/rooms/${encodeURIComponent(roomId)}/playback/set`,
      {
        method: "POST",
        auth: true,
        body: { trackIndex, paused, positionMs },
      },
    );
  },

  pause(gameId, roomId, { paused }) {
    return request(
      `${gamePrefix(gameId)}/rooms/${encodeURIComponent(roomId)}/playback/pause`,
      {
        method: "POST",
        auth: true,
        body: { paused },
      },
    );
  },

  seek(gameId, roomId, { positionMs }) {
    return request(
      `${gamePrefix(gameId)}/rooms/${encodeURIComponent(roomId)}/playback/seek`,
      {
        method: "POST",
        auth: true,
        body: { positionMs },
      },
    );
  },

  // Player actions
  buzz(gameId, roomId, { playerId }) {
    return request(
      `${gamePrefix(gameId)}/rooms/${encodeURIComponent(roomId)}/buzz`,
      {
        method: "POST",
        auth: false,
        body: { playerId },
      },
    );
  },

  resolveBuzz(gameId, roomId, { playerId, correct }) {
    return request(
      `${gamePrefix(gameId)}/rooms/${encodeURIComponent(roomId)}/buzz/resolve`,
      {
        method: "POST",
        auth: true,
        body: { playerId, correct },
      },
    );
  },

  // Profile / account
  getMe() {
    return request("/api/me", { auth: true });
  },

  putMe({ nickname, pictureUrl }) {
    return request("/api/me", {
      method: "PUT",
      auth: true,
      body: { nickname, pictureUrl },
    });
  },

  deleteMe() {
    return request("/api/me", { method: "DELETE", auth: true });
  },

  // Playlists (per-game, owned by authenticated user)
  listPlaylists(gameId) {
    return request(`${gamePrefix(gameId)}/playlists`, { auth: true });
  },

  createPlaylist(gameId, { name }) {
    return request(`${gamePrefix(gameId)}/playlists`, {
      method: "POST",
      auth: true,
      body: { name },
    });
  },

  patchPlaylist(gameId, playlistId, { name }) {
    return request(
      `${gamePrefix(gameId)}/playlists/${encodeURIComponent(playlistId)}`,
      {
        method: "PATCH",
        auth: true,
        body: { name },
      },
    );
  },

  addPlaylistItem(gameId, playlistId, { youtubeUrl }) {
    return request(
      `${gamePrefix(gameId)}/playlists/${encodeURIComponent(playlistId)}/items`,
      {
        method: "POST",
        auth: true,
        body: { youtubeUrl },
      },
    );
  },

  patchPlaylistItem(gameId, playlistId, itemId, { title }) {
    return request(
      `${gamePrefix(gameId)}/playlists/${encodeURIComponent(playlistId)}/items/${encodeURIComponent(itemId)}`,
      {
        method: "PATCH",
        auth: true,
        body: { title },
      },
    );
  },

  deletePlaylistItem(gameId, playlistId, itemId) {
    return request(
      `${gamePrefix(gameId)}/playlists/${encodeURIComponent(playlistId)}/items/${encodeURIComponent(itemId)}`,
      {
        method: "DELETE",
        auth: true,
      },
    );
  },
};

// --------------------
// WebSocket helper
// --------------------
//
// Backend WS endpoint: GET /api/games/:gameId/rooms/:roomId/ws
// Sends `room.snapshot` first, then incremental events.
//
// Note: Browser WebSocket cannot set custom headers reliably (no X-User-Sub),
// which is fine for read-only room events.
export function roomWebSocketUrl(gameId, roomId) {
  const base = getApiBaseUrl();
  const wsBase = base
    .replace(/^http:\/\//, "ws://")
    .replace(/^https:\/\//, "wss://");
  const prefix = gamePrefix(gameId);
  return `${wsBase}${prefix}/rooms/${encodeURIComponent(roomId)}/ws`;
}
