package auth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/findiepman/dashd/internal/config"
)

func TestPasswordRoundTrip(t *testing.T) {
	h, err := HashPassword("hunter2")
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := VerifyPassword(h, "hunter2"); !ok || err != nil {
		t.Fatalf("correct password rejected: %v", err)
	}
	if ok, _ := VerifyPassword(h, "hunter3"); ok {
		t.Fatal("wrong password accepted")
	}
	if _, err := VerifyPassword("plaintext", "x"); err == nil {
		t.Fatal("expected error for malformed hash")
	}
}

func TestTOTPVector(t *testing.T) {
	// RFC 6238 appendix B, SHA-1 seed "12345678901234567890", truncated to 6 digits.
	secret := b32.EncodeToString([]byte("12345678901234567890"))
	cases := map[int64]string{59: "287082", 1111111109: "081804", 2000000000: "279037"}
	for ts, want := range cases {
		got, err := TOTPCode(secret, time.Unix(ts, 0))
		if err != nil || got != want {
			t.Errorf("t=%d: got %s want %s (%v)", ts, got, want, err)
		}
	}
}

func TestTOTPDriftWindow(t *testing.T) {
	secret, _ := NewTOTPSecret()
	now := time.Unix(1_800_000_000, 0)
	prev, _ := TOTPCode(secret, now.Add(-30*time.Second))
	old, _ := TOTPCode(secret, now.Add(-90*time.Second))
	if _, ok := matchTOTP(secret, prev, now); !ok {
		t.Error("previous step should be accepted")
	}
	if _, ok := matchTOTP(secret, old, now); ok {
		t.Error("code from three steps ago should be rejected")
	}
}

func newTestAuth(t *testing.T) (*Authenticator, *time.Time) {
	t.Helper()
	hash, err := HashPassword("pw")
	if err != nil {
		t.Fatal(err)
	}
	secret, _ := NewTOTPSecret()
	a := New(config.Auth{Username: "fin", PasswordHash: hash, TOTPSecret: secret, SessionTTL: time.Hour})
	clock := time.Unix(1_800_000_000, 0)
	a.now = func() time.Time { return clock }
	a.limiter.now = a.now
	return a, &clock
}

func TestLoginFlow(t *testing.T) {
	a, clock := newTestAuth(t)
	code, _ := TOTPCode(a.cfg.TOTPSecret, *clock)

	token, _, err := a.Login("1.2.3.4", "fin", "pw", code)
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	if _, ok := a.Session(token); !ok {
		t.Fatal("session missing")
	}

	if _, _, err := a.Login("1.2.3.4", "fin", "pw", code); err != ErrInvalid {
		t.Fatalf("replayed code: got %v", err)
	}

	*clock = clock.Add(2 * time.Hour)
	if _, ok := a.Session(token); ok {
		t.Fatal("session should have expired")
	}
}

func TestLoginLockout(t *testing.T) {
	a, clock := newTestAuth(t)
	for i := 0; i < 5; i++ {
		if _, _, err := a.Login("5.5.5.5", "fin", "nope", "000000"); err != ErrInvalid {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	code, _ := TOTPCode(a.cfg.TOTPSecret, *clock)
	if _, _, err := a.Login("5.5.5.5", "fin", "pw", code); err != ErrLocked {
		t.Fatalf("expected lockout, got %v", err)
	}
	// Another IP is unaffected.
	if _, _, err := a.Login("6.6.6.6", "fin", "pw", code); err != nil {
		t.Fatalf("other key locked out: %v", err)
	}
	*clock = clock.Add(16 * time.Minute)
	if !a.limiter.Allow("5.5.5.5") {
		t.Fatal("lockout should expire")
	}
}

func TestGlobalLimit(t *testing.T) {
	l := NewLimiter(time.Minute, 5, 3)
	for _, k := range []string{"a", "b", "c"} {
		l.Fail(k)
	}
	if l.Allow("d") {
		t.Fatal("global cap should block new keys")
	}
}

func TestSessionsSurviveRestart(t *testing.T) {
	hash, _ := HashPassword("pw")
	secret, _ := NewTOTPSecret()
	path := filepath.Join(t.TempDir(), "sessions.json")
	cfg := config.Auth{Username: "fin", PasswordHash: hash, TOTPSecret: secret, SessionTTL: time.Hour, SessionsPath: path}
	clock := time.Unix(1_800_000_000, 0)

	a := New(cfg)
	a.now = func() time.Time { return clock }
	code, _ := TOTPCode(secret, clock)
	token, _, err := a.Login("ip", "fin", "pw", code)
	if err != nil {
		t.Fatal(err)
	}

	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), token) {
		t.Fatal("raw token must not be written to disk")
	}

	// "Restart": a fresh Authenticator reading the same file.
	b := New(cfg)
	b.now = func() time.Time { return clock }
	if _, ok := b.Session(token); !ok {
		t.Fatal("session lost across restart")
	}
	if _, _, err := b.Login("ip", "fin", "pw", code); err != ErrInvalid {
		t.Fatalf("code replayed after restart: %v", err)
	}

	b.Logout(token)
	c := New(cfg)
	c.now = func() time.Time { return clock }
	if _, ok := c.Session(token); ok {
		t.Fatal("logout didn't persist")
	}
}
