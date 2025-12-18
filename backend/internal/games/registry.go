package games

// Game describes an available game hosted on the platform.
// It is intentionally small and stable so the frontend can build a game selector.
type Game struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// List returns the current list of available games.
// In the future this can be backed by configuration or database entries.
func List() []Game {
	return []Game{
		{
			ID:          "name-that-tune",
			Name:        "Name That Tune",
			Description: "Guess songs as fast as you can. Rooms, playlists, buzzer, and synchronized playback state.",
		},
	}
}
