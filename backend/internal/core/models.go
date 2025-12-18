package core

import "time"

// UserProfile represents the user-customizable profile attributes.
// The stable identifier for an authenticated user is the OIDC "sub".
type UserProfile struct {
	Sub        string    `json:"sub"`
	Nickname   string    `json:"nickname"`
	PictureURL string    `json:"pictureUrl"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// Domain-level errors shared across games.
var (
	// Rooms / players
	ErrRoomNotFound   = errorString("room not found")
	ErrPlayerNotFound = errorString("player not found")
	ErrNotOwner       = errorString("not room owner")

	// Auth / input
	ErrUnauthorized = errorString("unauthorized")
	ErrInvalidInput = errorString("invalid input")
)

// errorString is a tiny internal error type to avoid importing "errors" here.
// It behaves like errors.New(...) but keeps this file dependency-light.
type errorString string

func (e errorString) Error() string { return string(e) }
