package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/valentin/bes-blind/backend/internal/game"
	"github.com/valentin/bes-blind/backend/internal/httpapi/testutil"
	"github.com/valentin/bes-blind/backend/internal/realtime"
)

func TestHealthz(t *testing.T) {
	t.Parallel()

	srv := newTestServerNoDB(t)
	h := srv.Handler(Options{AllowedOrigins: []string{"http://localhost:5173"}})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("expected json response, got unmarshal error: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("expected status ok, got %#v", body["status"])
	}
}

func TestRooms_CreateRoomRequiresAuth(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := freshDB(t, ctx)
	srv := newTestServer(t, pool)
	h := srv.Handler(Options{AllowedOrigins: []string{"http://localhost:5173"}})

	req := httptest.NewRequest(http.MethodPost, "/api/rooms", strings.NewReader(`{"name":"My Room"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestRooms_CreateListJoinLeaveAndGetSnapshot(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := freshDB(t, ctx)
	srv := newTestServer(t, pool)
	h := srv.Handler(Options{AllowedOrigins: []string{"http://localhost:5173"}})

	roomID := createRoom(t, h, "owner-sub", "Test Room")

	// List rooms shows our room
	{
		req := httptest.NewRequest(http.MethodGet, "/api/rooms", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("list rooms: expected 200, got %d: %s", rr.Code, rr.Body.String())
		}

		var res struct {
			Rooms []struct {
				RoomID        string `json:"roomId"`
				Name          string `json:"name"`
				OnlinePlayers int    `json:"onlinePlayers"`
			} `json:"rooms"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
			t.Fatalf("list rooms: unmarshal: %v", err)
		}

		found := false
		for _, r := range res.Rooms {
			if r.RoomID == roomID {
				found = true
				if r.Name != "Test Room" {
					t.Fatalf("expected room name %q, got %q", "Test Room", r.Name)
				}
				if r.OnlinePlayers != 0 {
					t.Fatalf("expected onlinePlayers 0 before join, got %d", r.OnlinePlayers)
				}
			}
		}
		if !found {
			t.Fatalf("room %q not found in list", roomID)
		}
	}

	// Join room anonymously
	playerID := joinRoom(t, h, roomID, "", `{"nickname":"Anon","pictureUrl":"https://example.com/p.png"}`)

	// Get snapshot should show player online
	{
		req := httptest.NewRequest(http.MethodGet, "/api/rooms/"+roomID, nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("get room: expected 200, got %d: %s", rr.Code, rr.Body.String())
		}

		var snap game.RoomSnapshot
		if err := json.Unmarshal(rr.Body.Bytes(), &snap); err != nil {
			t.Fatalf("get room: unmarshal: %v", err)
		}
		if snap.RoomID != roomID {
			t.Fatalf("expected snapshot roomId %q, got %q", roomID, snap.RoomID)
		}
		if snap.Name != "Test Room" {
			t.Fatalf("expected snapshot name %q, got %q", "Test Room", snap.Name)
		}
		if len(snap.Players) != 1 {
			t.Fatalf("expected 1 player, got %d", len(snap.Players))
		}
		if snap.Players[0].PlayerID != playerID {
			t.Fatalf("expected playerId %q, got %q", playerID, snap.Players[0].PlayerID)
		}
		if !snap.Players[0].Connected {
			t.Fatalf("expected player to be connected")
		}
	}

	// Leave room marks player offline (still in roster)
	{
		req := httptest.NewRequest(http.MethodPost, "/api/rooms/"+roomID+"/leave", strings.NewReader(`{"playerId":"`+playerID+`"}`))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("leave room: expected 200, got %d: %s", rr.Code, rr.Body.String())
		}

		req2 := httptest.NewRequest(http.MethodGet, "/api/rooms/"+roomID, nil)
		rr2 := httptest.NewRecorder()
		h.ServeHTTP(rr2, req2)

		var snap game.RoomSnapshot
		if err := json.Unmarshal(rr2.Body.Bytes(), &snap); err != nil {
			t.Fatalf("after leave: unmarshal: %v", err)
		}
		if len(snap.Players) != 1 {
			t.Fatalf("after leave: expected 1 player, got %d", len(snap.Players))
		}
		if snap.Players[0].PlayerID != playerID {
			t.Fatalf("after leave: expected playerId %q, got %q", playerID, snap.Players[0].PlayerID)
		}
		if snap.Players[0].Connected {
			t.Fatalf("after leave: expected player to be disconnected")
		}
	}

	// List rooms should now show 0 online
	{
		req := httptest.NewRequest(http.MethodGet, "/api/rooms", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("list rooms after leave: expected 200, got %d: %s", rr.Code, rr.Body.String())
		}

		var res struct {
			Rooms []struct {
				RoomID        string `json:"roomId"`
				OnlinePlayers int    `json:"onlinePlayers"`
			} `json:"rooms"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
			t.Fatalf("list rooms after leave: unmarshal: %v", err)
		}

		for _, r := range res.Rooms {
			if r.RoomID == roomID && r.OnlinePlayers != 0 {
				t.Fatalf("expected onlinePlayers 0 after leave, got %d", r.OnlinePlayers)
			}
		}
	}
}

func TestOwnerControls_ForbiddenForNonOwner(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := freshDB(t, ctx)
	srv := newTestServer(t, pool)
	h := srv.Handler(Options{AllowedOrigins: []string{"http://localhost:5173"}})

	roomID := createRoom(t, h, "owner-sub", "Room")
	playerID := joinRoom(t, h, roomID, "", `{"nickname":"P1"}`)

	req := httptest.NewRequest(http.MethodPost, "/api/rooms/"+roomID+"/kick", strings.NewReader(`{"playerId":"`+playerID+`"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Sub", "someone-else")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestOwnerControls_ScoreAndKick(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := freshDB(t, ctx)
	srv := newTestServer(t, pool)
	h := srv.Handler(Options{AllowedOrigins: []string{"http://localhost:5173"}})

	roomID := createRoom(t, h, "owner-sub", "Room")
	playerID := joinRoom(t, h, roomID, "", `{"nickname":"P1"}`)

	// Add score +2
	{
		req := httptest.NewRequest(http.MethodPost, "/api/rooms/"+roomID+"/score/add", strings.NewReader(`{"playerId":"`+playerID+`","delta":2}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-User-Sub", "owner-sub")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("score add: expected 200, got %d: %s", rr.Code, rr.Body.String())
		}

		var snap game.RoomSnapshot
		if err := json.Unmarshal(rr.Body.Bytes(), &snap); err != nil {
			t.Fatalf("score add: unmarshal: %v", err)
		}
		p := findPlayer(t, snap, playerID)
		if p.Score != 2 {
			t.Fatalf("expected score 2, got %d", p.Score)
		}
	}

	// Set score to 7
	{
		req := httptest.NewRequest(http.MethodPost, "/api/rooms/"+roomID+"/score/set", strings.NewReader(`{"playerId":"`+playerID+`","score":7}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-User-Sub", "owner-sub")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("score set: expected 200, got %d: %s", rr.Code, rr.Body.String())
		}

		var snap game.RoomSnapshot
		if err := json.Unmarshal(rr.Body.Bytes(), &snap); err != nil {
			t.Fatalf("score set: unmarshal: %v", err)
		}
		p := findPlayer(t, snap, playerID)
		if p.Score != 7 {
			t.Fatalf("expected score 7, got %d", p.Score)
		}
	}

	// Kick player
	{
		req := httptest.NewRequest(http.MethodPost, "/api/rooms/"+roomID+"/kick", strings.NewReader(`{"playerId":"`+playerID+`"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-User-Sub", "owner-sub")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("kick: expected 200, got %d: %s", rr.Code, rr.Body.String())
		}

		var snap game.RoomSnapshot
		if err := json.Unmarshal(rr.Body.Bytes(), &snap); err != nil {
			t.Fatalf("kick: unmarshal: %v", err)
		}
		if len(snap.Players) != 0 {
			t.Fatalf("expected 0 players after kick, got %d", len(snap.Players))
		}
	}
}

func TestProfileAndPlaylists_CRUDAndRoomLoad(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := freshDB(t, ctx)
	srv := newTestServer(t, pool)
	h := srv.Handler(Options{AllowedOrigins: []string{"http://localhost:5173"}})

	ownerSub := "owner-sub"

	// PUT /api/me
	{
		req := httptest.NewRequest(http.MethodPut, "/api/me", strings.NewReader(`{"nickname":"DJ","pictureUrl":"https://example.com/a.png"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-User-Sub", ownerSub)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("put me: expected 200, got %d: %s", rr.Code, rr.Body.String())
		}
	}

	// POST /api/me/playlists
	var playlistID string
	{
		req := httptest.NewRequest(http.MethodPost, "/api/me/playlists", strings.NewReader(`{"name":"Hits"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-User-Sub", ownerSub)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)

		if rr.Code != http.StatusCreated {
			t.Fatalf("create playlist: expected 201, got %d: %s", rr.Code, rr.Body.String())
		}

		var pl game.Playlist
		if err := json.Unmarshal(rr.Body.Bytes(), &pl); err != nil {
			t.Fatalf("create playlist: unmarshal: %v", err)
		}
		if pl.Name != "Hits" {
			t.Fatalf("expected playlist name %q, got %q", "Hits", pl.Name)
		}
		if pl.ID == "" {
			t.Fatalf("expected non-empty playlist id")
		}
		playlistID = pl.ID
	}

	// POST item
	{
		req := httptest.NewRequest(http.MethodPost, "/api/me/playlists/"+playlistID+"/items", strings.NewReader(`{"title":"Song","youtubeUrl":"https://www.youtube.com/watch?v=dQw4w9WgXcQ"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-User-Sub", ownerSub)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)

		if rr.Code != http.StatusCreated {
			t.Fatalf("add item: expected 201, got %d: %s", rr.Code, rr.Body.String())
		}
	}

	// PATCH rename
	{
		req := httptest.NewRequest(http.MethodPatch, "/api/me/playlists/"+playlistID, strings.NewReader(`{"name":"Renamed"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-User-Sub", ownerSub)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("rename playlist: expected 200, got %d: %s", rr.Code, rr.Body.String())
		}

		var pl game.Playlist
		if err := json.Unmarshal(rr.Body.Bytes(), &pl); err != nil {
			t.Fatalf("rename playlist: unmarshal: %v", err)
		}
		if pl.Name != "Renamed" {
			t.Fatalf("expected renamed playlist, got %q", pl.Name)
		}
		if len(pl.Items) != 1 {
			t.Fatalf("expected 1 item after rename response, got %d", len(pl.Items))
		}
	}

	// GET playlists should include items (server currently loads full playlist per list entry)
	{
		req := httptest.NewRequest(http.MethodGet, "/api/me/playlists", nil)
		req.Header.Set("X-User-Sub", ownerSub)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("list playlists: expected 200, got %d: %s", rr.Code, rr.Body.String())
		}

		var res struct {
			Playlists []game.Playlist `json:"playlists"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
			t.Fatalf("list playlists: unmarshal: %v", err)
		}
		if len(res.Playlists) != 1 {
			t.Fatalf("expected 1 playlist, got %d", len(res.Playlists))
		}
		if res.Playlists[0].ID != playlistID {
			t.Fatalf("expected playlist id %q, got %q", playlistID, res.Playlists[0].ID)
		}
		if len(res.Playlists[0].Items) != 1 {
			t.Fatalf("expected 1 playlist item, got %d", len(res.Playlists[0].Items))
		}
	}

	// Create room and load playlist into room
	roomID := createRoom(t, h, ownerSub, "Room")
	{
		req := httptest.NewRequest(http.MethodPost, "/api/rooms/"+roomID+"/playlist/load", strings.NewReader(`{"playlistId":"`+playlistID+`"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-User-Sub", ownerSub)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("load playlist: expected 200, got %d: %s", rr.Code, rr.Body.String())
		}

		var snap game.RoomSnapshot
		if err := json.Unmarshal(rr.Body.Bytes(), &snap); err != nil {
			t.Fatalf("load playlist: unmarshal: %v", err)
		}
		if snap.Playlist == nil || snap.Playlist.PlaylistID != playlistID {
			t.Fatalf("expected room to have loaded playlistId %q, got %#v", playlistID, snap.Playlist)
		}
		if snap.Playback.PlaylistID != playlistID {
			t.Fatalf("expected playback playlistId %q, got %q", playlistID, snap.Playback.PlaylistID)
		}
		if !snap.Playback.Paused {
			t.Fatalf("expected playback paused after load")
		}
		if len(snap.Playlist.Items) > 0 && snap.Playback.Track == nil {
			t.Fatalf("expected playback track to be set when playlist has items")
		}
	}
}

func TestRoomWebSocket_ReceivesInitialSnapshot(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := freshDB(t, ctx)
	srv := newTestServer(t, pool)
	h := srv.Handler(Options{AllowedOrigins: []string{"http://localhost:5173"}})
	ts := httptest.NewServer(h)
	defer ts.Close()

	roomID := createRoom(t, h, "owner-sub", "WS Room")

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/rooms/" + roomID + "/ws"

	dialCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	c, _, err := websocket.Dial(dialCtx, wsURL, nil)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	defer func() { _ = c.Close(websocket.StatusNormalClosure, "bye") }()

	_, data, err := c.Read(dialCtx)
	if err != nil {
		t.Fatalf("ws read: %v", err)
	}

	var ev struct {
		Type    string          `json:"type"`
		RoomID  string          `json:"roomId"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(data, &ev); err != nil {
		t.Fatalf("ws frame json: %v", err)
	}

	if ev.Type != "room.snapshot" {
		t.Fatalf("expected first event type room.snapshot, got %q (data=%s)", ev.Type, string(data))
	}
	if ev.RoomID != roomID {
		t.Fatalf("expected event roomId %q, got %q", roomID, ev.RoomID)
	}

	var snap game.RoomSnapshot
	if err := json.Unmarshal(ev.Payload, &snap); err != nil {
		t.Fatalf("ws snapshot payload unmarshal: %v", err)
	}
	if snap.RoomID != roomID || snap.Name != "WS Room" {
		t.Fatalf("unexpected snapshot: %#v", snap)
	}
}

// --------------------
// Test server wiring
// --------------------

func newTestServerNoDB(t *testing.T) *Server {
	t.Helper()

	// For routes that don't require DB (like /healthz), we can run a server with nil deps.
	// Handlers that touch DB will panic; tests must not call them here.
	return NewServer(nil, realtime.NewRegistry())
}

func newTestServer(t *testing.T, pool *pgxpool.Pool) *Server {
	t.Helper()
	repo := game.NewRepo(pool)
	rt := realtime.NewRegistry()
	return NewServer(repo, rt)
}

func freshDB(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()

	// If no DB configured, skip integration tests.
	testutil.SkipIfNoDB(t)

	pool := testutil.WithFreshDB(ctx, t)
	return pool
}

// --------------------
// Helpers
// --------------------

func createRoom(t *testing.T, h http.Handler, ownerSub, name string) string {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/rooms", strings.NewReader(`{"name":"`+jsonEscape(name)+`"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Sub", ownerSub)

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("create room: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var res struct {
		RoomID string `json:"roomId"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
		t.Fatalf("create room: unmarshal: %v", err)
	}
	if res.RoomID == "" {
		t.Fatalf("create room: missing roomId in response: %s", rr.Body.String())
	}
	return res.RoomID
}

func joinRoom(t *testing.T, h http.Handler, roomID, sub, body string) string {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/rooms/"+roomID+"/join", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if sub != "" {
		req.Header.Set("X-User-Sub", sub)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("join room: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var res struct {
		PlayerID string `json:"playerId"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
		t.Fatalf("join room: unmarshal: %v", err)
	}
	if res.PlayerID == "" {
		t.Fatalf("join room: missing playerId in response: %s", rr.Body.String())
	}
	return res.PlayerID
}

func findPlayer(t *testing.T, snap game.RoomSnapshot, playerID string) game.PlayerView {
	t.Helper()

	for _, p := range snap.Players {
		if p.PlayerID == playerID {
			return p
		}
	}
	t.Fatalf("player %q not found in snapshot", playerID)
	return game.PlayerView{}
}

func jsonEscape(s string) string {
	// Minimal JSON string escaping for test bodies.
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}
