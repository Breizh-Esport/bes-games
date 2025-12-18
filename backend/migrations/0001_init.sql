-- +goose Up
-- Initial schema for bes-games (initial game: Name That Tune)
--
-- Notes:
-- - We use OIDC subject ("sub") as the stable user identifier.
-- - Anonymous users can join rooms; they won't have a user record.
-- - Playlists belong to authenticated users only.
-- - Room owner must be an authenticated user.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- =========
-- Users / Profiles
-- =========
CREATE TABLE IF NOT EXISTS users (
  sub           TEXT PRIMARY KEY,         -- OIDC "sub"
  nickname      TEXT NOT NULL,
  picture_url   TEXT NOT NULL DEFAULT '',
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at    TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_users_deleted_at ON users (deleted_at);

-- =========
-- Playlists
-- =========
CREATE TABLE IF NOT EXISTS playlists (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_sub     TEXT NOT NULL REFERENCES users(sub) ON DELETE CASCADE,
  name          TEXT NOT NULL,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at    TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_playlists_owner_sub ON playlists (owner_sub);
CREATE INDEX IF NOT EXISTS idx_playlists_deleted_at ON playlists (deleted_at);

-- Playlist items (tracks)
CREATE TABLE IF NOT EXISTS playlist_items (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  playlist_id   UUID NOT NULL REFERENCES playlists(id) ON DELETE CASCADE,
  position      INTEGER NOT NULL, -- 0-based or 1-based is app-defined; keep consistent
  title         TEXT NOT NULL,
  youtube_url   TEXT NOT NULL,
  youtube_id    TEXT NOT NULL,
  duration_sec  INTEGER NOT NULL DEFAULT 0,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Enforce stable ordering within a playlist
CREATE UNIQUE INDEX IF NOT EXISTS uq_playlist_items_playlist_position
  ON playlist_items (playlist_id, position);

CREATE INDEX IF NOT EXISTS idx_playlist_items_playlist_id
  ON playlist_items (playlist_id);

-- =========
-- Rooms
-- =========
CREATE TABLE IF NOT EXISTS rooms (
  id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name               TEXT NOT NULL,
  owner_sub          TEXT NOT NULL REFERENCES users(sub) ON DELETE RESTRICT,
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),

  -- Loaded playlist (optional)
  loaded_playlist_id UUID NULL REFERENCES playlists(id) ON DELETE SET NULL,

  -- Playback state (optional, but kept on room for simplicity)
  playback_track_index INTEGER NOT NULL DEFAULT 0,
  playback_paused      BOOLEAN NOT NULL DEFAULT TRUE,
  playback_position_ms INTEGER NOT NULL DEFAULT 0,
  playback_updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_rooms_owner_sub ON rooms (owner_sub);
CREATE INDEX IF NOT EXISTS idx_rooms_updated_at ON rooms (updated_at DESC);

-- =========
-- Room players (presence + score)
-- =========
CREATE TABLE IF NOT EXISTS room_players (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(), -- "playerId" (per join/session)
  room_id       UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,

  -- Authenticated user (nullable for anonymous)
  user_sub      TEXT NULL REFERENCES users(sub) ON DELETE SET NULL,

  nickname      TEXT NOT NULL,
  picture_url   TEXT NOT NULL DEFAULT '',

  score         INTEGER NOT NULL DEFAULT 0,
  connected     BOOLEAN NOT NULL DEFAULT TRUE,

  joined_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  left_at       TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_room_players_room_id ON room_players (room_id);
CREATE INDEX IF NOT EXISTS idx_room_players_user_sub ON room_players (user_sub);
CREATE INDEX IF NOT EXISTS idx_room_players_connected ON room_players (room_id, connected);

-- =========
-- updated_at maintenance
-- =========
-- NOTE:
-- We intentionally do NOT create updated_at triggers here because this migration is
-- executed by goose and PL/pgSQL function bodies are prone to statement-splitting issues.
-- The application should explicitly set updated_at as needed (or we can introduce triggers
-- later via a safer migration approach).

-- +goose Down
-- No triggers/functions to drop (see Up section)

DROP TABLE IF EXISTS room_players;
DROP TABLE IF EXISTS rooms;
DROP TABLE IF EXISTS playlist_items;
DROP TABLE IF EXISTS playlists;
DROP TABLE IF EXISTS users;

DROP EXTENSION IF EXISTS pgcrypto;
