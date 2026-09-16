package api

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/findiepman/dashd/internal/provider"
)

const (
	maxLogSubs    = 4
	defaultTail   = 500
	maxTail       = 5000
	logBatchEvery = 120 * time.Millisecond
	logBatchMax   = 500
)

// Client → server messages:
//
//	{"op":"logs","unit":"docker:redis","tail":500}   start streaming a unit's logs
//	{"op":"unlogs","unit":"docker:redis"}            stop
//
// Server → client messages are envelopes: {"type": ..., "data": ...} with
// type hello, host, units, audit, logs or logs_end.
type clientMsg struct {
	Op   string `json:"op"`
	Unit string `json:"unit"`
	Tail int    `json:"tail"`
}

type logsBatch struct {
	Unit  string             `json:"unit"`
	Lines []provider.LogLine `json:"lines"`
}

type logsEnd struct {
	Unit  string `json:"unit"`
	Error string `json:"error,omitempty"`
}

type wsClient struct {
	conn   *websocket.Conn
	send   chan []byte
	cancel context.CancelFunc

	mu   sync.Mutex
	subs map[string]*logSub
}

type logSub struct{ stop context.CancelFunc }

func (s *Server) ws(w http.ResponseWriter, r *http.Request, c caller) {
	// Accept verifies the Origin header matches the Host.
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	conn.SetReadLimit(64 * 1024)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	cl := &wsClient{conn: conn, send: make(chan []byte, 128), cancel: cancel, subs: map[string]*logSub{}}

	hello, _ := json.Marshal(envelope{"hello", map[string]any{
		"history": s.Hub.History(),
		"units":   s.Hub.Units(),
		"user":    c.user,
	}})
	cl.send <- hello
	s.Hub.add(cl)
	defer s.Hub.remove(cl)

	go cl.writeLoop(ctx)
	go s.watchSession(ctx, cl, c.token)

	for {
		var m clientMsg
		_, raw, err := conn.Read(ctx)
		if err != nil {
			break
		}
		if json.Unmarshal(raw, &m) != nil {
			continue
		}
		switch m.Op {
		case "logs":
			s.subscribeLogs(ctx, cl, m)
		case "unlogs":
			cl.unsubscribe(m.Unit)
		}
	}
	cl.mu.Lock()
	for _, sub := range cl.subs {
		sub.stop()
	}
	cl.mu.Unlock()
	conn.CloseNow()
}

// watchSession closes the socket once the session is logged out or expires.
func (s *Server) watchSession(ctx context.Context, cl *wsClient, token string) {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if _, ok := s.Auth.Session(token); !ok {
				cl.conn.Close(4001, "session ended")
				cl.cancel()
				return
			}
		}
	}
}

func (cl *wsClient) writeLoop(ctx context.Context) {
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-cl.send:
			wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err := cl.conn.Write(wctx, websocket.MessageText, msg)
			cancel()
			if err != nil {
				cl.cancel()
				return
			}
		case <-ping.C:
			pctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err := cl.conn.Ping(pctx)
			cancel()
			if err != nil {
				cl.cancel()
				return
			}
		}
	}
}

// trySend drops a client that can't keep up instead of stalling everyone.
func (cl *wsClient) trySend(msg []byte) {
	select {
	case cl.send <- msg:
	default:
		// Cancelling unblocks the read loop, which closes the connection.
		cl.cancel()
	}
}

func (cl *wsClient) unsubscribe(unit string) {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	if sub, ok := cl.subs[unit]; ok {
		sub.stop()
		delete(cl.subs, unit)
	}
}

func (s *Server) subscribeLogs(ctx context.Context, cl *wsClient, m clientMsg) {
	tail := m.Tail
	if tail <= 0 {
		tail = defaultTail
	}
	tail = min(tail, maxTail)

	cl.mu.Lock()
	if sub, ok := cl.subs[m.Unit]; ok {
		sub.stop()
		delete(cl.subs, m.Unit)
	}
	if len(cl.subs) >= maxLogSubs {
		cl.mu.Unlock()
		s.sendEnvelope(ctx, cl, "logs_end", logsEnd{m.Unit, "Too many open log streams. Close one and try again."})
		return
	}
	sctx, stop := context.WithCancel(ctx)
	sub := &logSub{stop}
	cl.subs[m.Unit] = sub
	cl.mu.Unlock()

	go func() {
		defer func() {
			stop()
			cl.mu.Lock()
			if cl.subs[m.Unit] == sub {
				delete(cl.subs, m.Unit)
			}
			cl.mu.Unlock()
		}()
		lines, err := s.Registry.Logs(sctx, m.Unit, tail)
		if err != nil {
			s.sendEnvelope(ctx, cl, "logs_end", logsEnd{m.Unit, err.Error()})
			return
		}
		ticker := time.NewTicker(logBatchEvery)
		defer ticker.Stop()
		var batch []provider.LogLine
		flush := func() bool {
			if len(batch) == 0 {
				return true
			}
			ok := s.sendEnvelope(sctx, cl, "logs", logsBatch{m.Unit, batch})
			batch = nil
			return ok
		}
		for {
			select {
			case l, open := <-lines:
				if !open {
					flush()
					if sctx.Err() == nil {
						s.sendEnvelope(ctx, cl, "logs_end", logsEnd{Unit: m.Unit})
					}
					return
				}
				batch = append(batch, l)
				if len(batch) >= logBatchMax && !flush() {
					return
				}
			case <-ticker.C:
				if !flush() {
					return
				}
			case <-sctx.Done():
				return
			}
		}
	}()
}

// sendEnvelope blocks briefly: log output matters more than a dropped stats
// tick, but a stuck client still gets cut off.
func (s *Server) sendEnvelope(ctx context.Context, cl *wsClient, typ string, data any) bool {
	raw, err := json.Marshal(envelope{typ, data})
	if err != nil {
		return false
	}
	t := time.NewTimer(5 * time.Second)
	defer t.Stop()
	select {
	case cl.send <- raw:
		return true
	case <-ctx.Done():
		return false
	case <-t.C:
		cl.cancel()
		return false
	}
}
