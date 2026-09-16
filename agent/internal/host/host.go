// Package host samples machine-level telemetry: CPU, memory, disks, network.
package host

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	gohost "github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/sensors"
)

type Stats struct {
	T         int64     `json:"t"` // unix ms
	Uptime    uint64    `json:"uptime"`
	Load      []float64 `json:"load"`
	CPU       float64   `json:"cpu"`
	Cores     []float64 `json:"cores"`
	MemUsed   uint64    `json:"memUsed"`
	MemTotal  uint64    `json:"memTotal"`
	SwapUsed  uint64    `json:"swapUsed"`
	SwapTotal uint64    `json:"swapTotal"`
	Disks     []Disk    `json:"disks"`
	NetRx     float64   `json:"netRx"` // bytes per second
	NetTx     float64   `json:"netTx"`
	Temp      *float64  `json:"temp"`
}

type Disk struct {
	Mount string `json:"mount"`
	Used  uint64 `json:"used"`
	Total uint64 `json:"total"`
}

type Collector interface {
	Collect(ctx context.Context) (Stats, error)
}

// System reads the real machine through gopsutil.
type System struct {
	mu      sync.Mutex
	lastNet time.Time
	lastRx  uint64
	lastTx  uint64
}

func NewSystem() *System {
	s := &System{}
	// Prime the CPU counters so the first real sample has a delta.
	cpu.Percent(0, true)
	return s
}

// Filesystems worth showing. Everything else (overlay, tmpfs, squashfs
// snaps, ...) is noise on a Docker host.
var realFS = map[string]bool{
	"ext2": true, "ext3": true, "ext4": true, "xfs": true, "btrfs": true,
	"zfs": true, "f2fs": true, "ntfs": true, "NTFS": true,
}

func (s *System) Collect(ctx context.Context) (Stats, error) {
	now := time.Now()
	st := Stats{T: now.UnixMilli()}

	if cores, err := cpu.PercentWithContext(ctx, 0, true); err == nil && len(cores) > 0 {
		st.Cores = cores
		var sum float64
		for _, c := range cores {
			sum += c
		}
		st.CPU = sum / float64(len(cores))
	}
	if vm, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		st.MemTotal = vm.Total
		st.MemUsed = vm.Total - vm.Available
	}
	if sw, err := mem.SwapMemoryWithContext(ctx); err == nil {
		st.SwapTotal, st.SwapUsed = sw.Total, sw.Used
	}
	if up, err := gohost.UptimeWithContext(ctx); err == nil {
		st.Uptime = up
	}
	if l, err := load.AvgWithContext(ctx); err == nil {
		st.Load = []float64{l.Load1, l.Load5, l.Load15}
	}

	if parts, err := disk.PartitionsWithContext(ctx, false); err == nil {
		seen := map[string]bool{}
		for _, p := range parts {
			if !realFS[p.Fstype] || seen[p.Device] || strings.HasPrefix(p.Mountpoint, "/boot") ||
				strings.HasPrefix(p.Mountpoint, "/snap") || strings.Contains(p.Mountpoint, "/docker/") {
				continue
			}
			seen[p.Device] = true
			if u, err := disk.UsageWithContext(ctx, p.Mountpoint); err == nil && u.Total > 0 {
				st.Disks = append(st.Disks, Disk{Mount: p.Mountpoint, Used: u.Used, Total: u.Total})
			}
		}
		sort.Slice(st.Disks, func(i, j int) bool { return st.Disks[i].Mount < st.Disks[j].Mount })
	}

	if io, err := net.IOCountersWithContext(ctx, true); err == nil {
		var rx, tx uint64
		for _, c := range io {
			if c.Name == "lo" || strings.HasPrefix(c.Name, "veth") || strings.HasPrefix(c.Name, "docker") ||
				strings.HasPrefix(c.Name, "br-") || strings.HasPrefix(c.Name, "pterodactyl") {
				continue
			}
			rx += c.BytesRecv
			tx += c.BytesSent
		}
		s.mu.Lock()
		if !s.lastNet.IsZero() && rx >= s.lastRx && tx >= s.lastTx {
			secs := now.Sub(s.lastNet).Seconds()
			st.NetRx = float64(rx-s.lastRx) / secs
			st.NetTx = float64(tx-s.lastTx) / secs
		}
		s.lastNet, s.lastRx, s.lastTx = now, rx, tx
		s.mu.Unlock()
	}

	if temps, err := sensors.TemperaturesWithContext(ctx); err == nil {
		var hottest float64
		for _, t := range temps {
			if t.Temperature > hottest && t.Temperature < 125 {
				hottest = t.Temperature
			}
		}
		if hottest > 0 {
			st.Temp = &hottest
		}
	}
	if st.Load == nil {
		st.Load = []float64{}
	}
	return st, nil
}
