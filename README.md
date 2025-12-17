# bes-blind

"Name That Tune" (blindtest) web app.

This repository contains:
- Go backend (`backend/`) providing a REST + WebSocket API
- Vue.js frontend (`frontend/`) providing a multi-page UI (Home, Profile, Room)

> Note on authentication: the long-term plan is **OIDC login/registration**.
> Right now the frontend uses a **temporary placeholder** that stores an arbitrary `sub` in `localStorage` and sends it to the backend via `X-User-Sub`.
> Anonymous users can still join rooms.

---

## Project layout

- `backend/cmd/api/` — Go server entrypoint
- `backend/internal/game/` — in-memory domain/state (rooms, players, playlists, events)
- `backend/internal/httpapi/` — REST + WebSocket handlers (Chi router)
- `frontend/` — Vue app (Vue Router pages)

---

## Features implemented (current)

### Home page
- "Login" placeholder (set a `sub` string locally)
- Create a room (**requires auth**)
- List rooms (shows online player count best-effort)
- Join a room (**anonymous allowed**)

### Profile page (auth required)
- Modify nickname and profile picture (stored server-side, in-memory)
- Create/list/rename playlists
- Add tracks to playlists by providing:
  - track title
  - YouTube URL (parsed to extract YouTube ID)
- Delete account (removes profile; best-effort scrubbing of presence)

### Room page
- Live room roster via WebSocket (`room.snapshot` + events)
- Owner view (derived from `sub === ownerSub`):
  - see players (name, picture, score, online/offline)
  - kick players
  - increase/decrease score, set score
  - load one of your playlists into the room
  - control playback state (track index, pause/play, seek, next/prev/restart)
- Player view:
  - see players (name, picture, score)
  - local volume slider (UI only)
  - buzzer (sends event to the room)

> Audio playback: YouTube embedded playback is **not implemented yet**.
> Playback controls currently synchronize **state** to all clients.

---

## Run instructions

### Backend (Go)
From repo root:

```sh
go run ./backend/cmd/api
```

The server listens on:
- `BES_ADDR` (default `:8080`)

CORS:
- Default allowed origin: `http://localhost:5173`
- Override with `BES_CORS_ALLOWED_ORIGINS` (comma-separated)

Example:

```sh
BES_ADDR=":8080" BES_CORS_ALLOWED_ORIGINS="http://localhost:5173" go run ./backend/cmd/api
```

Health check:

```sh
curl http://localhost:8080/healthz
```

### Frontend (Vue)
From repo root:

```sh
cd frontend
npm install
npm run dev
```

The frontend can be pointed at a different backend URL using:

- `VITE_API_BASE_URL` (defaults to `http://localhost:8080`)

Example (PowerShell):

```powershell
$env:VITE_API_BASE_URL="http://localhost:8080"
npm run dev
```

Example (sh):

```sh
VITE_API_BASE_URL="http://localhost:8080" npm run dev
```

Build:

```sh
cd frontend
npm run build
npm run preview
```

---

## Backend API

### Authentication (temporary)
For endpoints that require authentication, the backend expects:

- `X-User-Sub: <oidc subject>`

Anonymous users can call endpoints that do not require auth.

### REST endpoints

#### Health
- `GET /healthz`

#### Rooms
- `GET  /api/rooms`
  - List available rooms, with online player counts (best-effort).
- `POST /api/rooms` (**auth required**)
  - Create room
  - Body: `{ "name": "My Room" }`
- `GET  /api/rooms/{roomId}`
  - Fetch a `RoomSnapshot`
- `POST /api/rooms/{roomId}/join` (anonymous allowed)
  - Body (optional): `{ "nickname": "...", "pictureUrl": "https://..." }`
  - Returns: `{ playerId, snapshot }`
- `POST /api/rooms/{roomId}/leave`
  - Body: `{ "playerId": "..." }`

#### Room owner controls (must be room owner; auth required)
- `POST /api/rooms/{roomId}/kick`
  - Body: `{ "playerId": "..." }`
- `POST /api/rooms/{roomId}/score/set`
  - Body: `{ "playerId": "...", "score": 10 }`
- `POST /api/rooms/{roomId}/score/add`
  - Body: `{ "playerId": "...", "delta": 1 }`
- `POST /api/rooms/{roomId}/playlist/load`
  - Body: `{ "playlistId": "..." }`
- `POST /api/rooms/{roomId}/playback/set`
  - Body: `{ "trackIndex": 0, "paused": true, "positionMs": 0 }` (fields optional as appropriate)
- `POST /api/rooms/{roomId}/playback/pause`
  - Body: `{ "paused": true }`
- `POST /api/rooms/{roomId}/playback/seek`
  - Body: `{ "positionMs": 30000 }`

#### Player actions
- `POST /api/rooms/{roomId}/buzz`
  - Body: `{ "playerId": "..." }`

#### Profile / account (auth required)
- `GET    /api/me`
- `PUT    /api/me`
  - Body: `{ "nickname": "...", "pictureUrl": "..." }`
- `DELETE /api/me`

#### Playlists (auth required)
- `GET  /api/me/playlists`
- `POST /api/me/playlists`
  - Body: `{ "name": "My Playlist" }`
- `PATCH /api/me/playlists/{playlistId}`
  - Body: `{ "name": "New Name" }`
- `POST /api/me/playlists/{playlistId}/items`
  - Body: `{ "title": "Song", "youtubeUrl": "https://www.youtube.com/watch?v=..." }`

### WebSocket endpoint

- `GET /api/rooms/{roomId}/ws`

The server sends:
- an initial event: `type = "room.snapshot"` with full room state
- subsequent events when changes happen (including new snapshots after most mutations)

Clients should treat snapshots as authoritative.

---

## Notes / Next steps

- Replace the placeholder auth flow with a real OIDC client in the frontend and proper token validation in the backend.
- Add persistence (DB) for profiles/playlists/rooms if you want state across restarts.
- Implement synchronized YouTube playback (embed player, leader controls, drift correction).
- Add game rules: lock buzzer, timers, answer validation, etc.