package httpapi

import (
	"github.com/go-chi/chi/v5"

	"github.com/valentin/bes-games/backend/internal/games"
)

type GameModule interface {
	Meta() games.Game
	Mount(r chi.Router, s *Server)
}

type nameThatTuneModule struct{}

func NewNameThatTuneModule() GameModule {
	return nameThatTuneModule{}
}

func (nameThatTuneModule) Meta() games.Game {
	return games.Game{
		ID:          "name-that-tune",
		Name:        "Name That Tune",
		Description: "Guess songs as fast as you can. Rooms, playlists, buzzer, and synchronized playback state.",
	}
}

func (nameThatTuneModule) Mount(r chi.Router, s *Server) {
	r.Get("/rooms", s.handleListRooms)
	r.Post("/rooms", s.requireAuth(s.handleCreateRoom))

	r.Route("/rooms/{roomId}", func(rr chi.Router) {
		rr.Get("/", s.handleGetRoom)
		rr.Post("/join", s.handleJoinRoom)
		rr.Post("/leave", s.handleLeaveRoom)
		rr.Get("/ws", s.handleRoomWS)
	})

	r.Get("/playlists", s.requireAuth(s.handleListPlaylists))
	r.Post("/playlists", s.requireAuth(s.handleCreatePlaylist))
	r.Patch("/playlists/{playlistId}", s.requireAuth(s.handlePatchPlaylist))
	r.Post("/playlists/{playlistId}/items", s.requireAuth(s.handleAddPlaylistItem))
	r.Patch("/playlists/{playlistId}/items/{itemId}", s.requireAuth(s.handlePatchPlaylistItem))
	r.Delete("/playlists/{playlistId}/items/{itemId}", s.requireAuth(s.handleDeletePlaylistItem))
}
