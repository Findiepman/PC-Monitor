package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// RFC 6238 TOTP with the settings every authenticator app assumes:
// SHA-1, 6 digits, 30 second steps.
const totpPeriod = 30

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewTOTPSecret returns a random 160-bit base32 secret.
func NewTOTPSecret() (string, error) {
	buf := make([]byte, 20)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return b32.EncodeToString(buf), nil
}

// TOTPURI builds the otpauth:// URI that authenticator apps scan.
func TOTPURI(secret, account, issuer string) string {
	label := url.PathEscape(issuer + ":" + account)
	q := url.Values{"secret": {secret}, "issuer": {issuer}}
	return "otpauth://totp/" + label + "?" + q.Encode()
}

func decodeSecret(secret string) ([]byte, error) {
	s := strings.ToUpper(strings.ReplaceAll(secret, " ", ""))
	s = strings.TrimRight(s, "=")
	key, err := b32.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("totp secret is not valid base32: %w", err)
	}
	return key, nil
}

func totpAt(key []byte, step int64) string {
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(step))
	mac := hmac.New(sha1.New, key)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	code := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", code%1_000_000)
}

// TOTPCode returns the code for time t.
func TOTPCode(secret string, t time.Time) (string, error) {
	key, err := decodeSecret(secret)
	if err != nil {
		return "", err
	}
	return totpAt(key, t.Unix()/totpPeriod), nil
}

// matchTOTP checks code against the current step and one step either side to
// absorb clock drift. It returns the matched step so callers can refuse
// replays of an already used code.
func matchTOTP(secret, code string, now time.Time) (int64, bool) {
	key, err := decodeSecret(secret)
	if err != nil || len(code) != 6 {
		return 0, false
	}
	cur := now.Unix() / totpPeriod
	for _, step := range []int64{cur, cur - 1, cur + 1} {
		if subtle.ConstantTimeCompare([]byte(totpAt(key, step)), []byte(code)) == 1 {
			return step, true
		}
	}
	return 0, false
}
