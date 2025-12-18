package namethattune

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/valentin/bes-games/backend/internal/core"
)

// Repo provides a Postgres-backed repository for Name That Tune state:
// playlists, rooms, players, and playback.
//
// This repository is intentionally "thin":
// - It persists and queries state in Postgres.
// - Realtime fanout (websocket broadcasts) is handled outside (HTTP layer / hubs).
//
// Schema expectations (see migrations/0001_init.sql):
// - users(sub PK, nickname, picture_url, deleted_at, ...)
// - playlists(id UUID PK, owner_sub FK users, name, deleted_at, ...)
// - playlist_items(id UUID PK, playlist_id FK playlists, position, title, youtube_url, youtube_id, ...)
// - rooms(id UUID PK, name, owner_sub FK users, loaded_playlist_id, playback_* ...)
// - room_players(id UUID PK, room_id FK rooms, user_sub nullable FK users, nickname, picture_url, score, connected, left_at ...)
//
// Notes:
// - We use UUIDs in DB but keep IDs as strings in API/domain.
type Repo struct {
	db *pgxpool.Pool
}

func NewRepo(db *pgxpool.Pool) *Repo {
	return &Repo{db: db}
}

func (r *Repo) ensureUserExists(ctx context.Context, sub string) error {
	if sub == "" {
		return core.ErrUnauthorized
	}

	const q = `
INSERT INTO users (sub, nickname, picture_url, deleted_at)
VALUES ($1, 'Player', '', NULL)
ON CONFLICT (sub) DO UPDATE
SET deleted_at = NULL
WHERE users.deleted_at IS NOT NULL;
`
	if _, err := r.db.Exec(ctx, q, sub); err != nil {
		return fmt.Errorf("ensure user exists: %w", err)
	}
	return nil
}

// CleanupUserData removes/neutralizes Name That Tune state owned by the given user.
//
// This is intentionally separate from core.Repo.DeleteAccount because soft deletes do not
// trigger FK ON DELETE actions, and each game can have its own cleanup logic.
func (r *Repo) CleanupUserData(ctx context.Context, sub string) error {
	if sub == "" {
		return core.ErrUnauthorized
	}

	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("cleanup user begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Soft-delete playlists owned by user.
	{
		const q = `UPDATE playlists SET deleted_at = now() WHERE owner_sub = $1 AND deleted_at IS NULL;`
		if _, err := tx.Exec(ctx, q, sub); err != nil {
			return fmt.Errorf("cleanup user soft-delete playlists: %w", err)
		}
	}

	// Scrub room_players with this sub: mark disconnected + anonymize.
	{
		const q = `
UPDATE room_players
SET connected = FALSE,
    left_at = COALESCE(left_at, now()),
    nickname = 'Deleted User',
    picture_url = '',
    user_sub = NULL
WHERE user_sub = $1;
`
		if _, err := tx.Exec(ctx, q, sub); err != nil {
			return fmt.Errorf("cleanup user scrub room_players: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("cleanup user commit: %w", err)
	}
	return nil
}

// ============================
// Playlists
// ============================

// DBPlaylist is a playlist with items, as returned by repo methods.
// It reuses the domain Playlist struct.
type DBPlaylist = Playlist

func (r *Repo) CreatePlaylist(ctx context.Context, ownerSub, name string) (Playlist, error) {
	if ownerSub == "" {
		return Playlist{}, core.ErrUnauthorized
	}
	if name == "" {
		return Playlist{}, core.ErrInvalidInput
	}

	// Ensure user exists (FK). We choose to auto-create a default profile row if missing.
	// This makes "first action is create playlist" work even if /api/me wasn't called.
	if err := r.ensureUserExists(ctx, ownerSub); err != nil {
		return Playlist{}, err
	}

	const q = `
INSERT INTO playlists (owner_sub, name, deleted_at)
VALUES ($1, $2, NULL)
RETURNING id::text, owner_sub, name, created_at, updated_at;
`
	var pl Playlist
	if err := r.db.QueryRow(ctx, q, ownerSub, name).Scan(&pl.ID, &pl.OwnerSub, &pl.Name, &pl.CreatedAt, &pl.UpdatedAt); err != nil {
		return Playlist{}, fmt.Errorf("create playlist: %w", err)
	}
	pl.Items = []PlaylistItem{}
	return pl, nil
}

func (r *Repo) ListPlaylists(ctx context.Context, ownerSub string) ([]Playlist, error) {
	if ownerSub == "" {
		return nil, core.ErrUnauthorized
	}

	const q = `
SELECT p.id::text, p.owner_sub, p.name, p.created_at, p.updated_at
FROM playlists p
WHERE p.owner_sub = $1 AND p.deleted_at IS NULL
ORDER BY p.updated_at DESC;
`
	rows, err := r.db.Query(ctx, q, ownerSub)
	if err != nil {
		return nil, fmt.Errorf("list playlists: %w", err)
	}
	defer rows.Close()

	out := make([]Playlist, 0, 8)
	for rows.Next() {
		var pl Playlist
		if err := rows.Scan(&pl.ID, &pl.OwnerSub, &pl.Name, &pl.CreatedAt, &pl.UpdatedAt); err != nil {
			return nil, fmt.Errorf("list playlists scan: %w", err)
		}
		// Load items lazily? For now return empty; callers can fetch with GetPlaylist.
		pl.Items = []PlaylistItem{}
		out = append(out, pl)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list playlists rows: %w", err)
	}
	return out, nil
}

func (r *Repo) GetPlaylist(ctx context.Context, ownerSub, playlistID string) (Playlist, error) {
	if ownerSub == "" {
		return Playlist{}, core.ErrUnauthorized
	}
	if playlistID == "" {
		return Playlist{}, core.ErrInvalidInput
	}

	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return Playlist{}, fmt.Errorf("get playlist begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var pl Playlist
	{
		const q = `
SELECT id::text, owner_sub, name, created_at, updated_at
FROM playlists
WHERE id::uuid = $1 AND owner_sub = $2 AND deleted_at IS NULL;
`
		err := tx.QueryRow(ctx, q, playlistID, ownerSub).Scan(&pl.ID, &pl.OwnerSub, &pl.Name, &pl.CreatedAt, &pl.UpdatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return Playlist{}, ErrPlaylistNotFound
		}
		if err != nil {
			return Playlist{}, fmt.Errorf("get playlist: %w", err)
		}
	}

	items, err := r.listPlaylistItemsTx(ctx, tx, playlistID)
	if err != nil {
		return Playlist{}, err
	}
	pl.Items = items

	if err := tx.Commit(ctx); err != nil {
		return Playlist{}, fmt.Errorf("get playlist commit: %w", err)
	}

	return pl, nil
}

func (r *Repo) UpdatePlaylistName(ctx context.Context, ownerSub, playlistID, name string) (Playlist, error) {
	if ownerSub == "" {
		return Playlist{}, core.ErrUnauthorized
	}
	if playlistID == "" || name == "" {
		return Playlist{}, core.ErrInvalidInput
	}

	const q = `
UPDATE playlists
SET name = $3
WHERE id::uuid = $1 AND owner_sub = $2 AND deleted_at IS NULL
RETURNING id::text, owner_sub, name, created_at, updated_at;
`
	var pl Playlist
	if err := r.db.QueryRow(ctx, q, playlistID, ownerSub, name).Scan(&pl.ID, &pl.OwnerSub, &pl.Name, &pl.CreatedAt, &pl.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Playlist{}, ErrPlaylistNotFound
		}
		return Playlist{}, fmt.Errorf("update playlist: %w", err)
	}

	// Include items for convenience.
	items, err := r.ListPlaylistItems(ctx, ownerSub, playlistID)
	if err != nil {
		return Playlist{}, err
	}
	pl.Items = items
	return pl, nil
}

func (r *Repo) AddPlaylistItem(ctx context.Context, ownerSub, playlistID, title, youtubeURL string) (PlaylistItem, Playlist, error) {
	if ownerSub == "" {
		return PlaylistItem{}, Playlist{}, core.ErrUnauthorized
	}
	if playlistID == "" || title == "" || youtubeURL == "" {
		return PlaylistItem{}, Playlist{}, core.ErrInvalidInput
	}

	yid, err := ExtractYouTubeID(youtubeURL)
	if err != nil {
		return PlaylistItem{}, Playlist{}, fmt.Errorf("%w: %s", core.ErrInvalidInput, err.Error())
	}

	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return PlaylistItem{}, Playlist{}, fmt.Errorf("add playlist item begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Ensure playlist exists and belongs to user; lock row for concurrent inserts.
	var pl Playlist
	{
		const q = `
SELECT id::text, owner_sub, name, created_at, updated_at
FROM playlists
WHERE id::uuid = $1 AND owner_sub = $2 AND deleted_at IS NULL
FOR UPDATE;
`
		err := tx.QueryRow(ctx, q, playlistID, ownerSub).Scan(&pl.ID, &pl.OwnerSub, &pl.Name, &pl.CreatedAt, &pl.UpdatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return PlaylistItem{}, Playlist{}, ErrPlaylistNotFound
		}
		if err != nil {
			return PlaylistItem{}, Playlist{}, fmt.Errorf("add playlist item load playlist: %w", err)
		}
	}

	// Determine next position.
	var pos int
	{
		const q = `SELECT COALESCE(MAX(position), -1) + 1 FROM playlist_items WHERE playlist_id::uuid = $1;`
		if err := tx.QueryRow(ctx, q, playlistID).Scan(&pos); err != nil {
			return PlaylistItem{}, Playlist{}, fmt.Errorf("add playlist item position: %w", err)
		}
	}

	var item PlaylistItem
	{
		const q = `
INSERT INTO playlist_items (playlist_id, position, title, youtube_url, youtube_id, duration_sec)
VALUES ($1::uuid, $2, $3, $4, $5, 0)
RETURNING id::text, title, youtube_url, youtube_id, duration_sec, created_at;
`
		if err := tx.QueryRow(ctx, q, playlistID, pos, title, youtubeURL, yid).Scan(&item.ID, &item.Title, &item.YouTubeURL, &item.YouTubeID, &item.DurationSec, &item.AddedAt); err != nil {
			return PlaylistItem{}, Playlist{}, fmt.Errorf("add playlist item insert: %w", err)
		}
	}

	// Touch playlist updated_at (trigger would do on UPDATE; we do explicit UPDATE to bump).
	{
		const q = `UPDATE playlists SET name = name WHERE id::uuid = $1;`
		if _, err := tx.Exec(ctx, q, playlistID); err != nil {
			return PlaylistItem{}, Playlist{}, fmt.Errorf("add playlist item touch playlist: %w", err)
		}
	}

	// Reload items for returned playlist.
	items, err := r.listPlaylistItemsTx(ctx, tx, playlistID)
	if err != nil {
		return PlaylistItem{}, Playlist{}, err
	}
	pl.Items = items

	if err := tx.Commit(ctx); err != nil {
		return PlaylistItem{}, Playlist{}, fmt.Errorf("add playlist item commit: %w", err)
	}

	return item, pl, nil
}

func (r *Repo) ListPlaylistItems(ctx context.Context, ownerSub, playlistID string) ([]PlaylistItem, error) {
	if ownerSub == "" {
		return nil, core.ErrUnauthorized
	}
	if playlistID == "" {
		return nil, core.ErrInvalidInput
	}

	// Authorization: ensure playlist belongs to owner.
	const authQ = `
SELECT 1
FROM playlists
WHERE id::uuid = $1 AND owner_sub = $2 AND deleted_at IS NULL;
`
	var one int
	if err := r.db.QueryRow(ctx, authQ, playlistID, ownerSub).Scan(&one); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPlaylistNotFound
		}
		return nil, fmt.Errorf("list playlist items auth: %w", err)
	}

	const q = `
SELECT id::text, title, youtube_url, youtube_id, duration_sec, created_at
FROM playlist_items
WHERE playlist_id::uuid = $1
ORDER BY position ASC;
`
	rows, err := r.db.Query(ctx, q, playlistID)
	if err != nil {
		return nil, fmt.Errorf("list playlist items: %w", err)
	}
	defer rows.Close()

	out := make([]PlaylistItem, 0, 16)
	for rows.Next() {
		var it PlaylistItem
		if err := rows.Scan(&it.ID, &it.Title, &it.YouTubeURL, &it.YouTubeID, &it.DurationSec, &it.AddedAt); err != nil {
			return nil, fmt.Errorf("list playlist items scan: %w", err)
		}
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list playlist items rows: %w", err)
	}
	return out, nil
}

func (r *Repo) listPlaylistItemsTx(ctx context.Context, tx pgx.Tx, playlistID string) ([]PlaylistItem, error) {
	const q = `
SELECT id::text, title, youtube_url, youtube_id, duration_sec, created_at
FROM playlist_items
WHERE playlist_id::uuid = $1
ORDER BY position ASC;
`
	rows, err := tx.Query(ctx, q, playlistID)
	if err != nil {
		return nil, fmt.Errorf("list playlist items (tx): %w", err)
	}
	defer rows.Close()

	out := make([]PlaylistItem, 0, 16)
	for rows.Next() {
		var it PlaylistItem
		if err := rows.Scan(&it.ID, &it.Title, &it.YouTubeURL, &it.YouTubeID, &it.DurationSec, &it.AddedAt); err != nil {
			return nil, fmt.Errorf("list playlist items (tx) scan: %w", err)
		}
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list playlist items (tx) rows: %w", err)
	}
	return out, nil
}

// ============================
// Rooms
// ============================

type DBRoomInfo struct {
	ID            string
	Name          string
	OwnerSub      string
	OnlinePlayers int
	UpdatedAt     time.Time
}

// CreateRoom creates a room and ensures the owner exists in users.
func (r *Repo) CreateRoom(ctx context.Context, ownerSub, name string) (string, error) {
	if ownerSub == "" {
		return "", core.ErrUnauthorized
	}
	if name == "" {
		name = "Room"
	}

	// Ensure owner exists in users.
	if err := r.ensureUserExists(ctx, ownerSub); err != nil {
		return "", err
	}

	const q = `
INSERT INTO rooms (name, owner_sub)
VALUES ($1, $2)
RETURNING id::text;
`
	var roomID string
	if err := r.db.QueryRow(ctx, q, name, ownerSub).Scan(&roomID); err != nil {
		return "", fmt.Errorf("create room: %w", err)
	}
	return roomID, nil
}

func (r *Repo) ListRooms(ctx context.Context) ([]DBRoomInfo, error) {
	const q = `
SELECT
  rm.id::text,
  rm.name,
  rm.owner_sub,
  rm.updated_at,
  COALESCE(SUM(CASE WHEN rp.connected THEN 1 ELSE 0 END), 0)::int AS online_players
FROM rooms rm
LEFT JOIN room_players rp ON rp.room_id = rm.id
GROUP BY rm.id
ORDER BY rm.updated_at DESC;
`
	rows, err := r.db.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list rooms: %w", err)
	}
	defer rows.Close()

	out := make([]DBRoomInfo, 0, 16)
	for rows.Next() {
		var ri DBRoomInfo
		if err := rows.Scan(&ri.ID, &ri.Name, &ri.OwnerSub, &ri.UpdatedAt, &ri.OnlinePlayers); err != nil {
			return nil, fmt.Errorf("list rooms scan: %w", err)
		}
		out = append(out, ri)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list rooms rows: %w", err)
	}
	return out, nil
}

func (r *Repo) GetRoomSnapshot(ctx context.Context, roomID string) (RoomSnapshot, error) {
	if roomID == "" {
		return RoomSnapshot{}, core.ErrInvalidInput
	}

	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return RoomSnapshot{}, fmt.Errorf("get room begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var snap RoomSnapshot
	var loadedPlaylistID *string
	{
		const q = `
SELECT id::text, name, owner_sub, loaded_playlist_id::text,
       playback_track_index, playback_paused, playback_position_ms, playback_updated_at
FROM rooms
WHERE id::uuid = $1;
`
		err := tx.QueryRow(ctx, q, roomID).Scan(
			&snap.RoomID,
			&snap.Name,
			&snap.OwnerSub,
			&loadedPlaylistID,
			&snap.Playback.TrackIndex,
			&snap.Playback.Paused,
			&snap.Playback.PositionMS,
			&snap.Playback.UpdatedAt,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return RoomSnapshot{}, core.ErrRoomNotFound
		}
		if err != nil {
			return RoomSnapshot{}, fmt.Errorf("get room: %w", err)
		}
	}

	// Players
	{
		const q = `
SELECT id::text, COALESCE(user_sub, ''), nickname, picture_url, score, connected
FROM room_players
WHERE room_id::uuid = $1
ORDER BY connected DESC, score DESC, nickname ASC;
`
		rows, err := tx.Query(ctx, q, roomID)
		if err != nil {
			return RoomSnapshot{}, fmt.Errorf("get room players: %w", err)
		}
		defer rows.Close()

		players := make([]PlayerView, 0, 16)
		for rows.Next() {
			var pv PlayerView
			if err := rows.Scan(&pv.PlayerID, &pv.Sub, &pv.Nickname, &pv.PictureURL, &pv.Score, &pv.Connected); err != nil {
				return RoomSnapshot{}, fmt.Errorf("get room players scan: %w", err)
			}
			players = append(players, pv)
		}
		if err := rows.Err(); err != nil {
			return RoomSnapshot{}, fmt.Errorf("get room players rows: %w", err)
		}
		snap.Players = players
	}

	// Loaded playlist (optional)
	if loadedPlaylistID != nil && *loadedPlaylistID != "" {
		pl, err := r.getPlaylistByIDTx(ctx, tx, *loadedPlaylistID)
		if err != nil {
			// If playlist is missing (deleted), treat as no playlist loaded.
			if errors.Is(err, ErrPlaylistNotFound) {
				snap.Playlist = nil
				snap.Playback.PlaylistID = ""
				snap.Playback.Track = nil
			} else {
				return RoomSnapshot{}, err
			}
		} else {
			items, err := r.listPlaylistItemsTx(ctx, tx, pl.ID)
			if err != nil {
				return RoomSnapshot{}, err
			}
			snap.Playlist = &PlaylistView{
				PlaylistID: pl.ID,
				Name:       pl.Name,
				Items:      items,
				LoadedAt:   time.Now().UTC(), // schema has no loaded_at; keep best-effort
			}
			snap.Playback.PlaylistID = pl.ID
			// Resolve track from track index.
			if snap.Playback.TrackIndex >= 0 && snap.Playback.TrackIndex < len(items) {
				track := items[snap.Playback.TrackIndex]
				snap.Playback.Track = &track
			} else {
				snap.Playback.Track = nil
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return RoomSnapshot{}, fmt.Errorf("get room commit: %w", err)
	}
	return snap, nil
}

func (r *Repo) getPlaylistByIDTx(ctx context.Context, tx pgx.Tx, playlistID string) (Playlist, error) {
	const q = `
SELECT id::text, owner_sub, name, created_at, updated_at
FROM playlists
WHERE id::uuid = $1 AND deleted_at IS NULL;
`
	var pl Playlist
	if err := tx.QueryRow(ctx, q, playlistID).Scan(&pl.ID, &pl.OwnerSub, &pl.Name, &pl.CreatedAt, &pl.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Playlist{}, ErrPlaylistNotFound
		}
		return Playlist{}, fmt.Errorf("get playlist by id: %w", err)
	}
	return pl, nil
}

// JoinRoom inserts a room_players row and returns playerId.
func (r *Repo) JoinRoom(ctx context.Context, roomID, userSub, nickname, pictureURL string) (string, error) {
	if roomID == "" {
		return "", core.ErrInvalidInput
	}

	// If userSub is provided, ensure user exists. Also, use stored profile as defaults.
	if userSub != "" {
		if err := r.ensureUserExists(ctx, userSub); err != nil {
			return "", err
		}

		profNick, profPic, err := r.profileDefaults(ctx, userSub)
		if err != nil {
			return "", err
		}

		if nickname == "" {
			nickname = profNick
		}
		if pictureURL == "" {
			pictureURL = profPic
		}
	} else if nickname == "" {
		nickname = "Anonymous"
	}

	const q = `
INSERT INTO room_players (room_id, user_sub, nickname, picture_url, connected)
VALUES ($1::uuid, NULLIF($2,''), $3, $4, TRUE)
RETURNING id::text;
`
	var playerID string
	if err := r.db.QueryRow(ctx, q, roomID, userSub, nickname, pictureURL).Scan(&playerID); err != nil {
		// Likely FK violation if room doesn't exist.
		return "", fmt.Errorf("join room: %w", err)
	}
	return playerID, nil
}

func (r *Repo) profileDefaults(ctx context.Context, sub string) (string, string, error) {
	const q = `
SELECT nickname, picture_url
FROM users
WHERE sub = $1 AND deleted_at IS NULL;
`
	var nick, pic string
	err := r.db.QueryRow(ctx, q, sub).Scan(&nick, &pic)
	if errors.Is(err, pgx.ErrNoRows) {
		return "Player", "", nil
	}
	if err != nil {
		return "", "", fmt.Errorf("load profile defaults: %w", err)
	}
	if nick == "" {
		nick = "Player"
	}
	return nick, pic, nil
}

func (r *Repo) LeaveRoom(ctx context.Context, roomID, playerID string) error {
	if roomID == "" || playerID == "" {
		return core.ErrInvalidInput
	}

	const q = `
UPDATE room_players
SET connected = FALSE,
    left_at = COALESCE(left_at, now())
WHERE id::uuid = $1 AND room_id::uuid = $2;
`
	ct, err := r.db.Exec(ctx, q, playerID, roomID)
	if err != nil {
		return fmt.Errorf("leave room: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return core.ErrPlayerNotFound
	}
	return nil
}

func (r *Repo) KickPlayer(ctx context.Context, roomID, ownerSub, playerID string) error {
	if roomID == "" || ownerSub == "" || playerID == "" {
		return core.ErrInvalidInput
	}

	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("kick begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Verify owner.
	if ok, err := r.isRoomOwnerTx(ctx, tx, roomID, ownerSub); err != nil {
		return err
	} else if !ok {
		return core.ErrNotOwner
	}

	const q = `DELETE FROM room_players WHERE id::uuid = $1 AND room_id::uuid = $2;`
	ct, err := tx.Exec(ctx, q, playerID, roomID)
	if err != nil {
		return fmt.Errorf("kick: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return core.ErrPlayerNotFound
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("kick commit: %w", err)
	}
	return nil
}

func (r *Repo) SetScore(ctx context.Context, roomID, ownerSub, playerID string, score int) error {
	if roomID == "" || ownerSub == "" || playerID == "" {
		return core.ErrInvalidInput
	}

	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("set score begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if ok, err := r.isRoomOwnerTx(ctx, tx, roomID, ownerSub); err != nil {
		return err
	} else if !ok {
		return core.ErrNotOwner
	}

	const q = `
UPDATE room_players
SET score = $3
WHERE id::uuid = $1 AND room_id::uuid = $2;
`
	ct, err := tx.Exec(ctx, q, playerID, roomID, score)
	if err != nil {
		return fmt.Errorf("set score: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return core.ErrPlayerNotFound
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("set score commit: %w", err)
	}
	return nil
}

func (r *Repo) AddScore(ctx context.Context, roomID, ownerSub, playerID string, delta int) error {
	if roomID == "" || ownerSub == "" || playerID == "" {
		return core.ErrInvalidInput
	}

	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("add score begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if ok, err := r.isRoomOwnerTx(ctx, tx, roomID, ownerSub); err != nil {
		return err
	} else if !ok {
		return core.ErrNotOwner
	}

	const q = `
UPDATE room_players
SET score = score + $3
WHERE id::uuid = $1 AND room_id::uuid = $2;
`
	ct, err := tx.Exec(ctx, q, playerID, roomID, delta)
	if err != nil {
		return fmt.Errorf("add score: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return core.ErrPlayerNotFound
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("add score commit: %w", err)
	}
	return nil
}

func (r *Repo) LoadPlaylistToRoom(ctx context.Context, roomID, ownerSub, playlistID string) error {
	if roomID == "" || ownerSub == "" || playlistID == "" {
		return core.ErrInvalidInput
	}

	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("load playlist begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if ok, err := r.isRoomOwnerTx(ctx, tx, roomID, ownerSub); err != nil {
		return err
	} else if !ok {
		return core.ErrNotOwner
	}

	// Ensure playlist belongs to owner and isn't deleted.
	const pQ = `
SELECT 1
FROM playlists
WHERE id::uuid = $1 AND owner_sub = $2 AND deleted_at IS NULL;
`
	var one int
	if err := tx.QueryRow(ctx, pQ, playlistID, ownerSub).Scan(&one); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrPlaylistNotFound
		}
		return fmt.Errorf("load playlist verify: %w", err)
	}

	const q = `
UPDATE rooms
SET loaded_playlist_id = $3::uuid,
    playback_track_index = 0,
    playback_paused = TRUE,
    playback_position_ms = 0,
    playback_updated_at = now()
WHERE id::uuid = $1 AND owner_sub = $2;
`
	ct, err := tx.Exec(ctx, q, roomID, ownerSub, playlistID)
	if err != nil {
		return fmt.Errorf("load playlist update room: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return core.ErrNotOwner
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("load playlist commit: %w", err)
	}
	return nil
}

func (r *Repo) SetPlayback(ctx context.Context, roomID, ownerSub string, trackIndex int, paused *bool, positionMS *int) error {
	if roomID == "" || ownerSub == "" {
		return core.ErrInvalidInput
	}

	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("set playback begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Must be owner.
	if ok, err := r.isRoomOwnerTx(ctx, tx, roomID, ownerSub); err != nil {
		return err
	} else if !ok {
		return core.ErrNotOwner
	}

	// Ensure there is a loaded playlist, and validate track index within range.
	var loadedPlaylistID *string
	{
		const q = `SELECT loaded_playlist_id::text FROM rooms WHERE id::uuid = $1 FOR UPDATE;`
		if err := tx.QueryRow(ctx, q, roomID).Scan(&loadedPlaylistID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return core.ErrRoomNotFound
			}
			return fmt.Errorf("set playback load room: %w", err)
		}
		if loadedPlaylistID == nil || *loadedPlaylistID == "" {
			return core.ErrInvalidInput
		}
	}
	{
		const q = `SELECT COUNT(1) FROM playlist_items WHERE playlist_id::uuid = $1;`
		var cnt int
		if err := tx.QueryRow(ctx, q, *loadedPlaylistID).Scan(&cnt); err != nil {
			return fmt.Errorf("set playback count tracks: %w", err)
		}
		if trackIndex < 0 || trackIndex >= cnt {
			return core.ErrInvalidInput
		}
	}

	// Apply updates.
	pausedVal := "playback_paused"
	if paused != nil {
		if *paused {
			pausedVal = "TRUE"
		} else {
			pausedVal = "FALSE"
		}
	}
	posVal := "playback_position_ms"
	if positionMS != nil {
		if *positionMS < 0 {
			return core.ErrInvalidInput
		}
		posVal = fmt.Sprintf("%d", *positionMS)
	}

	// NOTE: We can't parameterize identifiers easily; use a safe approach by using parameters for values,
	// except for optional updates. Here we simply set both fields via COALESCE-like.
	// Keep it simple and safe: always write both using parameters.
	newPaused := false
	if paused != nil {
		newPaused = *paused
	} else {
		// preserve
		const q = `SELECT playback_paused FROM rooms WHERE id::uuid = $1;`
		if err := tx.QueryRow(ctx, q, roomID).Scan(&newPaused); err != nil {
			return fmt.Errorf("set playback read paused: %w", err)
		}
	}
	newPos := 0
	if positionMS != nil {
		newPos = *positionMS
	} else {
		const q = `SELECT playback_position_ms FROM rooms WHERE id::uuid = $1;`
		if err := tx.QueryRow(ctx, q, roomID).Scan(&newPos); err != nil {
			return fmt.Errorf("set playback read pos: %w", err)
		}
	}

	_ = pausedVal
	_ = posVal

	const q = `
UPDATE rooms
SET playback_track_index = $3,
    playback_paused = $4,
    playback_position_ms = $5,
    playback_updated_at = now()
WHERE id::uuid = $1 AND owner_sub = $2;
`
	if _, err := tx.Exec(ctx, q, roomID, ownerSub, trackIndex, newPaused, newPos); err != nil {
		return fmt.Errorf("set playback update: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("set playback commit: %w", err)
	}
	return nil
}

func (r *Repo) TogglePause(ctx context.Context, roomID, ownerSub string, paused bool) error {
	p := paused
	return r.SetPlayback(ctx, roomID, ownerSub, -1, &p, nil) // trackIndex -1 invalid; handle separately below if needed
}

// TogglePauseSafe toggles pause without changing track index. Prefer this over TogglePause.
func (r *Repo) TogglePauseSafe(ctx context.Context, roomID, ownerSub string, paused bool) error {
	if roomID == "" || ownerSub == "" {
		return core.ErrInvalidInput
	}

	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("toggle pause begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if ok, err := r.isRoomOwnerTx(ctx, tx, roomID, ownerSub); err != nil {
		return err
	} else if !ok {
		return core.ErrNotOwner
	}

	const q = `
UPDATE rooms
SET playback_paused = $3,
    playback_updated_at = now()
WHERE id::uuid = $1 AND owner_sub = $2;
`
	ct, err := tx.Exec(ctx, q, roomID, ownerSub, paused)
	if err != nil {
		return fmt.Errorf("toggle pause: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return core.ErrRoomNotFound
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("toggle pause commit: %w", err)
	}
	return nil
}

func (r *Repo) Seek(ctx context.Context, roomID, ownerSub string, positionMS int) error {
	if roomID == "" || ownerSub == "" || positionMS < 0 {
		return core.ErrInvalidInput
	}

	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("seek begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if ok, err := r.isRoomOwnerTx(ctx, tx, roomID, ownerSub); err != nil {
		return err
	} else if !ok {
		return core.ErrNotOwner
	}

	const q = `
UPDATE rooms
SET playback_position_ms = $3,
    playback_updated_at = now()
WHERE id::uuid = $1 AND owner_sub = $2;
`
	ct, err := tx.Exec(ctx, q, roomID, ownerSub, positionMS)
	if err != nil {
		return fmt.Errorf("seek: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return core.ErrRoomNotFound
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("seek commit: %w", err)
	}
	return nil
}

func (r *Repo) isRoomOwnerTx(ctx context.Context, tx pgx.Tx, roomID, ownerSub string) (bool, error) {
	const q = `SELECT 1 FROM rooms WHERE id::uuid = $1 AND owner_sub = $2;`
	var one int
	if err := tx.QueryRow(ctx, q, roomID, ownerSub).Scan(&one); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Either room doesn't exist or not owner.
			return false, nil
		}
		return false, fmt.Errorf("isRoomOwner: %w", err)
	}
	return true, nil
}
