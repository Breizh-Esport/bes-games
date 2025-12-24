-- +goose Up
-- Add playlist thumbnails and room visibility/password fields.

ALTER TABLE playlist_items
  ADD COLUMN IF NOT EXISTS thumbnail_url TEXT NOT NULL DEFAULT '';

ALTER TABLE rooms
  ADD COLUMN IF NOT EXISTS visibility TEXT NOT NULL DEFAULT 'public',
  ADD COLUMN IF NOT EXISTS password_hash TEXT NOT NULL DEFAULT '';

-- +goose Down

ALTER TABLE rooms
  DROP COLUMN IF EXISTS password_hash,
  DROP COLUMN IF EXISTS visibility;

ALTER TABLE playlist_items
  DROP COLUMN IF EXISTS thumbnail_url;
