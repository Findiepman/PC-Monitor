package provider

import (
	"context"
	"errors"
	"testing"
)

type fake struct {
	name  string
	units []Unit
	err   error
	did   string
}

func (f *fake) Name() string  { return f.name }
func (f *fake) Label() string { return f.name }
func (f *fake) List(context.Context) ([]Unit, error) {
	return f.units, f.err
}
func (f *fake) Do(_ context.Context, id, action string) error {
	f.did = id + "/" + action
	return nil
}
func (f *fake) Logs(context.Context, string, int) (<-chan LogLine, error) {
	return nil, nil
}

func TestRegistryListAll(t *testing.T) {
	r := NewRegistry()
	ok := &fake{name: "docker", units: []Unit{{ID: "redis", Name: "redis"}, {ID: "app", Name: "App"}}}
	bad := &fake{name: "systemd", err: errors.New("dbus down")}
	if err := r.Add(ok); err != nil {
		t.Fatal(err)
	}
	if err := r.Add(bad); err != nil {
		t.Fatal(err)
	}
	if err := r.Add(&fake{name: "docker"}); err == nil {
		t.Fatal("duplicate provider accepted")
	}

	units, errs := r.ListAll(context.Background())
	if len(units) != 2 || units[0].ID != "docker:app" || units[1].ID != "docker:redis" {
		t.Fatalf("units = %+v", units)
	}
	if units[0].Health != HealthNone || units[0].Provider != "docker" {
		t.Errorf("defaults not applied: %+v", units[0])
	}
	if errs["systemd"] != "dbus down" {
		t.Errorf("errs = %v", errs)
	}
}

func TestRegistryDo(t *testing.T) {
	r := NewRegistry()
	f := &fake{name: "systemd"}
	_ = r.Add(f)
	if err := r.Do(context.Background(), "systemd:wings.service", "restart"); err != nil {
		t.Fatal(err)
	}
	if f.did != "wings.service/restart" {
		t.Errorf("did = %q", f.did)
	}
	if err := r.Do(context.Background(), "systemd:wings.service", "rm -rf"); err != ErrBadAction {
		t.Errorf("bad action: %v", err)
	}
	for _, id := range []string{"nope:x", "systemd", "systemd:"} {
		if err := r.Do(context.Background(), id, "restart"); err != ErrUnknownUnit {
			t.Errorf("%q: %v", id, err)
		}
	}
}

func TestRing(t *testing.T) {
	r := NewRing(3)
	if len(r.Last(10)) != 0 {
		t.Fatal("empty ring should return nothing")
	}
	for i := range 5 {
		r.Push(LogLine{T: int64(i)})
	}
	got := r.Last(10)
	if len(got) != 3 || got[0].T != 2 || got[2].T != 4 {
		t.Fatalf("got %+v", got)
	}
	if got := r.Last(2); got[0].T != 3 || got[1].T != 4 {
		t.Fatalf("last 2 = %+v", got)
	}
}

func TestCleanLine(t *testing.T) {
	cases := map[string]string{
		"\x1b[1m\x1b[33mcontainer@pterodactyl~ \x1b[0mServer marked as running": "container@pterodactyl~ Server marked as running",
		"plain":              "plain",
		"50%\r100%\r":        "100%",
		"line with crlf\r":   "line with crlf",
		"\x1b]0;title\x07ok": "ok",
	}
	for in, want := range cases {
		if got := CleanLine(in); got != want {
			t.Errorf("CleanLine(%q) = %q, want %q", in, got, want)
		}
	}
}
