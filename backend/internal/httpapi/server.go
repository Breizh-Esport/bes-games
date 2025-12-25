package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"

	"github.com/valentin/bes-games/backend/internal/core"
	"github.com/valentin/bes-games/backend/internal/games"
	"github.com/valentin/bes-games/backend/internal/games/namethattune"
	"github.com/valentin/bes-games/backend/internal/realtime"
)

const playbackSyncLead = 1500 * time.Millisecond

// Server provides the HTTP API (REST + WebSocket) for bes-games.
//
// Auth model (still temporary until real OIDC validation is implemented):
//   - Authenticated requests must include:
//     X-User-Sub: <oidc subject>
//   - Anonymous users can join rooms without it.
//
// Persistence model:
//   - Core user profile is stored in Postgres via core.Repo.
//   - Game state (rooms, players, playback, playlists) is stored in Postgres via a game repo.
//   - Realtime updates are fanned out via realtime.Registry (in-memory pub/sub), while the source of truth is DB.
//
// Endpoints (summary):
// - GET    /healthz
// - GET    /api/games
//
// Rooms (per-game):
// - GET    /api/games/{gameId}/rooms
// - POST   /api/games/{gameId}/rooms                          (auth required)
// - GET    /api/games/{gameId}/rooms/{roomId}
// - POST   /api/games/{gameId}/rooms/{roomId}/join            (anon allowed)
// - POST   /api/games/{gameId}/rooms/{roomId}/leave
// - WS     /api/games/{gameId}/rooms/{roomId}/ws
//
// Owner controls (auth required; must be room owner) (per-game):
//
// Profile (auth required):
// - GET    /api/me
// - PUT    /api/me
// - DELETE /api/me
//
// Playlists (per-game, auth required):
// - GET    /api/games/{gameId}/playlists
// - POST   /api/games/{gameId}/playlists
// - PATCH  /api/games/{gameId}/playlists/{playlistId}
// - POST   /api/games/{gameId}/playlists/{playlistId}/items
// - PATCH  /api/games/{gameId}/playlists/{playlistId}/items/{itemId}
// - DELETE /api/games/{gameId}/playlists/{playlistId}/items/{itemId}
//
// Player actions (per-game):
type Server struct {
	coreRepo *core.Repo
	nttRepo  *namethattune.Repo
	rt       *realtime.Registry
	rooms    *roomLifecycle
	buzzMu   sync.Mutex
	buzzCD   map[string]map[string]time.Time
	tokenMu      sync.Mutex
	playerTokens map[string]map[string]string
	ownerTokens  map[string]string
	playbackMu        sync.Mutex
	playbackBuffering map[string]map[string]bool
	playbackReady     map[string]map[string]bool
	playbackPending   map[string]bool
	playbackStartAt   map[string]time.Time
	playbackAutoPause map[string]bool
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

func NewServer(coreRepo *core.Repo, nttRepo *namethattune.Repo, rt *realtime.Registry) *Server {
	return &Server{
		coreRepo: coreRepo,
		nttRepo:  nttRepo,
		rt:       rt,
		rooms:    newRoomLifecycle(nttRepo, rt),
		buzzCD:   make(map[string]map[string]time.Time),
		playerTokens: make(map[string]map[string]string),
		ownerTokens:  make(map[string]string),
		playbackBuffering: make(map[string]map[string]bool),
		playbackReady:     make(map[string]map[string]bool),
		playbackPending:   make(map[string]bool),
		playbackStartAt:   make(map[string]time.Time),
		playbackAutoPause: make(map[string]bool),
	}
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
		api.Get("/games", s.handleListGames)

		// Canonical per-game routes (prepared for multiple games).
		api.Route("/games/name-that-tune", func(ntt chi.Router) {
			ntt.Get("/rooms", s.handleListRooms)
			ntt.Post("/rooms", s.requireAuth(s.handleCreateRoom))

			ntt.Route("/rooms/{roomId}", func(rr chi.Router) {
				rr.Get("/", s.handleGetRoom)
				rr.Post("/join", s.handleJoinRoom)
				rr.Post("/leave", s.handleLeaveRoom)

				rr.Get("/ws", s.handleRoomWS)
			})

			// Game-specific playlists (owned by the authenticated user).
			ntt.Get("/playlists", s.requireAuth(s.handleListPlaylists))
			ntt.Post("/playlists", s.requireAuth(s.handleCreatePlaylist))
			ntt.Patch("/playlists/{playlistId}", s.requireAuth(s.handlePatchPlaylist))
			ntt.Post("/playlists/{playlistId}/items", s.requireAuth(s.handleAddPlaylistItem))
			ntt.Patch("/playlists/{playlistId}/items/{itemId}", s.requireAuth(s.handlePatchPlaylistItem))
			ntt.Delete("/playlists/{playlistId}/items/{itemId}", s.requireAuth(s.handleDeletePlaylistItem))
		})

		// Profile / account
		api.Get("/me", s.requireAuth(s.handleGetMe))
		api.Put("/me", s.requireAuth(s.handlePutMe))
		api.Delete("/me", s.requireAuth(s.handleDeleteMe))
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

func playlistItemIDParam(r *http.Request) string {
	return strings.TrimSpace(chi.URLParam(r, "itemId"))
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

type apiError struct {
	Status  int
	Message string
}

func (e *apiError) Error() string {
	return e.Message
}

func mapAPIError(err error) (int, string) {
	if err == nil {
		return http.StatusOK, ""
	}
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		return apiErr.Status, apiErr.Message
	}
	return http.StatusInternalServerError, "internal server error"
}

func randomToken() string {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}

func (s *Server) getOrCreatePlayerToken(roomID, playerID string) string {
	s.tokenMu.Lock()
	defer s.tokenMu.Unlock()
	room := s.playerTokens[roomID]
	if room == nil {
		room = make(map[string]string)
		s.playerTokens[roomID] = room
	}
	if token, ok := room[playerID]; ok {
		return token
	}
	token := randomToken()
	room[playerID] = token
	return token
}

func (s *Server) clearPlayerToken(roomID, playerID string) {
	s.tokenMu.Lock()
	defer s.tokenMu.Unlock()
	room := s.playerTokens[roomID]
	if room == nil {
		return
	}
	delete(room, playerID)
	if len(room) == 0 {
		delete(s.playerTokens, roomID)
	}
}

func (s *Server) validatePlayerToken(roomID, playerID, token string) bool {
	if token == "" {
		return false
	}
	s.tokenMu.Lock()
	defer s.tokenMu.Unlock()
	room := s.playerTokens[roomID]
	if room == nil {
		return false
	}
	return room[playerID] == token
}

func (s *Server) getOrCreateOwnerToken(roomID string) string {
	s.tokenMu.Lock()
	defer s.tokenMu.Unlock()
	if token, ok := s.ownerTokens[roomID]; ok {
		return token
	}
	token := randomToken()
	s.ownerTokens[roomID] = token
	return token
}

func (s *Server) validateOwnerToken(roomID, token string) bool {
	if token == "" {
		return false
	}
	s.tokenMu.Lock()
	defer s.tokenMu.Unlock()
	return s.ownerTokens[roomID] == token
}

func (s *Server) buzzCooldownUntil(roomID, playerID string) (time.Time, bool) {
	s.buzzMu.Lock()
	defer s.buzzMu.Unlock()

	room := s.buzzCD[roomID]
	if room == nil {
		return time.Time{}, false
	}
	until, ok := room[playerID]
	return until, ok
}

func (s *Server) setBuzzCooldown(roomID, playerID string, until time.Time) {
	s.buzzMu.Lock()
	defer s.buzzMu.Unlock()

	room := s.buzzCD[roomID]
	if room == nil {
		room = make(map[string]time.Time)
		s.buzzCD[roomID] = room
	}
	room[playerID] = until
}

func (s *Server) clearBuzzCooldown(roomID, playerID string) {
	s.buzzMu.Lock()
	defer s.buzzMu.Unlock()

	room := s.buzzCD[roomID]
	if room == nil {
		return
	}
	delete(room, playerID)
	if len(room) == 0 {
		delete(s.buzzCD, roomID)
	}
}

func (s *Server) setPlaybackBuffering(roomID, playerID string, buffering bool) bool {
	s.playbackMu.Lock()
	defer s.playbackMu.Unlock()
	room := s.playbackBuffering[roomID]
	if room == nil {
		room = make(map[string]bool)
		s.playbackBuffering[roomID] = room
	}
	prev, ok := room[playerID]
	if !buffering {
		if ok {
			delete(room, playerID)
		}
		if len(room) == 0 {
			delete(s.playbackBuffering, roomID)
		}
		return ok
	}
	room[playerID] = true
	return !ok || !prev
}

func (s *Server) bufferingPlayers(roomID string, players []namethattune.PlayerView) []string {
	s.playbackMu.Lock()
	defer s.playbackMu.Unlock()
	room := s.playbackBuffering[roomID]
	if len(room) == 0 {
		return nil
	}
	connected := make(map[string]bool, len(players))
	for _, p := range players {
		if p.Connected {
			connected[p.PlayerID] = true
		}
	}
	out := make([]string, 0, len(room))
	for pid := range room {
		if connected[pid] {
			out = append(out, pid)
		}
	}
	if len(out) == 0 {
		delete(s.playbackBuffering, roomID)
	}
	return out
}

func (s *Server) setPlaybackReady(roomID, playerID string, ready bool) {
	s.playbackMu.Lock()
	defer s.playbackMu.Unlock()
	room := s.playbackReady[roomID]
	if room == nil {
		room = make(map[string]bool)
		s.playbackReady[roomID] = room
	}
	if ready {
		room[playerID] = true
		return
	}
	delete(room, playerID)
	if len(room) == 0 {
		delete(s.playbackReady, roomID)
	}
}

func (s *Server) allPlayersReady(roomID string, players []namethattune.PlayerView) bool {
	s.playbackMu.Lock()
	defer s.playbackMu.Unlock()
	room := s.playbackReady[roomID]
	for _, p := range players {
		if !p.Connected {
			continue
		}
		if room == nil || !room[p.PlayerID] {
			return false
		}
	}
	return true
}

func (s *Server) setPlaybackPending(roomID string, pending bool) {
	s.playbackMu.Lock()
	defer s.playbackMu.Unlock()
	if pending {
		s.playbackPending[roomID] = true
		return
	}
	delete(s.playbackPending, roomID)
}

func (s *Server) isPlaybackPending(roomID string) bool {
	s.playbackMu.Lock()
	defer s.playbackMu.Unlock()
	return s.playbackPending[roomID]
}

func (s *Server) setPlaybackStartAt(roomID string, startAt time.Time) {
	s.playbackMu.Lock()
	defer s.playbackMu.Unlock()
	s.playbackStartAt[roomID] = startAt
}

func (s *Server) getPlaybackStartAt(roomID string) (time.Time, bool) {
	s.playbackMu.Lock()
	defer s.playbackMu.Unlock()
	startAt, ok := s.playbackStartAt[roomID]
	return startAt, ok
}

func (s *Server) clearPlaybackStartAt(roomID string) {
	s.playbackMu.Lock()
	defer s.playbackMu.Unlock()
	delete(s.playbackStartAt, roomID)
}

func (s *Server) setPlaybackAutoPause(roomID string, paused bool) {
	s.playbackMu.Lock()
	defer s.playbackMu.Unlock()
	if paused {
		s.playbackAutoPause[roomID] = true
		return
	}
	delete(s.playbackAutoPause, roomID)
}

func (s *Server) isPlaybackAutoPaused(roomID string) bool {
	s.playbackMu.Lock()
	defer s.playbackMu.Unlock()
	return s.playbackAutoPause[roomID]
}

func (s *Server) clearPlaybackState(roomID string) {
	s.playbackMu.Lock()
	defer s.playbackMu.Unlock()
	delete(s.playbackBuffering, roomID)
	delete(s.playbackReady, roomID)
	delete(s.playbackPending, roomID)
	delete(s.playbackStartAt, roomID)
	delete(s.playbackAutoPause, roomID)
}

// =============================
// Room actions (shared by REST and WS)
// =============================

func (s *Server) doKick(ctx context.Context, roomID, sub, playerID string) (namethattune.RoomSnapshot, error) {
	if err := s.nttRepo.KickPlayer(ctx, roomID, sub, strings.TrimSpace(playerID)); err != nil {
		status, msg := mapDomainErr(err)
		return namethattune.RoomSnapshot{}, &apiError{Status: status, Message: msg}
	}

	snap, err := s.loadRoomSnapshot(ctx, roomID)
	if err != nil {
		status, msg := mapDomainErr(err)
		return namethattune.RoomSnapshot{}, &apiError{Status: status, Message: msg}
	}

	s.broadcastSnapshot(ctx, roomID)
	return snap, nil
}

func (s *Server) doScoreSet(ctx context.Context, roomID, sub, playerID string, score int) (namethattune.RoomSnapshot, error) {
	if err := s.nttRepo.SetScore(ctx, roomID, sub, strings.TrimSpace(playerID), score); err != nil {
		status, msg := mapDomainErr(err)
		return namethattune.RoomSnapshot{}, &apiError{Status: status, Message: msg}
	}

	snap, err := s.loadRoomSnapshot(ctx, roomID)
	if err != nil {
		status, msg := mapDomainErr(err)
		return namethattune.RoomSnapshot{}, &apiError{Status: status, Message: msg}
	}

	s.broadcastSnapshot(ctx, roomID)
	return snap, nil
}

func (s *Server) doScoreAdd(ctx context.Context, roomID, sub, playerID string, delta int) (namethattune.RoomSnapshot, error) {
	if err := s.nttRepo.AddScore(ctx, roomID, sub, strings.TrimSpace(playerID), delta); err != nil {
		status, msg := mapDomainErr(err)
		return namethattune.RoomSnapshot{}, &apiError{Status: status, Message: msg}
	}

	snap, err := s.loadRoomSnapshot(ctx, roomID)
	if err != nil {
		status, msg := mapDomainErr(err)
		return namethattune.RoomSnapshot{}, &apiError{Status: status, Message: msg}
	}

	s.broadcastSnapshot(ctx, roomID)
	return snap, nil
}

func (s *Server) doLoadPlaylist(ctx context.Context, roomID, sub, playlistID string) (namethattune.RoomSnapshot, error) {
	if err := s.nttRepo.LoadPlaylistToRoom(ctx, roomID, sub, strings.TrimSpace(playlistID)); err != nil {
		status, msg := mapDomainErr(err)
		return namethattune.RoomSnapshot{}, &apiError{Status: status, Message: msg}
	}

	s.clearPlaybackState(roomID)
	snap, err := s.loadRoomSnapshot(ctx, roomID)
	if err != nil {
		status, msg := mapDomainErr(err)
		return namethattune.RoomSnapshot{}, &apiError{Status: status, Message: msg}
	}

	s.broadcastSnapshot(ctx, roomID)
	return snap, nil
}

func (s *Server) doPlaybackSet(ctx context.Context, roomID, sub string, trackIndex int, paused *bool, positionMS *int) (namethattune.RoomSnapshot, error) {
	if err := s.nttRepo.SetPlayback(ctx, roomID, sub, trackIndex, paused, positionMS); err != nil {
		status, msg := mapDomainErr(err)
		return namethattune.RoomSnapshot{}, &apiError{Status: status, Message: msg}
	}

	s.clearPlaybackState(roomID)
	snap, err := s.loadRoomSnapshot(ctx, roomID)
	if err != nil {
		status, msg := mapDomainErr(err)
		return namethattune.RoomSnapshot{}, &apiError{Status: status, Message: msg}
	}

	s.broadcastSnapshot(ctx, roomID)
	return snap, nil
}

func (s *Server) doPlaybackPause(ctx context.Context, roomID, sub string, paused bool) (namethattune.RoomSnapshot, error) {
	if paused {
		if err := s.nttRepo.PausePlaybackWithPosition(ctx, roomID); err != nil {
			status, msg := mapDomainErr(err)
			return namethattune.RoomSnapshot{}, &apiError{Status: status, Message: msg}
		}
		s.setPlaybackPending(roomID, false)
		s.clearPlaybackStartAt(roomID)
		s.setPlaybackAutoPause(roomID, false)

		snap, err := s.loadRoomSnapshot(ctx, roomID)
		if err != nil {
			status, msg := mapDomainErr(err)
			return namethattune.RoomSnapshot{}, &apiError{Status: status, Message: msg}
		}

		s.broadcastSnapshot(ctx, roomID)
		return snap, nil
	}

	snap, err := s.loadRoomSnapshot(ctx, roomID)
	if err != nil {
		status, msg := mapDomainErr(err)
		return namethattune.RoomSnapshot{}, &apiError{Status: status, Message: msg}
	}

	if !s.allPlayersReady(roomID, snap.Players) {
		s.setPlaybackPending(roomID, true)
		s.clearPlaybackStartAt(roomID)
		s.setPlaybackAutoPause(roomID, false)
		s.decorateSnapshot(roomID, &snap)
		s.broadcastSnapshot(ctx, roomID)
		return snap, nil
	}

	if err := s.nttRepo.TogglePauseSafe(ctx, roomID, sub, false); err != nil {
		status, msg := mapDomainErr(err)
		return namethattune.RoomSnapshot{}, &apiError{Status: status, Message: msg}
	}

	startAt := time.Now().UTC().Add(playbackSyncLead)
	s.setPlaybackStartAt(roomID, startAt)
	s.setPlaybackPending(roomID, false)
	s.setPlaybackAutoPause(roomID, false)

	snap, err = s.loadRoomSnapshot(ctx, roomID)
	if err != nil {
		status, msg := mapDomainErr(err)
		return namethattune.RoomSnapshot{}, &apiError{Status: status, Message: msg}
	}

	s.broadcastSnapshot(ctx, roomID)
	return snap, nil
}

func (s *Server) doPlaybackSeek(ctx context.Context, roomID, sub string, positionMS int) (namethattune.RoomSnapshot, error) {
	if err := s.nttRepo.Seek(ctx, roomID, sub, positionMS); err != nil {
		status, msg := mapDomainErr(err)
		return namethattune.RoomSnapshot{}, &apiError{Status: status, Message: msg}
	}

	s.setPlaybackPending(roomID, false)
	s.clearPlaybackStartAt(roomID)
	snap, err := s.loadRoomSnapshot(ctx, roomID)
	if err != nil {
		status, msg := mapDomainErr(err)
		return namethattune.RoomSnapshot{}, &apiError{Status: status, Message: msg}
	}

	s.broadcastSnapshot(ctx, roomID)
	return snap, nil
}

func (s *Server) doPlaybackBuffering(ctx context.Context, roomID, playerID string, buffering bool) (namethattune.RoomSnapshot, error) {
	playerID = strings.TrimSpace(playerID)
	if playerID == "" {
		return namethattune.RoomSnapshot{}, &apiError{Status: http.StatusBadRequest, Message: "invalid input"}
	}

	snap, err := s.loadRoomSnapshot(ctx, roomID)
	if err != nil {
		status, msg := mapDomainErr(err)
		return namethattune.RoomSnapshot{}, &apiError{Status: status, Message: msg}
	}

	var player *namethattune.PlayerView
	for i := range snap.Players {
		if snap.Players[i].PlayerID == playerID {
			player = &snap.Players[i]
			break
		}
	}
	if player == nil {
		return namethattune.RoomSnapshot{}, &apiError{Status: http.StatusNotFound, Message: "player not found"}
	}
	if !player.Connected {
		return snap, nil
	}

	s.setPlaybackBuffering(roomID, playerID, buffering)
	s.setPlaybackReady(roomID, playerID, !buffering)

	now := time.Now().UTC()
	startAt := snap.Playback.StartAt
	waitingToStart := startAt != nil && startAt.After(now)

	if buffering {
		if !snap.Playback.Paused && !waitingToStart {
			if err := s.nttRepo.PausePlaybackWithPosition(ctx, roomID); err != nil {
				status, msg := mapDomainErr(err)
				return namethattune.RoomSnapshot{}, &apiError{Status: status, Message: msg}
			}
			s.setPlaybackAutoPause(roomID, true)
			s.setPlaybackPending(roomID, true)
		} else if waitingToStart || s.isPlaybackPending(roomID) {
			s.setPlaybackPending(roomID, true)
		}
		if waitingToStart {
			s.clearPlaybackStartAt(roomID)
		}
	} else {
		bufferingPlayers := s.bufferingPlayers(roomID, snap.Players)
		if len(bufferingPlayers) == 0 && s.isPlaybackPending(roomID) && s.allPlayersReady(roomID, snap.Players) {
			if err := s.nttRepo.TogglePauseSafe(ctx, roomID, snap.OwnerSub, false); err != nil {
				status, msg := mapDomainErr(err)
				return namethattune.RoomSnapshot{}, &apiError{Status: status, Message: msg}
			}
			s.setPlaybackAutoPause(roomID, false)
			s.setPlaybackPending(roomID, false)
			s.setPlaybackStartAt(roomID, time.Now().UTC().Add(playbackSyncLead))
		}
	}

	snap, err = s.loadRoomSnapshot(ctx, roomID)
	if err != nil {
		status, msg := mapDomainErr(err)
		return namethattune.RoomSnapshot{}, &apiError{Status: status, Message: msg}
	}

	s.broadcastSnapshot(ctx, roomID)
	return snap, nil
}

func (s *Server) doBuzz(ctx context.Context, roomID, playerID string) error {
	playerID = strings.TrimSpace(playerID)
	if until, ok := s.buzzCooldownUntil(roomID, playerID); ok && until.After(time.Now().UTC()) {
		return &apiError{Status: http.StatusBadRequest, Message: "buzz cooldown active"}
	}

	player, err := s.nttRepo.HandleBuzz(ctx, roomID, playerID)
	if err != nil {
		status, msg := mapDomainErr(err)
		return &apiError{Status: status, Message: msg}
	}

	if s.rt != nil {
		s.rt.Room(roomID).Broadcast(realtime.Event{
			Type:   "buzzer",
			RoomID: roomID,
			Payload: map[string]any{
				"player": player,
			},
		})
	}

	s.broadcastSnapshot(ctx, roomID)
	return nil
}

func (s *Server) doBuzzResolve(ctx context.Context, roomID, sub, playerID string, correct bool) error {
	playerID = strings.TrimSpace(playerID)
	if playerID == "" {
		return &apiError{Status: http.StatusBadRequest, Message: "invalid input"}
	}

	var cooldownUntil string
	if correct {
		s.clearBuzzCooldown(roomID, playerID)
		if err := s.nttRepo.AddScore(ctx, roomID, sub, playerID, 1); err != nil {
			status, msg := mapDomainErr(err)
			return &apiError{Status: status, Message: msg}
		}

		snap, err := s.loadRoomSnapshot(ctx, roomID)
		if err != nil {
			status, msg := mapDomainErr(err)
			return &apiError{Status: status, Message: msg}
		}
		if snap.Playlist != nil && len(snap.Playlist.Items) > 0 {
			nextIndex := snap.Playback.TrackIndex + 1
			max := (len(snap.Playlist.Items) - 1)
			if nextIndex > max {
				nextIndex = max
			}
			paused := true
			position := 0
			if err := s.nttRepo.SetPlayback(ctx, roomID, sub, nextIndex, &paused, &position); err != nil {
				status, msg := mapDomainErr(err)
				return &apiError{Status: status, Message: msg}
			}
			s.clearPlaybackState(roomID)
		} else {
			paused := true
			if err := s.nttRepo.TogglePauseSafe(ctx, roomID, sub, paused); err != nil {
				status, msg := mapDomainErr(err)
				return &apiError{Status: status, Message: msg}
			}
			s.clearPlaybackStartAt(roomID)
			s.setPlaybackPending(roomID, false)
			s.setPlaybackAutoPause(roomID, false)
		}
	} else {
		const cooldown = 5 * time.Second
		until := time.Now().UTC().Add(cooldown)
		s.setBuzzCooldown(roomID, playerID, until)
		cooldownUntil = until.Format(time.RFC3339Nano)
		paused := false
		if err := s.nttRepo.TogglePauseSafe(ctx, roomID, sub, paused); err != nil {
			status, msg := mapDomainErr(err)
			return &apiError{Status: status, Message: msg}
		}
		if !paused {
			s.setPlaybackStartAt(roomID, time.Now().UTC().Add(playbackSyncLead))
			s.setPlaybackPending(roomID, false)
			s.setPlaybackAutoPause(roomID, false)
		}
	}

	if s.rt != nil {
		s.rt.Room(roomID).Broadcast(realtime.Event{
			Type:   "buzzer.resolved",
			RoomID: roomID,
			Payload: map[string]any{
				"playerId": playerID,
				"correct":  correct,
			},
		})
		if !correct {
			s.rt.Room(roomID).Broadcast(realtime.Event{
				Type:   "buzzer.cooldown",
				RoomID: roomID,
				Payload: map[string]any{
					"playerId": playerID,
					"until":    cooldownUntil,
				},
			})
		}
	}

	s.broadcastSnapshot(ctx, roomID)
	return nil
}

func (s *Server) decorateSnapshot(roomID string, snap *namethattune.RoomSnapshot) {
	if snap == nil {
		return
	}
	if startAt, ok := s.getPlaybackStartAt(roomID); ok {
		t := startAt
		snap.Playback.StartAt = &t
	}
	buffering := s.bufferingPlayers(roomID, snap.Players)
	if len(buffering) > 0 {
		snap.Playback.BufferingPlayers = buffering
	}
	snap.Playback.WaitingForBuffer = s.isPlaybackPending(roomID) || len(buffering) > 0
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
	case errors.Is(err, core.ErrUnauthorized):
		return http.StatusUnauthorized, err.Error()
	case errors.Is(err, core.ErrNotOwner):
		return http.StatusForbidden, err.Error()
	case errors.Is(err, core.ErrRoomNotFound):
		return http.StatusNotFound, err.Error()
	case errors.Is(err, core.ErrPlayerNotFound):
		return http.StatusNotFound, err.Error()
	case errors.Is(err, namethattune.ErrPlaylistNotFound):
		return http.StatusNotFound, err.Error()
	case errors.Is(err, core.ErrInvalidInput):
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
	snap, err := s.loadRoomSnapshot(ctx, roomID)
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

func (s *Server) loadRoomSnapshot(ctx context.Context, roomID string) (namethattune.RoomSnapshot, error) {
	snap, err := s.nttRepo.GetRoomSnapshot(ctx, roomID)
	if err != nil {
		return snap, err
	}
	s.decorateSnapshot(roomID, &snap)
	return snap, nil
}

// =============================
// REST handlers: Rooms
// =============================

func (s *Server) handleListRooms(w http.ResponseWriter, r *http.Request) {
	rooms, err := s.nttRepo.ListRooms(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	// Map to the same JSON shape used previously.
	type roomInfo struct {
		RoomID        string    `json:"roomId"`
		Name          string    `json:"name"`
		OwnerSub      string    `json:"ownerSub,omitempty"`
		Visibility    string    `json:"visibility"`
		HasPassword   bool      `json:"hasPassword"`
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
			Visibility:    ri.Visibility,
			HasPassword:   ri.HasPassword,
			OnlinePlayers: ri.OnlinePlayers,
			Subscribers:   subs,
			UpdatedAt:     ri.UpdatedAt,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"rooms": out})
}

func (s *Server) handleListGames(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"games": games.List(),
	})
}

func (s *Server) handleCreateRoom(w http.ResponseWriter, r *http.Request) {
	type reqBody struct {
		Name       string `json:"name"`
		PlaylistID string `json:"playlistId"`
		Visibility string `json:"visibility"`
		Password   string `json:"password"`
	}
	var body reqBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	visibility := strings.TrimSpace(body.Visibility)
	if visibility == "" {
		visibility = "public"
	}
	if visibility != "public" && visibility != "private" {
		writeError(w, http.StatusBadRequest, "invalid room visibility")
		return
	}

	roomID, err := s.nttRepo.CreateRoom(
		r.Context(),
		userSub(r),
		strings.TrimSpace(body.Name),
		strings.TrimSpace(body.PlaylistID),
		visibility,
		strings.TrimSpace(body.Password),
	)
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
	snap, err := s.loadRoomSnapshot(r.Context(), roomID)
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
		Password   string `json:"password,omitempty"`
	}
	var body reqBody
	// Optional body. If empty, decodeJSON may return EOF; treat as ok.
	if err := decodeJSON(r, &body); err != nil && !isJSONEOF(err) {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	joinRes, err := s.nttRepo.JoinRoom(
		r.Context(),
		roomID,
		userSub(r),
		strings.TrimSpace(body.Nickname),
		strings.TrimSpace(body.PictureURL),
		strings.TrimSpace(body.Password),
	)
	if err != nil {
		status, msg := mapDomainErr(err)
		writeError(w, status, msg)
		return
	}

	snap, err := s.loadRoomSnapshot(r.Context(), roomID)
	if err != nil {
		status, msg := mapDomainErr(err)
		writeError(w, status, msg)
		return
	}

	// Owner came back online: cancel pending shutdown.
	if joinRes.IsOwner {
		s.rooms.cancelOwnerTimeout(roomID)
	}

	// Broadcast snapshot for all listeners.
	s.broadcastSnapshot(r.Context(), roomID)

	playerToken := s.getOrCreatePlayerToken(roomID, joinRes.PlayerID)
	ownerToken := ""
	if joinRes.IsOwner {
		ownerToken = s.getOrCreateOwnerToken(roomID)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"playerId": joinRes.PlayerID,
		"playerToken": playerToken,
		"ownerToken":  ownerToken,
		"owner": map[string]any{
			"playerId": joinRes.OwnerPlayerID,
			"online":   joinRes.OwnerConnected,
		},
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

	leaveRes, err := s.nttRepo.LeaveRoom(r.Context(), roomID, strings.TrimSpace(body.PlayerID))
	if err != nil {
		status, msg := mapDomainErr(err)
		writeError(w, status, msg)
		return
	}
	s.clearPlayerToken(roomID, strings.TrimSpace(body.PlayerID))

	closedReason := ""
	if leaveRes.OwnerLeft && leaveRes.ConnectedAfter == 0 {
		_ = s.rooms.closeRoom(r.Context(), roomID, reasonOwnerLeftEmpty)
		closedReason = string(reasonOwnerLeftEmpty)
	} else if !leaveRes.OwnerConnected && leaveRes.ConnectedAfter == 0 {
		_ = s.rooms.closeRoom(r.Context(), roomID, reasonOwnerLeftEmpty)
		closedReason = string(reasonOwnerLeftEmpty)
	} else if leaveRes.OwnerLeft {
		s.rooms.scheduleOwnerTimeout(roomID, 10*time.Minute)
	}

	if closedReason != "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "closed": true, "reason": closedReason})
		return
	}

	s.broadcastSnapshot(r.Context(), roomID)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// =============================
// REST handlers: Profile / account
// =============================

func (s *Server) handleGetMe(w http.ResponseWriter, r *http.Request) {
	sub := userSub(r)
	p, err := s.coreRepo.GetProfile(r.Context(), sub)
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

	p, err := s.coreRepo.UpsertProfile(r.Context(), sub, strings.TrimSpace(body.Nickname), strings.TrimSpace(body.PictureURL))
	if err != nil {
		status, msg := mapDomainErr(err)
		writeError(w, status, msg)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleDeleteMe(w http.ResponseWriter, r *http.Request) {
	sub := userSub(r)

	if err := s.nttRepo.CleanupUserData(r.Context(), sub); err != nil {
		status, msg := mapDomainErr(err)
		writeError(w, status, msg)
		return
	}

	if err := s.coreRepo.DeleteAccount(r.Context(), sub); err != nil {
		status, msg := mapDomainErr(err)
		writeError(w, status, msg)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// =============================
// REST handlers: Playlists
// =============================

func (s *Server) handleListPlaylists(w http.ResponseWriter, r *http.Request) {
	sub := userSub(r)

	pls, err := s.nttRepo.ListPlaylists(r.Context(), sub)
	if err != nil {
		status, msg := mapDomainErr(err)
		writeError(w, status, msg)
		return
	}

	// For the UI, include items as well (so it can show tracks).
	// This is N+1; acceptable for now. If needed, add a "list playlists with items" query.
	out := make([]namethattune.Playlist, 0, len(pls))
	for _, pl := range pls {
		full, err := s.nttRepo.GetPlaylist(r.Context(), sub, pl.ID)
		if err != nil {
			// If a playlist disappeared between list and get, just skip it.
			continue
		}
		out = append(out, full)
	}

	writeJSON(w, http.StatusOK, map[string]any{"playlists": out})
}

func (s *Server) handleCreatePlaylist(w http.ResponseWriter, r *http.Request) {
	sub := userSub(r)

	type reqBody struct {
		Name string `json:"name"`
	}
	var body reqBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	pl, err := s.nttRepo.CreatePlaylist(r.Context(), sub, strings.TrimSpace(body.Name))
	if err != nil {
		status, msg := mapDomainErr(err)
		writeError(w, status, msg)
		return
	}

	writeJSON(w, http.StatusCreated, pl)
}

func (s *Server) handlePatchPlaylist(w http.ResponseWriter, r *http.Request) {
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

	pl, err := s.nttRepo.UpdatePlaylistName(r.Context(), sub, playlistID, strings.TrimSpace(*body.Name))
	if err != nil {
		status, msg := mapDomainErr(err)
		writeError(w, status, msg)
		return
	}

	writeJSON(w, http.StatusOK, pl)
}

func (s *Server) handleAddPlaylistItem(w http.ResponseWriter, r *http.Request) {
	sub := userSub(r)
	playlistID := playlistIDParam(r)

	type reqBody struct {
		YouTubeURL string `json:"youtubeUrl"`
	}
	var body reqBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	youtubeURL := strings.TrimSpace(body.YouTubeURL)
	meta, err := namethattune.FetchYouTubeMetadata(r.Context(), youtubeURL)
	if err != nil {
		status, msg := mapDomainErr(fmt.Errorf("%w: %s", core.ErrInvalidInput, err.Error()))
		writeError(w, status, msg)
		return
	}

	item, pl, err := s.nttRepo.AddPlaylistItem(r.Context(), sub, playlistID, meta.Title, youtubeURL, meta.ThumbnailURL)
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

func (s *Server) handlePatchPlaylistItem(w http.ResponseWriter, r *http.Request) {
	sub := userSub(r)
	playlistID := playlistIDParam(r)
	itemID := playlistItemIDParam(r)

	type reqBody struct {
		Title *string `json:"title,omitempty"`
	}
	var body reqBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.Title == nil {
		writeError(w, http.StatusBadRequest, "invalid input")
		return
	}

	item, err := s.nttRepo.UpdatePlaylistItemTitle(r.Context(), sub, playlistID, itemID, *body.Title)
	if err != nil {
		status, msg := mapDomainErr(err)
		writeError(w, status, msg)
		return
	}

	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleDeletePlaylistItem(w http.ResponseWriter, r *http.Request) {
	sub := userSub(r)
	playlistID := playlistIDParam(r)
	itemID := playlistItemIDParam(r)

	if err := s.nttRepo.DeletePlaylistItem(r.Context(), sub, playlistID, itemID); err != nil {
		status, msg := mapDomainErr(err)
		writeError(w, status, msg)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
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
	snap, err := s.loadRoomSnapshot(r.Context(), roomID)
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

	type wsInbound struct {
		Type    string          `json:"type"`
		RoomID  string          `json:"roomId"`
		Payload json.RawMessage `json:"payload"`
	}
	type wsCommandPayload struct {
		Action     string `json:"action"`
		OwnerToken string `json:"ownerToken,omitempty"`
		PlayerToken string `json:"playerToken,omitempty"`
		PlayerID   string `json:"playerId,omitempty"`
		PlaylistID string `json:"playlistId,omitempty"`
		TrackIndex *int   `json:"trackIndex,omitempty"`
		Paused     *bool  `json:"paused,omitempty"`
		PositionMS *int   `json:"positionMs,omitempty"`
		Delta      *int   `json:"delta,omitempty"`
		Score      *int   `json:"score,omitempty"`
		Correct    *bool  `json:"correct,omitempty"`
		Buffering  *bool  `json:"buffering,omitempty"`
	}

	sendDirect := make(chan realtime.Event, 16)
	queueDirect := func(ev realtime.Event) {
		select {
		case sendDirect <- ev:
		default:
		}
	}

	// Reader: handle commands + drain to detect close/pings.
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		for {
			_, data, err := c.Read(r.Context())
			if err != nil {
				return
			}

			var msg wsInbound
			if err := json.Unmarshal(data, &msg); err != nil {
				queueDirect(realtime.Event{
					Type:   "room.command.error",
					RoomID: roomID,
					Payload: map[string]any{
						"message": "invalid json",
						"status":  http.StatusBadRequest,
					},
				})
				continue
			}
			if msg.Type != "room.command" {
				continue
			}
			if msg.RoomID != "" && msg.RoomID != roomID {
				queueDirect(realtime.Event{
					Type:   "room.command.error",
					RoomID: roomID,
					Payload: map[string]any{
						"message": "roomId mismatch",
						"status":  http.StatusBadRequest,
					},
				})
				continue
			}

			var payload wsCommandPayload
			if err := json.Unmarshal(msg.Payload, &payload); err != nil {
				queueDirect(realtime.Event{
					Type:   "room.command.error",
					RoomID: roomID,
					Payload: map[string]any{
						"message": "invalid command payload",
						"status":  http.StatusBadRequest,
					},
				})
				continue
			}
			action := strings.TrimSpace(payload.Action)
			if action == "" {
				queueDirect(realtime.Event{
					Type:   "room.command.error",
					RoomID: roomID,
					Payload: map[string]any{
						"message": "missing action",
						"status":  http.StatusBadRequest,
					},
				})
				continue
			}

			var cmdErr error
			var ownerSub string
			ownerSubForRoom := func() (string, error) {
				if ownerSub != "" {
					return ownerSub, nil
				}
				snap, err := s.loadRoomSnapshot(r.Context(), roomID)
				if err != nil {
					status, msg := mapDomainErr(err)
					return "", &apiError{Status: status, Message: msg}
				}
				if snap.OwnerSub == "" {
					return "", &apiError{Status: http.StatusForbidden, Message: "forbidden"}
				}
				ownerSub = snap.OwnerSub
				return ownerSub, nil
			}
			switch action {
			case "kick":
				if !s.validateOwnerToken(roomID, payload.OwnerToken) {
					cmdErr = &apiError{Status: http.StatusUnauthorized, Message: "unauthorized"}
					break
				}
				sub, err := ownerSubForRoom()
				if err != nil {
					cmdErr = err
					break
				}
				_, cmdErr = s.doKick(r.Context(), roomID, sub, payload.PlayerID)
			case "score.add":
				if !s.validateOwnerToken(roomID, payload.OwnerToken) {
					cmdErr = &apiError{Status: http.StatusUnauthorized, Message: "unauthorized"}
					break
				}
				if payload.Delta == nil {
					cmdErr = &apiError{Status: http.StatusBadRequest, Message: "invalid input"}
					break
				}
				sub, err := ownerSubForRoom()
				if err != nil {
					cmdErr = err
					break
				}
				_, cmdErr = s.doScoreAdd(r.Context(), roomID, sub, payload.PlayerID, *payload.Delta)
			case "score.set":
				if !s.validateOwnerToken(roomID, payload.OwnerToken) {
					cmdErr = &apiError{Status: http.StatusUnauthorized, Message: "unauthorized"}
					break
				}
				if payload.Score == nil {
					cmdErr = &apiError{Status: http.StatusBadRequest, Message: "invalid input"}
					break
				}
				sub, err := ownerSubForRoom()
				if err != nil {
					cmdErr = err
					break
				}
				_, cmdErr = s.doScoreSet(r.Context(), roomID, sub, payload.PlayerID, *payload.Score)
			case "playlist.load":
				if !s.validateOwnerToken(roomID, payload.OwnerToken) {
					cmdErr = &apiError{Status: http.StatusUnauthorized, Message: "unauthorized"}
					break
				}
				sub, err := ownerSubForRoom()
				if err != nil {
					cmdErr = err
					break
				}
				_, cmdErr = s.doLoadPlaylist(r.Context(), roomID, sub, payload.PlaylistID)
			case "playback.set":
				if !s.validateOwnerToken(roomID, payload.OwnerToken) {
					cmdErr = &apiError{Status: http.StatusUnauthorized, Message: "unauthorized"}
					break
				}
				if payload.TrackIndex == nil {
					cmdErr = &apiError{Status: http.StatusBadRequest, Message: "invalid input"}
					break
				}
				sub, err := ownerSubForRoom()
				if err != nil {
					cmdErr = err
					break
				}
				_, cmdErr = s.doPlaybackSet(r.Context(), roomID, sub, *payload.TrackIndex, payload.Paused, payload.PositionMS)
			case "playback.pause":
				if !s.validateOwnerToken(roomID, payload.OwnerToken) {
					cmdErr = &apiError{Status: http.StatusUnauthorized, Message: "unauthorized"}
					break
				}
				if payload.Paused == nil {
					cmdErr = &apiError{Status: http.StatusBadRequest, Message: "invalid input"}
					break
				}
				sub, err := ownerSubForRoom()
				if err != nil {
					cmdErr = err
					break
				}
				_, cmdErr = s.doPlaybackPause(r.Context(), roomID, sub, *payload.Paused)
			case "playback.seek":
				if !s.validateOwnerToken(roomID, payload.OwnerToken) {
					cmdErr = &apiError{Status: http.StatusUnauthorized, Message: "unauthorized"}
					break
				}
				if payload.PositionMS == nil {
					cmdErr = &apiError{Status: http.StatusBadRequest, Message: "invalid input"}
					break
				}
				sub, err := ownerSubForRoom()
				if err != nil {
					cmdErr = err
					break
				}
				_, cmdErr = s.doPlaybackSeek(r.Context(), roomID, sub, *payload.PositionMS)
			case "playback.buffer":
				if payload.Buffering == nil {
					cmdErr = &apiError{Status: http.StatusBadRequest, Message: "invalid input"}
					break
				}
				if payload.PlayerID == "" || !s.validatePlayerToken(roomID, payload.PlayerID, payload.PlayerToken) {
					cmdErr = &apiError{Status: http.StatusUnauthorized, Message: "unauthorized"}
					break
				}
				_, cmdErr = s.doPlaybackBuffering(r.Context(), roomID, payload.PlayerID, *payload.Buffering)
			case "buzz":
				if payload.PlayerID == "" || !s.validatePlayerToken(roomID, payload.PlayerID, payload.PlayerToken) {
					cmdErr = &apiError{Status: http.StatusUnauthorized, Message: "unauthorized"}
					break
				}
				cmdErr = s.doBuzz(r.Context(), roomID, payload.PlayerID)
			case "buzz.resolve":
				if !s.validateOwnerToken(roomID, payload.OwnerToken) {
					cmdErr = &apiError{Status: http.StatusUnauthorized, Message: "unauthorized"}
					break
				}
				if payload.Correct == nil {
					cmdErr = &apiError{Status: http.StatusBadRequest, Message: "invalid input"}
					break
				}
				sub, err := ownerSubForRoom()
				if err != nil {
					cmdErr = err
					break
				}
				cmdErr = s.doBuzzResolve(r.Context(), roomID, sub, payload.PlayerID, *payload.Correct)
			default:
				cmdErr = &apiError{Status: http.StatusBadRequest, Message: "unknown action"}
			}

			if cmdErr != nil {
				status, msg := mapAPIError(cmdErr)
				queueDirect(realtime.Event{
					Type:   "room.command.error",
					RoomID: roomID,
					Payload: map[string]any{
						"action":  action,
						"message": msg,
						"status":  status,
					},
				})
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
		case ev, ok := <-sendDirect:
			if !ok {
				return
			}
			if ev.RoomID == "" {
				ev.RoomID = roomID
			}
			if err := wsWriteJSON(r.Context(), c, ev); err != nil {
				return
			}
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
