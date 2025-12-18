package namethattune

import "time"

// ============================
// Playlists
// ============================

type Playlist struct {
	ID        string         `json:"id"`
	OwnerSub  string         `json:"ownerSub"`
	Name      string         `json:"name"`
	Items     []PlaylistItem `json:"items"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
}

type PlaylistItem struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	YouTubeURL  string    `json:"youTubeURL"` // keep legacy-ish name used previously in UI
	YouTubeID   string    `json:"youTubeID"`
	DurationSec int       `json:"durationSec"`
	AddedAt     time.Time `json:"addedAt"`
}

// PlaylistView is the denormalized playlist payload embedded in a room snapshot.
// It mirrors what the frontend expects for a "loaded playlist".
type PlaylistView struct {
	PlaylistID string         `json:"playlistId"`
	Name       string         `json:"name"`
	Items      []PlaylistItem `json:"items"`
	LoadedAt   time.Time      `json:"loadedAt"`
}

// ============================
// Rooms / Players / Playback
// ============================

// PlayerView is the roster entry shown in rooms.
type PlayerView struct {
	PlayerID   string `json:"playerId"`
	Sub        string `json:"sub,omitempty"`
	Nickname   string `json:"nickname"`
	PictureURL string `json:"pictureUrl,omitempty"`
	Score      int    `json:"score"`
	Connected  bool   `json:"connected"`
}

// PlaybackView is the client-visible playback state.
// The "track" is resolved from the loaded playlist items.
type PlaybackView struct {
	PlaylistID string        `json:"playlistId,omitempty"`
	TrackIndex int           `json:"trackIndex"`
	Track      *PlaylistItem `json:"track,omitempty"`
	Paused     bool          `json:"paused"`
	PositionMS int           `json:"positionMs"`
	UpdatedAt  time.Time     `json:"updatedAt"`
}

// RoomSnapshot is the main read model for room state (roster + loaded playlist + playback).
type RoomSnapshot struct {
	RoomID   string        `json:"roomId"`
	Name     string        `json:"name"`
	OwnerSub string        `json:"ownerSub,omitempty"`
	Players  []PlayerView  `json:"players"`
	Playlist *PlaylistView `json:"playlist,omitempty"`
	Playback PlaybackView  `json:"playback"`
}

var ErrPlaylistNotFound = errorString("playlist not found")

// errorString is a tiny internal error type to avoid importing "errors" here.
// It behaves like errors.New(...) but keeps this file dependency-light.
type errorString string

func (e errorString) Error() string { return string(e) }
