-- +goose Up
-- Remove buzz cooldown persistence (handled in memory).

ALTER TABLE room_players
  DROP COLUMN IF EXISTS buzz_cooldown_until;

-- +goose Down

ALTER TABLE room_players
  ADD COLUMN IF NOT EXISTS buzz_cooldown_until TIMESTAMPTZ NULL;
