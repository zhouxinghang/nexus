package httpapi

import "context"

// Testing hooks — exported helpers that let the HTTP-seam tests drive
// background behavior deterministically instead of waiting on real timers.

// CaptureSampleForTest takes a single Metric Sample synchronously.
func (s *Server) CaptureSampleForTest(ctx context.Context) error {
	return s.sampler.CaptureOnce(ctx)
}

// PruneSamplesForTest runs the retention prune and returns rows removed.
func (s *Server) PruneSamplesForTest(ctx context.Context) (int64, error) {
	return s.sampler.Prune(ctx)
}

// InsertAgedSampleForTest inserts a sample stamped at an arbitrary past time
// so retention-prune behavior can be exercised without waiting 30 days.
func (s *Server) InsertAgedSampleForTest(ctx context.Context, daysAgo int) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO metric_samples
		   (taken_at, cpu_percent, mem_total, mem_used, disk_total, disk_used,
		    net_rx, net_tx, load1, load5, load15, uptime_secs)
		 VALUES (now() - make_interval(days => $1), 0,0,0,0,0,0,0,0,0,0,0)`,
		daysAgo)
	return err
}
