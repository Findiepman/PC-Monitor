// Package docker talks to the Docker Engine API over its unix socket.
//
// It uses plain net/http instead of the Docker SDK: dashd needs six
// endpoints, and the SDK's module history makes it a heavy dependency.
package docker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/findiepman/dashd/internal/config"
	"github.com/findiepman/dashd/internal/provider"
)

type Provider struct {
	label         string
	client        *http.Client
	base          string
	socket        string
	excludeLabels map[string]string
	exclude       map[string]bool

	mu      sync.Mutex
	prevCPU map[string]cpuSample
}

type cpuSample struct {
	container, system uint64
}

func New(cfg config.Docker) *Provider {
	tr := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", cfg.Socket)
		},
		MaxIdleConns: 8,
	}
	p := newWithClient(cfg, &http.Client{Transport: tr}, "http://docker")
	p.socket = cfg.Socket
	return p
}

func newWithClient(cfg config.Docker, c *http.Client, base string) *Provider {
	p := &Provider{
		label:         cfg.Label,
		client:        c,
		base:          base,
		excludeLabels: map[string]string{},
		exclude:       map[string]bool{},
		prevCPU:       map[string]cpuSample{},
	}
	for _, kv := range cfg.ExcludeLabels {
		k, v, _ := strings.Cut(kv, "=")
		p.excludeLabels[k] = v
	}
	for _, n := range cfg.Exclude {
		p.exclude[n] = true
	}
	return p
}

func (p *Provider) Name() string  { return "docker" }
func (p *Provider) Label() string { return p.label }

type apiContainer struct {
	ID     string            `json:"Id"`
	Names  []string          `json:"Names"`
	Image  string            `json:"Image"`
	State  string            `json:"State"`
	Labels map[string]string `json:"Labels"`
}

type apiInspect struct {
	RestartCount int `json:"RestartCount"`
	State        struct {
		Status    string    `json:"Status"`
		ExitCode  int       `json:"ExitCode"`
		StartedAt time.Time `json:"StartedAt"`
		Health    *struct {
			Status string `json:"Status"`
		} `json:"Health"`
	} `json:"State"`
	Config struct {
		Tty bool `json:"Tty"`
	} `json:"Config"`
}

type apiStats struct {
	CPUStats struct {
		CPUUsage struct {
			TotalUsage uint64 `json:"total_usage"`
		} `json:"cpu_usage"`
		SystemUsage uint64 `json:"system_cpu_usage"`
		OnlineCPUs  uint32 `json:"online_cpus"`
	} `json:"cpu_stats"`
	MemoryStats struct {
		Usage uint64            `json:"usage"`
		Limit uint64            `json:"limit"`
		Stats map[string]uint64 `json:"stats"`
	} `json:"memory_stats"`
}

func (p *Provider) List(ctx context.Context) ([]provider.Unit, error) {
	var containers []apiContainer
	if err := p.getJSON(ctx, "/containers/json?all=1", &containers); err != nil {
		return nil, err
	}

	var (
		units []provider.Unit
		mu    sync.Mutex
		wg    sync.WaitGroup
		sem   = make(chan struct{}, 8)
	)
	for _, c := range containers {
		if p.hidden(c) {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			u := p.unit(ctx, c)
			mu.Lock()
			units = append(units, u)
			mu.Unlock()
		}()
	}
	wg.Wait()
	return units, nil
}

func (p *Provider) hidden(c apiContainer) bool {
	if p.exclude[containerName(c)] {
		return true
	}
	for k, v := range p.excludeLabels {
		if got, ok := c.Labels[k]; ok && got == v {
			return true
		}
	}
	return false
}

func containerName(c apiContainer) string {
	if len(c.Names) > 0 {
		return strings.TrimPrefix(c.Names[0], "/")
	}
	return c.ID[:12]
}

func (p *Provider) unit(ctx context.Context, c apiContainer) provider.Unit {
	name := containerName(c)
	u := provider.Unit{
		ID:     name,
		Name:   name,
		Kind:   "container",
		State:  mapState(c.State),
		Health: provider.HealthNone,
		Meta:   map[string]string{"image": c.Image},
	}

	var ins apiInspect
	if err := p.getJSON(ctx, "/containers/"+url.PathEscape(c.ID)+"/json", &ins); err == nil {
		if ins.State.Health != nil {
			u.Health = ins.State.Health.Status
		}
		if ins.RestartCount > 0 {
			u.Meta["restarts"] = strconv.Itoa(ins.RestartCount)
		}
		if u.State == provider.StateStopped {
			u.Meta["exit"] = strconv.Itoa(ins.State.ExitCode)
		}
		if u.State == provider.StateRunning && !ins.State.StartedAt.IsZero() {
			u.UptimeSec = int64(time.Since(ins.State.StartedAt).Seconds())
		}
	}

	if u.State == provider.StateRunning {
		var st apiStats
		if err := p.getJSON(ctx, "/containers/"+url.PathEscape(c.ID)+"/stats?stream=false&one-shot=true", &st); err == nil {
			u.CPU = p.cpuPercent(c.ID, st)
			mem := st.MemoryStats.Usage
			inactive := st.MemoryStats.Stats["inactive_file"] // cgroup v2
			if inactive == 0 {
				inactive = st.MemoryStats.Stats["total_inactive_file"] // cgroup v1
			}
			if inactive < mem {
				mem -= inactive
			}
			u.MemBytes = provider.Ptr(mem)
			if st.MemoryStats.Limit > 0 {
				u.MemLimit = provider.Ptr(st.MemoryStats.Limit)
			}
		}
	}
	u.Actions = provider.ActionsFor(u.State)
	return u
}

// cpuPercent computes usage from the delta against the previous poll. The
// one-shot stats call doesn't fill precpu_stats, so the first poll after
// startup reports nil.
func (p *Provider) cpuPercent(id string, st apiStats) *float64 {
	cur := cpuSample{st.CPUStats.CPUUsage.TotalUsage, st.CPUStats.SystemUsage}
	p.mu.Lock()
	prev, ok := p.prevCPU[id]
	p.prevCPU[id] = cur
	p.mu.Unlock()
	if !ok || cur.system <= prev.system || cur.container < prev.container {
		return nil
	}
	cpus := float64(st.CPUStats.OnlineCPUs)
	if cpus == 0 {
		cpus = 1
	}
	pct := float64(cur.container-prev.container) / float64(cur.system-prev.system) * cpus * 100
	return &pct
}

func mapState(s string) string {
	switch s {
	case "running":
		return provider.StateRunning
	case "restarting":
		return provider.StateRestarting
	case "paused":
		return provider.StatePaused
	case "removing":
		return provider.StateStopping
	case "dead":
		return provider.StateFailed
	case "created", "exited":
		return provider.StateStopped
	}
	return provider.StateUnknown
}

func (p *Provider) Do(ctx context.Context, id, action string) error {
	path := "/containers/" + url.PathEscape(id) + "/" + action
	switch action {
	case provider.ActionStop, provider.ActionRestart:
		path += "?t=10"
	case provider.ActionStart, provider.ActionKill:
	default:
		return provider.ErrBadAction
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.base+path, nil)
	if err != nil {
		return err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return p.connErr(err)
	}
	defer resp.Body.Close()
	// 304 means the container was already in the requested state.
	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusNotModified {
		return nil
	}
	return apiError(resp)
}

func (p *Provider) Logs(ctx context.Context, id string, tail int) (<-chan provider.LogLine, error) {
	var ins apiInspect
	if err := p.getJSON(ctx, "/containers/"+url.PathEscape(id)+"/json", &ins); err != nil {
		return nil, err
	}
	q := url.Values{
		"stdout": {"1"}, "stderr": {"1"}, "follow": {"1"},
		"timestamps": {"1"}, "tail": {strconv.Itoa(tail)},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		p.base+"/containers/"+url.PathEscape(id)+"/logs?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, p.connErr(err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, apiError(resp)
	}

	out := make(chan provider.LogLine, 256)
	go func() {
		defer close(out)
		defer resp.Body.Close()
		emit := func(line string, stderr bool) bool {
			select {
			case out <- parseLine(line, stderr):
				return true
			case <-ctx.Done():
				return false
			}
		}
		if ins.Config.Tty {
			readRaw(resp.Body, emit)
		} else {
			readMultiplexed(resp.Body, emit)
		}
	}()
	return out, nil
}

// readRaw handles TTY containers, whose output is a single unframed stream.
func readRaw(r io.Reader, emit func(string, bool) bool) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		if !emit(sc.Text(), false) {
			return
		}
	}
}

// readMultiplexed decodes Docker's framed stream: an 8 byte header (stream
// type, 3 padding bytes, big-endian payload length) before each chunk.
// Chunks don't align with lines, so partial lines are buffered per stream.
func readMultiplexed(r io.Reader, emit func(string, bool) bool) {
	br := bufio.NewReaderSize(r, 32*1024)
	var hdr [8]byte
	var pending [3][]byte
	for {
		if _, err := io.ReadFull(br, hdr[:]); err != nil {
			break
		}
		stream := hdr[0]
		size := binary.BigEndian.Uint32(hdr[4:])
		payload := make([]byte, size)
		if _, err := io.ReadFull(br, payload); err != nil {
			break
		}
		if stream > 2 {
			stream = 1
		}
		buf := append(pending[stream], payload...)
		for {
			i := bytes.IndexByte(buf, '\n')
			if i < 0 {
				break
			}
			if !emit(string(buf[:i]), stream == 2) {
				return
			}
			buf = buf[i+1:]
		}
		pending[stream] = append([]byte(nil), buf...)
	}
	for s, rest := range pending {
		if len(rest) > 0 {
			emit(string(rest), s == 2)
		}
	}
}

// parseLine splits off the RFC 3339 timestamp Docker prepends when
// timestamps=1.
func parseLine(line string, stderr bool) provider.LogLine {
	l := provider.LogLine{Err: stderr, Text: line}
	if ts, rest, ok := strings.Cut(line, " "); ok {
		if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
			l.T = t.UnixMilli()
			l.Text = rest
		}
	}
	l.Text = provider.CleanLine(l.Text)
	return l
}

func (p *Provider) getJSON(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.base+path, nil)
	if err != nil {
		return err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return p.connErr(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return apiError(resp)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (p *Provider) connErr(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if p.socket != "" {
		return fmt.Errorf("docker socket %s not reachable, check that dashd is in the docker group: %w", p.socket, err)
	}
	return err
}

func apiError(resp *http.Response) error {
	var body struct {
		Message string `json:"message"`
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if json.Unmarshal(raw, &body) == nil && body.Message != "" {
		return errors.New(body.Message)
	}
	return fmt.Errorf("docker API returned %s", resp.Status)
}
