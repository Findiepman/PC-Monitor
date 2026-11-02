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

var errNoLogs = errors.New("dashd can't read pm2 logs. On the server: pm2 logs <name>")

// Provider lists pm2 processes from the exporter's snapshot. It is view-only:
// no unit has actions and Do always refuses.
type Provider struct {
	label   string
	path    string
	exclude map[string]bool
	read    func() ([]byte, error)
	now     func() time.Time
}

func New(cfg config.PM2) *Provider {
	p := NewWithReader(cfg, func() ([]byte, error) { return readCapped(cfg.Path) })
	return p
}

// NewWithReader takes the snapshot from read instead of cfg.Path. Used by
// tests and mock mode.
func NewWithReader(cfg config.PM2, read func() ([]byte, error)) *Provider {
	p := &Provider{label: cfg.Label, path: cfg.Path, exclude: map[string]bool{}, read: read, now: time.Now}
	for _, n := range cfg.Exclude {
		p.exclude[strings.ToLower(strings.TrimSpace(n))] = true
	}
	return p
}

func (p *Provider) Name() string  { return "pm2" }
func (p *Provider) Label() string { return p.label }

func (p *Provider) List(context.Context) ([]provider.Unit, error) {
	raw, err := p.read()
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("no pm2 snapshot at %s. Is dashd-pm2.service running? (systemctl status dashd-pm2)", p.path)
	}
	if err != nil {
		return nil, err
	}
	var snap Snapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		return nil, fmt.Errorf("can't read the pm2 snapshot: %w", err)
	}
	now := p.now()
	// Allow three missed writes when the exporter runs slower than default.
	stale := max(StaleAfter, 3*time.Duration(min(snap.Every, int64(time.Hour/time.Millisecond)))*time.Millisecond)
	// A stamp far in the future (skewed or forged) would otherwise never age.
	age := now.Sub(time.UnixMilli(snap.T))
	if age < -stale {
		return nil, errors.New("the pm2 snapshot is dated in the future. Check the server clock, then restart dashd-pm2")
	}
	if age > stale {
		return nil, fmt.Errorf("the pm2 snapshot is %s old, so these bots may not be running. Is dashd-pm2.service running? (systemctl status dashd-pm2)", age.Round(time.Second))
	}
	if snap.Error != "" {
		return nil, errors.New(clean(snap.Error, 300))
	}
	if len(snap.Procs) > maxProcs {
		snap.Procs = snap.Procs[:maxProcs]
	}
	return Units(snap.Procs, p.exclude, now), nil
}

// Units turns snapshot processes into dashboard units.
func Units(procs []Proc, exclude map[string]bool, now time.Time) []provider.Unit {
	// The snapshot is written by another user; clean it again here.
	procs = append([]Proc(nil), procs...)
	for i := range procs {
		pr := &procs[i]
		pr.Name, pr.Status, pr.Runtime = clean(pr.Name, 64), clean(pr.Status, 32), clean(pr.Runtime, 32)
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
	units := []provider.Unit{}
	used := map[string]bool{}
	next := map[string]int{}
	for _, pr := range procs {
		if pr.Name == "" || exclude[strings.ToLower(pr.Name)] {
			continue
		}
		u := provider.Unit{
			ID:      pr.Name,
			Name:    pr.Name,
			Kind:    "bot",
			State:   MapStatus(pr.Status),
			Health:  provider.HealthNone,
			Actions: []string{},
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
		if pr.Restarts > 0 {
			u.Meta["restarts"] = strconv.Itoa(pr.Restarts)
		}
		if len(pr.Accounts) > 0 {
			u.Meta["accounts"] = strings.Join(pr.Accounts, "\n")
		}
		if u.State == provider.StateRunning {
			if pr.Started > 0 {
				if up := now.Sub(time.UnixMilli(pr.Started)); up > 0 {
					u.UptimeSec = int64(up / time.Second)
				}
			}
			u.CPU, u.MemBytes = pr.CPU, pr.Mem
		}
		units = append(units, u)
	}
	return units
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

func (p *Provider) Do(context.Context, string, string) error { return provider.ErrBadAction }

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
