# bes-games

Multi-game platform (initial game: "Name That Tune" / blindtest).

This repository contains:
- Go backend (`backend/`) providing a REST + WebSocket API
- Vue 3 frontend (`frontend/`) providing a multi-page UI (Games, Profile, per-game pages)

Authentication is currently a placeholder:
- The frontend stores an arbitrary `sub` in `localStorage`
- Authenticated backend requests send it via `X-User-Sub`

## Project layout

- `backend/cmd/api/` - Go server entrypoint
- `backend/internal/core/` - core domain + Postgres repo (profiles, shared domain errors)
- `backend/internal/games/` - game registry + game-specific packages
- `backend/internal/games/namethattune/` - Name That Tune domain + Postgres repo (rooms, playback, playlists)
- `backend/internal/httpapi/` - REST + WebSocket handlers (Chi router)
- `frontend/src/views/` - platform + per-game pages (games live under `frontend/src/views/games/`)

## Run instructions

### Backend (Go)

Requires Postgres (`DATABASE_URL` or `BES_DATABASE_URL`). By default, Goose migrations run on startup
(disable with `BES_MIGRATIONS_DISABLE=1`).

Example:

```sh
DATABASE_URL="postgres://postgres:postgres@localhost:5432/bes_games?sslmode=disable" go run ./backend/cmd/api
```

Health check:

```sh
curl http://localhost:8080/healthz
```

### Frontend (Vue)

```sh
cd frontend
npm install
npm run dev
```

Override backend base URL:
- `VITE_API_BASE_URL` (defaults to `http://localhost:8080`)

### Docker

```sh
docker compose up --build
```

## Backend API (high-level)

- `GET /healthz`
- `GET /api/games` - list available games (currently only `name-that-tune`)
- Rooms (per-game): `GET /api/games/{gameId}/rooms`, `POST /api/games/{gameId}/rooms`, `GET /api/games/{gameId}/rooms/{roomId}`, join/leave, WS snapshots
- Profile: `GET/PUT/DELETE /api/me`
- Playlists (per-game): `GET/POST/PATCH /api/games/{gameId}/playlists`, `POST /api/games/{gameId}/playlists/{playlistId}/items`

## Next steps

- Add `gameId` to the room schema so multiple games can coexist cleanly.
- Replace placeholder auth with real OIDC login/registration and token validation.
- Implement actual YouTube playback for Name That Tune (player embed + drift correction).

