package pm2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/findiepman/dashd/internal/config"
	"github.com/findiepman/dashd/internal/provider"
)

// StaleAfter is how old a snapshot may get before the page says the exporter
// stopped. The exporter writes every 10 seconds by default.
const StaleAfter = 45 * time.Second

// maxProcs caps how many processes one snapshot may list. Anything running
// as the pm2 user can write the snapshot, so dashd doesn't trust its size.
const maxProcs = 500

// maxSnapshot caps how much of the file is read.
const maxSnapshot = 1 << 20

var errNoLogs = errors.New("dashd can't show pm2 logs yet. On the server: pm2 logs <name>")

// Provider lists pm2 processes from the helper's snapshot and sends start,
// stop and restart to the helper's control socket.
type Provider struct {
	label   string
	path    string
	exclude map[string]bool
	read    func() ([]byte, error)
	now     func() time.Time
	// send delivers an action to the helper (SendControl on the socket).
	send func(ctx context.Context, req ControlRequest) error
}

func New(cfg config.PM2) *Provider {
	p := NewWithReader(cfg, func() ([]byte, error) { return readCapped(cfg.Path) })
	return p
}

// NewWithReader takes the snapshot from read instead of cfg.Path. Used by
// tests and mock mode.
func NewWithReader(cfg config.PM2, read func() ([]byte, error)) *Provider {
	p := &Provider{label: cfg.Label, path: cfg.Path, exclude: map[string]bool{}, read: read, now: time.Now,
		send: func(ctx context.Context, req ControlRequest) error { return SendControl(ctx, cfg.Control, req) }}
	for _, n := range cfg.Exclude {
		p.exclude[strings.ToLower(strings.TrimSpace(n))] = true
	}
	return p
}

// SetSender replaces how actions reach the helper. Mock mode uses it.
func (p *Provider) SetSender(send func(ctx context.Context, req ControlRequest) error) {
	p.send = send
}

func (p *Provider) Name() string  { return "pm2" }
func (p *Provider) Label() string { return p.label }

func (p *Provider) List(context.Context) ([]provider.Unit, error) {
	procs, now, err := p.snapshot()
	if err != nil {
		return nil, err
	}
	return Units(procs, p.exclude, now), nil
}

// snapshot reads the helper's file and checks it is fresh.
func (p *Provider) snapshot() ([]Proc, time.Time, error) {
	now := p.now()
	raw, err := p.read()
	if errors.Is(err, fs.ErrNotExist) {
		return nil, now, fmt.Errorf("no pm2 snapshot at %s. Is dashd-pm2.service running? (systemctl status dashd-pm2)", p.path)
	}
	if err != nil {
		return nil, now, err
	}
	var snap Snapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		return nil, now, fmt.Errorf("can't read the pm2 snapshot: %w", err)
	}
	// Allow three missed writes when the exporter runs slower than default.
	stale := max(StaleAfter, 3*time.Duration(min(snap.Every, int64(time.Hour/time.Millisecond)))*time.Millisecond)
	// A stamp far in the future (skewed or forged) would otherwise never age.
	age := now.Sub(time.UnixMilli(snap.T))
	if age < -stale {
		return nil, now, errors.New("the pm2 snapshot is dated in the future. Check the server clock, then restart dashd-pm2")
	}
	if age > stale {
		return nil, now, fmt.Errorf("the pm2 snapshot is %s old, so these bots may not be running. Is dashd-pm2.service running? (systemctl status dashd-pm2)", age.Round(time.Second))
	}
	if snap.Error != "" {
		return nil, now, errors.New(clean(snap.Error, 300))
	}
	if len(snap.Procs) > maxProcs {
		snap.Procs = snap.Procs[:maxProcs]
	}
	return snap.Procs, now, nil
}

// Units turns snapshot processes into dashboard units.
func Units(procs []Proc, exclude map[string]bool, now time.Time) []provider.Unit {
	bs := bind(procs, exclude, now)
	units := make([]provider.Unit, len(bs))
	for i, b := range bs {
		units[i] = b.unit
	}
	return units
}

// bound is a unit with the pm2 process it was made from, so Do can find the
// process behind a unit id exactly the way List named it.
type bound struct {
	unit provider.Unit
	proc Proc
}

func bind(procs []Proc, exclude map[string]bool, now time.Time) []bound {
	// The snapshot is written by another user; clean it again here.
	procs = append([]Proc(nil), procs...)
	for i := range procs {
		pr := &procs[i]
		pr.Name, pr.Status, pr.Runtime = clean(pr.Name, 64), clean(pr.Status, 32), clean(pr.Runtime, 32)
		pr.Discord = clean(pr.Discord, 64)
		if len(pr.Accounts) > maxAccounts {
			pr.Accounts = pr.Accounts[:maxAccounts]
		}
		accts := make([]string, 0, len(pr.Accounts))
		for _, a := range pr.Accounts {
			if a = clean(a, 64); a != "" {
				accts = append(accts, a)
			}
		}
		pr.Accounts = accts
	}
	count := map[string]int{}
	for _, pr := range procs {
		count[pr.Name]++
	}
	out := []bound{}
	used := map[string]bool{}
	next := map[string]int{}
	for _, pr := range procs {
		if pr.Name == "" || exclude[strings.ToLower(pr.Name)] {
			continue
		}
		state := MapStatus(pr.Status)
		u := provider.Unit{
			ID:      pr.Name,
			Name:    pr.Name,
			Kind:    "bot",
			State:   state,
			Health:  provider.HealthNone,
			Actions: Actions(state),
			Meta:    map[string]string{},
		}
		if count[pr.Name] > 1 {
			// Cluster instances share a name; keep ids unique.
			u.ID = pr.Name + "-" + strconv.Itoa(pr.ID)
			u.Name = pr.Name + " #" + strconv.Itoa(pr.ID)
		}
		// The UI keys tiles by id, so a duplicate would break the page. A
		// process really named "bot-3" next to cluster "bot" instance 3 can do it.
		// A counter per base keeps this linear even for thousands of repeats.
		for base := u.ID; used[u.ID]; {
			next[base]++
			u.ID = base + "~" + strconv.Itoa(next[base]+1)
		}
		used[u.ID] = true
		if pr.Runtime != "" {
			u.Meta["description"] = pr.Runtime
		}
		if pr.Discord != "" {
			u.Meta["discord"] = pr.Discord
		}
		if pr.Restarts > 0 {
			u.Meta["restarts"] = strconv.Itoa(pr.Restarts)
		}
		// One line per account, "name" or "name<TAB>discord name". clean()
		// strips tabs and newlines from both, so the format can't be faked.
		lines := make([]string, 0, len(pr.Accounts))
		for _, a := range pr.Accounts {
			if d := clean(pr.Logins[a], 64); d != "" {
				a += "\t" + d
			}
			lines = append(lines, a)
		}
		if len(lines) > 0 {
			u.Meta["accounts"] = strings.Join(lines, "\n")
		}
		if u.State == provider.StateRunning {
			if pr.Started > 0 {
				if up := now.Sub(time.UnixMilli(pr.Started)); up > 0 {
					u.UptimeSec = int64(up / time.Second)
				}
			}
			u.CPU, u.MemBytes = pr.CPU, pr.Mem
		}
		out = append(out, bound{u, pr})
	}
	return out
}

// Actions is what the page offers per state. pm2 has no kill; stop already
// ends the process.
func Actions(state string) []string {
	switch state {
	case provider.StateRunning:
		return []string{provider.ActionRestart, provider.ActionStop}
	case provider.StateStarting, provider.StateRestarting, provider.StateStopping:
		return []string{provider.ActionStop}
	case provider.StateStopped, provider.StateFailed:
		return []string{provider.ActionStart}
	}
	return []string{}
}

// MapStatus maps pm2's process states onto dashd's.
func MapStatus(s string) string {
	switch s {
	case "online":
		return provider.StateRunning
	case "launching":
		return provider.StateStarting
	case "waiting restart":
		return provider.StateRestarting
	case "stopping":
		return provider.StateStopping
	case "stopped", "one-launch-status":
		return provider.StateStopped
	case "errored":
		return provider.StateFailed
	}
	return provider.StateUnknown
}

func (p *Provider) Do(ctx context.Context, id, action string) error {
	switch action {
	case provider.ActionStart, provider.ActionStop, provider.ActionRestart:
	default:
		return provider.ErrBadAction
	}
	procs, now, err := p.snapshot()
	if err != nil {
		return err
	}
	for _, b := range bind(procs, p.exclude, now) {
		if b.unit.ID == id {
			return p.send(ctx, ControlRequest{Action: action, ID: b.proc.ID, Name: b.proc.Name})
		}
	}
	return provider.ErrUnknownUnit
}

func (p *Provider) Logs(context.Context, string, int) (<-chan provider.LogLine, error) {
	return nil, errNoLogs
}

func readCapped(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, maxSnapshot+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxSnapshot {
		return nil, fmt.Errorf("the pm2 snapshot at %s is larger than 1 MB", path)
	}
	return raw, nil
}
