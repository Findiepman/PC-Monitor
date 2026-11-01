package about

import (
	"testing"

	"github.com/findiepman/dashd/internal/provider"
)

func TestImageName(t *testing.T) {
	cases := map[string]string{
		"redis":                            "redis",
		"redis:7-alpine":                   "redis",
		"docker.io/library/redis:7":        "redis",
		"ghcr.io/pterodactyl/panel:latest": "pterodactyl/panel",
		"louislam/uptime-kuma:1":           "louislam/uptime-kuma",
		"localhost:5000/mine/app:2":        "mine/app",
		"mariadb@sha256:abc":               "mariadb",
		"Postgres:16":                      "postgres",
		"":                                 "",
	}
	for in, want := range cases {
		if got := imageName(in); got != want {
			t.Errorf("imageName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFor(t *testing.T) {
	b := New(map[string]string{
		"docker:strongroom":     "  My vault app.  ",
		"Kivo-Roadmap":          "Roadmap site.",
		"redis":                 "", // hide
		"systemd:wings.service": "Custom wings text.",
	})
	units := []provider.Unit{
		{ID: "docker:strongroom", Provider: "docker", Name: "strongroom", Meta: map[string]string{"image": "findiepman/strongroom:1.4"}},
		{ID: "docker:kivo-roadmap", Provider: "docker", Name: "kivo-roadmap"},
		{ID: "docker:redis", Provider: "docker", Name: "redis", Meta: map[string]string{"image": "redis:7-alpine"}},
		{ID: "docker:db", Provider: "docker", Name: "db", Meta: map[string]string{"image": "mariadb:11"}},
		{ID: "docker:unknown", Provider: "docker", Name: "unknown", Meta: map[string]string{"image": "someone/thing"}},
		{ID: "docker:nometa", Provider: "docker", Name: "nometa"},
		{ID: "systemd:wings.service", Provider: "systemd", Name: "wings"},
		{ID: "systemd:playit.service", Provider: "systemd", Name: "playit"},
		{ID: "systemd:other.service", Provider: "systemd", Name: "other"},
		{ID: "pterodactyl:abc", Provider: "pterodactyl", Name: "survival-smp"},
	}
	b.Apply(units)
	want := []string{
		"My vault app.",
		"Roadmap site.",
		"",
		byImage["mariadb"],
		"",
		"",
		"Custom wings text.",
		byName["systemd:playit"],
		"",
		byProvider["pterodactyl"],
	}
	for i, u := range units {
		if u.About != want[i] {
			t.Errorf("%s: About = %q, want %q", u.ID, u.About, want[i])
		}
	}
}

func TestNilBook(t *testing.T) {
	var b *Book
	u := provider.Unit{ID: "systemd:wings.service", Provider: "systemd", Name: "wings"}
	if b.For(u) != byName["systemd:wings"] {
		t.Fatal("nil book should still use built-ins")
	}
}
