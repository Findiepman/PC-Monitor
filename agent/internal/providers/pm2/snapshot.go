// Package pm2 shows the processes pm2 runs (the Discord bots) on the
// dashboard and lets you start, stop and restart them.
//
// dashd can't ask pm2 itself: pm2 belongs to another user, and dashd's
// service sandbox (ProtectHome, NoNewPrivileges) rules out both reading that
// user's ~/.pm2 and sudo. So `dashd pm2-export` runs as the pm2 user. It asks
// the pm2 daemon for its process list (rpc.go), writes a filtered snapshot to
// a file every few seconds that the provider reads, and takes start, stop and
// restart requests from dashd on a control socket (control.go). The filtering
// matters: pm2's process list includes every process's full environment,
// which can hold bot tokens. Only the fields below ever leave the helper.
package pm2

import (
	"bytes"
	"encoding/json"
	"errors"
	"path"
	"strings"
	"unicode"
)

// Snapshot is the file the exporter writes and the provider reads.
type Snapshot struct {
	// T is when the snapshot was taken, unix milliseconds.
	T int64 `json:"t"`
	// Every is the exporter's interval in milliseconds, so the reader knows
	// when a snapshot counts as stale.
	Every int64 `json:"every,omitempty"`
	// Error is set when pm2 couldn't be read; Procs is empty then.
	Error string `json:"error,omitempty"`
	Procs []Proc `json:"procs"`
}

type Proc struct {
	Name string `json:"name"`
	// ID is pm2's own process id. Names can repeat (cluster mode), ids can't.
	ID     int    `json:"id"`
	Status string `json:"status"`
	// Started is pm2's pm_uptime, unix milliseconds: when the process last
	// (re)started.
	Started  int64    `json:"started,omitempty"`
	CPU      *float64 `json:"cpu,omitempty"`
	Mem      *uint64  `json:"mem,omitempty"`
	Restarts int      `json:"restarts"`
	// Runtime is the interpreter's base name, e.g. "node" or "python".
	Runtime string `json:"runtime,omitempty"`
	// Accounts are the bot account names this process logs in, when the
	// exporter was told where to find them.
	Accounts []string `json:"accounts,omitempty"`
	// Discord is the name the bot logged in to Discord as, read from its own
	// log ("Logged in as AftermathCoordinator#3579").
	Discord string `json:"discord,omitempty"`
	// Logins maps an account to its Discord name, for processes that sign in
	// several bots ("Manager is signed in as 'Sigmabot1'").
	Logins map[string]string `json:"logins,omitempty"`
	// outLog is pm2's stdout log for the process; it never leaves the helper.
	outLog string
}

// jlistProc picks out the fields we use from pm2's process list. Everything
// else in pm2_env, the environment included, is never decoded.
type jlistProc struct {
	Name  string `json:"name"`
	PMID  int    `json:"pm_id"`
	Monit *struct {
		Memory *uint64  `json:"memory"`
		CPU    *float64 `json:"cpu"`
	} `json:"monit"`
	Env struct {
		Status      string `json:"status"`
		Uptime      int64  `json:"pm_uptime"`
		Restarts    int    `json:"restart_time"`
		Interpreter string `json:"exec_interpreter"`
		OutLog      string `json:"pm_out_log_path"`
	} `json:"pm2_env"`
}

// ParseProcs reads pm2's process list: the getMonitorData result, which is
// also what `pm2 jlist` prints.
func ParseProcs(out []byte) ([]Proc, error) {
	var raw []jlistProc
	if err := json.Unmarshal(out, &raw); err != nil || raw == nil {
		return nil, errors.New("pm2 sent no readable process list")
	}
	procs := make([]Proc, 0, len(raw))
	for _, r := range raw {
		p := Proc{
			Name:     clean(r.Name, 64),
			ID:       r.PMID,
			Status:   r.Env.Status,
			Started:  r.Env.Uptime,
			Restarts: r.Env.Restarts,
			Runtime:  runtimeName(r.Env.Interpreter),
			outLog:   r.Env.OutLog,
		}
		if r.Monit != nil {
			p.CPU, p.Mem = r.Monit.CPU, r.Monit.Memory
		}
		procs = append(procs, p)
	}
	return procs, nil
}

// runtimeName turns "./venv/bin/python" into "python". pm2 uses "none" for
// processes that run a binary directly.
func runtimeName(interp string) string {
	if interp == "" || interp == "none" {
		return ""
	}
	return clean(path.Base(strings.ReplaceAll(interp, `\`, "/")), 32)
}

const maxAccounts = 50

// AccountNames returns the top-level keys of a bots.json style file, in file
// order. Only the keys are read; the values (the tokens) are skipped without
// being kept.
func AccountNames(raw []byte) ([]string, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, errors.New("not a JSON object")
	}
	var names []string
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, _ := tok.(string)
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			return nil, err
		}
		if k := clean(key, 64); k != "" && len(names) < maxAccounts {
			names = append(names, k)
		}
	}
	return names, nil
}

// clean drops control characters and caps the length, so a strange name
// can't break the snapshot or the page layout.
func clean(s string, max int) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > max {
		s = string(r[:max])
	}
	return s
}
