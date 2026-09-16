package docker

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/findiepman/dashd/internal/config"
	"github.com/findiepman/dashd/internal/provider"
)

func fakeEngine(t *testing.T, calls *[]string) *httptest.Server {
	t.Helper()
	cpuTotal := uint64(1_000)
	sysTotal := uint64(100_000)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /containers/json", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[
		  {"Id":"aaa111222333","Names":["/redis"],"Image":"redis:7","State":"running","Labels":{}},
		  {"Id":"bbb111222333","Names":["/kivo"],"Image":"kivo:latest","State":"exited","Labels":{}},
		  {"Id":"ccc111222333","Names":["/3f0e-uuid"],"Image":"ghcr.io/pterodactyl/yolks:java_21","State":"running","Labels":{"Service":"Pterodactyl"}}
		]`)
	})
	mux.HandleFunc("GET /containers/{id}/json", func(w http.ResponseWriter, r *http.Request) {
		switch r.PathValue("id") {
		case "aaa111222333", "redis":
			fmt.Fprint(w, `{"RestartCount":2,"State":{"Status":"running","StartedAt":"2020-01-01T00:00:00Z","Health":{"Status":"healthy"}},"Config":{"Tty":false}}`)
		default:
			fmt.Fprint(w, `{"State":{"Status":"exited","ExitCode":137},"Config":{"Tty":false}}`)
		}
	})
	mux.HandleFunc("GET /containers/{id}/stats", func(w http.ResponseWriter, r *http.Request) {
		cpuTotal += 500
		sysTotal += 10_000
		fmt.Fprintf(w, `{"cpu_stats":{"cpu_usage":{"total_usage":%d},"system_cpu_usage":%d,"online_cpus":4},
		  "memory_stats":{"usage":3000,"limit":10000,"stats":{"inactive_file":1000}}}`, cpuTotal, sysTotal)
	})
	mux.HandleFunc("POST /containers/{id}/{action}", func(w http.ResponseWriter, r *http.Request) {
		*calls = append(*calls, r.PathValue("id")+"/"+r.PathValue("action")+"?"+r.URL.RawQuery)
		if r.PathValue("id") == "missing" {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"message":"No such container: missing"}`)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /containers/{id}/logs", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("tail") != "50" {
			t.Errorf("tail = %q", r.URL.Query().Get("tail"))
		}
		// A stdout line split across two frames, then a stderr line.
		w.Write(frame(1, "2026-09-16T10:00:00.123Z hello "))
		w.Write(frame(1, "world\n"))
		w.Write(frame(2, "2026-09-16T10:00:01Z \x1b[31moops\x1b[0m\n"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func frame(stream byte, s string) []byte {
	var b bytes.Buffer
	b.Write([]byte{stream, 0, 0, 0})
	binary.Write(&b, binary.BigEndian, uint32(len(s)))
	b.WriteString(s)
	return b.Bytes()
}

func newTest(t *testing.T, calls *[]string) *Provider {
	srv := fakeEngine(t, calls)
	cfg := config.Docker{Label: "Docker", ExcludeLabels: []string{"Service=Pterodactyl"}}
	return newWithClient(cfg, srv.Client(), srv.URL)
}

func TestList(t *testing.T) {
	var calls []string
	p := newTest(t, &calls)
	ctx := context.Background()

	units, err := p.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 2 {
		t.Fatalf("expected wings container to be hidden, got %d units", len(units))
	}
	byName := map[string]provider.Unit{}
	for _, u := range units {
		byName[u.Name] = u
	}
	redis := byName["redis"]
	if redis.State != provider.StateRunning || redis.Health != "healthy" || redis.Meta["restarts"] != "2" {
		t.Errorf("redis = %+v", redis)
	}
	if redis.CPU != nil {
		t.Error("first poll has no CPU delta and should report nil")
	}
	if redis.MemBytes == nil || *redis.MemBytes != 2000 {
		t.Errorf("mem should exclude inactive_file: %v", redis.MemBytes)
	}
	kivo := byName["kivo"]
	if kivo.State != provider.StateStopped || kivo.Meta["exit"] != "137" || kivo.Actions[0] != provider.ActionStart {
		t.Errorf("kivo = %+v", kivo)
	}

	units, _ = p.List(ctx)
	for _, u := range units {
		if u.Name == "redis" {
			// 500/10000 * 4 cpus * 100
			if u.CPU == nil || *u.CPU != 20 {
				t.Errorf("cpu = %v", u.CPU)
			}
		}
	}
}

func TestDo(t *testing.T) {
	var calls []string
	p := newTest(t, &calls)
	ctx := context.Background()
	if err := p.Do(ctx, "redis", provider.ActionRestart); err != nil {
		t.Fatal(err)
	}
	if calls[0] != "redis/restart?t=10" {
		t.Errorf("calls = %v", calls)
	}
	err := p.Do(ctx, "missing", provider.ActionStart)
	if err == nil || !strings.Contains(err.Error(), "No such container") {
		t.Errorf("err = %v", err)
	}
	if err := p.Do(ctx, "redis", "exec"); err != provider.ErrBadAction {
		t.Errorf("exec: %v", err)
	}
}

func TestLogs(t *testing.T) {
	var calls []string
	p := newTest(t, &calls)
	ch, err := p.Logs(context.Background(), "redis", 50)
	if err != nil {
		t.Fatal(err)
	}
	var lines []provider.LogLine
	for l := range ch {
		lines = append(lines, l)
	}
	if len(lines) != 2 {
		t.Fatalf("lines = %+v", lines)
	}
	if lines[0].Text != "hello world" || lines[0].Err || lines[0].T != 1789552800123 {
		t.Errorf("line 0 = %+v", lines[0])
	}
	if lines[1].Text != "oops" || !lines[1].Err {
		t.Errorf("line 1 = %+v", lines[1])
	}
}
