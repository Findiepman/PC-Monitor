package pm2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// ExportOptions configures `dashd pm2-export`.
type ExportOptions struct {
	// Out is the snapshot file dashd reads.
	Out   string
	Every time.Duration
	// Home is PM2_HOME (default $PM2_HOME, then ~/.pm2).
	Home string
	// Accounts maps a pm2 process name to a bots.json style file whose
	// top-level keys are the bot accounts that process logs in.
	Accounts map[string]string
	Log      *slog.Logger
}

// Exporter takes snapshots. Its fields are swappable for tests.
type Exporter struct {
	opts ExportOptions
	// list asks the pm2 daemon for its processes (see rpc.go).
	list     func(ctx context.Context) ([]byte, error)
	readFile func(string) ([]byte, error)
	now      func() time.Time
	// lastWarn keeps a repeating problem from filling the journal.
	lastWarn map[string]string
}

func NewExporter(o ExportOptions) *Exporter {
	if o.Every < time.Second {
		o.Every = 10 * time.Second
	}
	if o.Home == "" {
		if h := os.Getenv("PM2_HOME"); h != "" {
			o.Home = h
		} else if h, err := os.UserHomeDir(); err == nil {
			o.Home = filepath.Join(h, ".pm2")
		}
	}
	if o.Log == nil {
		o.Log = slog.Default()
	}
	e := &Exporter{opts: o, readFile: os.ReadFile, now: time.Now, lastWarn: map[string]string{}}
	e.list = func(ctx context.Context) ([]byte, error) {
		return monitorData(ctx, filepath.Join(e.opts.Home, "rpc.sock"))
	}
	return e
}

// Run writes a snapshot now and then every o.Every until ctx ends.
func (e *Exporter) Run(ctx context.Context) error {
	e.opts.Log.Info("pm2-export running", "out", e.opts.Out, "every", e.opts.Every, "pm2_home", e.opts.Home)
	t := time.NewTicker(e.opts.Every)
	defer t.Stop()
	for {
		if err := WriteSnapshot(e.opts.Out, e.Take(ctx)); err != nil {
			e.warn("write", "can't write the snapshot", err)
		} else {
			e.warn("write", "", nil)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
		}
	}
}

// Take reads pm2 once. Problems end up in Snapshot.Error, which the
// dashboard shows, so it never fails.
func (e *Exporter) Take(ctx context.Context) (snap Snapshot) {
	snap.Procs = []Proc{}
	snap.Every = e.opts.Every.Milliseconds()
	// Named result: the deferred stamp lands in what is returned.
	defer func() { snap.T = e.now().UnixMilli() }()

	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := e.list(cctx)
	if err != nil {
		e.warn("pm2", "can't read pm2", err)
		snap.Error = explain(err)
		return snap
	}
	e.warn("pm2", "", nil)
	procs, err := ParseProcs(out)
	if err != nil {
		snap.Error = err.Error()
		return snap
	}
	for i := range procs {
		file, ok := e.opts.Accounts[procs[i].Name]
		if !ok {
			continue
		}
		names, err := e.accounts(file)
		e.warn("accounts:"+file, "can't read bot account names", err)
		procs[i].Accounts = names
	}
	snap.Procs = procs
	return snap
}

func (e *Exporter) accounts(file string) ([]string, error) {
	raw, err := e.readFile(file)
	if err != nil {
		return nil, err
	}
	return AccountNames(raw)
}

// warn logs err once per key until it changes or clears.
func (e *Exporter) warn(key, msg string, err error) {
	s := ""
	if err != nil {
		s = err.Error()
	}
	if e.lastWarn[key] == s {
		return
	}
	e.lastWarn[key] = s
	if err != nil {
		e.opts.Log.Warn(msg, "err", err)
	}
}

// explain turns a pm2 connection error into the line the page shows. Known
// cases get fixed text, so paths and library wording stay in the journal.
func explain(err error) string {
	switch {
	case errors.Is(err, fs.ErrNotExist), errors.Is(err, syscall.ECONNREFUSED):
		return "pm2 isn't running for this user. Start it with: systemctl start pm2-" + currentUser()
	case errors.Is(err, fs.ErrPermission):
		return "dashd-pm2 can't reach pm2. Run it as the user pm2 runs under."
	case errors.Is(err, context.DeadlineExceeded), os.IsTimeout(err):
		return "pm2 didn't answer within 10 seconds."
	}
	return fmt.Sprintf("couldn't read pm2: %s", clean(err.Error(), 200))
}

func currentUser() string {
	if u := clean(os.Getenv("USER"), 32); u != "" {
		return u
	}
	return "<user>"
}

// WriteSnapshot replaces path atomically, so dashd never reads half a file.
// The file is world-readable: it only holds the filtered fields.
func WriteSnapshot(path string, snap Snapshot) error {
	raw, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".pm2-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
