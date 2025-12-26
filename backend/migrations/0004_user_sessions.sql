-- +goose Up
-- OIDC sessions (server-side)

CREATE TABLE IF NOT EXISTS user_sessions (
  id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  sub                 TEXT NOT NULL REFERENCES users(sub) ON DELETE CASCADE,
  sid                 TEXT NULL,
  refresh_token       TEXT NOT NULL,
  access_token        TEXT NOT NULL,
  id_token            TEXT NOT NULL,
  access_expires_at   TIMESTAMPTZ NOT NULL,
  refresh_expires_at  TIMESTAMPTZ NOT NULL,
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  revoked_at          TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_user_sessions_sub ON user_sessions (sub);
CREATE INDEX IF NOT EXISTS idx_user_sessions_sid ON user_sessions (sid);
CREATE INDEX IF NOT EXISTS idx_user_sessions_revoked_at ON user_sessions (revoked_at);

-- +goose Down
DROP TABLE IF EXISTS user_sessions;
