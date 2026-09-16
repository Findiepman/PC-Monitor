// Package pterodactyl lists and controls game servers through the Panel's
// client API, and streams their consoles from Wings over websocket.
package pterodactyl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/findiepman/dashd/internal/config"
	"github.com/findiepman/dashd/internal/provider"
)

type Provider struct {
	label  string
	base   string
	key    string
	all    bool
	client *http.Client
}

func New(cfg config.Pterodactyl) *Provider {
	return &Provider{
		label:  cfg.Label,
		base:   cfg.URL,
		key:    cfg.APIKey,
		all:    cfg.AllServers,
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (p *Provider) Name() string  { return "pterodactyl" }
func (p *Provider) Label() string { return p.label }

type serverList struct {
	Data []struct {
		Attributes struct {
			Identifier   string `json:"identifier"`
			Name         string `json:"name"`
			Node         string `json:"node"`
			IsSuspended  bool   `json:"is_suspended"`
			IsInstalling bool   `json:"is_installing"`
			Limits       struct {
				Memory int64 `json:"memory"` // MiB, 0 = unlimited
			} `json:"limits"`
		} `json:"attributes"`
	} `json:"data"`
}

type resources struct {
	Attributes struct {
		CurrentState string `json:"current_state"`
		Resources    struct {
			MemoryBytes uint64  `json:"memory_bytes"`
			CPUAbsolute float64 `json:"cpu_absolute"`
			Uptime      int64   `json:"uptime"` // milliseconds
		} `json:"resources"`
	} `json:"attributes"`
}

func (p *Provider) List(ctx context.Context) ([]provider.Unit, error) {
	path := "/api/client?per_page=100"
	if p.all {
		path += "&type=admin-all"
	}
	var list serverList
	if err := p.api(ctx, http.MethodGet, path, nil, &list); err != nil {
		return nil, err
	}

	units := make([]provider.Unit, len(list.Data))
	var wg sync.WaitGroup
	for i, s := range list.Data {
		a := s.Attributes
		u := provider.Unit{
			ID:     a.Identifier,
			Name:   a.Name,
			Kind:   "game server",
			State:  provider.StateUnknown,
			Health: provider.HealthNone,
			Meta:   map[string]string{"node": a.Node},
		}
		if a.Limits.Memory > 0 {
			u.MemLimit = provider.Ptr(uint64(a.Limits.Memory) << 20)
		}
		switch {
		case a.IsSuspended:
			u.State, u.Meta["note"] = provider.StateStopped, "suspended"
		case a.IsInstalling:
			u.State, u.Meta["note"] = provider.StateStarting, "installing"
		}
		units[i] = u
		if a.IsSuspended || a.IsInstalling {
			continue
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			var r resources
			if err := p.api(ctx, http.MethodGet, "/api/client/servers/"+url.PathEscape(u.ID)+"/resources", nil, &r); err != nil {
				// Usually means the node's Wings is down.
				units[i].Meta["error"] = err.Error()
				units[i].Actions = []string{}
				return
			}
			ra := r.Attributes
			units[i].State = mapState(ra.CurrentState)
			if units[i].State == provider.StateRunning {
				units[i].CPU = provider.Ptr(ra.Resources.CPUAbsolute)
				units[i].MemBytes = provider.Ptr(ra.Resources.MemoryBytes)
				units[i].UptimeSec = ra.Resources.Uptime / 1000
			}
			units[i].Actions = provider.ActionsFor(units[i].State)
		}()
	}
	wg.Wait()
	for i := range units {
		if units[i].Actions == nil {
			units[i].Actions = []string{}
		}
	}
	return units, nil
}

func mapState(s string) string {
	switch s {
	case "running":
		return provider.StateRunning
	case "starting":
		return provider.StateStarting
	case "stopping":
		return provider.StateStopping
	case "offline":
		return provider.StateStopped
	}
	return provider.StateUnknown
}

func (p *Provider) Do(ctx context.Context, id, action string) error {
	if !provider.ValidAction(action) {
		return provider.ErrBadAction
	}
	body := map[string]string{"signal": action}
	return p.api(ctx, http.MethodPost, "/api/client/servers/"+url.PathEscape(id)+"/power", body, nil)
}

type wsCreds struct {
	Data struct {
		Token  string `json:"token"`
		Socket string `json:"socket"`
	} `json:"data"`
}

type wsMessage struct {
	Event string   `json:"event"`
	Args  []string `json:"args,omitempty"`
}

// Logs connects to the server's Wings websocket. Wings replays its recent
// console buffer when asked ("send logs"), so tail is decided by Wings.
func (p *Provider) Logs(ctx context.Context, id string, _ int) (<-chan provider.LogLine, error) {
	creds, err := p.wsCreds(ctx, id)
	if err != nil {
		return nil, err
	}
	conn, _, err := websocket.Dial(ctx, creds.Data.Socket, &websocket.DialOptions{
		// Wings only accepts connections whose Origin is the panel.
		HTTPHeader: http.Header{"Origin": {p.base}},
	})
	if err != nil {
		return nil, fmt.Errorf("connect to wings console: %w", err)
	}
	conn.SetReadLimit(1 << 20)
	if err := send(ctx, conn, wsMessage{Event: "auth", Args: []string{creds.Data.Token}}); err != nil {
		conn.CloseNow()
		return nil, err
	}

	out := make(chan provider.LogLine, 256)
	go func() {
		defer close(out)
		defer conn.CloseNow()
		emit := func(text string, isErr bool) bool {
			for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
				select {
				case out <- provider.LogLine{T: time.Now().UnixMilli(), Err: isErr, Text: provider.CleanLine(line)}:
				case <-ctx.Done():
					return false
				}
			}
			return true
		}
		for {
			_, raw, err := conn.Read(ctx)
			if err != nil {
				if ctx.Err() == nil {
					emit("console connection closed: "+err.Error(), true)
				}
				return
			}
			var msg wsMessage
			if json.Unmarshal(raw, &msg) != nil {
				continue
			}
			arg := ""
			if len(msg.Args) > 0 {
				arg = msg.Args[0]
			}
			ok := true
			switch msg.Event {
			case "auth success":
				err = send(ctx, conn, wsMessage{Event: "send logs"})
			case "console output", "install output":
				ok = emit(arg, false)
			case "daemon error":
				ok = emit(arg, true)
			case "daemon message":
				ok = emit("[wings] "+arg, false)
			case "status":
				ok = emit("[wings] server marked as "+arg, false)
			case "token expiring", "token expired":
				var fresh wsCreds
				if fresh, err = p.wsCreds(ctx, id); err == nil {
					err = send(ctx, conn, wsMessage{Event: "auth", Args: []string{fresh.Data.Token}})
				}
			case "jwt error":
				emit("wings rejected the console token: "+arg, true)
				return
			}
			if !ok {
				return
			}
			if err != nil {
				emit("console error: "+err.Error(), true)
				return
			}
		}
	}()
	return out, nil
}

func (p *Provider) wsCreds(ctx context.Context, id string) (wsCreds, error) {
	var c wsCreds
	err := p.api(ctx, http.MethodGet, "/api/client/servers/"+url.PathEscape(id)+"/websocket", nil, &c)
	return c, err
}

func send(ctx context.Context, conn *websocket.Conn, m wsMessage) error {
	raw, _ := json.Marshal(m)
	wctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return conn.Write(wctx, websocket.MessageText, raw)
}

func (p *Provider) api(ctx context.Context, method, path string, body, out any) error {
	var rd io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, p.base+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+p.key)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("panel not reachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return panelError(resp)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func panelError(resp *http.Response) error {
	var body struct {
		Errors []struct {
			Detail string `json:"detail"`
		} `json:"errors"`
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if json.Unmarshal(raw, &body) == nil && len(body.Errors) > 0 && body.Errors[0].Detail != "" {
		return errors.New(body.Errors[0].Detail)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("panel rejected the API key (%s)", resp.Status)
	}
	return fmt.Errorf("panel returned %s", resp.Status)
}
