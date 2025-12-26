package httpapi

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/valentin/bes-games/backend/internal/core"
	"github.com/valentin/bes-games/backend/internal/games/namethattune"
	"github.com/valentin/bes-games/backend/internal/realtime"
)

type roomCloseReason string

const (
	reasonOwnerLeftEmpty roomCloseReason = "owner_left_empty"
	reasonOwnerTimeout   roomCloseReason = "owner_timeout"
)

type roomLifecycle struct {
	repo        *namethattune.Repo
	rt          *realtime.Registry
	cleanup     func(roomID string)
	mu          sync.Mutex
	ownerTimers map[string]*time.Timer
}

func newRoomLifecycle(repo *namethattune.Repo, rt *realtime.Registry, cleanup func(roomID string)) *roomLifecycle {
	return &roomLifecycle{
		repo:        repo,
		rt:          rt,
		cleanup:     cleanup,
		ownerTimers: make(map[string]*time.Timer),
	}
}

func (l *roomLifecycle) scheduleOwnerTimeout(roomID string, delay time.Duration) {
	if l == nil {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if t, ok := l.ownerTimers[roomID]; ok {
		t.Stop()
	}
	timer := time.AfterFunc(delay, func() {
		l.handleOwnerTimeout(roomID)
	})
	l.ownerTimers[roomID] = timer
}

func (l *roomLifecycle) cancelOwnerTimeout(roomID string) {
	if l == nil {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if t, ok := l.ownerTimers[roomID]; ok {
		t.Stop()
		delete(l.ownerTimers, roomID)
	}
}

func (l *roomLifecycle) handleOwnerTimeout(roomID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pres, err := l.repo.RoomPresence(ctx, roomID)
	if err != nil {
		return
	}
	if pres.OwnerConnected {
		l.cancelOwnerTimeout(roomID)
		return
	}
	_ = l.closeRoom(ctx, roomID, reasonOwnerTimeout)
}

func (l *roomLifecycle) closeRoom(ctx context.Context, roomID string, reason roomCloseReason) error {
	if l == nil {
		return nil
	}

	l.cancelOwnerTimeout(roomID)

	if l.rt != nil {
		l.rt.Room(roomID).Broadcast(realtime.Event{
			Type:    "room.closed",
			RoomID:  roomID,
			Payload: map[string]any{"reason": string(reason)},
		})
	}

	if err := l.repo.DeleteRoom(ctx, roomID); err != nil && !errors.Is(err, core.ErrRoomNotFound) {
		return err
	}

	if l.cleanup != nil {
		l.cleanup(roomID)
	}

	if l.rt != nil {
		l.rt.Remove(roomID)
	}

	return nil
}
