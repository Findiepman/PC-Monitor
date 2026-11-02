package pm2

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync/atomic"
	"time"
)

// The helper talks to the running pm2 daemon over its rpc.sock instead of
// running `pm2 jlist`. The CLI starts a fresh daemon whenever it can't reach
// one, so a jlist that lands while pm2 restarts could leave the bots running
// under this exporter's service. Connecting to a socket can never start
// anything, and it skips a Node start-up every few seconds.
//
// The wire format is pm2's (pm2-axon-rpc over axon's "amp" framing):
//   frame = 1 byte (version 1 << 4 | arg count), then per arg a 4-byte
//           big-endian length and the bytes; "j:" prefixes JSON, "s:" a string.
//   call  = [j:{"type":"call","method":M,"args":[...]}, s:<request id>]
//   reply = [j:{"args":[result...]}, s:<request id>], or
//           [j:{"error":"message","stack":...}, s:<request id>] when it failed.
// Checked against pm2 6.0.14 on the server.

const (
	ampVersion = 1
	// maxArg caps one frame argument. getMonitorData carries every process's
	// environment (a few hundred KB here); decoding needs a few copies, so this
	// stays well inside the unit's MemoryMax=128M.
	maxArg = 16 << 20
)

var callSeq atomic.Uint64

func encodeFrame(args ...[]byte) []byte {
	out := []byte{byte(ampVersion<<4 | len(args))}
	for _, a := range args {
		out = binary.BigEndian.AppendUint32(out, uint32(len(a)))
		out = append(out, a...)
	}
	return out
}

func readFrame(r io.Reader) ([][]byte, error) {
	var head [1]byte
	if _, err := io.ReadFull(r, head[:]); err != nil {
		return nil, err
	}
	if head[0]>>4 != ampVersion {
		return nil, fmt.Errorf("unknown pm2 frame version %d", head[0]>>4)
	}
	args := make([][]byte, head[0]&0x0f)
	for i := range args {
		var n [4]byte
		if _, err := io.ReadFull(r, n[:]); err != nil {
			return nil, err
		}
		size := binary.BigEndian.Uint32(n[:])
		if size > maxArg {
			return nil, fmt.Errorf("pm2 reply is larger than %d MB", maxArg>>20)
		}
		args[i] = make([]byte, size)
		if _, err := io.ReadFull(r, args[i]); err != nil {
			return nil, err
		}
	}
	return args, nil
}

// call runs one pm2 RPC method on the daemon behind sock and returns the
// results (the callback's arguments after err).
func call(ctx context.Context, sock, method string, args ...any) ([]json.RawMessage, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", sock)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(10 * time.Second)
	}
	conn.SetDeadline(deadline)

	id := "dashd:" + strconv.FormatUint(callSeq.Add(1), 10)
	if args == nil {
		args = []any{}
	}
	req, err := json.Marshal(map[string]any{"type": "call", "method": method, "args": args})
	if err != nil {
		return nil, err
	}
	if _, err := conn.Write(encodeFrame(append([]byte("j:"), req...), []byte("s:"+id))); err != nil {
		return nil, err
	}
	br := bufio.NewReader(conn)
	for {
		frame, err := readFrame(br)
		if err != nil {
			return nil, err
		}
		// The daemon could interleave other replies; only ours counts.
		if len(frame) < 2 || string(frame[len(frame)-1]) != "s:"+id {
			continue
		}
		return parseReply(frame[0])
	}
}

// monitorData returns pm2's process list as a raw JSON array (the same shape
// `pm2 jlist` prints).
func monitorData(ctx context.Context, sock string) ([]byte, error) {
	res, err := call(ctx, sock, "getMonitorData", map[string]any{})
	if err != nil {
		return nil, err
	}
	for _, a := range res {
		if len(a) > 0 && a[0] == '[' {
			return a, nil
		}
	}
	return nil, errors.New("pm2 sent no process list")
}

// pm2Error is a refusal from the daemon, e.g. "process already online".
type pm2Error string

func (e pm2Error) Error() string { return string(e) }

func parseReply(arg []byte) ([]json.RawMessage, error) {
	if len(arg) < 2 || string(arg[:2]) != "j:" {
		return nil, errors.New("pm2 sent an unexpected reply")
	}
	var reply struct {
		Args  []json.RawMessage `json:"args"`
		Error json.RawMessage   `json:"error"`
	}
	if err := json.Unmarshal(arg[2:], &reply); err != nil {
		return nil, errors.New("pm2 sent an unreadable reply")
	}
	// pm2-axon-rpc replies {error: message, stack} or {args: [...]}.
	if len(reply.Error) > 0 && string(reply.Error) != "null" {
		var msg string
		if json.Unmarshal(reply.Error, &msg) != nil {
			msg = "pm2 refused the request"
		}
		return nil, pm2Error(clean(msg, 200))
	}
	if reply.Args == nil {
		return nil, errors.New("pm2 refused the request")
	}
	return reply.Args, nil
}
