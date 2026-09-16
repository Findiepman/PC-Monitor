// Package provider defines the contract every controllable service type
// implements (Docker, systemd, Pterodactyl, ...) and a registry that routes
// requests to the right one.
package provider

import (
	"context"
	"errors"
	"regexp"
	"strings"
)

// Unit states. Providers map their native states onto these so the UI can
// treat every unit the same way.
const (
	StateRunning    = "running"
	StateStarting   = "starting"
	StateStopping   = "stopping"
	StateRestarting = "restarting"
	StateStopped    = "stopped"
	StateFailed     = "failed"
	StatePaused     = "paused"
	StateUnknown    = "unknown"
)

// Health values. HealthNone means the unit has no health check.
const (
	HealthHealthy   = "healthy"
	HealthUnhealthy = "unhealthy"
	HealthStarting  = "starting"
	HealthNone      = "none"
)

// Actions a unit may support.
const (
	ActionStart   = "start"
	ActionStop    = "stop"
	ActionRestart = "restart"
	ActionKill    = "kill"
)

var ErrBadAction = errors.New("unsupported action")

// Unit is one thing on the server that can be watched and controlled.
type Unit struct {
	// ID is unique across providers: "<provider>:<local id>". Providers
	// return the local id only; the registry adds the prefix.
	ID       string `json:"id"`
	Provider string `json:"provider"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	State    string `json:"state"`
	Health   string `json:"health"`
	// UptimeSec is 0 when the unit is not running or uptime is unknown.
	UptimeSec int64 `json:"uptime"`
	// CPU is a percentage where 100 is one full core. Nil when unknown.
	CPU      *float64          `json:"cpu"`
	MemBytes *uint64           `json:"mem"`
	MemLimit *uint64           `json:"memLimit"`
	Actions  []string          `json:"actions"`
	Meta     map[string]string `json:"meta,omitempty"`
}

// LogLine is a single line of output.
type LogLine struct {
	// T is a unix timestamp in milliseconds, 0 when the source has none.
	T    int64  `json:"t"`
	Err  bool   `json:"err,omitempty"`
	Text string `json:"text"`
}

type Provider interface {
	// Name is the stable id prefix, e.g. "docker".
	Name() string
	// Label is the heading shown in the UI.
	Label() string
	List(ctx context.Context) ([]Unit, error)
	Do(ctx context.Context, id, action string) error
	// Logs streams the last tail lines and then follows new output until ctx
	// is cancelled or the source ends. The channel is closed when done.
	Logs(ctx context.Context, id string, tail int) (<-chan LogLine, error)
}

// ActionsFor returns the conventional action set for a state.
func ActionsFor(state string) []string {
	switch state {
	case StateRunning:
		return []string{ActionRestart, ActionStop, ActionKill}
	case StateStarting, StateRestarting:
		return []string{ActionStop, ActionKill}
	case StateStopping:
		return []string{ActionKill}
	case StateStopped, StateFailed:
		return []string{ActionStart}
	default:
		return []string{ActionRestart}
	}
}

func ValidAction(a string) bool {
	switch a {
	case ActionStart, ActionStop, ActionRestart, ActionKill:
		return true
	}
	return false
}

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b[()][A-Za-z0-9]`)

// CleanLine strips ANSI escapes and carriage returns so copied logs paste
// cleanly.
func CleanLine(s string) string {
	s = ansiRE.ReplaceAllString(s, "")
	if i := strings.LastIndexByte(s, '\r'); i >= 0 {
		// A bare \r means the line was overwritten (progress bars); keep the
		// final state.
		if i == len(s)-1 {
			s = s[:i]
			if j := strings.LastIndexByte(s, '\r'); j >= 0 {
				s = s[j+1:]
			}
		} else {
			s = s[i+1:]
		}
	}
	return s
}

// Ptr is a small helper for the optional numeric fields on Unit.
func Ptr[T any](v T) *T { return &v }
