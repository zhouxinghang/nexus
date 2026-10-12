// Package sampler runs the background sampler that captures the Host's vitals
// every 30 seconds, persists them as Metric Samples, and prunes samples older
// than 30 days. No downsampling — raw 30-day retention only.
package sampler

import (
	"context"
	"encoding/json"
	"time"

	"nexus/internal/db/dbgen"
	"nexus/internal/host"
)

// Interval is how often the Host is sampled.
const Interval = 30 * time.Second

// Retention is how long raw samples are kept before being pruned.
const Retention = 30 * 24 * time.Hour

// Sampler captures and persists Metric Samples.
type Sampler struct {
	queries *dbgen.Queries
	host    host.Host
	now     func() time.Time
}

// New builds a Sampler.
func New(queries *dbgen.Queries, h host.Host) *Sampler {
	return &Sampler{queries: queries, host: h, now: time.Now}
}

// CaptureOnce takes a single sample and persists it. Exposed so tests can
// drive sampling deterministically through the HTTP seam's companion.
func (s *Sampler) CaptureOnce(ctx context.Context) error {
	v, err := s.host.Sample()
	if err != nil {
		return err
	}
	docker := s.host.Status()

	topProcs, _ := json.Marshal(v.TopProcs)
	containers, _ := json.Marshal(docker.Containers)
	if docker.Containers == nil {
		containers = []byte("[]")
	}
	if v.TopProcs == nil {
		topProcs = []byte("[]")
	}

	_, err = s.queries.InsertMetricSample(ctx, dbgen.InsertMetricSampleParams{
		CpuPercent: v.CPUPercent,
		MemTotal:   int64(v.MemTotal),
		MemUsed:    int64(v.MemUsed),
		DiskTotal:  int64(v.DiskTotal),
		DiskUsed:   int64(v.DiskUsed),
		NetRx:      int64(v.NetRx),
		NetTx:      int64(v.NetTx),
		Load1:      v.Load1,
		Load5:      v.Load5,
		Load15:     v.Load15,
		UptimeSecs: int64(v.UptimeSecs),
		TopProcs:   topProcs,
		Containers: containers,
	})
	return err
}

// Prune removes samples older than the retention window.
func (s *Sampler) Prune(ctx context.Context) (int64, error) {
	cutoff := s.now().Add(-Retention)
	return s.queries.PruneMetricSamplesBefore(ctx, cutoff)
}

// Run blocks, sampling every Interval and pruning on each tick, until the
// context is cancelled. Intended to be launched in a goroutine on startup.
func (s *Sampler) Run(ctx context.Context) {
	// Capture once immediately so the dashboard has data without waiting.
	_ = s.CaptureOnce(ctx)
	_, _ = s.Prune(ctx)

	ticker := time.NewTicker(Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = s.CaptureOnce(ctx)
			_, _ = s.Prune(ctx)
		}
	}
}
