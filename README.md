# bes-games

Multi-game platform (initial game: "Name That Tune" / blindtest).

This repository contains:
- Go backend (`backend/`) providing a REST + WebSocket API
- Vue 3 frontend (`frontend/`) providing a multi-page UI (Games, Profile, per-game pages)

Authentication uses OIDC with a server-managed session cookie.

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

### OIDC configuration

Set these env vars to enable authentication:

- `BES_OIDC_ISSUER_URL` (required)
- `BES_OIDC_CLIENT_ID` (required)
- `BES_OIDC_CLIENT_SECRET` (optional, depending on your provider)
- `BES_PUBLIC_URL` (required unless you set `BES_OIDC_REDIRECT_URL`)
- `BES_AUTH_COOKIE_SECRET` (required, random string)

Optional:
- `BES_OIDC_REDIRECT_URL` (override the default redirect)
- `BES_UI_BASE_URL` (allowed returnTo base, e.g. `http://localhost:5173`)
- `BES_OIDC_SCOPES` (comma-separated, default `openid,email,profile`)
- `BES_OIDC_PROMPT` (provider-specific prompt value)
- `BES_OIDC_OFFLINE_ACCESS` (`true` to request refresh tokens)
- `BES_AUTH_COOKIE_NAME` (default `besgames_session`)
- `BES_AUTH_COOKIE_DOMAIN`
- `BES_AUTH_COOKIE_SECURE` (defaults based on `BES_PUBLIC_URL`)
- `BES_AUTH_COOKIE_SAMESITE` (`lax`, `strict`, `none`)
- `BES_AUTH_REFRESH_TTL` (default `720h`)
- `BES_AUTH_ACCESS_TTL` (fallback default `5m`)

Redirect URI for your OIDC client:
- `${BES_PUBLIC_URL}/auth/callback` (or `BES_OIDC_REDIRECT_URL` if set)

Backchannel logout URI:
- `${BES_PUBLIC_URL}/auth/backchannel-logout`

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
- Implement actual YouTube playback for Name That Tune (player embed + drift correction).
