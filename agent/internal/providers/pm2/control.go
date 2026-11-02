package pm2

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"os/user"
	"strconv"
	"syscall"
	"time"
)

// The control socket is how dashd asks the helper to start, stop or restart
// a bot. dashd can't reach pm2 itself, so the helper does it on its behalf.
//
// Protocol: one JSON line each way per connection.
//   request: {"action":"restart","id":0,"name":"my-bot"}
//   reply:   {"ok":true} or {"error":"why"}
// The socket is mode 0600 plus an ACL entry for the dashd user, so only
// dashd (and root) can use it, and the helper needs no group membership: an
// extra group would also let it read /etc/dashd/config.yaml. The helper checks every request against pm2's live list:
// the pm2 id and the name must still belong together, so a request made from
// an old page can't hit a process that took over the id.

// ControlRequest is one action request from dashd.
type ControlRequest struct {
	Action string `json:"action"`
	ID     int    `json:"id"`
	Name   string `json:"name"`
}

type controlReply struct {
	OK    bool   `json:"ok,omitempty"`
	Error string `json:"error,omitempty"`
}

// ActionTimeout bounds one action. pm2 waits for a bot to exit before it
// starts it again, so a restart can take a few seconds.
const ActionTimeout = 60 * time.Second

func listenControl(path, allow string) (net.Listener, error) {
	// A socket left behind by a crash would block Listen.
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	fail := func(err error) (net.Listener, error) {
		ln.Close()
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fail(err)
	}
	if allow != "" {
		// A numeric uid works too, for accounts not in /etc/passwd.
		uid, err := strconv.Atoi(allow)
		if err != nil {
			u, lerr := user.Lookup(allow)
			if lerr != nil {
				return fail(fmt.Errorf("user %q: %w", allow, lerr))
			}
			if uid, err = strconv.Atoi(u.Uid); err != nil {
				return fail(err)
			}
		}
		if err := allowUser(path, uid); err != nil {
			return fail(fmt.Errorf("let %s use the control socket: %w", allow, err))
		}
	}
	return ln, nil
}

func (e *Exporter) serveControl(ctx context.Context, ln net.Listener) {
	go func() {
		<-ctx.Done()
		ln.Close()
	}()
	for {
		c, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return
			}
			e.opts.Log.Warn("control socket accept failed", "err", err)
			time.Sleep(200 * time.Millisecond)
			continue
		}
		go e.handleControl(ctx, c)
	}
}

func (e *Exporter) handleControl(ctx context.Context, c net.Conn) {
	defer c.Close()
	c.SetDeadline(time.Now().Add(ActionTimeout + 10*time.Second))
	line, err := bufio.NewReader(io.LimitReader(c, 1024)).ReadBytes('\n')
	if err != nil && len(line) == 0 {
		return
	}
	var req ControlRequest
	var reply controlReply
	if err := json.Unmarshal(line, &req); err != nil {
		reply.Error = "unreadable request"
	} else if err := e.act(ctx, req); err != nil {
		reply.Error = err.Error()
	} else {
		reply.OK = true
	}
	out, _ := json.Marshal(reply)
	c.Write(append(out, '\n'))
}

// act runs one request against pm2, one at a time. A second request while
// one runs is refused right away instead of queueing: a queued one could
// outlast dashd's wait and then still happen after the page reported a failure.
func (e *Exporter) act(ctx context.Context, req ControlRequest) error {
	switch req.Action {
	case "start", "stop", "restart":
	default:
		return fmt.Errorf("%q isn't something the helper can do", clean(req.Action, 20))
	}
	if !e.actMu.TryLock() {
		return errors.New("another bot action is still running. Try again in a few seconds")
	}
	defer e.actMu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, ActionTimeout)
	defer cancel()

	out, err := e.list(ctx)
	if err != nil {
		return errors.New(explain(err))
	}
	procs, err := ParseProcs(out)
	if err != nil {
		return err
	}
	found := false
	for _, p := range procs {
		if p.ID == req.ID && p.Name == req.Name {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("%s isn't in pm2 anymore. Reload the page", clean(req.Name, 64))
	}

	switch req.Action {
	case "start":
		_, err = e.rpc(ctx, "startProcessId", req.ID)
	case "stop":
		_, err = e.rpc(ctx, "stopProcessId", req.ID)
	case "restart":
		_, err = e.rpc(ctx, "restartProcessId", map[string]any{"id": req.ID, "env": map[string]any{}})
	}
	e.opts.Log.Info("pm2 action", "action", req.Action, "name", req.Name, "id", req.ID, "err", err)
	e.kickSoon()
	if err != nil {
		var refused pm2Error
		switch {
		case errors.As(err, &refused):
			return fmt.Errorf("pm2 said: %s", refused)
		case errors.Is(err, context.DeadlineExceeded) || os.IsTimeout(err):
			return fmt.Errorf("pm2 didn't finish the %s within %s. Check the bot's status", req.Action, ActionTimeout)
		}
		return errors.New(explain(err))
	}
	return nil
}

// kickSoon writes fresh snapshots now and over the next few seconds, so the
// page shows the bot going down and coming back without waiting a full tick.
func (e *Exporter) kickSoon() {
	kick := func() {
		select {
		case e.kick <- struct{}{}:
		default:
		}
	}
	kick()
	time.AfterFunc(2*time.Second, kick)
	time.AfterFunc(5*time.Second, kick)
}

// SendControl is dashd's side: one request over the helper's control socket.
func SendControl(ctx context.Context, sock string, req ControlRequest) error {
	var d net.Dialer
	c, err := d.DialContext(ctx, "unix", sock)
	switch {
	case errors.Is(err, syscall.ECONNREFUSED):
		// The socket file is kept across a restart, so this is the gap while
		// the helper comes back up.
		return errors.New("the dashd-pm2 helper is restarting. Try again in a few seconds")
	case errors.Is(err, fs.ErrNotExist):
		return errors.New("the dashd-pm2 helper isn't taking actions. On the server: cd ~/dashd && sudo ./deploy/update.sh (DEPLOY.md step 8b)")
	case errors.Is(err, fs.ErrPermission):
		return errors.New("dashd isn't allowed to use the helper's control socket. Check `-control-user dashd` in dashd-pm2.service and `getfacl /run/pm2-status/control.sock`")
	case err != nil:
		return err
	}
	defer c.Close()
	if dl, ok := ctx.Deadline(); ok {
		c.SetDeadline(dl)
	}
	line, _ := json.Marshal(req)
	if _, err := c.Write(append(line, '\n')); err != nil {
		return err
	}
	resp, err := bufio.NewReader(io.LimitReader(c, 4096)).ReadBytes('\n')
	if err != nil && len(resp) == 0 {
		if os.IsTimeout(err) {
			return errors.New("the helper didn't answer in time; the bot may still be restarting")
		}
		return fmt.Errorf("no answer from the helper: %w", err)
	}
	var reply controlReply
	if err := json.Unmarshal(resp, &reply); err != nil {
		return errors.New("the helper sent an unreadable answer")
	}
	if !reply.OK {
		if reply.Error == "" {
			reply.Error = "the helper refused"
		}
		return errors.New(clean(reply.Error, 300))
	}
	return nil
}
