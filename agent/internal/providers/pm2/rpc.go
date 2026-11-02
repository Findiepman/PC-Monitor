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

// The exporter asks the running pm2 daemon over its rpc.sock instead of
// running `pm2 jlist`. The CLI starts a fresh daemon whenever it can't reach
// one, so a jlist that lands while pm2 restarts could leave the bots running
// under this exporter's service. Connecting to a socket can never start
// anything, and it skips a Node start-up every few seconds.
//
// The wire format is pm2's (pm2-axon-rpc over axon's "amp" framing):
//   frame = 1 byte (version 1 << 4 | arg count), then per arg a 4-byte
//           big-endian length and the bytes; "j:" prefixes JSON, "s:" a string.
//   call  = [j:{"type":"call","method":M,"args":[...]}, s:<request id>]
//   reply = [j:{"args":[result...]}, s:<request id>], or an object without
//           "args" (an error) when the call failed.
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

// monitorData calls getMonitorData on the daemon behind sock and returns the
// raw JSON array of processes (the same shape `pm2 jlist` prints).
func monitorData(ctx context.Context, sock string) ([]byte, error) {
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
	call, _ := json.Marshal(map[string]any{"type": "call", "method": "getMonitorData", "args": []any{map[string]any{}}})
	if _, err := conn.Write(encodeFrame(append([]byte("j:"), call...), []byte("s:"+id))); err != nil {
		return nil, err
	}
	br := bufio.NewReader(conn)
	for {
		args, err := readFrame(br)
		if err != nil {
			return nil, err
		}
		// The daemon could interleave other replies; only ours counts.
		if len(args) < 2 || string(args[len(args)-1]) != "s:"+id {
			continue
		}
		return parseReply(args[0])
	}
}

func parseReply(arg []byte) ([]byte, error) {
	if len(arg) < 2 || string(arg[:2]) != "j:" {
		return nil, errors.New("pm2 sent an unexpected reply")
	}
	var reply struct {
		Args []json.RawMessage `json:"args"`
	}
	if err := json.Unmarshal(arg[2:], &reply); err != nil || reply.Args == nil {
		return nil, errors.New("pm2 refused the request")
	}
	// The process list is the first array among the results (an error slot,
	// when present, is null).
	for _, a := range reply.Args {
		if len(a) > 0 && a[0] == '[' {
			return a, nil
		}
	}
	return nil, errors.New("pm2 sent no process list")
}
