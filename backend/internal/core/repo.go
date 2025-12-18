package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repo provides the Postgres-backed persistence for platform-level data.
//
// At the moment this is limited to user profiles / accounts. Game-specific state
// (rooms, playlists, gameplay state) lives under backend/internal/games/*.
type Repo struct {
	db *pgxpool.Pool
}

func NewRepo(db *pgxpool.Pool) *Repo {
	return &Repo{db: db}
}

// UpsertProfile creates or updates a user profile.
// If the user doesn't exist, it is created.
// If it exists and was soft-deleted (deleted_at != NULL), it is "revived".
func (r *Repo) UpsertProfile(ctx context.Context, sub, nickname, pictureURL string) (UserProfile, error) {
	if sub == "" {
		return UserProfile{}, ErrUnauthorized
	}
	if nickname == "" {
		nickname = "Player"
	}

	const q = `
INSERT INTO users (sub, nickname, picture_url, deleted_at)
VALUES ($1, $2, $3, NULL)
ON CONFLICT (sub) DO UPDATE
SET nickname = EXCLUDED.nickname,
    picture_url = EXCLUDED.picture_url,
    deleted_at = NULL
RETURNING sub, nickname, picture_url, updated_at;
`
	var out UserProfile
	if err := r.db.QueryRow(ctx, q, sub, nickname, pictureURL).Scan(&out.Sub, &out.Nickname, &out.PictureURL, &out.UpdatedAt); err != nil {
		return UserProfile{}, fmt.Errorf("upsert profile: %w", err)
	}
	return out, nil
}

// GetProfile returns the user profile. If it doesn't exist, returns a default profile (not created).
func (r *Repo) GetProfile(ctx context.Context, sub string) (UserProfile, error) {
	if sub == "" {
		return UserProfile{}, ErrUnauthorized
	}

	const q = `
SELECT sub, nickname, picture_url, updated_at
FROM users
WHERE sub = $1 AND deleted_at IS NULL;
`
	var out UserProfile
	err := r.db.QueryRow(ctx, q, sub).Scan(&out.Sub, &out.Nickname, &out.PictureURL, &out.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return UserProfile{
			Sub:       sub,
			Nickname:  "Player",
			UpdatedAt: time.Now().UTC(),
		}, nil
	}
	if err != nil {
		return UserProfile{}, fmt.Errorf("get profile: %w", err)
	}
	return out, nil
}

// DeleteAccount soft-deletes the user row.
//
// Game-specific user data cleanup is handled by each game repo (because soft deletes
// do not trigger FK ON DELETE actions).
func (r *Repo) DeleteAccount(ctx context.Context, sub string) error {
	if sub == "" {
		return ErrUnauthorized
	}

	const q = `UPDATE users SET deleted_at = now() WHERE sub = $1 AND deleted_at IS NULL;`
	if _, err := r.db.Exec(ctx, q, sub); err != nil {
		return fmt.Errorf("delete account: %w", err)
	}
	return nil
}
