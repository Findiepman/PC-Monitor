package pm2

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/findiepman/dashd/internal/provider"
)

func appendTo(t *testing.T, path, s string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(s)
	f.Close()
}

func TestLogFileFollow(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bot-out.log")
	for i := range 30 {
		appendTo(t, path, fmt.Sprintf("line %d\n", i))
	}
	f := &logFile{path: path}
	lines, err := f.open(5)
	if err != nil || strings.Join(lines, ",") != "line 25,line 26,line 27,line 28,line 29" {
		t.Fatalf("tail = %v, %v", lines, err)
	}
	if got := f.poll(); got != nil {
		t.Errorf("nothing new yet: %v", got)
	}
	appendTo(t, path, "\x1b[32mgreen\x1b[0m\nhalf a li")
	if got := f.poll(); strings.Join(got, "|") != "green" {
		t.Errorf("new lines = %q", got)
	}
	appendTo(t, path, "ne\n")
	if got := f.poll(); strings.Join(got, "|") != "half a line" {
		t.Errorf("a line written in two goes = %q", got)
	}
	// `pm2 flush` empties the file.
	os.WriteFile(path, []byte("after flush\n"), 0o644)
	if got := f.poll(); strings.Join(got, "|") != "after flush" {
		t.Errorf("after truncate = %q", got)
	}
	// logrotate moves it away and a new file appears. Windows' SameFile
	// re-reads by path, so this only shows on Linux (the server).
	if runtime.GOOS != "linux" {
		return
	}
	os.Rename(path, path+".1")
	appendTo(t, path, "fresh file with a longer first line than before\n")
	if got := f.poll(); len(got) != 1 || got[0] != "fresh file with a longer first line than before" {
		t.Errorf("after rotation = %q", got)
	}

	if _, err := (&logFile{path: "/dev/null"}).open(5); err == nil {
		t.Error("/dev/null isn't a log")
	}
	if _, err := (&logFile{path: dir}).open(5); err == nil {
		t.Error("a directory isn't a log")
	}
}

// A tail window inside one huge line has nothing whole to show.
func TestTailInsideHugeLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "huge.log")
	appendTo(t, path, "first\n"+strings.Repeat("y", 64<<10))
	buf, _, err := readTail(path, 1024)
	if err != nil || len(buf) != 0 {
		t.Errorf("tail = %d bytes, %v", len(buf), err)
	}
}

func TestLogLinesAreCapped(t *testing.T) {
	path := filepath.Join(t.TempDir(), "long.log")
	appendTo(t, path, "start\n")
	f := &logFile{path: path}
	if _, err := f.open(10); err != nil {
		t.Fatal(err)
	}
	appendTo(t, path, strings.Repeat("x", 3*maxLogLine)+"\nshort\n")
	lines := f.poll()
	if len(lines) != 2 || len(lines[0]) != maxLogLine || lines[1] != "short" {
		t.Fatalf("lines = %d", len(lines))
	}
}

// The whole path: dashd's streamFromHelper talks to the helper's handler over
// a unix socket, the helper reads the files pm2 names.
func TestLogStream(t *testing.T) {
	dir := t.TempDir()
	out, errLog := filepath.Join(dir, "bot-out.log"), filepath.Join(dir, "bot-error.log")
	appendTo(t, out, "Ready! Logged in as Bot#0001\n")
	appendTo(t, errLog, "TypeError: boom\n")
	list := `[{"name":"bot","pm_id":4,"pm2_env":{"status":"online","pm_out_log_path":` + strconvQuote(out) +
		`,"pm_err_log_path":` + strconvQuote(errLog) + `}}]`

	e := testExporter(t)
	e.list = func(context.Context) ([]byte, error) { return []byte(list), nil }
	sock := filepath.Join(dir, "control.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Skip("no unix sockets here:", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go e.serveControl(ctx, ln)

	sctx, stop := context.WithCancel(ctx)
	lines, err := streamFromHelper(sctx, sock, ControlRequest{Action: "logs", ID: 4, Name: "bot", Tail: 50})
	if err != nil {
		t.Fatal(err)
	}
	next := func() provider.LogLine {
		select {
		case l := <-lines:
			return l
		case <-time.After(5 * time.Second):
			t.Fatal("no line within 5s")
		}
		return provider.LogLine{}
	}
	if l := next(); l.Text != "Ready! Logged in as Bot#0001" || l.Err || l.T != 0 {
		t.Errorf("first = %+v", l)
	}
	if l := next(); !strings.Contains(l.Text, "error log (bot-error.log)") {
		t.Errorf("header = %+v", l)
	}
	if l := next(); l.Text != "TypeError: boom" || !l.Err {
		t.Errorf("err line = %+v", l)
	}
	appendTo(t, out, "a new line\n")
	if l := next(); l.Text != "a new line" || l.T == 0 {
		t.Errorf("followed line = %+v", l)
	}
	stop()
	for range lines { // the channel closes once the page stops following
	}
	// The helper notices the hang-up and frees the slot.
	deadline := time.Now().Add(3 * time.Second)
	for len(e.logSlots) != 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if len(e.logSlots) != 0 {
		t.Error("the log stream slot was never released")
	}

	if _, err := streamFromHelper(ctx, sock, ControlRequest{Action: "logs", ID: 9, Name: "gone", Tail: 5}); err == nil ||
		!strings.Contains(err.Error(), "isn't in pm2") {
		t.Errorf("unknown bot: %v", err)
	}
}

// A helper from before log streaming answers {"error":"\"logs\" isn't ..."}.
func TestLogStreamOldHelper(t *testing.T) {
	dir, err := os.MkdirTemp("", "pm2o")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	sock := filepath.Join(dir, "c.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Skip("no unix sockets here:", err)
	}
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		buf := make([]byte, 512)
		c.Read(buf)
		c.Write([]byte(`{"error":"\"logs\" isn't something the helper can do"}` + "\n"))
	}()
	_, err = streamFromHelper(context.Background(), sock, ControlRequest{Action: "logs", ID: 0, Name: "x", Tail: 5})
	if err == nil || !strings.Contains(err.Error(), "can't stream logs yet") {
		t.Errorf("old helper: %v", err)
	}
}
