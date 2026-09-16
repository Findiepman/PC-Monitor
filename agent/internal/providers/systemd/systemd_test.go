package systemd

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/findiepman/dashd/internal/config"
	"github.com/findiepman/dashd/internal/provider"
)

const showOutput = `Id=wings.service
Description=Pterodactyl Wings Daemon
LoadState=loaded
ActiveState=active
SubState=running
MainPID=812
ActiveEnterTimestampMonotonic=5000000
MemoryCurrent=52428800
CPUUsageNSec=1000000000
NRestarts=1

Id=playit.service
Description=Playit agent
LoadState=loaded
ActiveState=failed
SubState=failed
MainPID=0
ActiveEnterTimestampMonotonic=0
MemoryCurrent=[not set]
CPUUsageNSec=18446744073709551615
NRestarts=0

Id=sshd.service
LoadState=loaded
ActiveState=active
`

func TestList(t *testing.T) {
	var gotArgs []string
	p := New(config.Systemd{Label: "Services", Units: []string{"wings.service", "playit.service"}})
	p.run = func(_ context.Context, name string, args ...string) ([]byte, error) {
		gotArgs = append([]string{name}, args...)
		return []byte(showOutput), nil
	}
	p.monoNow = func() (uint64, error) { return 65_000_000, nil }

	units, err := p.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(gotArgs, " "), "-- wings.service playit.service") {
		t.Errorf("args = %v", gotArgs)
	}
	if len(units) != 2 {
		t.Fatalf("sshd is not allowlisted and must be dropped; got %d units", len(units))
	}
	w := units[0]
	if w.Name != "wings" || w.State != provider.StateRunning || w.UptimeSec != 60 {
		t.Errorf("wings = %+v", w)
	}
	if w.MemBytes == nil || *w.MemBytes != 52428800 || w.Meta["restarts"] != "1" {
		t.Errorf("wings stats = %+v", w)
	}
	pl := units[1]
	if pl.State != provider.StateFailed || pl.MemBytes != nil || pl.Actions[0] != provider.ActionStart {
		t.Errorf("playit = %+v", pl)
	}
}

func TestDoAllowlist(t *testing.T) {
	var ran []string
	p := New(config.Systemd{Units: []string{"wings.service"}})
	p.run = func(_ context.Context, name string, args ...string) ([]byte, error) {
		ran = append([]string{name}, args...)
		return nil, nil
	}
	if err := p.Do(context.Background(), "sshd.service", "stop"); !errors.Is(err, provider.ErrUnknownUnit) {
		t.Errorf("non-allowlisted unit: %v", err)
	}
	if ran != nil {
		t.Fatal("systemctl must not run for a non-allowlisted unit")
	}
	if err := p.Do(context.Background(), "wings.service", "restart"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(ran, " ") != "systemctl --no-ask-password restart -- wings.service" {
		t.Errorf("ran %v", ran)
	}
}

func TestParseJournal(t *testing.T) {
	colored, _ := json.Marshal(map[string]string{
		"__REALTIME_TIMESTAMP": "1789552800123456",
		"PRIORITY":             "3",
		"MESSAGE":              "\x1b[31mfailed to bind\x1b[0m",
	})
	l, ok := parseJournal(colored)
	if !ok || l.T != 1789552800123 || !l.Err || l.Text != "failed to bind" {
		t.Errorf("got %+v", l)
	}
	l, ok = parseJournal([]byte(`{"__REALTIME_TIMESTAMP":"1","PRIORITY":"6","MESSAGE":[104,105,255]}`))
	if !ok || l.Err || l.Text != "hi?" {
		t.Errorf("byte message: %+v", l)
	}
}
