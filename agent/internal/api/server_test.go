package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/coder/websocket"

	"github.com/findiepman/dashd/internal/audit"
	"github.com/findiepman/dashd/internal/auth"
	"github.com/findiepman/dashd/internal/config"
	"github.com/findiepman/dashd/internal/host"
	"github.com/findiepman/dashd/internal/provider"
	"github.com/findiepman/dashd/internal/providers/mock"
)

type testEnv struct {
	srv    *httptest.Server
	client *http.Client
	secret string
	audit  *audit.Log
}

func setup(t *testing.T) *testEnv {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	hash, _ := auth.HashPassword("correct horse")
	secret, _ := auth.NewTOTPSecret()
	reg := provider.NewRegistry()
	for _, p := range mock.Set(ctx) {
		reg.Add(p)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	hub := NewHub(reg, host.NewMock(), log)
	go hub.Run(ctx)
	al, err := audit.Open(filepath.Join(t.TempDir(), "audit.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { al.Close() })

	s := &Server{
		Hostname:       "test-srv",
		InsecureCookie: true,
		Auth:           auth.New(config.Auth{Username: "fin", PasswordHash: hash, TOTPSecret: secret, SessionTTL: time.Hour}),
		Registry:       reg,
		Hub:            hub,
		Audit:          al,
		Static:         fstest.MapFS{"index.html": {Data: []byte("<!doctype html>app")}},
		Log:            log,
	}
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
	return &testEnv{srv: srv, client: &http.Client{Jar: jar}, secret: secret, audit: al}
}

func (e *testEnv) do(t *testing.T, method, path, body string, origin bool) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(method, e.srv.URL+path, strings.NewReader(body))
	if origin {
		req.Header.Set("Origin", e.srv.URL)
	}
	resp, err := e.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func (e *testEnv) login(t *testing.T) {
	t.Helper()
	code, _ := auth.TOTPCode(e.secret, time.Now())
	resp := e.do(t, "POST", "/api/login", `{"username":"fin","password":"correct horse","code":"`+code+`"}`, true)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("login: %d %s", resp.StatusCode, b)
	}
}

func TestAuthRequired(t *testing.T) {
	e := setup(t)
	for _, p := range []string{"/api/units", "/api/meta", "/api/audit", "/ws"} {
		if resp := e.do(t, "GET", p, "", false); resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s: %d", p, resp.StatusCode)
		}
	}
	// The SPA shell itself is public; it contains no data.
	if resp := e.do(t, "GET", "/some/client/route", "", false); resp.StatusCode != http.StatusOK {
		t.Errorf("spa fallback: %d", resp.StatusCode)
	}
}

func TestLoginRejectsCrossOrigin(t *testing.T) {
	e := setup(t)
	code, _ := auth.TOTPCode(e.secret, time.Now())
	resp := e.do(t, "POST", "/api/login", `{"username":"fin","password":"correct horse","code":"`+code+`"}`, false)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("missing Origin should be refused, got %d", resp.StatusCode)
	}
}

func TestWrongPassword(t *testing.T) {
	e := setup(t)
	resp := e.do(t, "POST", "/api/login", `{"username":"fin","password":"nope","code":"123456"}`, true)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("got %d", resp.StatusCode)
	}
	if h := resp.Header.Get("Set-Cookie"); h != "" {
		t.Fatalf("no cookie on failure, got %q", h)
	}
}

func TestActionIsAudited(t *testing.T) {
	e := setup(t)
	e.login(t)

	resp := e.do(t, "POST", "/api/units/docker:redis/restart", "", true)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("restart: %d %s", resp.StatusCode, b)
	}
	got := e.audit.Recent(1)
	if len(got) != 1 || got[0].Unit != "docker:redis" || got[0].User != "fin" || !got[0].OK {
		t.Fatalf("audit = %+v", got)
	}

	if resp := e.do(t, "POST", "/api/units/docker:redis/exec", "", true); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("bad action: %d", resp.StatusCode)
	}
	if resp := e.do(t, "POST", "/api/units/docker:nope/restart", "", true); resp.StatusCode != http.StatusBadGateway && resp.StatusCode != http.StatusNotFound {
		t.Errorf("unknown unit: %d", resp.StatusCode)
	}
	if resp := e.do(t, "POST", "/api/logout", "", true); resp.StatusCode != http.StatusNoContent {
		t.Errorf("logout: %d", resp.StatusCode)
	}
	if resp := e.do(t, "GET", "/api/units", "", false); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("after logout: %d", resp.StatusCode)
	}
}

func TestWebsocketHelloAndLogs(t *testing.T) {
	e := setup(t)
	e.login(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	wsURL := "ws" + strings.TrimPrefix(e.srv.URL, "http") + "/ws"
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPClient: e.client,
		HTTPHeader: http.Header{"Origin": {e.srv.URL}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	conn.SetReadLimit(1 << 22)

	read := func() (string, json.RawMessage) {
		_, raw, err := conn.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var env struct {
			Type string          `json:"type"`
			Data json.RawMessage `json:"data"`
		}
		json.Unmarshal(raw, &env)
		return env.Type, env.Data
	}
	if typ, _ := read(); typ != "hello" {
		t.Fatalf("first message = %s", typ)
	}
	conn.Write(ctx, websocket.MessageText, []byte(`{"op":"logs","unit":"systemd:wings.service","tail":20}`))
	for {
		typ, data := read()
		if typ != "logs" {
			continue
		}
		var batch logsBatch
		json.Unmarshal(data, &batch)
		if batch.Unit != "systemd:wings.service" || len(batch.Lines) != 20 {
			t.Fatalf("batch = %s with %d lines", batch.Unit, len(batch.Lines))
		}
		return
	}
}
