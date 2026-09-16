// Package audit records every control action taken through the dashboard.
package audit

import (
	"bufio"
	"encoding/json"
	"os"
	"sync"
	"time"
)

type Entry struct {
	Time   time.Time `json:"time"`
	User   string    `json:"user"`
	IP     string    `json:"ip"`
	Unit   string    `json:"unit"`
	Action string    `json:"action"`
	OK     bool      `json:"ok"`
	Error  string    `json:"error,omitempty"`
}

const keep = 200

// Log appends entries to a JSONL file and keeps the newest in memory.
type Log struct {
	mu     sync.Mutex
	f      *os.File
	recent []Entry
}

func Open(path string) (*Log, error) {
	l := &Log{}
	if existing, err := os.Open(path); err == nil {
		sc := bufio.NewScanner(existing)
		for sc.Scan() {
			var e Entry
			if json.Unmarshal(sc.Bytes(), &e) == nil {
				l.push(e)
			}
		}
		existing.Close()
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	l.f = f
	return l, nil
}

func (l *Log) push(e Entry) {
	l.recent = append(l.recent, e)
	if len(l.recent) > keep {
		l.recent = l.recent[len(l.recent)-keep:]
	}
}

func (l *Log) Append(e Entry) error {
	raw, err := json.Marshal(e)
	if err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.push(e)
	_, err = l.f.Write(append(raw, '\n'))
	return err
}

// Recent returns up to n entries, newest first.
func (l *Log) Recent(n int) []Entry {
	l.mu.Lock()
	defer l.mu.Unlock()
	if n > len(l.recent) {
		n = len(l.recent)
	}
	out := make([]Entry, n)
	for i := range n {
		out[i] = l.recent[len(l.recent)-1-i]
	}
	return out
}

func (l *Log) Close() error { return l.f.Close() }
