package pm2

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/findiepman/dashd/internal/provider"
)

// Log streaming, like `pm2 logs`: the last lines of the output log, then of
// the error log under a header line, then new lines from both as they are
// written. pm2's log files have no timestamps, so old lines carry t=0 and new
// ones the time they were seen.

const (
	maxLogTail    = 2000
	maxLogLine    = 8 << 10 // longer lines are cut
	logPollEvery  = 500 * time.Millisecond
	maxLogStreams = 8
	// maxLogRead caps what one poll reads, so a burst can't stall the stream.
	maxLogRead = 1 << 20
)

// logLine is one streamed line, or Error when the stream can't start.
type logLine struct {
	T     int64  `json:"t"`
	Err   bool   `json:"err,omitempty"`
	Text  string `json:"text,omitempty"`
	Error string `json:"error,omitempty"`
}

func (e *Exporter) streamLogs(ctx context.Context, c net.Conn, req ControlRequest) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	enc := json.NewEncoder(c)
	send := func(l logLine) bool {
		c.SetWriteDeadline(time.Now().Add(10 * time.Second))
		return enc.Encode(l) == nil
	}
	fail := func(msg string) { send(logLine{Error: msg}) }

	select {
	case e.logSlots <- struct{}{}:
		defer func() { <-e.logSlots }()
	default:
		fail("too many log streams are open")
		return
	}
	c.SetReadDeadline(time.Time{})
	// dashd hangs up when the page stops following; notice it even while
	// the logs are quiet.
	go func() {
		io.Copy(io.Discard, c)
		cancel()
	}()

	lctx, lcancel := context.WithTimeout(ctx, 10*time.Second)
	out, err := e.list(lctx)
	lcancel()
	if err != nil {
		fail(explain(err))
		return
	}
	procs, err := ParseProcs(out)
	if err != nil {
		fail(err.Error())
		return
	}
	var proc *Proc
	for i := range procs {
		if procs[i].ID == req.ID && procs[i].Name == req.Name {
			proc = &procs[i]
		}
	}
	if proc == nil {
		fail(fmt.Sprintf("%s isn't in pm2 anymore", clean(req.Name, 64)))
		return
	}

	tail := min(max(req.Tail, 1), maxLogTail)
	files := []*logFile{{path: proc.outLog}}
	// pm2 lets out_file and error_file be the same file; follow it once.
	if !sameLog(proc.outLog, proc.errLog) {
		files = append(files, &logFile{path: proc.errLog, err: true})
	}
	started := false
	for _, f := range files {
		lines, err := f.open(tail)
		if err != nil {
			continue
		}
		if f.err && len(lines) > 0 {
			if !send(logLine{Text: "── error log (" + filepath.Base(f.path) + ") ──"}) {
				return
			}
			started = true
		}
		for _, l := range lines {
			if !send(logLine{Err: f.err, Text: l}) {
				return
			}
			started = true
		}
	}
	// The client waits for a first line to know the stream started.
	if !started && !send(logLine{Text: "(no output yet)"}) {
		return
	}

	t := time.NewTicker(logPollEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		now := time.Now().UnixMilli()
		for _, f := range files {
			for _, l := range f.poll() {
				if !send(logLine{T: now, Err: f.err, Text: l}) {
					return
				}
			}
		}
	}
}

// logFile follows one log file by path. pm2 truncates on `pm2 flush` and
// logrotate may replace the file, so a shrink or a new inode starts over.
type logFile struct {
	path    string
	err     bool
	ok      bool
	off     int64
	partial []byte
	info    os.FileInfo
	// missing means the file didn't exist when the stream opened, so when it
	// appears it is new and is read from the start.
	missing bool
	// skip drops bytes up to the next newline after starting mid-file.
	skip bool
}

func usableLog(path string) bool {
	return path != "" && path != "/dev/null"
}

func sameLog(a, b string) bool {
	if a == b {
		return true
	}
	sa, errA := os.Stat(a)
	sb, errB := os.Stat(b)
	return errA == nil && errB == nil && os.SameFile(sa, sb)
}

// open returns the last n lines and remembers where the file ends.
func (f *logFile) open(n int) ([]string, error) {
	if !usableLog(f.path) {
		return nil, errors.New("no log file")
	}
	st, err := os.Stat(f.path)
	if err != nil {
		f.missing = errors.Is(err, os.ErrNotExist)
		return nil, err
	}
	// Opening a FIFO with no writer would block forever.
	if !st.Mode().IsRegular() {
		return nil, errors.New("not a regular file")
	}
	// Lines are usually short; read generously and keep the last n.
	buf, st, err := readTail(f.path, int64(n)*512+maxLogLine)
	// st is the opened handle's: the path may have been swapped since the Stat.
	if err != nil || st == nil || !st.Mode().IsRegular() {
		return nil, errors.New("unreadable")
	}
	f.ok, f.off, f.info = true, st.Size(), st
	// A line still being written stays back until its newline arrives.
	if cut := bytes.LastIndexByte(buf, '\n'); cut < len(buf)-1 {
		f.partial = capLine(append([]byte(nil), buf[cut+1:]...))
		buf = buf[:cut+1]
	}
	lines := splitLines(buf)
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines, nil
}

// poll returns the complete lines written since the last call.
func (f *logFile) poll() []string {
	if !usableLog(f.path) {
		return nil
	}
	st, err := os.Stat(f.path)
	if err != nil || !st.Mode().IsRegular() {
		return nil
	}
	switch {
	case !f.ok && !f.missing:
		// It existed but the first read failed: don't replay a whole old
		// file, just follow it from here.
		f.ok, f.off, f.partial, f.info = true, st.Size(), nil, st
		return nil
	case !f.ok || st.Size() < f.off || (f.info != nil && !os.SameFile(f.info, st)):
		// New, truncated or replaced: read it from the start, or from the
		// last part if it is already big, so the page isn't flooded.
		f.ok, f.off, f.partial = true, max(0, st.Size()-logTail), nil
		f.skip = f.off > 0
	}
	f.info = st
	if st.Size() == f.off {
		return nil
	}
	fh, err := openLog(f.path)
	if err != nil {
		return nil
	}
	defer fh.Close()
	if hst, err := fh.Stat(); err != nil || !hst.Mode().IsRegular() {
		return nil
	}
	buf := make([]byte, min(st.Size()-f.off, maxLogRead))
	got, _ := fh.ReadAt(buf, f.off)
	f.off += int64(got)
	data := append(f.partial, buf[:got]...)
	if f.skip {
		// Started mid-file: drop the cut-off first line.
		i := bytes.IndexByte(data, '\n')
		if i < 0 {
			f.partial = nil
			return nil
		}
		data, f.skip = data[i+1:], false
	}
	cut := bytes.LastIndexByte(data, '\n')
	if cut < 0 {
		f.partial = capLine(data)
		return nil
	}
	f.partial = capLine(append([]byte(nil), data[cut+1:]...))
	return splitLines(data[:cut+1])
}

func capLine(b []byte) []byte {
	if len(b) > maxLogLine {
		return b[:maxLogLine]
	}
	return b
}

func splitLines(b []byte) []string {
	var out []string
	for _, line := range bytes.Split(bytes.TrimSuffix(b, []byte("\n")), []byte("\n")) {
		if len(b) == 0 {
			break
		}
		if len(line) > maxLogLine {
			line = line[:maxLogLine]
		}
		out = append(out, provider.CleanLine(string(bytes.ToValidUTF8(line, []byte("?")))))
	}
	return out
}

// streamFromHelper is dashd's side: open a logs stream on the control socket
// and turn it into provider log lines until ctx ends or the helper hangs up.
func streamFromHelper(ctx context.Context, sock string, req ControlRequest) (<-chan provider.LogLine, error) {
	var d net.Dialer
	c, err := d.DialContext(ctx, "unix", sock)
	if err != nil {
		return nil, controlDialError(err)
	}
	// Closing the conn when the page stops following also ends a wait for
	// the first line.
	stop := context.AfterFunc(ctx, func() { c.Close() })
	fail := func(err error) (<-chan provider.LogLine, error) {
		stop()
		c.Close()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	line, _ := json.Marshal(req)
	c.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if _, err := c.Write(append(line, '\n')); err != nil {
		return fail(err)
	}
	dec := json.NewDecoder(bufio.NewReader(c))
	// The first line says whether the stream started.
	c.SetReadDeadline(time.Now().Add(15 * time.Second))
	var first logLine
	if err := dec.Decode(&first); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return fail(errors.New("the dashd-pm2 helper closed the log stream before it started"))
		}
		return fail(fmt.Errorf("no logs from the helper: %w", err))
	}
	if first.Error != "" {
		// A helper from before log streaming refuses the unknown action.
		if strings.Contains(first.Error, `"logs" isn't`) {
			return fail(errors.New("the dashd-pm2 helper can't stream logs yet. On the server, run deploy/update.sh in the dashd checkout"))
		}
		return fail(errors.New(clean(first.Error, 300)))
	}
	c.SetReadDeadline(time.Time{})

	out := make(chan provider.LogLine, 256)
	go func() {
		defer close(out)
		defer stop()
		defer c.Close()
		l := first
		for {
			pl := provider.LogLine{T: l.T, Err: l.Err, Text: capText(l.Text)}
			select {
			case out <- pl:
			case <-ctx.Done():
				return
			}
			l = logLine{}
			if err := dec.Decode(&l); err != nil {
				if ctx.Err() == nil {
					select {
					case out <- provider.LogLine{T: time.Now().UnixMilli(), Err: true, Text: "The log stream ended (the dashd-pm2 helper restarted or stopped)."}:
					case <-ctx.Done():
					}
				}
				return
			}
			if l.Error != "" {
				l = logLine{T: time.Now().UnixMilli(), Err: true, Text: l.Error}
			}
		}
	}()
	return out, nil
}

// capText bounds a log line from the helper. Unlike clean() it keeps tabs
// and leading spaces, so stack traces stay indented; the helper already
// stripped ANSI codes and made it valid UTF-8.
func capText(s string) string {
	if len(s) > maxLogLine {
		s = strings.ToValidUTF8(s[:maxLogLine], "")
	}
	return s
}
