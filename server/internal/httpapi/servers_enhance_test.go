package httpapi_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"nexus/internal/host"
)

// seedSamples inserts n samples one minute apart ending at end, with
// cpu_percent = i (0..n-1) so averages are easy to predict.
func (h *harness) seedSamples(end time.Time, n int) {
	h.t.Helper()
	for i := 0; i < n; i++ {
		at := end.Add(-time.Duration(n-1-i) * time.Minute)
		if _, err := h.pool.Exec(context.Background(),
			`INSERT INTO metric_samples
			   (taken_at, cpu_percent, mem_total, mem_used, disk_total, disk_used,
			    net_rx, net_tx, load1, load5, load15, uptime_secs)
			 VALUES ($1, $2, 100, 50, 1000, 500, $3, $3, 1, 1, 1, $3)`,
			at, float64(i), int64(i)); err != nil {
			h.t.Fatalf("seed sample: %v", err)
		}
	}
}

func (h *harness) samples(from, to time.Time, extra string) []sampleResp {
	h.t.Helper()
	q := "?from=" + url.QueryEscape(from.UTC().Format(time.RFC3339)) +
		"&to=" + url.QueryEscape(to.UTC().Format(time.RFC3339)) + extra
	r := h.do(http.MethodGet, "/api/servers/samples"+q, nil)
	mustStatus(h.t, r, http.StatusOK)
	var out []sampleResp
	decode(h.t, r, &out)
	return out
}

// --- Time range + downsampling ---

func TestSamplesRangeReturnsOnlyInRange(t *testing.T) {
	h := newHarness(t)
	end := time.Now().UTC().Truncate(time.Minute)
	h.seedSamples(end, 120) // two hours of minutely samples

	got := h.samples(end.Add(-30*time.Minute), end, "")
	if len(got) != 31 {
		t.Fatalf("last-30m window = %d samples, want 31", len(got))
	}
	for i := 1; i < len(got); i++ {
		if got[i].TakenAt.Before(got[i-1].TakenAt) {
			t.Fatal("raw samples not ascending")
		}
	}
	if got[0].CPUPercent != 89 || got[30].CPUPercent != 119 {
		t.Errorf("window edges cpu = %v..%v, want 89..119", got[0].CPUPercent, got[30].CPUPercent)
	}
}

func TestSamplesDownsamplingCapsPoints(t *testing.T) {
	h := newHarness(t)
	end := time.Now().UTC().Truncate(time.Minute)
	h.seedSamples(end, 120)
	from := end.Add(-119 * time.Minute)

	raw := h.samples(from, end, "")
	if len(raw) != 120 {
		t.Fatalf("raw = %d samples, want 120 (back-compat without max_points)", len(raw))
	}

	got := h.samples(from, end, "&max_points=12")
	if len(got) == 0 || len(got) > 12 {
		t.Fatalf("bucketed = %d points, want 1..12", len(got))
	}
	for i := 1; i < len(got); i++ {
		if !got[i].TakenAt.After(got[i-1].TakenAt) {
			t.Fatal("bucketed points not strictly ascending")
		}
	}
	// Coverage: the first bucket starts at `from`, and the last reaches the
	// newest sample (cpu 119 averaged into the final bucket).
	if !got[0].TakenAt.Equal(from) {
		t.Errorf("first bucket at %v, want %v", got[0].TakenAt, from)
	}
	if got[len(got)-1].CPUPercent < 110 {
		t.Errorf("last bucket cpu = %v, want it to include the newest samples", got[len(got)-1].CPUPercent)
	}
	// Averaging: 120 samples into 12 buckets of ~10 → first bucket ≈ avg(0..9).
	if c := got[0].CPUPercent; c < 4 || c > 5.5 {
		t.Errorf("first bucket cpu = %v, want ≈4.5", c)
	}
	// Gauges are averaged (every seeded mem_used is 50).
	if got[0].MemUsed != 50 {
		t.Errorf("first bucket mem_used = %d, want 50", got[0].MemUsed)
	}

	// Fewer samples than max_points: one point per sample at most.
	few := h.samples(end.Add(-4*time.Minute), end, "&max_points=300")
	if len(few) != 5 {
		t.Errorf("sparse range = %d points, want 5", len(few))
	}
}

func TestSamplesDownsamplingRejectsBadParams(t *testing.T) {
	h := newHarness(t)
	for _, q := range []string{"?max_points=0", "?max_points=-3", "?max_points=lots"} {
		r := h.do(http.MethodGet, "/api/servers/samples"+q, nil)
		mustStatus(t, r, http.StatusBadRequest)
		r.Body.Close()
	}
	now := time.Now().UTC()
	q := fmt.Sprintf("?from=%s&to=%s&max_points=10",
		url.QueryEscape(now.Format(time.RFC3339)), url.QueryEscape(now.Add(-time.Hour).Format(time.RFC3339)))
	r := h.do(http.MethodGet, "/api/servers/samples"+q, nil)
	mustStatus(t, r, http.StatusBadRequest)
	r.Body.Close()
}

// --- Disk by partition ---

type disksResp struct {
	DiskTotal uint64 `json:"disk_total"`
	DiskUsed  uint64 `json:"disk_used"`
	Disks     []struct {
		Path        string  `json:"path"`
		Fstype      string  `json:"fstype"`
		Total       uint64  `json:"total"`
		Used        uint64  `json:"used"`
		UsedPercent float64 `json:"used_percent"`
	} `json:"disks"`
}

func TestServersCurrentDisksByPartition(t *testing.T) {
	h := newHarness(t)
	v := cannedVitals()
	v.Disks = []host.DiskUsage{
		{Path: "/", Fstype: "ext4", Total: 512 << 30, Used: 100 << 30, UsedPercent: 19.5},
		{Path: "/data", Fstype: "xfs", Total: 4 << 40, Used: 3 << 40, UsedPercent: 75},
	}
	h.host.SetVitals(v)

	r := h.do(http.MethodGet, "/api/servers/current", nil)
	mustStatus(t, r, http.StatusOK)
	var out disksResp
	decode(t, r, &out)
	if len(out.Disks) != 2 || out.Disks[1].Path != "/data" || out.Disks[1].Fstype != "xfs" ||
		out.Disks[1].Total != 4<<40 || out.Disks[1].UsedPercent != 75 {
		t.Errorf("disks = %+v", out.Disks)
	}
	// Legacy aggregate still reflects root.
	if out.DiskTotal != 512<<30 || out.DiskUsed != 100<<30 {
		t.Errorf("legacy disk = %d/%d", out.DiskUsed, out.DiskTotal)
	}
}

func TestServersCurrentEmptyDisks(t *testing.T) {
	h := newHarness(t)
	h.host.SetVitals(cannedVitals()) // no Disks set
	r := h.do(http.MethodGet, "/api/servers/current", nil)
	mustStatus(t, r, http.StatusOK)
	var raw map[string]interface{}
	decode(t, r, &raw)
	disks, ok := raw["disks"].([]interface{})
	if !ok || len(disks) != 0 {
		t.Errorf("disks = %#v, want an empty array", raw["disks"])
	}
}

// --- Process / container details ---

func TestProcessDetail(t *testing.T) {
	h := newHarness(t)
	started := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	h.host.SetProcessDetail(host.ProcessDetail{
		PID: 4242, PPID: 1, Name: "postgres", Exe: "/usr/lib/postgresql/16/bin/postgres",
		Cmdline: "postgres -D /var/lib/postgresql/data", User: "postgres", Status: "sleep",
		CreatedAt: started, NumThreads: 7, CPU: 3.5, MemRSS: 64 << 20, MemPerc: 0.8,
	})

	r := h.do(http.MethodGet, "/api/servers/processes/4242", nil)
	mustStatus(t, r, http.StatusOK)
	var d host.ProcessDetail
	decode(t, r, &d)
	if d.Cmdline != "postgres -D /var/lib/postgresql/data" || d.User != "postgres" ||
		d.NumThreads != 7 || !d.CreatedAt.Equal(started) || d.MemRSS != 64<<20 {
		t.Errorf("process detail = %+v", d)
	}

	mustStatus(t, h.do(http.MethodGet, "/api/servers/processes/9999", nil), http.StatusNotFound)
	mustStatus(t, h.do(http.MethodGet, "/api/servers/processes/abc", nil), http.StatusBadRequest)
}

func TestContainerDetail(t *testing.T) {
	h := newHarness(t)
	created := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

	// Docker disabled → clear 503, not a panic.
	r := h.do(http.MethodGet, "/api/servers/containers/abc123", nil)
	mustStatus(t, r, http.StatusServiceUnavailable)
	var e map[string]string
	decode(t, r, &e)
	if e["error"] == "" {
		t.Error("expected an error message for docker-disabled detail")
	}

	h.host.SetDocker(host.DockerStatus{Enabled: true})
	h.host.SetContainerDetail(host.ContainerDetail{
		ID: "abc123", Name: "/postgres", Image: "postgres:16", State: "running", Status: "Up 2 hours",
		CreatedAt: created, Command: "docker-entrypoint.sh postgres",
		Ports:  []host.PortBinding{{ContainerPort: "5432/tcp", HostIP: "0.0.0.0", HostPort: "5432"}},
		Mounts: []host.Mount{{Type: "volume", Source: "/var/lib/docker/volumes/pg", Destination: "/var/lib/postgresql/data"}},
	})

	r = h.do(http.MethodGet, "/api/servers/containers/abc123", nil)
	mustStatus(t, r, http.StatusOK)
	var d host.ContainerDetail
	decode(t, r, &d)
	if d.Image != "postgres:16" || d.Status != "Up 2 hours" || !d.CreatedAt.Equal(created) ||
		d.Command != "docker-entrypoint.sh postgres" || len(d.Ports) != 1 || d.Ports[0].HostPort != "5432" ||
		len(d.Mounts) != 1 {
		t.Errorf("container detail = %+v", d)
	}

	mustStatus(t, h.do(http.MethodGet, "/api/servers/containers/gone", nil), http.StatusNotFound)
}
