package audit

import (
	"path/filepath"
	"testing"
	"time"
)

func TestAppendAndReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	l, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	l.Append(Entry{Time: time.Now(), User: "fin", Unit: "docker:redis", Action: "restart", OK: true})
	l.Append(Entry{Time: time.Now(), User: "fin", Unit: "systemd:wings.service", Action: "stop", Error: "denied"})
	l.Close()

	l, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	got := l.Recent(10)
	if len(got) != 2 || got[0].Unit != "systemd:wings.service" || got[1].Action != "restart" {
		t.Fatalf("got %+v", got)
	}
}
