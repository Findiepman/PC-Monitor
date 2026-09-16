package host

import (
	"context"
	"math"
	"math/rand/v2"
	"time"
)

// Mock produces plausible numbers for UI development: a slow CPU wave with
// the occasional spike, and memory that drifts.
type Mock struct {
	start time.Time
}

func NewMock() *Mock { return &Mock{start: time.Now()} }

func (m *Mock) Collect(context.Context) (Stats, error) {
	now := time.Now()
	el := now.Sub(m.start).Seconds()

	base := 18 + 10*math.Sin(el/40) + 6*math.Sin(el/7)
	if rand.Float64() < 0.06 {
		base += 35 + rand.Float64()*30
	}
	cores := make([]float64, 8)
	var sum float64
	for i := range cores {
		c := clamp(base+rand.NormFloat64()*9+float64(i%3)*4, 0.5, 100)
		cores[i] = c
		sum += c
	}
	temp := 48 + sum/8*0.3
	const gb = 1 << 30
	return Stats{
		T:         now.UnixMilli(),
		Uptime:    uint64(41*86400+3*3600) + uint64(el),
		Load:      []float64{sum / 100 * 0.8, 0.51, 0.40},
		CPU:       sum / 8,
		Cores:     cores,
		MemUsed:   uint64((11.2 + 0.8*math.Sin(el/90)) * gb),
		MemTotal:  32 * gb,
		SwapUsed:  gb / 5,
		SwapTotal: 4 * gb,
		Disks: []Disk{
			{Mount: "/", Used: 142 * gb, Total: 468 * gb},
			{Mount: "/srv/pterodactyl", Used: 1690 * gb, Total: 1863 * gb},
		},
		NetRx: 180_000 + math.Abs(rand.NormFloat64())*400_000,
		NetTx: 60_000 + math.Abs(rand.NormFloat64())*900_000,
		Temp:  &temp,
	}, nil
}

func clamp(v, lo, hi float64) float64 {
	return math.Max(lo, math.Min(hi, v))
}
