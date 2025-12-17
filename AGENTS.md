# Repository Guidelines

## Project Context
- **Project goal:** Build a “Name That Tune” (blindtest) web app with rooms and real-time interactions.
- **Functional perimeter:**
  - Go backend exposes a REST + WebSocket API for rooms, players/roster, profiles, and playlists.
  - Room features include owner controls (kick, score changes, playlist load, playback state updates) and player actions (buzz).
  - Vue frontend provides a multi-page UI (`Home`, `Profile`, `Room`) consuming that API.
- **Major constraints:**
  - Backend requires Postgres (`DATABASE_URL` or `BES_DATABASE_URL`) and runs Goose migrations on startup (disable with `BES_MIGRATIONS_DISABLE=1`).
  - Authentication is currently a placeholder: a `sub` stored in `localStorage` and sent as `X-User-Sub`.
  - YouTube embedded playback is not implemented yet; playback controls currently synchronize state only.
- **Current project state:** Core flows are implemented (create/list/join/leave rooms; profile + playlist CRUD; room snapshots/events via WebSocket). OIDC login/registration is planned but not wired.
- **Technical choices already made:** Go `1.25.x` + Chi router + `pgx` (Postgres) + Goose migrations + `coder/websocket`; Vue 3 + Vue Router + Vite; `docker-compose.yml` provides Postgres and runs the backend container.

## Project Structure & Module Organization
- `backend/`: Go API server.
  - `backend/cmd/api/`: server entrypoint.
  - `backend/internal/httpapi/`: REST + WebSocket handlers.
  - `backend/internal/game/`: domain models + Postgres-backed repo.
  - `backend/migrations/`: Goose SQL migrations.
- `frontend/`: Vue 3 + Vite SPA.
  - `frontend/src/views/`: route pages (e.g. `HomePage.vue`, `RoomPage.vue`).
  - `frontend/src/lib/`: client helpers (e.g. `api.js`).
  - `frontend/src/stores/`: UI state (temporary auth uses `localStorage`).

Do not commit generated artifacts (see `.gitignore`, notably `frontend/dist/` and `frontend/node_modules/`).

## Build, Test, and Development Commands
Backend:
- `docker compose up -d db`: start local Postgres.
- `DATABASE_URL="postgres://postgres:postgres@localhost:5432/bes_blind?sslmode=disable" go run ./backend/cmd/api`: run API (default `:8080`).
- `curl http://localhost:8080/healthz`: verify server health.

Frontend:
- `cd frontend; npm install; npm run dev`: run Vite dev server (default `http://localhost:5173`).
- `npm run build` / `npm run preview`: production build + local preview.

Docker:
- `docker compose up --build`: run backend + db.

## Coding Style & Naming Conventions
- Go: format with `gofmt` (tabs), keep packages lowercase.
- Vue/JS: follow existing patterns (`<script setup>`, single quotes, no semicolons); use `PascalCase.vue` for components/views.

## Testing Guidelines
- Backend tests live alongside code (e.g. `backend/internal/httpapi/server_test.go`) and include Postgres-backed integration tests.
- Run with a DB configured: `TEST_DATABASE_URL=... go test ./...` (tests skip DB cases when no URL is set).

## Commit & Pull Request Guidelines
- If commit history is available, match its conventions; otherwise use a conventional prefix like `feat:`, `fix:`, `refactor:`, `test:`, `chore:`.
- PRs should include: a brief summary, motivation/linked issue, and any API or env-var changes (update `README.md` when behavior changes).

## Configuration & Security Notes
- Backend auth is currently a placeholder header (`X-User-Sub`); do not treat it as secure.
- Useful env vars: `BES_ADDR`, `BES_CORS_ALLOWED_ORIGINS`, `DATABASE_URL`/`BES_DATABASE_URL`, `BES_MIGRATIONS_DISABLE`, `BES_MIGRATIONS_DIR`, and frontend `VITE_API_BASE_URL`.
