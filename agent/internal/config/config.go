// Package config loads dashd's YAML configuration.
package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	// Listen is the address the HTTP server binds to. Keep it on loopback and
	// let cloudflared reach it; dashd has no TLS of its own.
	Listen string `yaml:"listen"`
	// Hostname overrides the name shown in the top bar.
	Hostname string `yaml:"hostname"`
	// Mock replaces every provider and the host collector with fake data.
	Mock bool `yaml:"mock"`

	Auth      Auth      `yaml:"auth"`
	Audit     Audit     `yaml:"audit"`
	Providers Providers `yaml:"providers"`
}

type Auth struct {
	Username     string        `yaml:"username"`
	PasswordHash string        `yaml:"password_hash"`
	TOTPSecret   string        `yaml:"totp_secret"`
	SessionTTL   time.Duration `yaml:"session_ttl"`
	// SessionsPath is where sessions are saved so a restart or reboot
	// doesn't sign you out. "none" keeps them in memory only.
	SessionsPath string `yaml:"sessions_path"`
	// InsecureCookie drops the Secure flag so login works over plain
	// http://localhost during development. Never enable it in production.
	InsecureCookie bool `yaml:"insecure_cookie"`
}

type Audit struct {
	Path string `yaml:"path"`
}

// Providers holds one optional block per provider type. A nil block means the
// provider is disabled. Adding a provider type means adding a field here and
// a constructor in cmd/dashd.
type Providers struct {
	Docker      *Docker      `yaml:"docker"`
	Systemd     *Systemd     `yaml:"systemd"`
	Pterodactyl *Pterodactyl `yaml:"pterodactyl"`
}

type Docker struct {
	Label  string `yaml:"label"`
	Socket string `yaml:"socket"`
	// ExcludeLabels hides containers carrying any of these key=value labels.
	// Wings-managed game servers are hidden by default because the
	// pterodactyl provider already shows them.
	ExcludeLabels []string `yaml:"exclude_labels"`
	// Exclude hides containers by name.
	Exclude []string `yaml:"exclude"`
}

type Systemd struct {
	Label string `yaml:"label"`
	// Units is the allowlist. dashd never lists or touches other units.
	Units []string `yaml:"units"`
}

type Pterodactyl struct {
	Label string `yaml:"label"`
	// URL is the panel's public base URL, e.g. https://panel.findiepman.dev
	URL string `yaml:"url"`
	// APIKey is a client API key (ptlc_...). Use "env:NAME" to read it from
	// the environment.
	APIKey string `yaml:"api_key"`
	// AllServers lists every server on the panel instead of only the ones
	// the key's account owns. Requires an admin account.
	AllServers bool `yaml:"all_servers"`
}

func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(raw)
}

func Parse(raw []byte) (*Config, error) {
	var c Config
	dec := yaml.NewDecoder(strings.NewReader(string(raw)))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if err := c.normalize(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) normalize() error {
	if c.Listen == "" {
		c.Listen = "127.0.0.1:7070"
	}
	if c.Hostname == "" {
		c.Hostname, _ = os.Hostname()
	}

	a := &c.Auth
	a.PasswordHash = resolveSecret(a.PasswordHash)
	a.TOTPSecret = resolveSecret(a.TOTPSecret)
	if a.SessionTTL == 0 {
		a.SessionTTL = 12 * time.Hour
	}
	switch a.SessionsPath {
	case "":
		a.SessionsPath = "sessions.json"
	case "none":
		a.SessionsPath = ""
	}
	var missing []string
	if a.Username == "" {
		missing = append(missing, "auth.username")
	}
	if a.PasswordHash == "" {
		missing = append(missing, "auth.password_hash")
	}
	if a.TOTPSecret == "" {
		missing = append(missing, "auth.totp_secret")
	}
	if len(missing) > 0 {
		return fmt.Errorf("config is missing %s (run `dashd init` to generate them)", strings.Join(missing, ", "))
	}

	if c.Audit.Path == "" {
		c.Audit.Path = "audit.jsonl"
	}

	if d := c.Providers.Docker; d != nil {
		if d.Label == "" {
			d.Label = "Docker"
		}
		if d.Socket == "" {
			d.Socket = "/var/run/docker.sock"
		}
		if d.ExcludeLabels == nil {
			d.ExcludeLabels = []string{"Service=Pterodactyl"}
		}
	}
	if s := c.Providers.Systemd; s != nil {
		if s.Label == "" {
			s.Label = "Services"
		}
		for i, u := range s.Units {
			if !strings.Contains(u, ".") {
				s.Units[i] = u + ".service"
			}
		}
	}
	if p := c.Providers.Pterodactyl; p != nil {
		if p.Label == "" {
			p.Label = "Pterodactyl"
		}
		p.URL = strings.TrimRight(p.URL, "/")
		p.APIKey = resolveSecret(p.APIKey)
		if p.URL == "" || p.APIKey == "" {
			return errors.New("providers.pterodactyl needs both url and api_key")
		}
	}
	return nil
}

// ListenIsLoopback reports whether the server only accepts local connections.
func (c *Config) ListenIsLoopback() bool {
	host, _, err := net.SplitHostPort(c.Listen)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// resolveSecret turns "env:NAME" into the value of $NAME.
func resolveSecret(v string) string {
	if name, ok := strings.CutPrefix(v, "env:"); ok {
		return os.Getenv(name)
	}
	return v
}
