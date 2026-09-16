// Package auth handles password + TOTP login, sessions and brute-force limits.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/findiepman/dashd/internal/config"
)

var (
	ErrInvalid = errors.New("username, password or code is wrong")
	ErrLocked  = errors.New("too many failed attempts")
)

type Session struct {
	User    string    `json:"user"`
	Expires time.Time `json:"expires"`
}

type Authenticator struct {
	cfg     config.Auth
	limiter *Limiter
	now     func() time.Time

	mu sync.Mutex
	// sessions is keyed by the SHA-256 of the cookie token, so the sessions
	// file on disk can't be replayed as cookies if it leaks.
	sessions map[string]Session
	lastStep int64
}

// stateFile is what survives a restart: live sessions and the last TOTP step,
// so a code used just before a restart can't be replayed after it.
type stateFile struct {
	Sessions map[string]Session `json:"sessions"`
	LastStep int64              `json:"lastStep"`
}

// New creates an Authenticator and restores sessions from cfg.SessionsPath
// when it's set. A missing or unreadable file just means everyone signs in
// again.
func New(cfg config.Auth) *Authenticator {
	a := &Authenticator{
		cfg:      cfg,
		limiter:  NewLimiter(15*time.Minute, 5, 30),
		now:      time.Now,
		sessions: map[string]Session{},
	}
	a.load()
	return a
}

// Login verifies all three factors and returns a new session token.
// key identifies the caller for rate limiting, normally the client IP.
func (a *Authenticator) Login(key, user, password, code string) (string, Session, error) {
	if !a.limiter.Allow(key) {
		return "", Session{}, ErrLocked
	}

	// Always run the expensive hash so a wrong username takes as long as a
	// wrong password.
	userOK := subtle.ConstantTimeCompare([]byte(user), []byte(a.cfg.Username)) == 1
	passOK, err := VerifyPassword(a.cfg.PasswordHash, password)
	if err != nil {
		return "", Session{}, err
	}
	step, codeOK := matchTOTP(a.cfg.TOTPSecret, code, a.now())

	a.mu.Lock()
	defer a.mu.Unlock()
	if codeOK && step <= a.lastStep {
		codeOK = false // replayed code
	}
	if !userOK || !passOK || !codeOK {
		a.limiter.Fail(key)
		return "", Session{}, ErrInvalid
	}
	a.lastStep = step
	a.limiter.Reset(key)

	token, err := randomToken()
	if err != nil {
		return "", Session{}, err
	}
	s := Session{User: a.cfg.Username, Expires: a.now().Add(a.cfg.SessionTTL)}
	a.sessions[hashToken(token)] = s
	a.save()
	return token, s, nil
}

func (a *Authenticator) Session(token string) (Session, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	key := hashToken(token)
	s, ok := a.sessions[key]
	if !ok {
		return Session{}, false
	}
	if a.now().After(s.Expires) {
		delete(a.sessions, key)
		a.save()
		return Session{}, false
	}
	return s, true
}

func (a *Authenticator) Logout(token string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.sessions, hashToken(token))
	a.save()
}

func (a *Authenticator) load() {
	if a.cfg.SessionsPath == "" {
		return
	}
	raw, err := os.ReadFile(a.cfg.SessionsPath)
	if err != nil {
		return
	}
	var st stateFile
	if json.Unmarshal(raw, &st) != nil {
		return
	}
	now := a.now()
	for k, s := range st.Sessions {
		// Sessions for a renamed user or past their expiry don't come back.
		if s.User == a.cfg.Username && now.Before(s.Expires) {
			a.sessions[k] = s
		}
	}
	a.lastStep = st.LastStep
}

// save writes the state atomically. It must be called with a.mu held.
// A failed write isn't fatal: sessions keep working until the next restart.
func (a *Authenticator) save() {
	if a.cfg.SessionsPath == "" {
		return
	}
	raw, err := json.Marshal(stateFile{Sessions: a.sessions, LastStep: a.lastStep})
	if err != nil {
		return
	}
	if err := writeFileAtomic(a.cfg.SessionsPath, raw); err != nil {
		fmt.Fprintln(os.Stderr, "dashd: could not save sessions:", err)
	}
}

func writeFileAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".sessions-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func randomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
