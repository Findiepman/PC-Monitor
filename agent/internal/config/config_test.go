package config

import (
	"strings"
	"testing"
	"time"
)

const base = `
auth:
  username: fin
  password_hash: "$argon2id$v=19$m=65536,t=3,p=2$c2FsdA$aGFzaA"
  totp_secret: JBSWY3DPEHPK3PXP
`

func TestDefaults(t *testing.T) {
	c, err := Parse([]byte(base + `
providers:
  docker: {}
  systemd:
    units: [wings, playit.service]
`))
	if err != nil {
		t.Fatal(err)
	}
	if c.Listen != "127.0.0.1:7070" || !c.ListenIsLoopback() {
		t.Errorf("listen = %q", c.Listen)
	}
	if c.Auth.SessionTTL != 12*time.Hour {
		t.Errorf("ttl = %v", c.Auth.SessionTTL)
	}
	if c.Providers.Docker.Socket != "/var/run/docker.sock" {
		t.Errorf("socket = %q", c.Providers.Docker.Socket)
	}
	if got := c.Providers.Systemd.Units; got[0] != "wings.service" || got[1] != "playit.service" {
		t.Errorf("units = %v", got)
	}
	if c.Providers.Pterodactyl != nil {
		t.Error("pterodactyl should stay disabled")
	}
}

func TestPasswordHashIsNotEnvExpanded(t *testing.T) {
	c, err := Parse([]byte(base))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(c.Auth.PasswordHash, "$argon2id$") {
		t.Errorf("hash mangled: %q", c.Auth.PasswordHash)
	}
}

func TestEnvSecret(t *testing.T) {
	t.Setenv("PTERO_KEY", "ptlc_abc")
	c, err := Parse([]byte(base + `
providers:
  pterodactyl:
    url: https://panel.example.com/
    api_key: env:PTERO_KEY
`))
	if err != nil {
		t.Fatal(err)
	}
	p := c.Providers.Pterodactyl
	if p.APIKey != "ptlc_abc" || p.URL != "https://panel.example.com" {
		t.Errorf("got %+v", p)
	}
}

func TestMissingAuth(t *testing.T) {
	_, err := Parse([]byte("listen: 127.0.0.1:1\n"))
	if err == nil || !strings.Contains(err.Error(), "auth.username") {
		t.Fatalf("err = %v", err)
	}
}

func TestUnknownField(t *testing.T) {
	if _, err := Parse([]byte(base + "lisen: oops\n")); err == nil {
		t.Fatal("expected unknown field error")
	}
}

func TestNonLoopback(t *testing.T) {
	c, err := Parse([]byte(base + "listen: 0.0.0.0:7070\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.ListenIsLoopback() {
		t.Error("0.0.0.0 is not loopback")
	}
}

func TestAbout(t *testing.T) {
	c, err := Parse([]byte(base))
	if err != nil || c.About != nil {
		t.Fatalf("config without about: %v %v", err, c.About)
	}
	c, err = Parse([]byte(base + `
about:
  docker:strongroom: My password vault.
  redis: ""
`))
	if err != nil {
		t.Fatal(err)
	}
	if c.About["docker:strongroom"] != "My password vault." {
		t.Errorf("about = %v", c.About)
	}
	if v, ok := c.About["redis"]; !ok || v != "" {
		t.Errorf("empty value should be kept to hide a description: %v", c.About)
	}
}

func TestPM2(t *testing.T) {
	c, err := Parse([]byte(base + `
providers:
  pm2: {}
`))
	if err != nil {
		t.Fatal(err)
	}
	p := c.Providers.PM2
	if p.Label != "Discord bots" || p.Path != "/run/pm2-status/pm2.json" {
		t.Errorf("pm2 defaults = %+v", p)
	}
	c, err = Parse([]byte(base))
	if err != nil || c.Providers.PM2 != nil {
		t.Fatalf("pm2 should stay disabled without a block: %v", err)
	}
}
