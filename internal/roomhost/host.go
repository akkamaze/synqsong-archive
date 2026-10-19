package roomhost

import (
	"context"
	"fmt"
	"sync"

	"github.com/akkamaze/synqsong/internal/room"
)

type HostConfig struct {
	Store  Store
	Queues Queues
	Clock  Clock
}

// Host runs every room this process serves, one actor per room.
type Host struct {
	ctx   context.Context
	cfg   HostConfig
	mu    sync.Mutex
	rooms map[room.ID]*Room
}

func NewHost(ctx context.Context, cfg HostConfig) *Host {
	return &Host{ctx: ctx, cfg: cfg, rooms: map[room.ID]*Room{}}
}

// Room returns the running room, loading and starting it first if needed. A room
// that stopped, for example because another process took it over, is loaded again.
func (h *Host) Room(ctx context.Context, id room.ID) (*Room, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if r, ok := h.rooms[id]; ok && !r.stopped() {
		return r, nil
	}

	state, rev, err := h.cfg.Store.Load(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("load room %d: %w", id, err)
	}

	r := Start(h.ctx, Config{ID: id, State: state, Rev: rev, Store: h.cfg.Store, Queues: h.cfg.Queues, Clock: h.cfg.Clock})
	h.rooms[id] = r

	return r, nil
}
