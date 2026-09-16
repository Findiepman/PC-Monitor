package provider

import "sync"

// Ring keeps the most recent log lines in a fixed amount of memory.
type Ring struct {
	mu    sync.Mutex
	buf   []LogLine
	start int
	n     int
}

func NewRing(size int) *Ring {
	return &Ring{buf: make([]LogLine, size)}
}

func (r *Ring) Push(l LogLine) {
	r.mu.Lock()
	defer r.mu.Unlock()
	end := (r.start + r.n) % len(r.buf)
	r.buf[end] = l
	if r.n < len(r.buf) {
		r.n++
	} else {
		r.start = (r.start + 1) % len(r.buf)
	}
}

// Last returns up to n of the newest lines, oldest first.
func (r *Ring) Last(n int) []LogLine {
	r.mu.Lock()
	defer r.mu.Unlock()
	if n > r.n || n <= 0 {
		n = r.n
	}
	out := make([]LogLine, n)
	skip := r.n - n
	for i := range n {
		out[i] = r.buf[(r.start+skip+i)%len(r.buf)]
	}
	return out
}
