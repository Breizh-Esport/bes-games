package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

func (r *Repo) CreateSession(ctx context.Context, sess UserSession) (UserSession, error) {
	if sess.Sub == "" {
		return UserSession{}, ErrUnauthorized
	}

	const q = `
INSERT INTO user_sessions (
  sub, sid, refresh_token, access_token, id_token, access_expires_at, refresh_expires_at
)
VALUES ($1, NULLIF($2,''), $3, $4, $5, $6, $7)
RETURNING id::text, sub, COALESCE(sid,''), refresh_token, access_token, id_token,
          access_expires_at, refresh_expires_at, created_at, updated_at, revoked_at;
`
	var out UserSession
	if err := r.db.QueryRow(
		ctx,
		q,
		sess.Sub,
		sess.SID,
		sess.RefreshToken,
		sess.AccessToken,
		sess.IDToken,
		sess.AccessExpiresAt,
		sess.RefreshExpiresAt,
	).Scan(
		&out.ID,
		&out.Sub,
		&out.SID,
		&out.RefreshToken,
		&out.AccessToken,
		&out.IDToken,
		&out.AccessExpiresAt,
		&out.RefreshExpiresAt,
		&out.CreatedAt,
		&out.UpdatedAt,
		&out.RevokedAt,
	); err != nil {
		return UserSession{}, fmt.Errorf("create session: %w", err)
	}
	return out, nil
}

func (r *Repo) GetSession(ctx context.Context, id string) (UserSession, error) {
	if id == "" {
		return UserSession{}, ErrUnauthorized
	}

	const q = `
SELECT id::text, sub, COALESCE(sid,''), refresh_token, access_token, id_token,
       access_expires_at, refresh_expires_at, created_at, updated_at, revoked_at
FROM user_sessions
WHERE id::uuid = $1 AND revoked_at IS NULL;
`
	var out UserSession
	err := r.db.QueryRow(ctx, q, id).Scan(
		&out.ID,
		&out.Sub,
		&out.SID,
		&out.RefreshToken,
		&out.AccessToken,
		&out.IDToken,
		&out.AccessExpiresAt,
		&out.RefreshExpiresAt,
		&out.CreatedAt,
		&out.UpdatedAt,
		&out.RevokedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return UserSession{}, ErrUnauthorized
	}
	if err != nil {
		return UserSession{}, fmt.Errorf("get session: %w", err)
	}
	return out, nil
}

func (r *Repo) UpdateSessionTokens(ctx context.Context, id string, accessToken, idToken string, accessExpiresAt time.Time, refreshToken *string, refreshExpiresAt *time.Time) error {
	if id == "" {
		return ErrUnauthorized
	}

	const q = `
UPDATE user_sessions
SET access_token = $2,
    id_token = $3,
    access_expires_at = $4,
    refresh_token = COALESCE($5, refresh_token),
    refresh_expires_at = COALESCE($6, refresh_expires_at),
    updated_at = now()
WHERE id::uuid = $1 AND revoked_at IS NULL;
`
	ct, err := r.db.Exec(ctx, q, id, accessToken, idToken, accessExpiresAt, refreshToken, refreshExpiresAt)
	if err != nil {
		return fmt.Errorf("update session tokens: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrUnauthorized
	}
	return nil
}

func (r *Repo) RevokeSession(ctx context.Context, id string) error {
	if id == "" {
		return ErrUnauthorized
	}

	const q = `UPDATE user_sessions SET revoked_at = now(), updated_at = now() WHERE id::uuid = $1 AND revoked_at IS NULL;`
	if _, err := r.db.Exec(ctx, q, id); err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}

func (r *Repo) RevokeSessionsBySub(ctx context.Context, sub string) error {
	if sub == "" {
		return ErrUnauthorized
	}

	const q = `UPDATE user_sessions SET revoked_at = now(), updated_at = now() WHERE sub = $1 AND revoked_at IS NULL;`
	if _, err := r.db.Exec(ctx, q, sub); err != nil {
		return fmt.Errorf("revoke sessions by sub: %w", err)
	}
	return nil
}

func (r *Repo) RevokeSessionsBySID(ctx context.Context, sid string) error {
	if sid == "" {
		return ErrUnauthorized
	}

	const q = `UPDATE user_sessions SET revoked_at = now(), updated_at = now() WHERE sid = $1 AND revoked_at IS NULL;`
	if _, err := r.db.Exec(ctx, q, sid); err != nil {
		return fmt.Errorf("revoke sessions by sid: %w", err)
	}
	return nil
}
