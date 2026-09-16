// Package systemd controls an allowlist of systemd units through systemctl
// and reads their logs with journalctl.
//
// Shelling out keeps dashd free of D-Bus bindings and means permission is
// decided by polkit (see deploy/50-dashd.rules), exactly as for a human
// running systemctl.
package systemd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/findiepman/dashd/internal/config"
	"github.com/findiepman/dashd/internal/provider"
)

type runner func(ctx context.Context, name string, args ...string) ([]byte, error)

type Provider struct {
	label   string
	units   []string
	allowed map[string]bool
	run     runner
	// monoNow returns CLOCK_MONOTONIC in microseconds, the clock systemd's
	// *TimestampMonotonic properties use.
	monoNow func() (uint64, error)

	mu   sync.Mutex
	prev map[string]cpuPrev
}

type cpuPrev struct {
	nsec uint64
	at   time.Time
}

func New(cfg config.Systemd) *Provider {
	p := &Provider{
		label:   cfg.Label,
		units:   cfg.Units,
		allowed: map[string]bool{},
		run:     execRun,
		monoNow: procUptime,
		prev:    map[string]cpuPrev{},
	}
	for _, u := range cfg.Units {
		p.allowed[u] = true
	}
	return p
}

func (p *Provider) Name() string  { return "systemd" }
func (p *Provider) Label() string { return p.label }

var showProps = "Id,Description,LoadState,ActiveState,SubState,MainPID,ActiveEnterTimestampMonotonic,MemoryCurrent,CPUUsageNSec,NRestarts"

func (p *Provider) List(ctx context.Context) ([]provider.Unit, error) {
	if len(p.units) == 0 {
		return nil, nil
	}
	args := append([]string{"show", "--no-pager", "--property=" + showProps, "--"}, p.units...)
	out, err := p.run(ctx, "systemctl", args...)
	if err != nil {
		return nil, err
	}
	mono, _ := p.monoNow()
	now := time.Now()

	var units []provider.Unit
	for _, props := range parseShow(out) {
		id := props["Id"]
		if !p.allowed[id] {
			continue
		}
		units = append(units, p.unit(id, props, mono, now))
	}
	return units, nil
}

func (p *Provider) unit(id string, props map[string]string, mono uint64, now time.Time) provider.Unit {
	u := provider.Unit{
		ID:     id,
		Name:   strings.TrimSuffix(id, ".service"),
		Kind:   "service",
		State:  mapState(props["ActiveState"]),
		Health: provider.HealthNone,
		Meta:   map[string]string{},
	}
	if d := props["Description"]; d != "" {
		u.Meta["description"] = d
	}
	if s := props["SubState"]; s != "" {
		u.Meta["sub"] = s
	}
	if n := props["NRestarts"]; n != "" && n != "0" {
		u.Meta["restarts"] = n
	}
	if props["LoadState"] == "not-found" {
		u.State = provider.StateFailed
		u.Meta["error"] = "unit not found"
	}

	if u.State == provider.StateRunning {
		if enter, ok := num(props["ActiveEnterTimestampMonotonic"]); ok && enter > 0 && mono > enter {
			u.UptimeSec = int64((mono - enter) / 1_000_000)
		}
		if mem, ok := num(props["MemoryCurrent"]); ok {
			u.MemBytes = provider.Ptr(mem)
		}
		if nsec, ok := num(props["CPUUsageNSec"]); ok {
			p.mu.Lock()
			prev, seen := p.prev[id]
			p.prev[id] = cpuPrev{nsec, now}
			p.mu.Unlock()
			if seen && nsec >= prev.nsec {
				wall := now.Sub(prev.at).Nanoseconds()
				if wall > 0 {
					u.CPU = provider.Ptr(float64(nsec-prev.nsec) / float64(wall) * 100)
				}
			}
		}
	}
	u.Actions = provider.ActionsFor(u.State)
	return u
}

// num parses a numeric property. systemd reports unset values as "[not set]"
// or as the max uint64.
func num(s string) (uint64, bool) {
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil || v == ^uint64(0) {
		return 0, false
	}
	return v, true
}

// parseShow splits `systemctl show` output for several units. Each unit is a
// block of Key=Value lines, blocks separated by a blank line.
func parseShow(out []byte) []map[string]string {
	var blocks []map[string]string
	cur := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			if len(cur) > 0 {
				blocks = append(blocks, cur)
				cur = map[string]string{}
			}
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok {
			cur[k] = v
		}
	}
	if len(cur) > 0 {
		blocks = append(blocks, cur)
	}
	return blocks
}

func mapState(s string) string {
	switch s {
	case "active":
		return provider.StateRunning
	case "activating", "reloading":
		return provider.StateStarting
	case "deactivating":
		return provider.StateStopping
	case "inactive":
		return provider.StateStopped
	case "failed":
		return provider.StateFailed
	}
	return provider.StateUnknown
}

func (p *Provider) Do(ctx context.Context, id, action string) error {
	if !p.allowed[id] {
		return provider.ErrUnknownUnit
	}
	if !provider.ValidAction(action) {
		return provider.ErrBadAction
	}
	_, err := p.run(ctx, "systemctl", "--no-ask-password", action, "--", id)
	return err
}

func (p *Provider) Logs(ctx context.Context, id string, tail int) (<-chan provider.LogLine, error) {
	if !p.allowed[id] {
		return nil, provider.ErrUnknownUnit
	}
	cmd := exec.CommandContext(ctx, "journalctl",
		"--unit", id, "--lines", strconv.Itoa(tail), "--follow",
		"--output", "json", "--no-pager",
		"--output-fields", "MESSAGE,PRIORITY")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start journalctl: %w", err)
	}

	out := make(chan provider.LogLine, 256)
	go func() {
		defer close(out)
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		for sc.Scan() {
			l, ok := parseJournal(sc.Bytes())
			if !ok {
				continue
			}
			select {
			case out <- l:
			case <-ctx.Done():
			}
			if ctx.Err() != nil {
				break
			}
		}
		if err := cmd.Wait(); err != nil && ctx.Err() == nil {
			msg := strings.TrimSpace(stderr.String())
			if msg == "" {
				msg = err.Error()
			}
			select {
			case out <- provider.LogLine{T: time.Now().UnixMilli(), Err: true, Text: "journalctl: " + msg}:
			case <-ctx.Done():
			}
		}
	}()
	return out, nil
}

type journalEntry struct {
	Realtime string          `json:"__REALTIME_TIMESTAMP"`
	Priority string          `json:"PRIORITY"`
	Message  json.RawMessage `json:"MESSAGE"`
}

func parseJournal(raw []byte) (provider.LogLine, bool) {
	var e journalEntry
	if err := json.Unmarshal(raw, &e); err != nil {
		return provider.LogLine{}, false
	}
	var l provider.LogLine
	if us, err := strconv.ParseInt(e.Realtime, 10, 64); err == nil {
		l.T = us / 1000
	}
	if prio, err := strconv.Atoi(e.Priority); err == nil && prio <= 3 {
		l.Err = true
	}
	// MESSAGE is a string, or an array of bytes when it isn't valid UTF-8.
	var s string
	if err := json.Unmarshal(e.Message, &s); err != nil {
		var b []byte
		var ints []int
		if json.Unmarshal(e.Message, &ints) == nil {
			for _, i := range ints {
				b = append(b, byte(i))
			}
		}
		s = string(bytes.ToValidUTF8(b, []byte("?")))
	}
	l.Text = provider.CleanLine(s)
	return l, true
}

func execRun(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return out, fmt.Errorf("%s", msg)
		}
		return out, fmt.Errorf("%s: %w", name, err)
	}
	return out, nil
}

func procUptime() (uint64, error) {
	raw, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0, err
	}
	first, _, _ := strings.Cut(string(raw), " ")
	sec, err := strconv.ParseFloat(first, 64)
	if err != nil {
		return 0, err
	}
	return uint64(sec * 1_000_000), nil
}
