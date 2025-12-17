package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"

	"github.com/valentin/bes-blind/backend/internal/game"
	"github.com/valentin/bes-blind/backend/internal/realtime"
)

// Server provides the HTTP API (REST + WebSocket) for the name-that-tune game.
//
// Auth model (still temporary until real OIDC validation is implemented):
//   - Authenticated requests must include:
//     X-User-Sub: <oidc subject>
//   - Anonymous users can join rooms without it.
//
// Persistence model:
//   - All state (profiles, playlists, rooms, players, playback) is stored in Postgres via game.Repo.
//   - Realtime updates are fanned out via realtime.Registry (in-memory pub/sub), while the source of truth is DB.
//
// Endpoints (summary):
// - GET    /healthz
// - GET    /api/rooms
// - POST   /api/rooms                    (auth required)
// - GET    /api/rooms/{roomId}
// - POST   /api/rooms/{roomId}/join      (anon allowed)
// - POST   /api/rooms/{roomId}/leave
// - WS     /api/rooms/{roomId}/ws
//
// Owner controls (auth required; must be room owner):
// - POST   /api/rooms/{roomId}/kick
// - POST   /api/rooms/{roomId}/score/set
// - POST   /api/rooms/{roomId}/score/add
// - POST   /api/rooms/{roomId}/playlist/load
// - POST   /api/rooms/{roomId}/playback/set
// - POST   /api/rooms/{roomId}/playback/pause
// - POST   /api/rooms/{roomId}/playback/seek
//
// Profile (auth required):
// - GET    /api/me
// - PUT    /api/me
// - DELETE /api/me
//
// Playlists (auth required):
// - GET    /api/me/playlists
// - POST   /api/me/playlists
// - PATCH  /api/me/playlists/{playlistId}
// - POST   /api/me/playlists/{playlistId}/items
//
// Player actions:
// - POST   /api/rooms/{roomId}/buzz
type Server struct {
	repo *game.Repo
	rt   *realtime.Registry
}

type wsOriginPatternsCtxKey struct{}

type Options struct {
	// AllowedOrigins configures CORS allowed origins.
	// If empty, defaults to http://localhost:5173 (Vite default).
	AllowedOrigins []string

	// WSOriginPatterns configures WebSocket origin patterns for websocket.Accept.
	// WebSocket origin checks are independent from HTTP CORS middleware.
	// If empty, defaults to AllowedOrigins (or the same localhost default).
	WSOriginPatterns []string

	// ReadHeaderTimeout is applied to the underlying http.Server if you use Handler() with your own server.
	// (This file only exposes an http.Handler.)
	ReadHeaderTimeout time.Duration
}

func NewServer(repo *game.Repo, rt *realtime.Registry) *Server {
	return &Server{repo: repo, rt: rt}
}

func (s *Server) Handler(opts Options) http.Handler {
	r := chi.NewRouter()

	allowed := opts.AllowedOrigins
	if len(allowed) == 0 {
		allowed = []string{"http://localhost:5173"}
	}

	wsOriginPatterns := opts.WSOriginPatterns
	if len(wsOriginPatterns) == 0 {
		wsOriginPatterns = allowed
	}

	// Make WS origin patterns available to the WS handler without changing its signature.
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), wsOriginPatternsCtxKey{}, wsOriginPatterns)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   allowed,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-User-Sub"},
		ExposedHeaders:   []string{},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"status": "ok",
			"time":   time.Now().UTC().Format(time.RFC3339Nano),
		})
	})

	r.Route("/api", func(api chi.Router) {
		api.Get("/rooms", s.handleListRooms)
		api.Post("/rooms", s.requireAuth(s.handleCreateRoom))

		api.Route("/rooms/{roomId}", func(rr chi.Router) {
			rr.Get("/", s.handleGetRoom)
			rr.Post("/join", s.handleJoinRoom)
			rr.Post("/leave", s.handleLeaveRoom)

			rr.Get("/ws", s.handleRoomWS)

			// Owner controls
			rr.Post("/kick", s.requireAuth(s.handleKick))
			rr.Post("/score/set", s.requireAuth(s.handleScoreSet))
			rr.Post("/score/add", s.requireAuth(s.handleScoreAdd))
			rr.Post("/playlist/load", s.requireAuth(s.handleLoadPlaylist))
			rr.Post("/playback/set", s.requireAuth(s.handlePlaybackSet))
			rr.Post("/playback/pause", s.requireAuth(s.handlePlaybackPause))
			rr.Post("/playback/seek", s.requireAuth(s.handlePlaybackSeek))

			// Player actions
			rr.Post("/buzz", s.handleBuzz)
		})

		// Profile / account
		api.Get("/me", s.requireAuth(s.handleGetMe))
		api.Put("/me", s.requireAuth(s.handlePutMe))
		api.Delete("/me", s.requireAuth(s.handleDeleteMe))

		// Playlists
		api.Get("/me/playlists", s.requireAuth(s.handleListMyPlaylists))
		api.Post("/me/playlists", s.requireAuth(s.handleCreateMyPlaylist))
		api.Patch("/me/playlists/{playlistId}", s.requireAuth(s.handlePatchMyPlaylist))
		api.Post("/me/playlists/{playlistId}/items", s.requireAuth(s.handleAddMyPlaylistItem))
	})

	return r
}

// =============================
// Middleware / helpers
// =============================

func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if userSub(r) == "" {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next(w, r)
	}
}

func userSub(r *http.Request) string {
	return strings.TrimSpace(r.Header.Get("X-User-Sub"))
}

func roomIDParam(r *http.Request) string {
	return strings.TrimSpace(chi.URLParam(r, "roomId"))
}

func playlistIDParam(r *http.Request) string {
	return strings.TrimSpace(chi.URLParam(r, "playlistId"))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{
		"error":  message,
		"status": status,
	})
}

func decodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

func mapDomainErr(err error) (int, string) {
	switch {
	case err == nil:
		return http.StatusOK, ""
	case errors.Is(err, game.ErrUnauthorized):
		return http.StatusUnauthorized, err.Error()
	case errors.Is(err, game.ErrNotOwner):
		return http.StatusForbidden, err.Error()
	case errors.Is(err, game.ErrRoomNotFound):
		return http.StatusNotFound, err.Error()
	case errors.Is(err, game.ErrPlayerNotFound):
		return http.StatusNotFound, err.Error()
	case errors.Is(err, game.ErrPlaylistNotFound):
		return http.StatusNotFound, err.Error()
	case errors.Is(err, game.ErrInvalidInput):
		return http.StatusBadRequest, err.Error()
	default:
		return http.StatusInternalServerError, "internal server error"
	}
}

// After state-changing operations, we broadcast a fresh snapshot to the room websocket subscribers.
func (s *Server) broadcastSnapshot(ctx context.Context, roomID string) {
	if s.rt == nil {
		return
	}
	snap, err := s.repo.GetRoomSnapshot(ctx, roomID)
	if err != nil {
		// If snapshot can't be fetched, do not broadcast.
		return
	}
	s.rt.Room(roomID).Broadcast(realtime.Event{
		Type:    "room.snapshot",
		RoomID:  roomID,
		Payload: snap,
	})
}

// =============================
// REST handlers: Rooms
// =============================

func (s *Server) handleListRooms(w http.ResponseWriter, r *http.Request) {
	rooms, err := s.repo.ListRooms(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	// Map to the same JSON shape used previously.
	type roomInfo struct {
		RoomID        string    `json:"roomId"`
		Name          string    `json:"name"`
		OwnerSub      string    `json:"ownerSub,omitempty"`
		OnlinePlayers int       `json:"onlinePlayers"`
		Subscribers   int       `json:"subscribers"`
		UpdatedAt     time.Time `json:"updatedAt"`
	}

	out := make([]roomInfo, 0, len(rooms))
	for _, ri := range rooms {
		subs := 0
		if s.rt != nil {
			subs = s.rt.Room(ri.ID).SubscriberCount()
		}
		out = append(out, roomInfo{
			RoomID:        ri.ID,
			Name:          ri.Name,
			OwnerSub:      ri.OwnerSub,
			OnlinePlayers: ri.OnlinePlayers,
			Subscribers:   subs,
			UpdatedAt:     ri.UpdatedAt,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"rooms": out})
}

func (s *Server) handleCreateRoom(w http.ResponseWriter, r *http.Request) {
	type reqBody struct {
		Name string `json:"name"`
	}
	var body reqBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	roomID, err := s.repo.CreateRoom(r.Context(), userSub(r), strings.TrimSpace(body.Name))
	if err != nil {
		status, msg := mapDomainErr(err)
		writeError(w, status, msg)
		return
	}

	// Create-room is a state change; broadcast snapshot (will include empty players).
	s.broadcastSnapshot(r.Context(), roomID)

	writeJSON(w, http.StatusCreated, map[string]any{"roomId": roomID})
}

func (s *Server) handleGetRoom(w http.ResponseWriter, r *http.Request) {
	roomID := roomIDParam(r)
	snap, err := s.repo.GetRoomSnapshot(r.Context(), roomID)
	if err != nil {
		status, msg := mapDomainErr(err)
		writeError(w, status, msg)
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

func (s *Server) handleJoinRoom(w http.ResponseWriter, r *http.Request) {
	roomID := roomIDParam(r)

	type reqBody struct {
		Nickname   string `json:"nickname,omitempty"`
		PictureURL string `json:"pictureUrl,omitempty"`
	}
	var body reqBody
	// Optional body. If empty, decodeJSON may return EOF; treat as ok.
	if err := decodeJSON(r, &body); err != nil && !isJSONEOF(err) {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	playerID, err := s.repo.JoinRoom(r.Context(), roomID, userSub(r), strings.TrimSpace(body.Nickname), strings.TrimSpace(body.PictureURL))
	if err != nil {
		status, msg := mapDomainErr(err)
		writeError(w, status, msg)
		return
	}

	snap, err := s.repo.GetRoomSnapshot(r.Context(), roomID)
	if err != nil {
		status, msg := mapDomainErr(err)
		writeError(w, status, msg)
		return
	}

	// Broadcast snapshot for all listeners.
	s.broadcastSnapshot(r.Context(), roomID)

	writeJSON(w, http.StatusOK, map[string]any{
		"playerId": playerID,
		"snapshot": snap,
	})
}

func (s *Server) handleLeaveRoom(w http.ResponseWriter, r *http.Request) {
	roomID := roomIDParam(r)

	type reqBody struct {
		PlayerID string `json:"playerId"`
	}
	var body reqBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	if err := s.repo.LeaveRoom(r.Context(), roomID, strings.TrimSpace(body.PlayerID)); err != nil {
		status, msg := mapDomainErr(err)
		writeError(w, status, msg)
		return
	}

	s.broadcastSnapshot(r.Context(), roomID)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// =============================
// REST handlers: Owner controls
// =============================

func (s *Server) handleKick(w http.ResponseWriter, r *http.Request) {
	roomID := roomIDParam(r)

	type reqBody struct {
		PlayerID string `json:"playerId"`
	}
	var body reqBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	if err := s.repo.KickPlayer(r.Context(), roomID, userSub(r), strings.TrimSpace(body.PlayerID)); err != nil {
		status, msg := mapDomainErr(err)
		writeError(w, status, msg)
		return
	}

	snap, err := s.repo.GetRoomSnapshot(r.Context(), roomID)
	if err != nil {
		status, msg := mapDomainErr(err)
		writeError(w, status, msg)
		return
	}

	s.broadcastSnapshot(r.Context(), roomID)
	writeJSON(w, http.StatusOK, snap)
}

func (s *Server) handleScoreSet(w http.ResponseWriter, r *http.Request) {
	roomID := roomIDParam(r)

	type reqBody struct {
		PlayerID string `json:"playerId"`
		Score    int    `json:"score"`
	}
	var body reqBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	if err := s.repo.SetScore(r.Context(), roomID, userSub(r), strings.TrimSpace(body.PlayerID), body.Score); err != nil {
		status, msg := mapDomainErr(err)
		writeError(w, status, msg)
		return
	}

	snap, err := s.repo.GetRoomSnapshot(r.Context(), roomID)
	if err != nil {
		status, msg := mapDomainErr(err)
		writeError(w, status, msg)
		return
	}

	s.broadcastSnapshot(r.Context(), roomID)
	writeJSON(w, http.StatusOK, snap)
}

func (s *Server) handleScoreAdd(w http.ResponseWriter, r *http.Request) {
	roomID := roomIDParam(r)

	type reqBody struct {
		PlayerID string `json:"playerId"`
		Delta    int    `json:"delta"`
	}
	var body reqBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	if err := s.repo.AddScore(r.Context(), roomID, userSub(r), strings.TrimSpace(body.PlayerID), body.Delta); err != nil {
		status, msg := mapDomainErr(err)
		writeError(w, status, msg)
		return
	}

	snap, err := s.repo.GetRoomSnapshot(r.Context(), roomID)
	if err != nil {
		status, msg := mapDomainErr(err)
		writeError(w, status, msg)
		return
	}

	s.broadcastSnapshot(r.Context(), roomID)
	writeJSON(w, http.StatusOK, snap)
}

func (s *Server) handleLoadPlaylist(w http.ResponseWriter, r *http.Request) {
	roomID := roomIDParam(r)

	type reqBody struct {
		PlaylistID string `json:"playlistId"`
	}
	var body reqBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	if err := s.repo.LoadPlaylistToRoom(r.Context(), roomID, userSub(r), strings.TrimSpace(body.PlaylistID)); err != nil {
		status, msg := mapDomainErr(err)
		writeError(w, status, msg)
		return
	}

	snap, err := s.repo.GetRoomSnapshot(r.Context(), roomID)
	if err != nil {
		status, msg := mapDomainErr(err)
		writeError(w, status, msg)
		return
	}

	s.broadcastSnapshot(r.Context(), roomID)
	writeJSON(w, http.StatusOK, snap)
}

func (s *Server) handlePlaybackSet(w http.ResponseWriter, r *http.Request) {
	roomID := roomIDParam(r)

	type reqBody struct {
		TrackIndex int   `json:"trackIndex"`
		Paused     *bool `json:"paused,omitempty"`
		PositionMS *int  `json:"positionMs,omitempty"`
	}
	var body reqBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	// track index is mandatory in this endpoint
	if err := s.repo.SetPlayback(r.Context(), roomID, userSub(r), body.TrackIndex, body.Paused, body.PositionMS); err != nil {
		status, msg := mapDomainErr(err)
		writeError(w, status, msg)
		return
	}

	snap, err := s.repo.GetRoomSnapshot(r.Context(), roomID)
	if err != nil {
		status, msg := mapDomainErr(err)
		writeError(w, status, msg)
		return
	}

	s.broadcastSnapshot(r.Context(), roomID)
	writeJSON(w, http.StatusOK, snap)
}

func (s *Server) handlePlaybackPause(w http.ResponseWriter, r *http.Request) {
	roomID := roomIDParam(r)

	type reqBody struct {
		Paused bool `json:"paused"`
	}
	var body reqBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	if err := s.repo.TogglePauseSafe(r.Context(), roomID, userSub(r), body.Paused); err != nil {
		status, msg := mapDomainErr(err)
		writeError(w, status, msg)
		return
	}

	snap, err := s.repo.GetRoomSnapshot(r.Context(), roomID)
	if err != nil {
		status, msg := mapDomainErr(err)
		writeError(w, status, msg)
		return
	}

	s.broadcastSnapshot(r.Context(), roomID)
	writeJSON(w, http.StatusOK, snap)
}

func (s *Server) handlePlaybackSeek(w http.ResponseWriter, r *http.Request) {
	roomID := roomIDParam(r)

	type reqBody struct {
		PositionMS int `json:"positionMs"`
	}
	var body reqBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	if err := s.repo.Seek(r.Context(), roomID, userSub(r), body.PositionMS); err != nil {
		status, msg := mapDomainErr(err)
		writeError(w, status, msg)
		return
	}

	snap, err := s.repo.GetRoomSnapshot(r.Context(), roomID)
	if err != nil {
		status, msg := mapDomainErr(err)
		writeError(w, status, msg)
		return
	}

	s.broadcastSnapshot(r.Context(), roomID)
	writeJSON(w, http.StatusOK, snap)
}

// =============================
// REST handlers: Player actions
// =============================

func (s *Server) handleBuzz(w http.ResponseWriter, r *http.Request) {
	roomID := roomIDParam(r)

	type reqBody struct {
		PlayerID string `json:"playerId"`
	}
	var body reqBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	// We don't persist buzzes in DB yet; just broadcast the event.
	if s.rt != nil {
		// Best-effort include player info by fetching snapshot and matching playerId.
		snap, err := s.repo.GetRoomSnapshot(r.Context(), roomID)
		if err == nil {
			var pv *game.PlayerView
			for i := range snap.Players {
				if snap.Players[i].PlayerID == strings.TrimSpace(body.PlayerID) {
					pv = &snap.Players[i]
					break
				}
			}
			s.rt.Room(roomID).Broadcast(realtime.Event{
				Type:   "buzzer",
				RoomID: roomID,
				Payload: map[string]any{
					"player": pv,
				},
			})
		} else {
			s.rt.Room(roomID).Broadcast(realtime.Event{
				Type:   "buzzer",
				RoomID: roomID,
				Payload: map[string]any{
					"playerId": strings.TrimSpace(body.PlayerID),
				},
			})
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// =============================
// REST handlers: Profile / account
// =============================

func (s *Server) handleGetMe(w http.ResponseWriter, r *http.Request) {
	sub := userSub(r)
	p, err := s.repo.GetProfile(r.Context(), sub)
	if err != nil {
		status, msg := mapDomainErr(err)
		writeError(w, status, msg)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handlePutMe(w http.ResponseWriter, r *http.Request) {
	sub := userSub(r)

	type reqBody struct {
		Nickname   string `json:"nickname"`
		PictureURL string `json:"pictureUrl"`
	}
	var body reqBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	p, err := s.repo.UpsertProfile(r.Context(), sub, strings.TrimSpace(body.Nickname), strings.TrimSpace(body.PictureURL))
	if err != nil {
		status, msg := mapDomainErr(err)
		writeError(w, status, msg)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleDeleteMe(w http.ResponseWriter, r *http.Request) {
	sub := userSub(r)

	if err := s.repo.DeleteAccount(r.Context(), sub); err != nil {
		status, msg := mapDomainErr(err)
		writeError(w, status, msg)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// =============================
// REST handlers: Playlists
// =============================

func (s *Server) handleListMyPlaylists(w http.ResponseWriter, r *http.Request) {
	sub := userSub(r)

	pls, err := s.repo.ListPlaylists(r.Context(), sub)
	if err != nil {
		status, msg := mapDomainErr(err)
		writeError(w, status, msg)
		return
	}

	// For the UI, include items as well (so it can show tracks).
	// This is N+1; acceptable for now. If needed, add a "list playlists with items" query.
	out := make([]game.Playlist, 0, len(pls))
	for _, pl := range pls {
		full, err := s.repo.GetPlaylist(r.Context(), sub, pl.ID)
		if err != nil {
			// If a playlist disappeared between list and get, just skip it.
			continue
		}
		out = append(out, full)
	}

	writeJSON(w, http.StatusOK, map[string]any{"playlists": out})
}

func (s *Server) handleCreateMyPlaylist(w http.ResponseWriter, r *http.Request) {
	sub := userSub(r)

	type reqBody struct {
		Name string `json:"name"`
	}
	var body reqBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	pl, err := s.repo.CreatePlaylist(r.Context(), sub, strings.TrimSpace(body.Name))
	if err != nil {
		status, msg := mapDomainErr(err)
		writeError(w, status, msg)
		return
	}

	writeJSON(w, http.StatusCreated, pl)
}

func (s *Server) handlePatchMyPlaylist(w http.ResponseWriter, r *http.Request) {
	sub := userSub(r)
	playlistID := playlistIDParam(r)

	type reqBody struct {
		Name *string `json:"name,omitempty"`
	}
	var body reqBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	if body.Name == nil {
		writeError(w, http.StatusBadRequest, "invalid input")
		return
	}

	pl, err := s.repo.UpdatePlaylistName(r.Context(), sub, playlistID, strings.TrimSpace(*body.Name))
	if err != nil {
		status, msg := mapDomainErr(err)
		writeError(w, status, msg)
		return
	}

	writeJSON(w, http.StatusOK, pl)
}

func (s *Server) handleAddMyPlaylistItem(w http.ResponseWriter, r *http.Request) {
	sub := userSub(r)
	playlistID := playlistIDParam(r)

	type reqBody struct {
		Title      string `json:"title"`
		YouTubeURL string `json:"youtubeUrl"`
	}
	var body reqBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	item, pl, err := s.repo.AddPlaylistItem(r.Context(), sub, playlistID, strings.TrimSpace(body.Title), strings.TrimSpace(body.YouTubeURL))
	if err != nil {
		status, msg := mapDomainErr(err)
		writeError(w, status, msg)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"item":     item,
		"playlist": pl,
	})
}

// =============================
// WebSocket: room events
// =============================

func (s *Server) handleRoomWS(w http.ResponseWriter, r *http.Request) {
	roomID := roomIDParam(r)
	if roomID == "" {
		writeError(w, http.StatusBadRequest, "missing roomId")
		return
	}

	if s.rt == nil {
		writeError(w, http.StatusInternalServerError, "realtime not configured")
		return
	}

	events, cancel := s.rt.Room(roomID).Subscribe(256)
	defer cancel()

	originPatterns := []string{
		"http://localhost:5173",
		"http://127.0.0.1:5173",
	}
	if v := r.Context().Value(wsOriginPatternsCtxKey{}); v != nil {
		if patterns, ok := v.([]string); ok && len(patterns) > 0 {
			originPatterns = patterns
		}
	}

	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: originPatterns,
	})
	if err != nil {
		// Make failures visible (otherwise the browser just shows "WebSocket connection failed").
		// This log should help identify issues like:
		// - missing/invalid Upgrade headers
		// - rejected Origin
		// - intermediary/proxy not forwarding WS upgrades
		log.Printf(
			"ws accept failed: roomId=%s remote=%s origin=%q conn=%q upgrade=%q sec-websocket-key=%q err=%v",
			roomID,
			r.RemoteAddr,
			r.Header.Get("Origin"),
			r.Header.Get("Connection"),
			r.Header.Get("Upgrade"),
			r.Header.Get("Sec-WebSocket-Key"),
			err,
		)

		// Best-effort HTTP error for non-upgrade requests (some failures will already have a response written by Accept).
		// Use 400 here so it's obvious it's the WS handshake that failed, not a missing route.
		http.Error(w, "websocket upgrade failed", http.StatusBadRequest)
		return
	}
	defer func() { _ = c.Close(websocket.StatusNormalClosure, "bye") }()

	// Send initial snapshot.
	snap, err := s.repo.GetRoomSnapshot(r.Context(), roomID)
	if err != nil {
		status, msg := mapDomainErr(err)
		_ = c.Close(websocket.StatusPolicyViolation, msg)
		_ = status
		return
	}

	if err := wsWriteJSON(r.Context(), c, realtime.Event{
		Type:    "room.snapshot",
		RoomID:  roomID,
		Payload: snap,
	}); err != nil {
		return
	}

	// Reader: drain to detect close/pings.
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		for {
			_, _, err := c.Read(r.Context())
			if err != nil {
				return
			}
		}
	}()

	// Writer loop: forward events.
	for {
		select {
		case <-r.Context().Done():
			_ = c.Close(websocket.StatusNormalClosure, "context done")
			return
		case <-readDone:
			return
		case ev, ok := <-events:
			if !ok {
				return
			}
			// Ensure roomId is set.
			if ev.RoomID == "" {
				ev.RoomID = roomID
			}
			if err := wsWriteJSON(r.Context(), c, ev); err != nil {
				return
			}
		}
	}
}

func wsWriteJSON(ctx context.Context, c *websocket.Conn, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	wctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return c.Write(wctx, websocket.MessageText, b)
}

// isJSONEOF treats empty request body as ok for optional bodies.
func isJSONEOF(err error) bool {
	if err == nil {
		return false
	}
	// encoding/json uses io.EOF; avoid importing io here by checking string.
	// This is adequate for our server-side usage.
	return strings.Contains(err.Error(), "EOF")
}
