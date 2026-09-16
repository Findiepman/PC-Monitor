package auth

import (
	"sync"
	"time"
)

// Limiter counts failed logins in a sliding window, per key and globally.
// The global cap stops an attacker who rotates IPs.
type Limiter struct {
	window   time.Duration
	perKey   int
	global   int
	now      func() time.Time
	mu       sync.Mutex
	fails    map[string][]time.Time
	allFails []time.Time
}

func NewLimiter(window time.Duration, perKey, global int) *Limiter {
	return &Limiter{
		window: window,
		perKey: perKey,
		global: global,
		now:    time.Now,
		fails:  map[string][]time.Time{},
	}
}

func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := l.now().Add(-l.window)
	l.fails[key] = prune(l.fails[key], cutoff)
	if len(l.fails[key]) == 0 {
		delete(l.fails, key)
	}
	l.allFails = prune(l.allFails, cutoff)
	return len(l.fails[key]) < l.perKey && len(l.allFails) < l.global
}

func (l *Limiter) Fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	t := l.now()
	l.fails[key] = append(l.fails[key], t)
	l.allFails = append(l.allFails, t)
}

func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	delete(l.fails, key)
	l.mu.Unlock()
}

func prune(ts []time.Time, cutoff time.Time) []time.Time {
	i := 0
	for i < len(ts) && ts[i].Before(cutoff) {
		i++
	}
	return ts[i:]
}
