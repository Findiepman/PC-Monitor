package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/findiepman/dashd/internal/host"
	"github.com/findiepman/dashd/internal/provider"
)

const (
	hostInterval  = 2 * time.Second
	unitsInterval = 5 * time.Second
	// historyLen samples of host stats: five minutes at hostInterval.
	historyLen = 150
)

type UnitsSnapshot struct {
	Units  []provider.Unit   `json:"units"`
	Errors map[string]string `json:"errors"`
	T      int64             `json:"t"`
}

// Hub polls the host and providers on a schedule and fans the results out to
// every connected websocket client.
type Hub struct {
	reg  *provider.Registry
	host host.Collector
	log  *slog.Logger

	mu      sync.RWMutex
	clients map[*wsClient]struct{}
	history []host.Stats
	units   UnitsSnapshot

	refresh chan struct{}
}

func NewHub(reg *provider.Registry, hc host.Collector, log *slog.Logger) *Hub {
	return &Hub{
		reg:     reg,
		host:    hc,
		log:     log,
		clients: map[*wsClient]struct{}{},
		units:   UnitsSnapshot{Units: []provider.Unit{}, Errors: map[string]string{}},
		refresh: make(chan struct{}, 1),
	}
}

func (h *Hub) Run(ctx context.Context) {
	h.pollUnits(ctx)
	h.pollHost(ctx)

	hostT := time.NewTicker(hostInterval)
	unitsT := time.NewTicker(unitsInterval)
	defer hostT.Stop()
	defer unitsT.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-hostT.C:
			h.pollHost(ctx)
		case <-unitsT.C:
			if h.clientCount() > 0 {
				h.pollUnits(ctx)
			}
		case <-h.refresh:
			h.pollUnits(ctx)
			unitsT.Reset(unitsInterval)
		}
	}
}

// RefreshUnits asks for a poll now and another shortly after, so state
// changes caused by an action show up without waiting for the next tick.
func (h *Hub) RefreshUnits() {
	kick := func() {
		select {
		case h.refresh <- struct{}{}:
		default:
		}
	}
	kick()
	time.AfterFunc(1500*time.Millisecond, kick)
	time.AfterFunc(4*time.Second, kick)
}

func (h *Hub) pollHost(ctx context.Context) {
	cctx, cancel := context.WithTimeout(ctx, hostInterval-200*time.Millisecond)
	defer cancel()
	st, err := h.host.Collect(cctx)
	if err != nil {
		h.log.Warn("host stats failed", "err", err)
		return
	}
	h.mu.Lock()
	h.history = append(h.history, st)
	if len(h.history) > historyLen {
		h.history = h.history[len(h.history)-historyLen:]
	}
	h.mu.Unlock()
	h.Broadcast("host", st)
}

func (h *Hub) pollUnits(ctx context.Context) {
	cctx, cancel := context.WithTimeout(ctx, unitsInterval-500*time.Millisecond)
	defer cancel()
	units, errs := h.reg.ListAll(cctx)
	snap := UnitsSnapshot{Units: units, Errors: errs, T: time.Now().UnixMilli()}
	h.mu.Lock()
	h.units = snap
	h.mu.Unlock()
	h.Broadcast("units", snap)
}

func (h *Hub) Units() UnitsSnapshot {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.units
}

func (h *Hub) History() []host.Stats {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return append([]host.Stats(nil), h.history...)
}

type envelope struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

func (h *Hub) Broadcast(typ string, data any) {
	raw, err := json.Marshal(envelope{typ, data})
	if err != nil {
		h.log.Error("marshal broadcast", "err", err)
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients {
		c.trySend(raw)
	}
}

func (h *Hub) add(c *wsClient) {
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
}

func (h *Hub) remove(c *wsClient) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
}

func (h *Hub) clientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
