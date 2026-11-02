package pm2

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/findiepman/dashd/internal/config"
	"github.com/findiepman/dashd/internal/provider"
)

// Field shapes copied from pm2's process list (pm2 6.0.14) on the server; the env is
// cut down and its secret is fake. pm2 puts the process environment straight
// into pm2_env and again under pm2_env.env.
const jlist = `[{"pid":3335,"name":"my-bot","pm2_env":{"DISCORD_TOKEN":"FAKE-SECRET-TOKEN","HOME":"/home/fin",` +
	`"env":{"DISCORD_TOKEN":"FAKE-SECRET-TOKEN"},"status":"online","pm_uptime":1793574033265,"restart_time":2,` +
	`"unstable_restarts":0,"exec_interpreter":"node","pm_cwd":"/home/fin/Aftermath-Bot","created_at":1774184328676},` +
	`"pm_id":0,"monit":{"memory":95371264,"cpu":0.2}},` +
	`{"pid":1863,"name":"fin-and-co","pm2_env":{"status":"online","pm_uptime":1793574027095,"restart_time":0,` +
	`"exec_interpreter":"./venv/bin/python","exit_code":null},"pm_id":1,"monit":{"memory":56406016,"cpu":0.1}},` +
	`{"pid":0,"name":"old-bot","pm2_env":{"status":"stopped","pm_uptime":1793000000000,"restart_time":14,` +
	`"exec_interpreter":"none"},"pm_id":2,"monit":{"memory":0,"cpu":0}}]`

const botsJSON = `{"Manager": {"token": "FAKE-SECRET-TOKEN", "status": "Managing Fin & Co"},
 "Daryl": {"token": "FAKE-SECRET-TOKEN", "status": "Hunting bugs"},
 "Michonne": "FAKE-SECRET-TOKEN"}`

func TestParseProcs(t *testing.T) {
	procs, err := ParseProcs([]byte(jlist))
	if err != nil {
		t.Fatal(err)
	}
	if len(procs) != 3 {
		t.Fatalf("got %d procs", len(procs))
	}
	b := procs[0]
	if b.Name != "my-bot" || b.ID != 0 || b.Status != "online" || b.Started != 1793574033265 ||
		b.Restarts != 2 || b.Runtime != "node" || *b.Mem != 95371264 || *b.CPU != 0.2 {
		t.Errorf("my-bot = %+v", b)
	}
	if procs[1].Runtime != "python" {
		t.Errorf("runtime = %q, want python", procs[1].Runtime)
	}
	if procs[2].Runtime != "" {
		t.Errorf(`"none" interpreter should give no runtime, got %q`, procs[2].Runtime)
	}
	if procs, err := ParseProcs([]byte("[]")); err != nil || len(procs) != 0 {
		t.Errorf("empty list: %v %v", procs, err)
	}
	for _, bad := range []string{"", "null", "{}", "[{\"name\":"} {
		if _, err := ParseProcs([]byte(bad)); err == nil {
			t.Errorf("%q should fail", bad)
		}
	}
}

func TestAccountNames(t *testing.T) {
	names, err := AccountNames([]byte(botsJSON))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(names, ",") != "Manager,Daryl,Michonne" {
		t.Errorf("names = %v, want file order", names)
	}
	for _, bad := range []string{"", "[]", `{"a":`, "nope"} {
		if _, err := AccountNames([]byte(bad)); err == nil {
			t.Errorf("%q should fail", bad)
		}
	}
	names, _ = AccountNames([]byte("{\"  Bad\\u0007Name\\n \": 1, \"\": 2}"))
	if len(names) != 1 || names[0] != "BadName" {
		t.Errorf("control chars and empty keys: %q", names)
	}
}

func TestSnapshotNeverHoldsSecrets(t *testing.T) {
	e := testExporter(t)
	raw, _ := json.Marshal(e.Take(context.Background()))
	if strings.Contains(string(raw), "FAKE-SECRET") || strings.Contains(string(raw), "/home/fin") {
		t.Fatalf("snapshot leaked env or token: %s", raw)
	}
	if !strings.Contains(string(raw), `"accounts":["Manager","Daryl","Michonne"]`) {
		t.Errorf("accounts missing: %s", raw)
	}
}

func testExporter(t *testing.T) *Exporter {
	t.Helper()
	e := NewExporter(ExportOptions{Home: "/nowhere", Accounts: map[string]string{"fin-and-co": "bots.json"}})
	e.list = func(context.Context) ([]byte, error) { return []byte(jlist), nil }
	e.readFile = func(string) ([]byte, error) { return []byte(botsJSON), nil }
	e.now = func() time.Time { return time.UnixMilli(1793616339109) }
	return e
}

func TestTake(t *testing.T) {
	e := testExporter(t)
	snap := e.Take(context.Background())
	if snap.T != 1793616339109 || snap.Every != 10000 || snap.Error != "" || len(snap.Procs) != 3 {
		t.Fatalf("snap = %+v", snap)
	}
	if snap.Procs[0].Accounts != nil {
		t.Error("my-bot has no accounts file")
	}

	for err, want := range map[error]string{
		fs.ErrNotExist:              "pm2 isn't running",
		syscall.ECONNREFUSED:        "pm2 isn't running",
		fs.ErrPermission:            "can't reach pm2",
		context.DeadlineExceeded:    "within 10 seconds",
		errors.New("pm2 [31mboom"): "couldn't read pm2: pm2 [31mboom",
	} {
		e.list = func(context.Context) ([]byte, error) { return nil, err }
		snap = e.Take(context.Background())
		if !strings.Contains(snap.Error, want) || snap.T == 0 || snap.Procs == nil {
			t.Errorf("%v: snap = %+v, want %q", err, snap, want)
		}
	}

	// An unreadable accounts file drops the names, not the process.
	e.list = func(context.Context) ([]byte, error) { return []byte(jlist), nil }
	e.readFile = func(string) ([]byte, error) { return nil, fs.ErrPermission }
	if snap = e.Take(context.Background()); snap.Error != "" || len(snap.Procs) != 3 || snap.Procs[1].Accounts != nil {
		t.Errorf("bad accounts file: %+v", snap)
	}
}

// fakeDaemon answers like pm2's rpc.sock. Reply frames copy what pm2 6.0.14
// sent on the server: [j:{"args":[[...]]}, s:<id>].
func fakeDaemon(t *testing.T, reply func(id []byte) [][]byte) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "pm2t")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	sock := filepath.Join(dir, "rpc.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Skip("no unix sockets here:", err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				args, err := readFrame(c)
				if err != nil || len(args) != 2 || !strings.Contains(string(args[0]), `"method":"getMonitorData"`) {
					return
				}
				for _, f := range reply(args[1]) {
					c.Write(f)
				}
				io.Copy(io.Discard, c) // stay connected like the daemon does
			}()
		}
	}()
	return sock
}

func TestMonitorData(t *testing.T) {
	sock := fakeDaemon(t, func(id []byte) [][]byte {
		return [][]byte{
			encodeFrame([]byte(`j:{"args":[[]]}`), []byte("s:someone-else:1")), // not ours: skipped
			encodeFrame([]byte(`j:{"args":[null,`+jlist+`]}`), id),
		}
	})
	out, err := monitorData(context.Background(), sock)
	if err != nil {
		t.Fatal(err)
	}
	if procs, err := ParseProcs(out); err != nil || len(procs) != 3 {
		t.Fatalf("procs = %d, %v", len(procs), err)
	}

	refused := fakeDaemon(t, func(id []byte) [][]byte {
		return [][]byte{encodeFrame([]byte(`j:{"message":"no such method"}`), id)}
	})
	if _, err := monitorData(context.Background(), refused); err == nil {
		t.Error("an error reply should fail")
	}

	silent := fakeDaemon(t, func([]byte) [][]byte { return nil })
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if _, err := monitorData(ctx, silent); err == nil || !os.IsTimeout(err) {
		t.Errorf("silent daemon: %v", err)
	}

	// Windows reports a missing socket with its own errno; the server is Linux.
	if _, err := monitorData(context.Background(), filepath.Join(t.TempDir(), "rpc.sock")); runtime.GOOS == "linux" &&
		!strings.Contains(explain(err), "isn't running") {
		t.Errorf("no daemon: %v -> %q", err, explain(err))
	}
}

func TestReadFrame(t *testing.T) {
	f := encodeFrame([]byte("j:{}"), []byte("s:x"))
	if f[0] != 0x12 {
		t.Errorf("header = %#x, pm2 sends 0x12 for two args", f[0])
	}
	args, err := readFrame(strings.NewReader(string(f)))
	if err != nil || len(args) != 2 || string(args[1]) != "s:x" {
		t.Fatalf("round trip: %q %v", args, err)
	}
	huge := []byte{0x11, 0xff, 0xff, 0xff, 0xff}
	if _, err := readFrame(strings.NewReader(string(huge))); err == nil {
		t.Error("an oversized argument must be refused before allocating")
	}
	if _, err := readFrame(strings.NewReader("!")); err == nil {
		t.Error("unknown version should fail")
	}
}

func TestWriteSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pm2.json")
	for i := range 2 { // the second write replaces the first
		if err := WriteSnapshot(path, Snapshot{T: int64(i), Procs: []Proc{}}); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) != `{"t":1,"procs":[]}` {
		t.Fatalf("file = %s, %v", raw, err)
	}
	left, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".pm2-*"))
	if len(left) != 0 {
		t.Errorf("temp files left: %v", left)
	}
}

func newTestProvider(snap *Snapshot, err error) (*Provider, time.Time) {
	now := time.UnixMilli(1793616339109)
	p := NewWithReader(config.PM2{Label: "Discord bots", Path: "/run/pm2-status/pm2.json", Exclude: []string{" Old-Bot "}},
		func() ([]byte, error) {
			if err != nil {
				return nil, err
			}
			return json.Marshal(snap)
		})
	p.now = func() time.Time { return now }
	return p, now
}

func TestList(t *testing.T) {
	procs, _ := ParseProcs([]byte(jlist))
	procs[1].Accounts = []string{"Manager", "Daryl"}
	p, now := newTestProvider(&Snapshot{T: 1793616339109 - 4000, Procs: procs}, nil)
	units, err := p.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 2 {
		t.Fatalf("exclude should hide old-bot, case-insensitive: %+v", units)
	}
	u := units[0]
	wantUp := int64(now.Sub(time.UnixMilli(1793574033265)) / time.Second)
	if u.ID != "my-bot" || u.State != provider.StateRunning || u.Kind != "bot" || u.UptimeSec != wantUp ||
		u.Meta["restarts"] != "2" || u.Meta["description"] != "node" || len(u.Actions) != 0 || *u.MemBytes != 95371264 {
		t.Errorf("my-bot = %+v", u)
	}
	if units[1].Meta["accounts"] != "Manager\nDaryl" {
		t.Errorf("accounts = %q", units[1].Meta["accounts"])
	}
	if p.Do(context.Background(), "my-bot", provider.ActionRestart) != provider.ErrBadAction {
		t.Error("pm2 must stay view-only")
	}
	if _, err := p.Logs(context.Background(), "my-bot", 10); err == nil {
		t.Error("logs should explain they aren't available")
	}
}

func TestListProblems(t *testing.T) {
	cases := []struct {
		name string
		snap *Snapshot
		err  error
		want string
	}{
		{"no file", nil, fs.ErrNotExist, "dashd-pm2.service"},
		{"stale", &Snapshot{T: 1793616339109 - 46_000}, nil, "46s old"},
		{"slow exporter, still fresh", &Snapshot{T: 1793616339109 - 46_000, Every: 60_000, Error: "fresh"}, nil, "fresh"},
		{"slow exporter, stale", &Snapshot{T: 1793616339109 - 181_000, Every: 60_000}, nil, "3m1s old"},
		{"from the future", &Snapshot{T: 1793616339109 + 3_600_000}, nil, "in the future"},
		{"never stamped", &Snapshot{}, nil, "old"},
		{"pm2 error", &Snapshot{T: 1793616339109, Error: "pm2 isn't running"}, nil, "pm2 isn't running"},
	}
	for _, c := range cases {
		p, _ := newTestProvider(c.snap, c.err)
		if _, err := p.List(context.Background()); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want %q", c.name, err, c.want)
		}
	}
	p := NewWithReader(config.PM2{}, func() ([]byte, error) { return []byte("{half"), nil })
	if _, err := p.List(context.Background()); err == nil {
		t.Error("corrupt file should fail")
	}
}

func TestUnitsStatesAndIDs(t *testing.T) {
	now := time.UnixMilli(2_000_000_000_000)
	procs := []Proc{
		{Name: "bot", ID: 3, Status: "online", Started: now.UnixMilli() + 5000, CPU: provider.Ptr(1.0)},
		{Name: "bot", ID: 4, Status: "errored", CPU: provider.Ptr(9.0)},
		{Name: "bot-3", ID: 5, Status: "waiting restart"},
		{Name: "", ID: 6, Status: "online"},
	}
	units := Units(procs, nil, now)
	if len(units) != 3 {
		t.Fatalf("nameless process should be skipped: %+v", units)
	}
	ids := map[string]bool{}
	for _, u := range units {
		if ids[u.ID] {
			t.Errorf("duplicate id %q", u.ID)
		}
		ids[u.ID] = true
	}
	if units[0].Name != "bot #3" || units[0].UptimeSec != 0 {
		t.Errorf("cluster name / future start: %+v", units[0])
	}
	if units[1].State != provider.StateFailed || units[1].CPU != nil {
		t.Errorf("errored bot must not show as running or carry cpu: %+v", units[1])
	}
	if units[2].State != provider.StateRestarting {
		t.Errorf("waiting restart = %s", units[2].State)
	}
	for in, want := range map[string]string{"online": "running", "launching": "starting", "stopping": "stopping",
		"stopped": "stopped", "one-launch-status": "stopped", "errored": "failed", "weird": "unknown"} {
		if got := MapStatus(in); got != want {
			t.Errorf("MapStatus(%q) = %q, want %q", in, got, want)
		}
	}
}
