package pterodactyl

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/coder/websocket"

	"github.com/findiepman/dashd/internal/config"
	"github.com/findiepman/dashd/internal/provider"
)

func fakePanel(t *testing.T, power *string) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/client", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer ptlc_test" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		fmt.Fprint(w, `{"data":[
		  {"attributes":{"identifier":"1a7ce997","name":"survival-smp","node":"home","limits":{"memory":4096}}},
		  {"attributes":{"identifier":"5b2e0c11","name":"creative","node":"home","limits":{"memory":0}}},
		  {"attributes":{"identifier":"dead0000","name":"old","node":"home","is_suspended":true,"limits":{"memory":0}}}
		]}`)
	})
	mux.HandleFunc("GET /api/client/servers/{id}/resources", func(w http.ResponseWriter, r *http.Request) {
		state := "offline"
		if r.PathValue("id") == "1a7ce997" {
			state = "running"
		}
		fmt.Fprintf(w, `{"attributes":{"current_state":%q,"resources":{"memory_bytes":2147483648,"cpu_absolute":37.5,"uptime":90000}}}`, state)
	})
	mux.HandleFunc("POST /api/client/servers/{id}/power", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		*power = r.PathValue("id") + ":" + body["signal"]
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /api/client/servers/{id}/websocket", func(w http.ResponseWriter, r *http.Request) {
		sock := "ws" + strings.TrimPrefix(srv.URL, "http") + "/wings"
		fmt.Fprintf(w, `{"data":{"token":"jwt","socket":%q}}`, sock)
	})
	mux.HandleFunc("/wings", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") != srv.URL {
			t.Errorf("origin = %q", r.Header.Get("Origin"))
		}
		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		defer c.CloseNow()
		ctx := r.Context()
		read := func() wsMessage {
			_, raw, _ := c.Read(ctx)
			var m wsMessage
			json.Unmarshal(raw, &m)
			return m
		}
		write := func(event, arg string) {
			raw, _ := json.Marshal(wsMessage{Event: event, Args: []string{arg}})
			c.Write(ctx, websocket.MessageText, raw)
		}
		if m := read(); m.Event != "auth" || m.Args[0] != "jwt" {
			t.Errorf("first message = %+v", m)
		}
		write("auth success", "")
		if m := read(); m.Event != "send logs" {
			t.Errorf("expected send logs, got %+v", m)
		}
		write("console output", "\x1b[33m[Server] Done (3.2s)!\x1b[0m\nline two")
		write("status", "running")
		c.Close(websocket.StatusNormalClosure, "")
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestListAndPower(t *testing.T) {
	var power string
	srv := fakePanel(t, &power)
	p := New(config.Pterodactyl{Label: "Game servers", URL: srv.URL, APIKey: "ptlc_test"})
	ctx := context.Background()

	units, err := p.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 3 {
		t.Fatalf("units = %+v", units)
	}
	smp := units[0]
	if smp.State != provider.StateRunning || *smp.CPU != 37.5 || smp.UptimeSec != 90 || *smp.MemLimit != 4096<<20 {
		t.Errorf("smp = %+v", smp)
	}
	if units[1].State != provider.StateStopped || units[1].Actions[0] != provider.ActionStart {
		t.Errorf("creative = %+v", units[1])
	}
	if units[2].Meta["note"] != "suspended" || len(units[2].Actions) != 0 {
		t.Errorf("suspended server = %+v", units[2])
	}

	if err := p.Do(ctx, "1a7ce997", "restart"); err != nil {
		t.Fatal(err)
	}
	if power != "1a7ce997:restart" {
		t.Errorf("power = %q", power)
	}

	bad := New(config.Pterodactyl{URL: srv.URL, APIKey: "wrong"})
	if _, err := bad.List(ctx); err == nil || !strings.Contains(err.Error(), "API key") {
		t.Errorf("bad key error = %v", err)
	}
}

func TestConsole(t *testing.T) {
	var power string
	srv := fakePanel(t, &power)
	p := New(config.Pterodactyl{URL: srv.URL, APIKey: "ptlc_test"})
	ch, err := p.Logs(context.Background(), "1a7ce997", 100)
	if err != nil {
		t.Fatal(err)
	}
	var texts []string
	for l := range ch {
		texts = append(texts, l.Text)
	}
	want := []string{"[Server] Done (3.2s)!", "line two", "[wings] server marked as running"}
	if len(texts) < 3 || strings.Join(texts[:3], "|") != strings.Join(want, "|") {
		t.Errorf("texts = %q", texts)
	}
}
