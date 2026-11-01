// Package about gives every unit a one-line "what is this and what does it
// do" description. Built-in blurbs cover well-known services; the optional
// `about:` block in config.yaml overrides them or adds your own.
package about

import (
	"sort"
	"strings"

	"github.com/findiepman/dashd/internal/provider"
)

// byName holds built-in blurbs keyed by "<provider>:<unit name>".
var byName = map[string]string{
	"systemd:wings":        "Pterodactyl's node daemon: it starts, stops and runs your game servers in Docker containers.",
	"systemd:playit":       "playit.gg agent: tunnels game server ports to the internet, so players can join without port forwarding.",
	"systemd:pteroq":       "Pterodactyl panel's queue worker: runs the panel's background jobs, such as schedules and emails.",
	"systemd:cloudflared":  "Cloudflare Tunnel: carries traffic for your domains to the server without open ports.",
	"systemd:nginx":        "Web server and reverse proxy: serves websites and passes requests on to apps on this server.",
	"systemd:mariadb":      "MariaDB database server: stores the data of the apps that connect to it.",
	"systemd:mysql":        "MySQL database server: stores the data of the apps that connect to it.",
	"systemd:postgresql":   "PostgreSQL database server: stores the data of the apps that connect to it.",
	"systemd:redis-server": "Redis: a fast in-memory store that apps use for caching, sessions and job queues.",
	"systemd:redis":        "Redis: a fast in-memory store that apps use for caching, sessions and job queues.",
}

// byImage holds built-in blurbs for Docker containers, keyed by image name
// without registry or tag ("redis", "pterodactyl/panel").
var byImage = map[string]string{
	"pterodactyl/panel":      "Pterodactyl panel: the website where you create, manage and configure your game servers.",
	"mariadb":                "MariaDB database server: stores the data of the apps that connect to it.",
	"mysql":                  "MySQL database server: stores the data of the apps that connect to it.",
	"postgres":               "PostgreSQL database server: stores the data of the apps that connect to it.",
	"redis":                  "Redis: a fast in-memory store that apps use for caching, sessions and job queues.",
	"louislam/uptime-kuma":   "Uptime Kuma: checks that your sites and services are up and alerts you when one goes down.",
	"nginx":                  "Web server and reverse proxy: serves websites and passes requests on to apps on this server.",
	"cloudflare/cloudflared": "Cloudflare Tunnel: carries traffic for your domains to the server without open ports.",
}

// byProvider is the fallback when nothing more specific matches.
var byProvider = map[string]string{
	"pterodactyl": "Game server on the Pterodactyl panel; Wings runs it in its own Docker container.",
}

// Book resolves descriptions: config overrides first, then built-ins.
type Book struct {
	overrides map[string]string
}

// New takes the `about:` block from config. Keys are a full unit id
// ("docker:redis", "systemd:wings.service") or a bare unit name ("redis").
// A bare name matches that name in every provider. An empty value hides the
// description for that unit. Keys are case-insensitive; if two keys differ
// only in case, the one that sorts last wins, so the result is stable.
func New(overrides map[string]string) *Book {
	keys := make([]string, 0, len(overrides))
	for k := range overrides {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	m := make(map[string]string, len(overrides))
	for _, k := range keys {
		m[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(overrides[k])
	}
	return &Book{overrides: m}
}

// Apply fills in About on every unit. Units must already carry their full id
// and provider name (as returned by Registry.ListAll).
func (b *Book) Apply(units []provider.Unit) {
	for i := range units {
		units[i].About = b.For(units[i])
	}
}

// For returns the description for one unit, or "" when there is none.
func (b *Book) For(u provider.Unit) string {
	if b != nil {
		for _, k := range []string{u.ID, u.Name} {
			if v, ok := b.overrides[strings.ToLower(k)]; ok {
				return v
			}
		}
	}
	name := strings.ToLower(u.Name)
	if v, ok := byName[u.Provider+":"+name]; ok {
		return v
	}
	if u.Provider == "docker" && u.Meta != nil {
		if v, ok := byImage[imageName(u.Meta["image"])]; ok {
			return v
		}
	}
	return byProvider[u.Provider]
}

// imageName strips the registry host, tag and digest from an image
// reference and drops Docker Hub's implicit "library/" prefix:
// "ghcr.io/pterodactyl/panel:latest" -> "pterodactyl/panel",
// "docker.io/library/redis:7-alpine" -> "redis".
func imageName(ref string) string {
	ref = strings.ToLower(strings.TrimSpace(ref))
	if i := strings.IndexByte(ref, '@'); i >= 0 {
		ref = ref[:i]
	}
	if i := strings.LastIndexByte(ref, ':'); i > strings.LastIndexByte(ref, '/') {
		ref = ref[:i]
	}
	parts := strings.Split(ref, "/")
	if len(parts) > 1 && (strings.ContainsAny(parts[0], ".:") || parts[0] == "localhost") {
		parts = parts[1:]
	}
	if len(parts) > 1 && parts[0] == "library" {
		parts = parts[1:]
	}
	return strings.Join(parts, "/")
}
