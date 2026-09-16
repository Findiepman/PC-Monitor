// Package mock provides fake providers so the dashboard can be developed and
// demoed without Docker, systemd or a Pterodactyl panel.
package mock

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"sync"
	"time"

	"github.com/findiepman/dashd/internal/provider"
)

type unit struct {
	provider.Unit
	started time.Time
	ring    *provider.Ring
	lines   []string
	subs    map[chan provider.LogLine]struct{}
	cpuBase float64
	memBase uint64
	// fails counts restart attempts; every other one fails, to exercise
	// the error path in the UI.
	flaky int
}

type Provider struct {
	name, label string
	mu          sync.Mutex
	units       []*unit
}

// Set returns mock stand-ins for the docker, pterodactyl and systemd
// providers, already generating log output.
func Set(ctx context.Context) []provider.Provider {
	const mb = 1 << 20
	docker := newProvider("docker", "Docker", []*unit{
		mk("pterodactyl-panel", "container", provider.HealthHealthy, 3.1, 310*mb, map[string]string{"image": "ghcr.io/pterodactyl/panel:latest"}, webLines),
		mk("pterodactyl-db", "container", provider.HealthHealthy, 1.2, 420*mb, map[string]string{"image": "mariadb:11"}, dbLines),
		mk("redis", "container", provider.HealthHealthy, 0.4, 18*mb, map[string]string{"image": "redis:7-alpine"}, redisLines),
		mk("strongroom", "container", provider.HealthUnhealthy, 0.9, 140*mb, map[string]string{"image": "findiepman/strongroom:1.4", "restarts": "3"}, strongroomLines),
		mk("kivo-roadmap", "container", provider.HealthNone, 0, 0, map[string]string{"image": "findiepman/kivo:latest", "exit": "137"}, webLines),
		mk("uptime-kuma", "container", provider.HealthHealthy, 1.8, 96*mb, map[string]string{"image": "louislam/uptime-kuma:1"}, webLines),
	})
	docker.units[4].State = provider.StateStopped

	ptero := newProvider("pterodactyl", "Pterodactyl", []*unit{
		mk("survival-smp", "game server", provider.HealthNone, 42, 3900*mb, map[string]string{"node": "home"}, mcLines),
		mk("creative", "game server", provider.HealthNone, 0, 0, map[string]string{"node": "home"}, mcLines),
		mk("skript-testing", "game server", provider.HealthNone, 12, 1500*mb, map[string]string{"node": "home"}, mcLines),
	})
	ptero.units[1].State = provider.StateStopped
	for _, u := range ptero.units {
		u.MemLimit = provider.Ptr(uint64(6144 * mb))
	}

	systemd := newProvider("systemd", "Services", []*unit{
		mk("wings", "service", provider.HealthNone, 2.2, 61*mb, map[string]string{"description": "Pterodactyl Wings Daemon"}, wingsLines),
		mk("playit", "service", provider.HealthNone, 0.6, 22*mb, map[string]string{"description": "Playit.gg agent"}, playitLines),
		mk("pteroq", "service", provider.HealthNone, 0.3, 48*mb, map[string]string{"description": "Pterodactyl Queue Worker"}, webLines),
		mk("cloudflared", "service", provider.HealthNone, 0.5, 31*mb, map[string]string{"description": "cloudflared"}, tunnelLines),
		mk("nginx", "service", provider.HealthNone, 0.2, 12*mb, map[string]string{"description": "A high performance web server"}, webLines),
	})
	for _, u := range systemd.units {
		u.ID = u.Name + ".service"
	}

	all := []*Provider{docker, ptero, systemd}
	for _, p := range all {
		go p.run(ctx)
	}
	return []provider.Provider{docker, ptero, systemd}
}

func newProvider(name, label string, units []*unit) *Provider {
	p := &Provider{name: name, label: label, units: units}
	for _, u := range units {
		for i := range 120 {
			u.ring.Push(p.fakeLine(u, time.Now().Add(-time.Duration(120-i)*29*time.Second)))
		}
	}
	return p
}

func mk(name, kind, health string, cpu float64, mem uint64, meta map[string]string, lines []string) *unit {
	started := time.Now().Add(-time.Duration(3600+rand.IntN(40*86400)) * time.Second)
	return &unit{
		Unit: provider.Unit{
			ID: name, Name: name, Kind: kind,
			State: provider.StateRunning, Health: health, Meta: meta,
		},
		started: started,
		ring:    provider.NewRing(2000),
		lines:   lines,
		subs:    map[chan provider.LogLine]struct{}{},
		cpuBase: cpu,
		memBase: mem,
	}
}

func (p *Provider) Name() string  { return p.name }
func (p *Provider) Label() string { return p.label }

func (p *Provider) find(id string) (*unit, error) {
	for _, u := range p.units {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, provider.ErrUnknownUnit
}

func (p *Provider) List(context.Context) ([]provider.Unit, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]provider.Unit, 0, len(p.units))
	for _, u := range p.units {
		v := u.Unit
		v.Actions = provider.ActionsFor(v.State)
		if v.State == provider.StateRunning {
			v.UptimeSec = int64(time.Since(u.started).Seconds())
			v.CPU = provider.Ptr(max(0, u.cpuBase*(0.7+rand.Float64()*0.6)))
			v.MemBytes = provider.Ptr(uint64(float64(u.memBase) * (0.95 + rand.Float64()*0.1)))
		}
		out = append(out, v)
	}
	return out, nil
}

func (p *Provider) Do(_ context.Context, id, action string) error {
	time.Sleep(300 * time.Millisecond) // feel like a real round trip
	p.mu.Lock()
	defer p.mu.Unlock()
	u, err := p.find(id)
	if err != nil {
		return err
	}

	if id == "nginx.service" && action == provider.ActionRestart {
		u.flaky++
		if u.flaky%2 == 1 {
			p.emit(u, provider.LogLine{T: time.Now().UnixMilli(), Err: true, Text: "nginx: [emerg] unknown directive \"proxy_passs\" in /etc/nginx/sites-enabled/panel.conf:24"})
			return errors.New("Job for nginx.service failed because the control process exited with error code.")
		}
	}

	say := func(s string, isErr bool) {
		p.emit(u, provider.LogLine{T: time.Now().UnixMilli(), Err: isErr, Text: s})
	}
	switch action {
	case provider.ActionRestart:
		u.State = provider.StateRestarting
		say(fmt.Sprintf("Received restart signal, stopping %s", u.Name), false)
		p.after(2500*time.Millisecond, u, provider.StateRunning, "Started "+u.Name)
	case provider.ActionStop:
		u.State = provider.StateStopping
		say("Received SIGTERM, shutting down gracefully", false)
		p.after(1500*time.Millisecond, u, provider.StateStopped, "Stopped "+u.Name)
	case provider.ActionStart:
		if u.State != provider.StateStopped && u.State != provider.StateFailed {
			return fmt.Errorf("%s is already %s", u.Name, u.State)
		}
		u.State = provider.StateStarting
		say("Starting "+u.Name, false)
		p.after(2000*time.Millisecond, u, provider.StateRunning, u.Name+" is ready")
	case provider.ActionKill:
		u.State = provider.StateStopped
		say("Killed with SIGKILL", true)
	default:
		return provider.ErrBadAction
	}
	return nil
}

func (p *Provider) after(d time.Duration, u *unit, state, msg string) {
	time.AfterFunc(d, func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		u.State = state
		if state == provider.StateRunning {
			u.started = time.Now()
		}
		p.emit(u, provider.LogLine{T: time.Now().UnixMilli(), Text: msg})
	})
}

// emit must be called with p.mu held.
func (p *Provider) emit(u *unit, l provider.LogLine) {
	u.ring.Push(l)
	for ch := range u.subs {
		select {
		case ch <- l:
		default:
		}
	}
}

func (p *Provider) Logs(ctx context.Context, id string, tail int) (<-chan provider.LogLine, error) {
	p.mu.Lock()
	u, err := p.find(id)
	if err != nil {
		p.mu.Unlock()
		return nil, err
	}
	ch := make(chan provider.LogLine, 512)
	for _, l := range u.ring.Last(tail) {
		ch <- l
		if len(ch) == cap(ch) {
			break
		}
	}
	u.subs[ch] = struct{}{}
	p.mu.Unlock()

	go func() {
		<-ctx.Done()
		p.mu.Lock()
		delete(u.subs, ch)
		close(ch)
		p.mu.Unlock()
	}()
	return ch, nil
}

func (p *Provider) run(ctx context.Context) {
	t := time.NewTicker(450 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		p.mu.Lock()
		for _, u := range p.units {
			if u.State == provider.StateRunning && rand.Float64() < 0.18 {
				p.emit(u, p.fakeLine(u, time.Now()))
			}
		}
		p.mu.Unlock()
	}
}

func (p *Provider) fakeLine(u *unit, at time.Time) provider.LogLine {
	text := u.lines[rand.IntN(len(u.lines))]
	isErr := strings.Contains(text, "WARN") || strings.Contains(text, "ERROR")
	if u.Health == provider.HealthUnhealthy && rand.Float64() < 0.3 {
		text, isErr = "ERROR healthcheck: GET /healthz timed out after 5s", true
	}
	if strings.Contains(text, "%d") {
		text = fmt.Sprintf(text, rand.IntN(900)+100)
	}
	return provider.LogLine{T: at.UnixMilli(), Err: isErr, Text: text}
}

var (
	webLines = []string{
		`INFO  GET /api/client/servers 200 %dms`,
		`INFO  POST /api/client/servers/1a7ce997/power 204 %dms`,
		`INFO  GET /auth/login 200 %dms`,
		`INFO  queue: processed job Pterodactyl\Jobs\Schedule\RunTaskJob in %dms`,
		`WARN  slow query detected (%dms): select * from servers where node_id = ?`,
	}
	dbLines = []string{
		`[Note] Aborted connection %d to db: 'panel' user: 'pterodactyl' (Got timeout reading communication packets)`,
		`[Note] InnoDB: Buffer pool(s) load completed, %d pages`,
	}
	redisLines = []string{
		`1:M * 100 changes in 300 seconds. Saving...`,
		`1:M * Background saving started by pid %d`,
		`1:C * DB saved on disk`,
	}
	strongroomLines = []string{
		`INFO  upload complete: 14.2 MB in %dms`,
		`WARN  thumbnail worker lagging, %d items queued`,
		`ERROR s3 mirror: connection reset by peer (attempt 3/5)`,
	}
	mcLines = []string{
		`[Server thread/INFO]: Steve joined the game`,
		`[Server thread/INFO]: Alex left the game`,
		`[Server thread/INFO]: Saving the game (this may take a moment!)`,
		`[Server thread/INFO]: Saved the game`,
		`[Server thread/WARN]: Can't keep up! Is the server overloaded? Running %dms or 20 ticks behind`,
		`[Skript] Loaded 14 scripts with a total of %d structures`,
		`[Async Chat Thread - #3/INFO]: <Steve> anyone got iron`,
	}
	wingsLines = []string{
		`INFO: [Sep 16 12:01:03.%d] processing power action on server  action=restart server=1a7ce997`,
		`INFO: [Sep 16 12:01:04.%d] updated server usage statistics`,
		`DEBUG: [Sep 16 12:01:05.%d] websocket: authenticated connection`,
		`WARN: [Sep 16 12:01:06.%d] failed to pull image, using local copy`,
	}
	playitLines = []string{
		`tunnel 7f3c: forwarding tcp mc.findiepman.dev:25565 -> 127.0.0.1:25565 (%d active)`,
		`agent: keepalive ok, latency %dms`,
		`WARN  agent: control channel reconnecting (attempt 1)`,
	}
	tunnelLines = []string{
		`INF Registered tunnel connection connIndex=0 location=ams%d protocol=quic`,
		`INF Updated to new configuration version=%d`,
	}
)
